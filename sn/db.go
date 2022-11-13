package sn

import (
	"bytes"
	"database/sql"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"time"

	"github.com/araddon/dateparse"
	"github.com/arpitgogia/rake"
	"github.com/gernest/front"
	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
	"github.com/radovskyb/watcher"
	"github.com/spf13/viper"

	_ "modernc.org/sqlite"
	//_ "github.com/mattn/go-sqlite3"
)

var db *sql.DB

func schema() string {
	return `
	CREATE TABLE IF NOT EXISTS "items" (
		"id" integer PRIMARY KEY AUTOINCREMENT NOT NULL,
		"slug" varchar(255) NOT NULL,
		"repo" varchar(255) NOT NULL,
		"publishedon" timestamp(128),
		"rawpublishedon" varchar(128),
		"raw" text(128),
		"html" text(128),
		"source" varchar(128),
		"title" varchar(255) NOT NULL
	  );
	  
	  CREATE INDEX IF NOT EXISTS items_repo ON "items" ("repo" ASC);
	  
	  CREATE UNIQUE INDEX IF NOT EXISTS items_repo_slug ON "items" ("slug" ASC, "repo" ASC);
	  
	  CREATE INDEX IF NOT EXISTS items_published_on ON "items" ("publishedon" ASC);
	  
	  CREATE TABLE IF NOT EXISTS "authors" (
		"id" integer PRIMARY KEY AUTOINCREMENT NOT NULL,
		"author" varchar(128)
	  );
	  
	  CREATE UNIQUE INDEX IF NOT EXISTS authors_author ON "authors" ("author" ASC);
	  
	  CREATE TABLE IF NOT EXISTS "tags" (
		"id" integer PRIMARY KEY AUTOINCREMENT NOT NULL,
		"tag" varchar(128) NOT NULL
	  );
	  
	  CREATE UNIQUE INDEX IF NOT EXISTS tags_tag ON "tags" ("tag" ASC);
	  
	  CREATE TABLE IF NOT EXISTS "frontmatter" (
		"id" integer PRIMARY KEY AUTOINCREMENT NOT NULL,
		"item_id" integer(128) NOT NULL,
		"fieldname" varchar(255) NOT NULL,
		"value" text(128),
		FOREIGN KEY (item_id) REFERENCES "items" (id)
	  );
	  
	  CREATE INDEX IF NOT EXISTS frontmatter_fieldname ON "frontmatter" ("fieldname" ASC);
	  
	  CREATE INDEX IF NOT EXISTS frontmatter_item_id ON "frontmatter" ("item_id" ASC);
	  
	  CREATE INDEX IF NOT EXISTS frontmatter_fieldname_value ON "frontmatter" ("fieldname" ASC, "value" ASC);
	  
	  CREATE TABLE IF NOT EXISTS "items_authors" (
		"id" integer PRIMARY KEY AUTOINCREMENT NOT NULL,
		"item_id" integer(128) NOT NULL,
		"author_id" integer(128),
		FOREIGN KEY (item_id) REFERENCES "items" (id),
		FOREIGN KEY (author_id) REFERENCES "authors" (id)
	  );
	  
	  CREATE INDEX IF NOT EXISTS items_authors_author_id ON "items_authors" ("author_id" ASC);
	  
	  CREATE UNIQUE INDEX IF NOT EXISTS items_authors_item_id_author_id ON "items_authors" ("item_id" ASC, "author_id" ASC);
	  
	  CREATE INDEX IF NOT EXISTS iterms_authors_item_id ON "items_authors" ("item_id" ASC);
	  
	  CREATE TABLE IF NOT EXISTS "items_tags" (
		"id" integer PRIMARY KEY AUTOINCREMENT NOT NULL,
		"item_id" integer(128),
		"tag_id" integer(128),
		FOREIGN KEY (item_id) REFERENCES "items" (id),
		FOREIGN KEY (tag_id) REFERENCES "tags" (id)
	  );
	  
	  CREATE INDEX IF NOT EXISTS items_tags_item_id ON "items_tags" ("item_id" ASC);
	  
	  CREATE INDEX IF NOT EXISTS items_tags_tag_id ON "items_tags" ("tag_id" ASC);	  
	`
}

func DBConnect() {
	dbfile := ConfigPath("dbfile", WithDefault(":memory:"), OptionallyExist())
	var err error
	db, err = sql.Open("sqlite", dbfile)

	if err != nil {
		log.Fatal(err)
	}

	db.Exec(schema())
}

func DBClose() {
	db.Close()
}

func DBLoadRepos() {
	for repo := range viper.GetStringMap("repos") {
		DBLoadRepo(repo)
	}
}

func DBLoadRepo(repoName string) {
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
	repoPath := ConfigPath(fmt.Sprintf("repos.%s.path", repoName))

	fmt.Printf("Loading repo %s from %s...", repoName, repoPath)
	if !DirExists(repoPath) {
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
	//startWatching(repoPath, repoName)
}

func loadItem(repoName string, filename string) {
	var item Item

	file, err := ioutil.ReadFile(filename)
	if err != nil {
		panic(err)
	}

	m := front.NewMatter()
	m.Handle("---", front.YAMLHandler)

	fmt.Println(filename)
	if len(file) < 3 {
		return
	}

	f, body, err := m.Parse(bytes.NewReader(file))
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

	for _, category := range categories {
		stmt, _ := db.Prepare("INSERT INTO tags (tag) VALUES (?)")
		stmt.Exec(category)
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

	_, err = db.Query(
		"INSERT INTO items (slug, repo, publishedon, rawpublishedon, raw, html, source, title) VALUES (?,?,?,?,?,?,?,?)",
		item.Slug,
		item.Repo,
		item.Date,
		item.RawDate,
		item.Raw,
		item.Html,
		filename,
		item.Title,
	)

	if err != nil {
		fmt.Printf("Error inserting item \"%s\": %s\n", item.Slug, err)
	}
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

// @todo refactor this to use specific tuple values instead of the full context?
func ItemsFromQuery(query string, context map[string]interface{}) ItemResult {
	var items []Item
	var pg int

	items = make([]Item, 0)

	pathvars := context["pathvars"].(map[string]string)
	perPage := 5
	pg = 1
	if page, ok := context["params"].(url.Values)["page"]; ok {
		pg, _ = strconv.Atoi(page[0])
	}
	if page, ok := pathvars["page"]; ok {
		pg, _ = strconv.Atoi(page)
	}
	//front := (pg - 1) * perPage

	fmt.Printf("Bleve search \"%s\"\n", query)

	rows, err := db.Query("SELECT id, repo, title, slug, publishedon, rawpublishedon, raw, html, source FROM items WHERE " + query)
	if err != nil {
		panic(err)
	}
	defer rows.Close()

	if err != nil {
		fmt.Printf("Error: %#v", err)
	}
	var item Item
	itemCount := 0
	for rows.Next() {
		var id int
		var source string
		var interimDate string
		err = rows.Scan(&id, &item.Repo, &item.Title, &item.Slug, &interimDate, &item.RawDate, &item.RawDate, &item.Html, &source)
		item.Date, _ = dateparse.ParseLocal(interimDate)
		if err != nil {
			panic(err)
		}
		items = append(items, item)
		itemCount++
	}

	return ItemResult{Items: items, Total: int(itemCount), Pages: int(math.Ceil(float64(itemCount) / float64(perPage))), Page: pg}
}
