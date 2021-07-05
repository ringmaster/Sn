package main

import (
	"bytes"
	"crypto/tls"
	"fmt"
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
	"github.com/gernest/front"
	"github.com/go-git/go-git/v5"
	"github.com/go-http-utils/etag"
	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/hashicorp/go-memdb"
	"github.com/radovskyb/watcher"
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

var index bleve.Index

func gitHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("Rendering git handler\n")

	routeName := mux.CurrentRoute(r).GetName()
	routeConfigLocation := fmt.Sprintf("routes.%s", routeName)
	var remote string

	path := configPath(viper.GetString(fmt.Sprintf("%s.dir", routeConfigLocation)))
	if viper.IsSet(fmt.Sprintf("%s.remote", routeConfigLocation)) {
		remote = viper.GetString(fmt.Sprintf("%s.remote", routeConfigLocation))
	} else {
		remote = "origin"
	}

	repo, err := git.PlainOpen(path)
	if err != nil {
		fmt.Printf("Git PlainOpen (%s): %#v\n", path, err)
	}
	worktree, err := repo.Worktree()
	if err != nil {
		fmt.Printf("Git Worktree: %#v\n", err)
	}
	err = worktree.Pull(&git.PullOptions{RemoteName: remote})
	if err != nil {
		fmt.Printf("Git PullOptions: %#v\n", err)
	}

	ref, _ := repo.Head()
	commit, _ := repo.CommitObject(ref.Hash())
	fmt.Printf("Current commit hash on %s: %s\n%s\n", path, ref.Hash(), commit)

	w.Header().Add("Content-Type", "text/plain")
	w.Header().Add("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
	w.Header().Add("X-Frame-Options", "SAMEORIGIN")
	w.Header().Add("X-Content-Type-Options", "nosniff")
	w.Header().Add("Upgrade-Insecure-Requests", "1")
	w.Header().Add("Referrer-Policy", "strict-origin-when-cross-origin")
	w.Header().Add("Permissions-Policy", "geolocation=(self), microphone=()")

	w.Write([]byte(commit.Hash.String() + ": " + commit.Message))
}

func postHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("Rendering posts handler\n")
	routeName := mux.CurrentRoute(r).GetName()
	routeConfigLocation := fmt.Sprintf("routes.%s", routeName)
	layoutfilename := getTemplateFileFromConfig(fmt.Sprintf("%s.layout", routeConfigLocation), "layout.html.hb")
	templatefilename := getTemplateFileFromConfig(fmt.Sprintf("%s.template", routeConfigLocation), "template.html.hb")
	fmt.Printf("Rendering template: %s\n", templatefilename)
	context := viper.GetStringMap(routeConfigLocation)
	context["pathvars"] = mux.Vars(r)
	context["params"] = r.URL.Query()
	context["post"] = nil

	pathvars := context["pathvars"]
	fmt.Printf("Pathvars: %+v\n", pathvars)

	// Find the itemquery instances, loop over, assign results to context
	for outVarName, value := range viper.GetStringMap(routeConfigLocation) {
		if _, is_map := value.(map[string]interface{}); is_map {
			query := viper.GetString(fmt.Sprintf("%s.%s.query", routeConfigLocation, outVarName))
			queryTemplate := template.Must(template.New("").Parse(query))
			buf := bytes.Buffer{}
			queryTemplate.Execute(&buf, pathvars)
			var renderedQuery string = buf.String()
			fmt.Printf("Rendered Query: %#v\n", renderedQuery)
			itemResult := itemsFromQuery(renderedQuery, context)

			context[outVarName] = itemResult
		}
	}

	context["mime"] = "text/html"
	if viper.IsSet(fmt.Sprintf("%s.content-type", routeConfigLocation)) {
		context["mime"] = viper.GetString(fmt.Sprintf("%s.content-type", routeConfigLocation))
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
	// May use context here to set additional headers, as defined by the handler
	w.Header().Add("Content-Type", context["mime"].(string))
	w.Header().Add("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
	w.Header().Add("X-Frame-Options", "SAMEORIGIN")
	w.Header().Add("X-Content-Type-Options", "nosniff")
	w.Header().Add("Upgrade-Insecure-Requests", "1")
	w.Header().Add("Referrer-Policy", "strict-origin-when-cross-origin")
	w.Header().Add("Permissions-Policy", "geolocation=(self), microphone=()")

	w.Write([]byte(layoutRendered))
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

// @todo refactor this to use specific tuple values instead of the full context?
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

func getTemplateFileFromConfig(configPath string, alternative string) string {
	var template string
	if template = viper.GetString(configPath); template == "" {
		template = alternative
	}
	return path.Join(viper.GetString("path"), viper.GetString("template_dir"), template)
}

func renderTemplateFile(filename string, context map[string]interface{}) (string, error) {
	file, err := ioutil.ReadFile(filename)
	if err != nil {
		return string(file), err
	}

	return raymond.Render(string(file), context)
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
	viper.SetDefault("path", filepath.Dir(viper.ConfigFileUsed()))
	fmt.Printf("Used configuration file: %s\n", viper.ConfigFileUsed())
}

func dirExists(dir string) bool {
	_, err := os.Stat(dir)
	return !os.IsNotExist(err)
}

func configPath(shortpath string) string {
	configVars := map[string]string{
		"template_dir": viper.GetString("template_dir"),
	}

	pathTemplate := template.Must(template.New("").Parse(shortpath))
	buf := bytes.Buffer{}
	pathTemplate.Execute(&buf, configVars)
	var renderedPathTemplate string = buf.String()
	fmt.Printf("Rendered path template: %#q\n", renderedPathTemplate)

	if renderedPathTemplate[0] == '/' && dirExists(renderedPathTemplate) {
		return renderedPathTemplate
	}

	base, err := filepath.Abs(viper.GetString("path"))
	if err != nil {
		panic(fmt.Sprintf("Configpath for %s does not have absolute path at %s", renderedPathTemplate, viper.GetString("path")))
	}

	fmt.Printf("configPath: %s %s\n", base, renderedPathTemplate)
	base = path.Join(base, renderedPathTemplate)
	if !dirExists(base) {
		panic(fmt.Sprintf("Configpath for %s does not exist at %s", renderedPathTemplate, base))
	}
	return base
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
		if _, err := os.Stat(viper.GetString("filedb")); os.IsNotExist(err) {
			index, _ = bleve.New(viper.GetString("filedb"), bmap)
		} else {
			index, _ = bleve.Open(viper.GetString("filedb"))
		}
	} else {
		index, _ = bleve.NewMemOnly(bmap)
	}

	for repo := range viper.GetStringMap("repos") {
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
	repoPath := configPath(viper.GetString(fmt.Sprintf("repos.%s.path", repoName)))

	fmt.Printf("Loading repo %s from %s...", repoName, repoPath)
	if !dirExists(repoPath) {
		panic(fmt.Sprintf("Repo path %s does not exist", repoPath))
	}

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
	if err != nil {
		panic(err)
	}
	defer file.Close()

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
		extensions := parser.CommonExtensions | parser.AutoHeadingIDs | parser.Mmark | parser.Footnotes | parser.AutoHeadingIDs | parser.Attributes | parser.DefinitionLists
		parser := parser.NewWithExtensions(extensions)

		md := []byte(body)

		htmlFlags := html.CommonFlags | html.HrefTargetBlank | html.FootnoteReturnLinks
		opts := html.RendererOptions{Flags: htmlFlags}
		renderer := html.NewRenderer(opts)
		item.Html = string(markdown.ToHTML(md, parser, renderer))
		//item.Html = string(blackfriday.Run([]byte(body), blackfriday.WithExtensions(blackfriday.CommonExtensions|blackfriday.HardLineBreak)))
	}

	var categories []string
	var authors []string
	var arr []interface{}
	if _, ok := f["categories"]; ok {
		arr = f["categories"].([]interface{})
		categories = make([]string, len(arr))
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
	item.Date, _ = dateparse.ParseLocal(item.RawDate)

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
		re := regexp.MustCompile(`<!--\s*more\s*-->`)
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

func catchallHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("Rendering default handler\n")
	routeName := mux.CurrentRoute(r).GetName()
	routeConfigLocation := fmt.Sprintf("routes.%s", routeName)
	layoutfilename := getTemplateFileFromConfig(fmt.Sprintf("%s.layout", routeConfigLocation), "layout.html.hb")
	templatefilename := getTemplateFileFromConfig(fmt.Sprintf("%s.template", routeConfigLocation), "template.html.hb")

	fmt.Printf("Rendering template: %s\n", templatefilename)
	context := viper.GetStringMap(routeConfigLocation)
	context["pathvars"] = mux.Vars(r)
	context["params"] = r.URL.Query()
	context["post"] = nil
	content, _ := renderTemplateFile(templatefilename, context)
	context["content"] = content

	fmt.Printf("Rendering layout: %s\n", layoutfilename)
	output, _ := renderTemplateFile(layoutfilename, context)

	if viper.IsSet(fmt.Sprintf("%s.status", routeConfigLocation)) {
		fmt.Printf("Setting custom status: %d\n", viper.GetInt(fmt.Sprintf("%s.status", routeConfigLocation)))
		w.WriteHeader(viper.GetInt(fmt.Sprintf("%s.status", routeConfigLocation)))
	}

	if viper.IsSet(fmt.Sprintf("%s.content-type", routeConfigLocation)) {
		fmt.Printf("Setting custom content-type: %s\n", viper.GetString(fmt.Sprintf("%s.content-type", routeConfigLocation)))
		w.Header().Add("Content-Type", viper.GetString(fmt.Sprintf("%s.content-type", routeConfigLocation)))
	} else {
		w.Header().Add("Content-Type", "text/html")
	}
	w.Header().Add("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
	w.Header().Add("X-Frame-Options", "SAMEORIGIN")
	w.Header().Add("X-Content-Type-Options", "nosniff")
	w.Header().Add("Upgrade-Insecure-Requests", "1")
	w.Header().Add("Referrer-Policy", "strict-origin-when-cross-origin")
	w.Header().Add("Permissions-Policy", "geolocation=(self), microphone=()")
	w.Write([]byte(output))
}

func setupRoutes(router *mux.Router) {
	routelist := make([]string, 0, len(viper.GetStringMap("routes")))
	for key := range viper.GetStringMap("routes") {
		routelist = append(routelist, key)
	}
	sort.Strings(routelist)
	for _, routeName := range routelist {
		routeConfigLocation := fmt.Sprintf("routes.%s", routeName)
		routePath := viper.GetString(fmt.Sprintf("%s.path", routeConfigLocation))
		switch viper.GetString(fmt.Sprintf("%s.handler", routeConfigLocation)) {
		case "posts":
			router.HandleFunc(routePath, postHandler).Name(routeName)
		case "static":
			dir := configPath(viper.GetString(fmt.Sprintf("%s.dir", routeConfigLocation)))
			router.PathPrefix(routePath).Handler(http.StripPrefix(routePath, http.FileServer(http.Dir(dir))))
		case "git":
			router.HandleFunc(routePath, gitHandler).Name(routeName)
		case "redirect":
		default:
			router.PathPrefix("/").HandlerFunc(catchallHandler)
		}
	}
}

func main() {
	setupConfig()
	makeDB()
	loadRepos()
	registerTemplateHelpers()
	router := mux.NewRouter()
	setupRoutes(router)

	http.Handle("/", etag.Handler(handlers.CompressHandler(router), false))

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
