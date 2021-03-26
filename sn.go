package main

import (
	"bytes"
	"compress/gzip"
	"crypto/sha1"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/araddon/dateparse"
	"github.com/arpitgogia/rake"
	"github.com/aymerick/raymond"
	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/analysis/analyzer/keyword"
	"github.com/blevesearch/bleve/v2/mapping"
	"github.com/gernest/front"
	"github.com/go-git/go-git/v5"
	"github.com/hashicorp/go-memdb"
	"github.com/radovskyb/watcher"
	"github.com/russross/blackfriday/v2"
	"github.com/spf13/viper"
	"golang.org/x/crypto/acme/autocert"
)

// Item is...
type Item struct {
	Title      string
	Slug       string
	Repo       string
	Categories []string
	Authors    []string
	Date       time.Time
	RawDate    string
	Raw        string
	Html       string
}

type ItemResult struct {
	Items    []Item
	Total    int
	Paginate int
	Pages    int
	Page     int
}

type Category struct {
	Name  string
	Count int
}

type Author struct {
	Name  string
	Count int
}

var db *memdb.MemDB

var bmap mapping.IndexMappingImpl
var index bleve.Index

func handler(w http.ResponseWriter, r *http.Request) {
	var context map[string]interface{}
	var layoutfilename string
	var pathVars map[string]string
	var output string
	var staticfile string
	var mime string

	context = make(map[string]interface{}, 0)
	context["config"] = viper.AllSettings()

	fmt.Printf("Path requested: %s\n", r.URL.Path)

	routeMatch, pathVars, err := getMatchingRoute(r.URL.Path)
	context["pathvars"] = pathVars
	context["params"] = r.URL.Query()

	if err != nil {
		fmt.Printf("Rendering 404\n")
		// This should render a 404 if we know how
	} else {
		fmt.Printf("Matched Route: %s\n", routeMatch)
		switch viper.GetString(fmt.Sprintf("%s.handler", routeMatch)) {
		case "posts":
			output, _ := postHandler(routeMatch, context)
			// May use context here to set additional headers, as defined by the handler
			w.Header().Add("Content-Type", "text/html")
			w.Header().Add("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			w.Header().Add("X-Frame-Options", "SAMEORIGIN")
			w.Header().Add("X-Content-Type-Options", "nosniff")
			w.Header().Add("Upgrade-Insecure-Requests", "1")
			w.Header().Add("Referrer-Policy", "strict-origin-when-cross-origin")
			w.Header().Add("Permissions-Policy", "geolocation=(self), microphone=()")
			w.Write([]byte(output))
			break
		case "static":
			staticfile = viper.GetString(fmt.Sprintf("%s.file", routeMatch))
			if pathVars["file"] != "" {
				staticfile = pathVars["file"]
			}
			staticfile = path.Join(viper.GetString("path"), viper.GetString("template_path"), viper.GetString(fmt.Sprintf("%s.path", routeMatch)), staticfile)
			fmt.Printf("Rendering static file: %s\n", staticfile)
			f, err := os.Open(staticfile)
			if f == nil || err != nil {
				fmt.Printf("Could not open static file!\n")
			} else {
				// Read file into memory
				fileBytes, err := ioutil.ReadAll(f)
				if err != nil {
					log.Println(err)
					_, _ = fmt.Fprintf(w, "Error file bytes")
					return
				}

				file, err := os.Stat(staticfile)

				// Check mime
				switch strings.ToLower(filepath.Ext(staticfile)) {
				case ".css":
					mime = "text/css"
					break
				default:
					mime = http.DetectContentType(fileBytes)
				}

				h := sha1.New()
				h.Write(fileBytes)
				bs := h.Sum(nil)

				// Custom headers
				w.Header().Add("Content-Type", mime)

				w.Header().Add("Cache-Control", "public, min-fresh=86400, max-age=31536000")
				w.Header().Add("Content-Description", "File Transfer")
				w.Header().Add("Pragma", "public")
				w.Header().Add("Last-Modified", file.ModTime().String())
				w.Header().Add("ETag", fmt.Sprintf("%x\n", bs))
				w.Header().Add("Expires", "Fri, 1 Jan 3030 00:00:00 GMT")
				w.Header().Add("Content-Length", strconv.Itoa(len(fileBytes)))
				w.Header().Add("Access-Control-Allow-Origin", r.Host)
				w.Write(fileBytes)
				//fmt.Fprint(w, fileBytes)
			}
			//http.ServeFile(w, r, staticfile)
			break
		case "git":
			output, _ := gitHandler(routeMatch, context)
			fmt.Fprint(w, output)
			break
		default:
			fmt.Printf("Rendering default handler\n")
			layoutfilename = path.Join(viper.GetString("path"), viper.GetString("template_path"), "layout.html.hb")
			fmt.Printf("Rendering layout: %s\n", layoutfilename)
			output, _ = renderTemplateFile(layoutfilename, context)

			w.Header().Add("Content-Type", "text/html")
			w.Header().Add("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			w.Header().Add("X-Frame-Options", "SAMEORIGIN")
			w.Header().Add("X-Content-Type-Options", "nosniff")
			w.Header().Add("Upgrade-Insecure-Requests", "1")
			w.Header().Add("Referrer-Policy", "strict-origin-when-cross-origin")
			w.Header().Add("Permissions-Policy", "geolocation=(self), microphone=()")
			w.Write([]byte(output))
		}
	}
}

func gitHandler(routeMatch string, context map[string]interface{}) (string, map[string]interface{}) {
	var remote string
	fmt.Printf("Rendering git handler\n")

	path := viper.GetString(fmt.Sprintf("%s.path", routeMatch))
	if viper.IsSet(fmt.Sprintf("%s.remote", routeMatch)) {
		remote = viper.GetString(fmt.Sprintf("%s.remote", routeMatch))
	} else {
		remote = "origin"
	}

	r, err := git.PlainOpen(path)
	if err != nil {
		fmt.Printf("%v#\n", err)
	}
	w, err := r.Worktree()
	if err != nil {
		fmt.Printf("%v#\n", err)
	}
	err = w.Pull(&git.PullOptions{RemoteName: remote})
	if err != nil {
		fmt.Printf("%v#\n", err)
	}

	ref, _ := r.Head()
	commit, _ := r.CommitObject(ref.Hash())
	fmt.Printf("Current commit hash on %s: %s\n%s\n", path, ref.Hash(), commit)

	return commit.Hash.String() + ": " + commit.Message, context
}

func postHandler(routeMatch string, context map[string]interface{}) (string, map[string]interface{}) {
	fmt.Printf("Rendering posts handler\n")
	layoutfilename := getTemplateFileFromConfig(fmt.Sprintf("%s.layout", routeMatch), "layout.html.hb")
	templatefilename := getTemplateFileFromConfig(fmt.Sprintf("%s.template", routeMatch), "template.html.hb")
	fmt.Printf("Rendering template: %s\n", templatefilename)
	context["post"] = nil

	pathvars := context["pathvars"].(map[string]string)
	fmt.Printf("Pathvars: %+v\n", pathvars)

	// Find the itemquery instances, loop over, assign results to context
	for name, value := range viper.GetStringMap(fmt.Sprintf("%s", routeMatch)) {
		if _, is_map := value.(map[string]interface{}); is_map {
			query := viper.GetString(fmt.Sprintf("%s.%s.query", routeMatch, name))
			queryTemplate := template.Must(template.New("").Parse(query))
			buf := bytes.Buffer{}
			queryTemplate.Execute(&buf, pathvars)
			var renderedQuery string = buf.String()
			fmt.Printf("Rendered Query: %#v\n", renderedQuery)
			itemResult := itemsFromQuery(renderedQuery, context)

			context[name] = itemResult
		}
	}

	rendered, err := renderTemplateFile(templatefilename, context)
	if err != nil {
		fmt.Printf("Error rendering template: %s\n", err)
		context["content"] = fmt.Sprintf("<div class=\"notification is-danger\">Error rendering template: %s</div>\n", err)
	} else {
		context["content"] = rendered
	}
	fmt.Printf("Rendering layout: %s\n", layoutfilename)

	layoutRendered, err := renderTemplateFile(layoutfilename, context)
	if err != nil {
		fmt.Printf("Error rendering layout: %s\n", err)
		context["content"] = fmt.Sprintf("<div class=\"notification is-danger\">Error rendering layout: %s</div>\n", err)
		layoutRendered = "<html>" + context["content"].(string) + "</html>"
	}

	return layoutRendered, context
}

func MinOf(vars ...int) int {
	min := vars[0]

	for _, i := range vars {
		if min > i {
			min = i
		}
	}

	return min
}

func itemsFromQuery(query string, context map[string]interface{}) ItemResult {
	var items []Item
	var pg int

	items = make([]Item, 0)

	txn := db.Txn(false)
	defer txn.Abort()

	pathvars := context["pathvars"].(map[string]string)
	perPage := 5
	pg = 1
	if page, ok := context["params"].(url.Values)["page"]; ok {
		pg, _ = strconv.Atoi(page[0])
	}
	if page, ok := pathvars["page"]; ok {
		pg, _ = strconv.Atoi(page)
	}
	front := (pg - 1) * perPage

	fmt.Printf("Bleve search \"%s\"\n", query)
	search := bleve.NewQueryStringQuery(query)
	searchRequest := bleve.NewSearchRequest(search)
	searchRequest.SortBy([]string{"-Date"})
	searchRequest.Size = perPage
	searchRequest.From = front
	searchResults, err := index.Search(searchRequest)
	if err != nil {
		fmt.Printf("Error: %#v", err)
	}
	for _, result := range searchResults.Hits {
		fmt.Printf("Queuing ID: %s\n", result.ID)
		raw, err := txn.First("items", "id", result.ID)
		if err != nil {
			panic(err)
		}
		if raw == nil {
			fmt.Printf("Could not find ID!: %s\n", result.ID)
		} else {
			item := raw.(Item)
			items = append(items, item)
		}
	}

	fmt.Printf("SearchResults:\n%v\n", searchResults)

	return ItemResult{Items: items, Total: int(searchResults.Total), Pages: int(math.Ceil(float64(searchResults.Total) / float64(perPage))), Page: pg}
}

func itemsFromVars(context map[string]interface{}) ItemResult {
	var items []Item
	var pg int

	searchComplete := false

	items = make([]Item, 0)
	pathvars := context["pathvars"].(map[string]string)
	fmt.Printf("Pathvars: %+v\n", pathvars)

	txn := db.Txn(false)
	defer txn.Abort()
	if slug, ok := pathvars["slug"]; ok {
		fmt.Printf("Searching for slug \"%s\"\n", slug)
		raw, err := txn.First("items", "id", slug)
		if err != nil {
			panic(err)
		}
		if raw != nil {
			item := raw.(Item)
			items = append(items, item)
		}
		searchComplete = true
	}
	if category, ok := pathvars["category"]; ok {
		fmt.Printf("Searching for tag \"%s\"\n", category)
		raw, err := txn.Get("items", "categories", category)
		if err != nil {
			panic(err)
		}
		for obj := raw.Next(); obj != nil; obj = raw.Next() {
			item := obj.(Item)
			items = append(items, item)
		}
		searchComplete = true
	}
	if b, ok := pathvars["b"]; ok {
		fmt.Printf("Bleve search \"%s\"\n", b)
		search := bleve.NewQueryStringQuery(b)
		searchRequest := bleve.NewSearchRequest(search)
		searchResults, err := index.Search(searchRequest)
		if err != nil {
			fmt.Printf("Error: %#v", err)
		}
		for _, result := range searchResults.Hits {
			raw, err := txn.First("items", "id", result.ID)
			if err != nil {
				panic(err)
			}
			if raw != nil {
				item := raw.(Item)
				items = append(items, item)
			}
		}
		fmt.Printf("Results:\n%v\n", searchResults)
		searchComplete = true
	}

	if !searchComplete {
		fmt.Printf("Returning all items\n")
		raw, err := txn.Get("items", "id")
		if err != nil {
			panic(err)
		}
		for obj := raw.Next(); obj != nil; obj = raw.Next() {
			item := obj.(Item)
			items = append(items, item)
		}
		searchComplete = true
	}

	sort.SliceStable(items, func(i, j int) bool { return items[i].Date.After(items[j].Date) })

	perPage := 5
	pg = 1
	if page, ok := context["params"].(url.Values)["page"]; ok {
		pg, _ = strconv.Atoi(page[0])
	}
	if page, ok := pathvars["page"]; ok {
		pg, _ = strconv.Atoi(page)
	}
	front := (pg - 1) * perPage
	back := front + perPage
	back = MinOf(len(items), back)

	return ItemResult{Items: items[front:back], Total: len(items), Pages: int(math.Ceil(float64(len(items)) / float64(perPage))), Page: pg}
}

func getTemplateFileFromConfig(configPath string, alternative string) string {
	var template string
	if template = viper.GetString(configPath); template == "" {
		template = alternative
	}
	return path.Join(viper.GetString("path"), viper.GetString("template_path"), template)
}

func renderTemplateFile(filename string, context map[string]interface{}) (string, error) {
	file, err := ioutil.ReadFile(filename)
	if err != nil {
		return string(file), err
	}

	return raymond.Render(string(file), context)
}

func getMatchingRoute(url string) (string, map[string]string, error) {
	f := func(c rune) bool {
		return c == '/'
	}

	p := strings.FieldsFunc(url, f)

	var pathComponents map[string]string
	pathComponents = make(map[string]string, 0)

	routelist := make([]string, 0, len(viper.GetStringMap("routes")))
	for key := range viper.GetStringMap("routes") {
		routelist = append(routelist, key)
	}
	sort.Strings(routelist)

ROUTES:
	for _, route := range routelist {
		fullRoute := fmt.Sprintf("routes.%s", route)
		routeRoute := fmt.Sprintf("%s.route", fullRoute)
		rp := strings.FieldsFunc(viper.GetString(routeRoute), f)
		if len(rp) != len(p) {
			continue ROUTES
		}
		//fmt.Printf("------------\nRoute: %s   URL: %s\n", strings.Join(rp, "/"), strings.Join(p, "/"))
		for z := 0; z < len(p); z++ {
			//fmt.Printf("Route: %s   URL: %s\n", rp[z], p[z])
			if (rp[z] == "" && p[z] != "") || (rp[z][0] != ':' && rp[z] != p[z]) {
				continue ROUTES
			}
		}
		for z := 0; z < len(p); z++ {
			if rp[0] != "" && rp[z][0] == ':' {
				fmt.Printf("Setting pathvar \"%s\" to: %s\n", rp[z][1:], p[z])
				pathComponents[rp[z][1:]] = p[z]
			}
		}
		return fullRoute, pathComponents, nil
	}
	return "", pathComponents, errors.New("No Routes :(")
}

func setupConfig() {
	viper.SetConfigName("sn")
	viper.SetConfigType("toml")
	viper.AddConfigPath(".")
	if snConfigFile := os.Getenv("SN_CONFIG"); snConfigFile != "" {
		fmt.Printf("Loading configuration file: %s\n", snConfigFile)
		viper.SetConfigFile(snConfigFile)
	}
	viper.WatchConfig()
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			fmt.Println("Could not find configuration file")
		} else {
			fmt.Println("Error while loading configuration file")
			fmt.Printf("%q", err)
		}
	}
	fmt.Printf("Used configuration file: %s\n", viper.ConfigFileUsed())
}

func makeDB() {
	var err error

	schema := &memdb.DBSchema{
		Tables: map[string]*memdb.TableSchema{
			"items": {
				Name: "items",
				Indexes: map[string]*memdb.IndexSchema{
					"id": {
						Name:    "id",
						Unique:  true,
						Indexer: &memdb.StringFieldIndex{Field: "Slug"},
					},
					"repo": {
						Name:    "repo",
						Unique:  false,
						Indexer: &memdb.StringFieldIndex{Field: "Repo"},
					},
					"categories": {
						Name:         "categories",
						Unique:       false,
						Indexer:      &memdb.StringSliceFieldIndex{Field: "Categories"},
						AllowMissing: true,
					},
					"authors": {
						Name:         "authors",
						Unique:       false,
						Indexer:      &memdb.StringSliceFieldIndex{Field: "Authors"},
						AllowMissing: true,
					},
					"date": {
						Name:    "date",
						Unique:  false,
						Indexer: &memdb.StringFieldIndex{Field: "Date"},
					},
				},
			},
			"category": {
				Name: "category",
				Indexes: map[string]*memdb.IndexSchema{
					"id": {
						Name:    "id",
						Unique:  true,
						Indexer: &memdb.StringFieldIndex{Field: "Name"},
					},
				},
			},
			"author": {
				Name: "author",
				Indexes: map[string]*memdb.IndexSchema{
					"id": {
						Name:    "id",
						Unique:  true,
						Indexer: &memdb.StringFieldIndex{Field: "Name"},
					},
				},
			},
		},
	}

	db, err = memdb.NewMemDB(schema)
	if err != nil {
		panic(err)
	}
}

func loadRepos() {
	slugMapping := bleve.NewTextFieldMapping()
	slugMapping.Analyzer = keyword.Name

	itemMapping := bleve.NewDocumentMapping()
	itemMapping.AddFieldMappingsAt("Slug", slugMapping)
	itemMapping.AddFieldMappingsAt("Categories", slugMapping)

	bmap := bleve.NewIndexMapping()
	bmap.DefaultAnalyzer = "en"
	bmap.AddDocumentMapping("item", itemMapping)
	bmap.DefaultType = "item"

	if viper.IsSet("filedb") {
		index, _ = bleve.New(viper.GetString("filedb"), bmap)
	} else {
		index, _ = bleve.NewMemOnly(bmap)
	}

	for repo, _ := range viper.GetStringMap("repos") {
		loadRepo(repo)
	}
}

func loadRepo(repoName string) {
	const bufferLen = 5000
	itempaths := make(chan string, bufferLen)

	const workers = 1
	for w := 0; w < workers; w++ {
		go func(id int, itempaths <-chan string) {
			for path := range itempaths {
				loadItem(repoName, path)
			}
		}(w, itempaths)
	}

	repoPath := path.Join(viper.GetString("path"), viper.GetString(fmt.Sprintf("repos.%s.path", repoName)))
	err := filepath.Walk(repoPath, func(path string, info os.FileInfo, err error) error {
		if info.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".md" {
			return nil
		}
		itempaths <- path
		return nil
	})
	if err != nil {
		panic(err)
	}
	close(itempaths)
	startWatching(repoPath, repoName)
}

func loadItem(repoName string, filename string) {
	var item Item

	file, err := os.Open(filename)
	defer file.Close()
	if err != nil {
		panic(err)
	}

	m := front.NewMatter()
	m.Handle("---", front.YAMLHandler)

	f, body, err := m.Parse(file)
	if err != nil {
		panic(err)
	}

	item.Title = f["title"].(string)
	if val, ok := f["slug"]; ok {
		item.Slug = fmt.Sprintf("%v", val)
	} else {
		item.Slug = path.Base(filename)
	}
	item.Raw = body
	item.Repo = repoName

	ishtml, ok := f["html"]
	if ok && ishtml.(bool) {
		item.Html = body
	} else {
		item.Html = string(blackfriday.Run([]byte(body), blackfriday.WithExtensions(blackfriday.CommonExtensions|blackfriday.HardLineBreak)))
	}

	var categories []string
	var authors []string
	var arr []interface{}
	if _, ok := f["categories"]; ok {
		arr = f["categories"].([]interface{})
		categories := make([]string, len(arr))
		for i, v := range arr {
			categories[i] = fmt.Sprint(v)
			category := new(Category)
			category.Name = fmt.Sprint(v)
			category.Count = 1
		}
	}

	if viper.IsSet("rake_minimum") {
		rakes := rake.WithText(body)
		keys := make([]string, 0, len(rakes))
		for k := range rakes {
			if rakes[k] > viper.GetFloat64("rake_minimum") {
				keys = append(keys, k)
			}
		}
		sort.SliceStable(keys, func(i, j int) bool { return rakes[keys[i]] > rakes[keys[j]] })
		categories = append(categories, keys...)
	}

	for category := range categories {
		txn := db.Txn(true)
		txn.Insert("categories", category)
		txn.Commit()
	}

	item.Categories = categories
	if _, ok := f["authors"]; ok {
		arr = f["authors"].([]interface{})
		authors := make([]string, len(arr))
		for i, v := range arr {
			authors[i] = fmt.Sprint(v)
		}
	}
	item.Authors = authors

	if _, ok := f["date"]; ok {
		item.RawDate = f["date"].(string)
	} else {
		filestat, _ := os.Stat(filename)
		item.RawDate = filestat.ModTime().String()
	}
	item.Date, err = dateparse.ParseLocal(item.RawDate)

	txn := db.Txn(true)
	err = txn.Insert("items", item)
	if err != nil {
		fmt.Printf("Error inserting item \"%s\": %s\n", item.Slug, err)
	}
	txn.Commit()

	index.Index(item.Slug, item)
}

func startWatching(path string, repoName string) {
	fmt.Printf("Starting recursive watch of %s repo: %s\n", repoName, path)
	w := watcher.New()
	w.SetMaxEvents(1)
	w.FilterOps(watcher.Create, watcher.Write)
	r := regexp.MustCompile(".md$")
	w.AddFilterHook(watcher.RegexFilterHook(r, false))

	go func() {
		for {
			select {
			case event := <-w.Event:
				fmt.Println(event) // Print the event's info.
				loadItem(repoName, event.Path)
			case err := <-w.Error:
				log.Fatalln(err)
			case <-w.Closed:
				return
			}
		}
	}()

	if err := w.AddRecursive(path); err != nil {
		log.Fatalln(err)
	}

	go func() {
		if err := w.Start(time.Millisecond * 100); err != nil {
			log.Fatalln(err)
		}
	}()

	fmt.Printf("Started recursive watch of %s repo\n", repoName)
}

func registerTemplateHelpers() {
	raymond.RegisterHelper("dateformat", func(t time.Time, format string) string {
		return t.Format(format)
	})
	raymond.RegisterHelper("more", func(html string, pcount int, options *raymond.Options) string {
		more := ""
		re := regexp.MustCompile("<!--\\s*more\\s*-->")
		split := re.Split(html, -1)
		if len(split) > 1 {
			return split[0] + options.Fn()
		}

		doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
		if err != nil {
			return "<p>NewDocument() error</p>" + html
		}

		doc.Find("p").EachWithBreak(func(i int, sel *goquery.Selection) bool {
			tp, err := goquery.OuterHtml(sel)
			if err == nil {
				if sel.Text() != "" {
					more = more + tp
					pcount--
				}
			}
			if pcount <= 0 {
				return false
			}
			return true
		})

		return more + options.Fn()
	})
	raymond.RegisterHelper("paginate", func(context interface{}, paragraphs int, options *raymond.Options) string {
		return options.FnWith(context)
	})
}

type gzipResponseWriter struct {
	io.Writer
	http.ResponseWriter
}

func (w gzipResponseWriter) Write(b []byte) (int, error) {
	return w.Writer.Write(b)
}

func makeGzipHandler(fn http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			fn(w, r)
			return
		}
		w.Header().Set("Content-Encoding", "gzip")
		gz := gzip.NewWriter(w)
		defer gz.Close()
		gzr := gzipResponseWriter{Writer: gz, ResponseWriter: w}
		fn(gzr, r)
	}
}

func main() {
	setupConfig()
	makeDB()
	loadRepos()
	registerTemplateHelpers()

	http.HandleFunc("/", makeGzipHandler(handler))

	if viper.IsSet("ssldomains") {
		certManager := autocert.Manager{
			Prompt:     autocert.AcceptTOS,
			HostPolicy: autocert.HostWhitelist(viper.GetStringSlice("ssldomains")...), //Your domain here
			Cache:      autocert.DirCache("certs"),                                    //Folder for storing certificates
		}

		server := &http.Server{
			Addr: ":https",
			TLSConfig: &tls.Config{
				GetCertificate: certManager.GetCertificate,
			},
		}

		go http.ListenAndServe(":http", certManager.HTTPHandler(nil))
		fmt.Printf("Starting TLS HTTPS server on localhost, and HTTP server for LetsEncrypt.\n")
		log.Fatal(server.ListenAndServeTLS("", ""))
	} else {
		fmt.Printf("Starting HTTP server on localhost:%d\n", viper.GetInt("port"))
		log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", viper.GetInt("port")), nil))
	}

}
