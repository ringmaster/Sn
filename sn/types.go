package sn

import "time"

// Item is...
type Item struct {
	Title       string
	Slug        string
	Repo        string
	Categories  []string
	Authors     []string
	Frontmatter map[string]string
	Date        time.Time
	RawDate     string
	Raw         string
	Html        string
	Source      string
	Id          int64
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
