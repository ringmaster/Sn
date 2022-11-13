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
	  
	  CREATE TABLE IF NOT EXISTS "categories" (
		"id" integer PRIMARY KEY AUTOINCREMENT NOT NULL,
		"category" varchar(128) NOT NULL
	  );
	  
	  CREATE UNIQUE INDEX IF NOT EXISTS categories_category ON "categories" ("category" ASC);
	  
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
	  
	  CREATE TABLE IF NOT EXISTS "items_categories" (
		"id" integer PRIMARY KEY AUTOINCREMENT NOT NULL,
		"item_id" integer(128),
		"category_id" integer(128),
		FOREIGN KEY (item_id) REFERENCES "items" (id),
		FOREIGN KEY (category_id) REFERENCES "categories" (id)
	  );
	  
	  CREATE INDEX IF NOT EXISTS items_categories_item_id ON "items_categories" ("item_id" ASC);
	  
	  CREATE INDEX IF NOT EXISTS items_categories_category_id ON "items_categories" ("category_id" ASC);	  
	`
}

func DBConnect() {
	dbfile := ConfigPath("dbfile", WithDefault(":memory:"), OptionallyExist())

	if viper.IsSet("cleandb") && viper.GetBool("cleandb") {
		fmt.Printf("DELETING database file %s\n", dbfile)
		os.Remove(dbfile)
	}

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
				item, err := loadItem(repoName, path)
				if err == nil {
					insertItem(item)
				} else {
					fmt.Println(err)
				}
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

func loadItem(repoName string, filename string) (Item, error) {
	var item Item

	item.Source = filename

	file, err := ioutil.ReadFile(filename)
	if err != nil {
		panic(err)
	}

	m := front.NewMatter()
	m.Handle("---", front.YAMLHandler)

	fmt.Println(filename)
	if len(file) < 3 {
		return item, fmt.Errorf("the file %s is too short to have frontmatter", filename)
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
	}

	// Get Categories from frontmatter
	var categories []string
	if _, ok := f["categories"]; ok {
		arr := f["categories"].([]interface{})
		categories = make([]string, len(arr))
		for i, v := range arr {
			categories[i] = fmt.Sprint(v)
		}
	}

	// Optionally derive categories from item content
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

	item.Categories = categories

	// Get authors from frontmatter
	var authors []string
	if _, ok := f["authors"]; ok {
		arr := f["authors"].([]interface{})
		authors = make([]string, len(arr))
		for i, v := range arr {
			authors[i] = fmt.Sprint(v)
		}
	}
	item.Authors = authors

	// Get a real date from frontmatter or from filesystem
	if _, ok := f["date"]; ok {
		item.RawDate = f["date"].(string)
	} else {
		filestat, _ := os.Stat(filename)
		item.RawDate = filestat.ModTime().String()
	}
	item.Date, _ = dateparse.ParseLocal(item.RawDate)

	return item, nil
}

func insertItem(item Item) (int64, error) {
	result, err := db.Exec(
		"INSERT INTO items (slug, repo, publishedon, rawpublishedon, raw, html, source, title) VALUES (?,?,?,?,?,?,?,?)",
		item.Slug,
		item.Repo,
		item.Date,
		item.RawDate,
		item.Raw,
		item.Html,
		item.Source,
		item.Title,
	)

	if err != nil {
		fmt.Printf("Error inserting item \"%s\": %s\n", item.Slug, err)
		return 0, fmt.Errorf("error inserting item \"%s\": %s", item.Slug, err)
	}

	item.Id, _ = result.LastInsertId()

	insertCategories(item)
	insertAuthors(item)

	return item.Id, nil
}

func insertCategories(item Item) {
	var categorymap map[string]int64 = make(map[string]int64, len(item.Categories))

	for _, category := range item.Categories {
		stmt, _ := db.Prepare("INSERT INTO categories (category) VALUES (?)")
		result, err := stmt.Exec(category)
		var category_id int64
		if err == nil {
			category_id, err = result.LastInsertId()
			if err != nil {
				panic(err)
			}
		} else {
			db.QueryRow("SELECT id FROM categories WHERE category = ?", category).Scan(&category_id)
		}
		categorymap[category] = category_id
	}

	for _, category_id := range categorymap {
		stmt, _ := db.Prepare("INSERT INTO items_categories (item_id, category_id) VALUES (?, ?)")
		stmt.Exec(item.Id, category_id)
	}
}

func insertAuthors(item Item) {
	var authormap map[string]int64 = make(map[string]int64, len(item.Authors))

	for _, author := range item.Authors {
		stmt, _ := db.Prepare("INSERT INTO authors (author) VALUES (?)")
		result, err := stmt.Exec(author)
		var author_id int64
		if err == nil {
			author_id, err = result.LastInsertId()
			if err != nil {
				panic(err)
			}
		} else {
			db.QueryRow("SELECT id FROM authors WHERE author = ?", author).Scan(&author_id)
		}
		authormap[author] = author_id
	}

	for _, author_id := range authormap {
		stmt, _ := db.Prepare("INSERT INTO items_authors (item_id, author_id) VALUES (?, ?)")
		stmt.Exec(item.Id, author_id)
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
