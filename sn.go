package main

import (
	"crypto/sha1"
	"crypto/tls"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/araddon/dateparse"
	"github.com/aymerick/raymond"
	"github.com/gernest/front"
	"github.com/hashicorp/go-memdb"
	"github.com/radovskyb/watcher"
	"github.com/russross/blackfriday/v2"
	"github.com/spf13/viper"
	"golang.org/x/crypto/acme/autocert"
)

// Post is...
type Post struct {
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

var db *memdb.MemDB

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
	if err != nil {
		fmt.Printf("Rendering 404\n")
		// This should render a 404 if we know how
	} else {
		fmt.Printf("Matched Route: %s\n", routeMatch)
		switch viper.GetString(fmt.Sprintf("%s.handler", routeMatch)) {
		case "posts":
			output, err = postHandler(routeMatch, context)
			fmt.Fprint(w, output)
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
				w.Write(fileBytes)
				//fmt.Fprint(w, fileBytes)
			}
			//http.ServeFile(w, r, staticfile)
			break
		default:
			fmt.Printf("Rendering default handler\n")
			layoutfilename = path.Join(viper.GetString("path"), viper.GetString("template_path"), "layout.html.hb")
			fmt.Printf("Rendering layout: %s\n", layoutfilename)
			output, _ = renderTemplateFile(layoutfilename, context)
			fmt.Fprint(w, output)
		}
	}
}

func postHandler(routeMatch string, context map[string]interface{}) (string, error) {
	fmt.Printf("Rendering posts handler\n")
	layoutfilename := getTemplateFileFromConfig(fmt.Sprintf("%s.layout", routeMatch), "layout.html.hb")
	templatefilename := getTemplateFileFromConfig(fmt.Sprintf("%s.template", routeMatch), "template.html.hb")
	fmt.Printf("Rendering template: %s\n", templatefilename)
	context["post"] = nil
	if posts, postcount := postsFromVars(context); len(posts) > 0 {
		context["posts"] = posts
		context["post"] = posts[0]
		context["postcount"] = postcount
	}
	rendered, err := renderTemplateFile(templatefilename, context)
	if err != nil {
		fmt.Printf("Error rendering template: %s\n", err)
	}
	context["content"] = rendered
	fmt.Printf("Rendering layout: %s\n", layoutfilename)
	return renderTemplateFile(layoutfilename, context)
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

func postsFromVars(context map[string]interface{}) ([]Post, int) {
	var posts []Post

	searchComplete := false

	posts = make([]Post, 0)
	pathvars := context["pathvars"].(map[string]string)
	fmt.Printf("Pathvars: %+v\n", pathvars)

	txn := db.Txn(false)
	defer txn.Abort()
	if slug, ok := pathvars["slug"]; ok {
		fmt.Printf("Searching for slug \"%s\"\n", slug)
		raw, err := txn.First("post", "id", slug)
		if err != nil {
			panic(err)
		}
		if raw != nil {
			post := raw.(Post)
			posts = append(posts, post)
		}
		searchComplete = true
	}
	if category, ok := pathvars["category"]; ok {
		fmt.Printf("Searching for tag \"%s\"\n", category)
		raw, err := txn.Get("post", "categories", category)
		if err != nil {
			panic(err)
		}
		for obj := raw.Next(); obj != nil; obj = raw.Next() {
			post := obj.(Post)
			posts = append(posts, post)
		}
		searchComplete = true
	}

	if !searchComplete {
		fmt.Printf("Returning all posts\n")
		raw, err := txn.Get("post", "id")
		if err != nil {
			panic(err)
		}
		for obj := raw.Next(); obj != nil; obj = raw.Next() {
			post := obj.(Post)
			posts = append(posts, post)
		}
		searchComplete = true
	}

	sort.SliceStable(posts, func(i, j int) bool { return posts[i].Date.After(posts[j].Date) })

	front := 0
	back := 5

	if page, ok := pathvars["page"]; ok {
		pg, _ := strconv.Atoi(page)
		front = (pg - 1) * 5
		back = front + 5
	}
	back = MinOf(len(posts), back)

	return posts[front:back], len(posts)
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
			"post": {
				Name: "post",
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
						Name:    "categories",
						Unique:  false,
						Indexer: &memdb.StringSliceFieldIndex{Field: "Categories"},
					},
					"authors": {
						Name:    "authors",
						Unique:  false,
						Indexer: &memdb.StringSliceFieldIndex{Field: "Authors"},
					},
					"date": {
						Name:    "date",
						Unique:  false,
						Indexer: &memdb.StringFieldIndex{Field: "Date"},
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
	for repo, _ := range viper.GetStringMap("repos") {
		loadRepo(repo)
	}
}

func loadRepo(repoName string) {
	repoPath := path.Join(viper.GetString("path"), viper.GetString(fmt.Sprintf("repos.%s.path", repoName)))
	err := filepath.Walk(repoPath, func(path string, info os.FileInfo, err error) error {
		if info.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".md" {
			return nil
		}
		go loadPost(repoName, path)
		return nil
	})
	if err != nil {
		panic(err)
	}
	startWatching(repoPath, repoName)
}

func loadPost(repoName string, filename string) {
	var post Post

	file, err := os.Open(filename)
	if err != nil {
		panic(err)
	}

	m := front.NewMatter()
	m.Handle("---", front.YAMLHandler)

	f, body, err := m.Parse(file)
	if err != nil {
		panic(err)
	}

	post.Title = f["title"].(string)
	if val, ok := f["slug"]; ok {
		post.Slug = fmt.Sprintf("%v", val)
	} else {
		post.Slug = path.Base(filename)
	}
	post.Raw = body
	post.Repo = repoName

	ishtml, ok := f["html"]
	if ok && ishtml.(bool) {
		post.Html = body
	} else {
		post.Html = string(blackfriday.Run([]byte(body), blackfriday.WithExtensions(blackfriday.CommonExtensions|blackfriday.HardLineBreak)))
	}

	arr := f["categories"].([]interface{})
	categories := make([]string, len(arr))
	for i, v := range arr {
		categories[i] = fmt.Sprint(v)
	}
	post.Categories = categories
	arr = f["authors"].([]interface{})
	authors := make([]string, len(arr))
	for i, v := range arr {
		authors[i] = fmt.Sprint(v)
	}
	post.Authors = authors

	post.RawDate = f["date"].(string)
	post.Date, err = dateparse.ParseLocal(post.RawDate)

	txn := db.Txn(true)
	txn.Insert("post", post)
	txn.Commit()
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
				loadPost(repoName, event.Path)
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

	/*
		for path, f := range w.WatchedFiles() {
			fmt.Printf("%s: %s\n", path, f.Name())
		}
	*/

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
}

func main() {
	setupConfig()
	makeDB()
	loadRepos()
	registerTemplateHelpers()

	http.HandleFunc("/", handler)

	if viper.IsSet("ssldomain") {
		certManager := autocert.Manager{
			Prompt:     autocert.AcceptTOS,
			HostPolicy: autocert.HostWhitelist(viper.GetString("ssldomain")), //Your domain here
			Cache:      autocert.DirCache("certs"),                           //Folder for storing certificates
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
