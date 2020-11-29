package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/aymerick/raymond"
	"github.com/gernest/front"
	"github.com/hashicorp/go-memdb"
	"github.com/russross/blackfriday/v2"
	"github.com/spf13/viper"
)

// Post is...
type Post struct {
	Title      string
	Slug       string
	Repo       string
	Categories []string
	Authors    []int
	Date       time.Time
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
			http.ServeFile(w, r, staticfile)
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
	context["posts"] = postsFromVars(context)
	context["post"] = context["posts"].([]Post)[0]
	rendered, err := renderTemplateFile(templatefilename, context)
	if err != nil {
		fmt.Printf("Error rendering template: %s\n", err)
	}
	context["content"] = rendered
	fmt.Printf("Rendering layout: %s\n", layoutfilename)
	return renderTemplateFile(layoutfilename, context)
}

func postsFromVars(context map[string]interface{}) []Post {
	var posts []Post
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
	}
	return posts
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
	post.Html = string(blackfriday.Run([]byte(body), blackfriday.WithExtensions(blackfriday.CommonExtensions|blackfriday.HardLineBreak)))

	txn := db.Txn(true)
	txn.Insert("post", post)
	txn.Commit()
}

func main() {
	setupConfig()
	makeDB()
	loadRepos()

	http.HandleFunc("/", handler)
	fmt.Printf("Starting server on localhost:%d\n", viper.GetInt("port"))
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", viper.GetInt("port")), nil))
}
