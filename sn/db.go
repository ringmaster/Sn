package sn

import (
	"bytes"
	"database/sql"
	"fmt"
	"log"
	"log/slog"
	"math"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/araddon/dateparse"
	"github.com/arpitgogia/rake"
	"github.com/radovskyb/watcher"
	"github.com/spf13/viper"
	"github.com/yuin/goldmark"
	emoji "github.com/yuin/goldmark-emoji"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	meta "github.com/yuin/goldmark-meta"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"

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
	var dburi string
	dbfile := ConfigPath("dbfile", WithDefault(":memory:"), OptionallyExist())

	if dbfile == ":memory:" {
		dburi = "file:sn?mode=memory&cache=shared"
	} else {
		dburi = fmt.Sprintf("file:%s", dbfile)
	}

	if viper.IsSet("cleandb") && viper.GetBool("cleandb") {
		os.Remove(dbfile)
	}

	var err error
	db, err = sql.Open("sqlite", dburi)

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

func DBQuery(query string) (*sql.Rows, error) {
	return db.Query(query)
}

func DBLoadReposSync() {
	for repoName := range viper.GetStringMap("repos") {
		repoPath := ConfigPath(fmt.Sprintf("repos.%s.path", repoName))

		if !DirExists(repoPath) {
			panic(fmt.Sprintf("Repo path %s does not exist", repoPath))
		}

		errz := filepath.Walk(repoPath, func(path string, info os.FileInfo, err error) error {
			if info.IsDir() {
				return nil
			}
			if filepath.Ext(path) != ".md" {
				return nil
			}
			item, err := loadItem(repoName, repoPath, path)
			if err == nil {
				insertItem(item)
			} else {
				slog.Error(fmt.Sprintln(err))
			}
			return nil
		})
		if errz != nil {
			panic(errz)
		}
	}
}

func RowToMapSlice(rows *sql.Rows) ([][]string, error) {
	// Slice to hold the maps
	var maps [][]string

	// Get column names from the rows
	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("failed to get columns: %w", err)
	}

	// Prepare a slice of interfaces to hold the values for each column
	for rows.Next() {
		// Create a slice of interface{}'s to represent each column,
		// and a slice of string to hold the values (as all values will be converted to strings)
		values := make([]interface{}, len(cols))
		stringValues := make([]string, len(cols))
		for i := range values {
			// Point each interface{} to the corresponding string in the stringValues slice
			values[i] = &stringValues[i]
		}

		// Scan the row values into the interfaces
		err := rows.Scan(values...)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		// Append the map to the slice
		maps = append(maps, stringValues)
	}

	// Check for errors from iterating over rows
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %w", err)
	}

	return maps, nil
}

func DBLoadRepo(repoName string) {
	const bufferLen = 5000
	itempaths := make(chan string, bufferLen)
	repoPath := ConfigPath(fmt.Sprintf("repos.%s.path", repoName))

	const workers = 1
	for w := 0; w < workers; w++ {
		go func(id int, itempaths <-chan string) {
			for path := range itempaths {
				item, err := loadItem(repoName, repoPath, path)
				if err == nil {
					insertItem(item)
				} else {
					slog.Error(fmt.Sprintln(err))
				}
			}
		}(w, itempaths)
	}

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
	startWatching(repoPath, repoName)
}

func reloadItem(repoName string, repoPath string, filename string) (Item, error) {
	var item_id int64
	if err := db.QueryRow("SELECT id FROM items WHERE repo = ? and source = ?", repoName, filename).Scan(&item_id); err == nil && item_id > 0 {
		db.Exec("DELETE FROM items_tags WHERE item_id = ?", item_id)
		db.Exec("DELETE FROM items_authors WHERE item_id = ?", item_id)
		db.Exec("DELETE FROM frontmatter WHERE item_id = ?", item_id)
		db.Exec("DELETE FROM items WHERE repo = ? and source = ?", repoName, filename)
	} else {
		slog.Warn(fmt.Sprintf("No existing file in repo %s source file %s\n", repoName, filename))
	}

	item, err := loadItem(repoName, repoPath, filename)
	if err == nil {
		insertItem(item)
	}
	return item, err
}

func loadItem(repoName string, repoPath string, filename string) (Item, error) {
	var item Item

	item.Source = filename

	file, err := os.ReadFile(filename)
	if err != nil {
		panic(err)
	}

	var buf bytes.Buffer
	md := goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			meta.New(
				meta.WithStoresInDocument(),
			),
			emoji.Emoji,
			highlighting.Highlighting,
			extension.Typographer,
		),
		goldmark.WithParserOptions(
			parser.WithBlockParsers(),
			parser.WithInlineParsers(),
			parser.WithParagraphTransformers(),
			parser.WithAutoHeadingID(),
			parser.WithAttribute(),
		),
		goldmark.WithRendererOptions(
			html.WithHardWraps(),
			html.WithUnsafe(),
		),
	)
	context := parser.NewContext()
	if err := md.Convert(file, &buf, parser.WithContext(context)); err != nil {
		panic(err)
	}
	f := meta.Get(context)

	if len(file) < 3 {
		return item, fmt.Errorf("    -- %s is too short to have frontmatter", filename)
	}

	if val, ok := f["title"]; ok {
		item.Title = fmt.Sprintf("%v", val)
	} else {
		item.Title = path.Base(filename)
	}
	if val, ok := f["slug"]; ok {
		item.Slug = fmt.Sprintf("%v", val)
	} else if val, ok := f["permalink"]; ok {
		item.Slug = fmt.Sprintf("%v", val)
	} else {
		// chop off the repo directory and keep any extra paths with the filename
		tfilename := path.Base(filename)
		item.Slug = tfilename[:len(tfilename)-len(filepath.Ext(tfilename))]
	}
	if len(repoPath) < len(path.Dir(filename)) {
		item.Slug = path.Join(path.Dir(filename)[len(repoPath)+1:], item.Slug)
	}
	item.Raw = string(file[:])
	item.Repo = repoName

	ishtml, ok := f["html"]
	if ok && ishtml.(bool) {
		item.Html = item.Raw
	} else {
		item.Html = buf.String()
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
		rakes := rake.WithText(string(file[:]))
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

	// Get frontmatter from frontmatter
	item.Frontmatter = make(map[string]string)
	for fk, fv := range f {
		switch fk {
		case "authors", "categories", "slug", "title", "date":
		default:
			switch zz := fv.(type) {
			case string:
				item.Frontmatter[fk] = zz
			case int, float64:
				item.Frontmatter[fk] = fmt.Sprint(zz)
			case bool:
				if zz {
					item.Frontmatter[fk] = "true"
				} else {
					item.Frontmatter[fk] = "false"
				}
			default:
				// do nothing, sadly
			}
		}
	}

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
		return 0, fmt.Errorf("error inserting item \"%s\": %s", item.Slug, err)
	}

	item.Id, _ = result.LastInsertId()

	insertCategories(item)
	insertAuthors(item)
	insertFrontmatter(item)

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

func insertFrontmatter(item Item) {
	for k, v := range item.Frontmatter {
		stmt, _ := db.Prepare("INSERT INTO frontmatter (item_id, fieldname, value) VALUES (?,?,?)")
		stmt.Exec(item.Id, k, v)
	}
}

func startWatching(path string, repoName string) {
	w := watcher.New()
	w.SetMaxEvents(1)
	w.FilterOps(watcher.Create, watcher.Write)
	r := regexp.MustCompile(".md$")
	w.AddFilterHook(watcher.RegexFilterHook(r, false))

	go func() {
		for {
			select {
			case event := <-w.Event:
				reloadItem(repoName, path, event.Path)
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
}

type ItemQuery struct {
	PerPage     int
	Page        int
	Slug        *string
	Repo        *string
	Category    *string
	Author      *string
	Search      *string
	OrderBy     *string
	Frontmatter map[string]string
}

func setQryValue(field **string, params map[string]interface{}, key string) {
	if values, ok := params[key]; ok {
		str := values.(string)
		*field = &str
	}
}

func ItemsFromOutvals(outVariableParams map[string]interface{}, context map[string]interface{}) ItemResult {
	qry := ItemQuery{Page: 1, Frontmatter: make(map[string]string)}

	var ok bool
	// routeParameters is a map of the parameters that define this route path
	routeParameters := context["pathvars"].(map[string]string)
	if qry.PerPage, ok = outVariableParams["paginate_count"].(int); !ok {
		qry.PerPage = 5
	}
	var paginate_name string
	if paginate_name, ok = outVariableParams["paginate_name"].(string); !ok {
		paginate_name = "page"
	}
	if page, ok := context["params"].(url.Values)[paginate_name]; ok {
		qry.Page, _ = strconv.Atoi(page[0])
	}
	if page, ok := routeParameters[paginate_name]; ok {
		qry.Page, _ = strconv.Atoi(page)
	}
	// Add the query params into the route parameters for replacement in the outVariable params
	for param, param_value := range context["params"].(url.Values) {
		routeParameters[fmt.Sprintf("params.%s", param)] = param_value[0]
	}

	params := replaceParams(outVariableParams, routeParameters)
	setQryValue(&qry.Slug, params, "slug")
	setQryValue(&qry.Repo, params, "repo")
	setQryValue(&qry.Category, params, "category")
	setQryValue(&qry.Author, params, "author")
	setQryValue(&qry.Search, params, "search")
	setQryValue(&qry.OrderBy, params, "order_by")

	return ItemsFromItemQuery(qry)
}

func ItemsFromItemQuery(qry ItemQuery) ItemResult {
	var items []Item
	var pg int

	items = make([]Item, 0)

	front := (qry.Page - 1) * qry.PerPage
	pg = qry.Page

	var sql string = `FROM items
	LEFT JOIN items_authors ON items.id = items_authors.item_id
   LEFT JOIN authors ON authors.id = items_authors.author_id
	LEFT JOIN items_categories ON items.id = items_categories.item_id
   LEFT JOIN categories ON categories.id = items_categories.category_id WHERE 1`
	var queryvals []any

	sql, queryvals = andSQL("slug", qry.Slug, sql, queryvals)
	sql, queryvals = andSQL("repo", qry.Repo, sql, queryvals)
	sql, queryvals = andSQL("category", qry.Category, sql, queryvals)
	sql, queryvals = andSQL("author", qry.Author, sql, queryvals)
	if qry.Search != nil {
		queryvals = append(queryvals, fmt.Sprintf("%%%s%%", *qry.Search))
		sql = fmt.Sprintf("%s AND raw LIKE ?", sql)
	}

	var orderby string = "ORDER BY publishedon DESC"
	if qry.OrderBy != nil {
		orderby = fmt.Sprintf("ORDER BY %s", *qry.OrderBy)
	}

	countsql := fmt.Sprintf("SELECT count(distinct items.id) %s", sql)

	var itemCount int
	db.QueryRow(countsql, queryvals...).Scan(&itemCount)

	if itemCount > 0 {
		sql = fmt.Sprintf("SELECT distinct items.id, repo, title, slug, publishedon, rawpublishedon, raw, html, source %s %s LIMIT %d, %d", sql, orderby, front, qry.PerPage)

		rows, err := db.Query(sql, queryvals...)

		if err != nil {
			slog.Error(fmt.Sprintf("Error: %#v", err))
			return ItemResult{Items: []Item{}, Total: 0, Pages: 0, Page: 0}
		}
		defer rows.Close()

		for rows.Next() {
			var item Item
			var interimDate string
			err = rows.Scan(&item.Id, &item.Repo, &item.Title, &item.Slug, &interimDate, &item.RawDate, &item.Raw, &item.Html, &item.Source)

			item.Date, _ = dateparse.ParseLocal(interimDate)
			if err != nil {
				panic(err)
			}

			categories, err := db.Query("SELECT category FROM categories INNER JOIN items_categories ON items_categories.category_id = categories.id WHERE items_categories.item_id = ?", item.Id)

			if err != nil {
				panic(err)
			}

			var category string
			for categories.Next() {
				categories.Scan(&category)
				item.Categories = append(item.Categories, category)
			}

			authors, err := db.Query("SELECT author FROM authors INNER JOIN items_authors ON items_authors.author_id = authors.id WHERE items_authors.item_id = ?", item.Id)

			if err != nil {
				panic(err)
			}

			var author string
			for authors.Next() {
				authors.Scan(&author)
				item.Authors = append(item.Authors, author)
			}

			frontmatters, err := db.Query("SELECT fieldname, value FROM frontmatter WHERE item_id = ?", item.Id)

			if err != nil {
				panic(err)
			}

			var fm_key, fm_value string
			item.Frontmatter = make(map[string]string)
			for frontmatters.Next() {
				frontmatters.Scan(&fm_key, &fm_value)
				item.Frontmatter[fm_key] = fm_value
			}

			items = append(items, item)
		}
	}

	return ItemResult{Items: items, Total: int(itemCount), Pages: int(math.Ceil(float64(itemCount) / float64(qry.PerPage))), Page: pg}
}

func replaceParams(values map[string]interface{}, params map[string]string) map[string]interface{} {
	for k1, v1 := range values {
		temp := v1
		for k, v := range params {
			switch nv := temp.(type) {
			case string:
				temp = strings.ReplaceAll(nv, fmt.Sprintf("{%s}", k), v)
			}
		}
		values[k1] = temp
	}
	return values
}

func andSQL(paramName string, qryParam *string, sql string, queryvals []any) (string, []any) {
	if qryParam != nil {
		queryvals = append(queryvals, qryParam)
		sql = fmt.Sprintf("%s AND %s = ?", sql, paramName)
	}
	return sql, queryvals
}
