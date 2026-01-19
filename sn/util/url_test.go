package util

import (
	"testing"
	"time"

	"github.com/spf13/viper"
)

// testItem mirrors sn.Item fields for testing without import
type testItem struct {
	Title string
	Slug  string
	Repo  string
	Date  time.Time
}

func TestGetItemURL(t *testing.T) {
	tests := []struct {
		name       string
		setup      func()
		item       testItem
		wantURL    string
		wantSuffix string // Use this when exact URL depends on dynamic config
	}{
		{
			name: "standard posts route",
			setup: func() {
				viper.Reset()
				viper.Set("rooturl", "https://example.com/")
				viper.Set("routes.posts.handler", "posts")
				viper.Set("routes.posts.path", "/posts/{slug}")
				viper.Set("routes.posts.out.posts.repo", "posts")
				viper.Set("routes.posts.out.posts.slug", "{slug}")
			},
			item:    testItem{Repo: "posts", Slug: "my-test-post"},
			wantURL: "https://example.com/posts/my-test-post",
		},
		{
			name: "custom content path",
			setup: func() {
				viper.Reset()
				viper.Set("rooturl", "https://myblog.com")
				viper.Set("routes.articles.handler", "posts")
				viper.Set("routes.articles.path", "/content/articles/{slug}")
				viper.Set("routes.articles.out.items.repo", "articles")
				viper.Set("routes.articles.out.items.slug", "{slug}")
			},
			item:    testItem{Repo: "articles", Slug: "hello-world"},
			wantURL: "https://myblog.com/content/articles/hello-world",
		},
		{
			name: "pages route with pageslug",
			setup: func() {
				viper.Reset()
				viper.Set("rooturl", "https://example.com/")
				viper.Set("routes.pages.handler", "posts")
				viper.Set("routes.pages.path", "/pages/{pageslug}")
				viper.Set("routes.pages.out.pages.repo", "pages")
				viper.Set("routes.pages.out.pages.slug", "{pageslug}")
			},
			item:    testItem{Repo: "pages", Slug: "about"},
			wantURL: "https://example.com/pages/about",
		},
		{
			name: "activitypub rooturl override",
			setup: func() {
				viper.Reset()
				viper.Set("rooturl", "http://localhost:8080/")
				viper.Set("activitypub.rooturl", "https://public.example.com/")
				viper.Set("routes.posts.handler", "posts")
				viper.Set("routes.posts.path", "/posts/{slug}")
				viper.Set("routes.posts.out.posts.repo", "posts")
				viper.Set("routes.posts.out.posts.slug", "{slug}")
			},
			item:    testItem{Repo: "posts", Slug: "test"},
			wantURL: "https://public.example.com/posts/test",
		},
		{
			name: "slug with regex constraint",
			setup: func() {
				viper.Reset()
				viper.Set("rooturl", "https://example.com/")
				viper.Set("routes.posts.handler", "posts")
				viper.Set("routes.posts.path", "/posts/{slug:[^/]+}")
				viper.Set("routes.posts.out.posts.repo", "posts")
				viper.Set("routes.posts.out.posts.slug", "{slug}")
			},
			item:    testItem{Repo: "posts", Slug: "2024-01-15-my-post"},
			wantURL: "https://example.com/posts/2024-01-15-my-post",
		},
		{
			name: "fallback when no route matches",
			setup: func() {
				viper.Reset()
				viper.Set("rooturl", "https://example.com/")
				// No matching route configured
			},
			item:       testItem{Repo: "unknown-repo", Slug: "test-post"},
			wantSuffix: "/posts/test-post", // Falls back to default pattern
		},
		{
			name: "multiple repos with different paths",
			setup: func() {
				viper.Reset()
				viper.Set("rooturl", "https://example.com/")
				viper.Set("routes.blog.handler", "posts")
				viper.Set("routes.blog.path", "/blog/{slug}")
				viper.Set("routes.blog.out.items.repo", "blog")
				viper.Set("routes.blog.out.items.slug", "{slug}")
				viper.Set("routes.docs.handler", "posts")
				viper.Set("routes.docs.path", "/documentation/{slug}")
				viper.Set("routes.docs.out.items.repo", "docs")
				viper.Set("routes.docs.out.items.slug", "{slug}")
			},
			item:    testItem{Repo: "docs", Slug: "getting-started"},
			wantURL: "https://example.com/documentation/getting-started",
		},
		{
			name: "date-based URL pattern",
			setup: func() {
				viper.Reset()
				viper.Set("rooturl", "https://example.com/")
				viper.Set("routes.posts.handler", "posts")
				viper.Set("routes.posts.path", "/posts/{year}/{month}/{slug}")
				viper.Set("routes.posts.out.posts.repo", "posts")
				viper.Set("routes.posts.out.posts.slug", "{slug}")
			},
			item: testItem{
				Repo: "posts",
				Slug: "my-post",
				Date: time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC),
			},
			wantURL: "https://example.com/posts/2024/03/my-post",
		},
		{
			name: "date-based URL with day",
			setup: func() {
				viper.Reset()
				viper.Set("rooturl", "https://example.com/")
				viper.Set("routes.posts.handler", "posts")
				viper.Set("routes.posts.path", "/{year}/{month}/{day}/{slug}")
				viper.Set("routes.posts.out.posts.repo", "posts")
				viper.Set("routes.posts.out.posts.slug", "{slug}")
			},
			item: testItem{
				Repo: "posts",
				Slug: "daily-post",
				Date: time.Date(2025, 12, 25, 0, 0, 0, 0, time.UTC),
			},
			wantURL: "https://example.com/2025/12/25/daily-post",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()

			got := GetItemURL(tt.item)

			if tt.wantURL != "" {
				if got != tt.wantURL {
					t.Errorf("GetItemURL() = %q, want %q", got, tt.wantURL)
				}
			} else if tt.wantSuffix != "" {
				if len(got) < len(tt.wantSuffix) || got[len(got)-len(tt.wantSuffix):] != tt.wantSuffix {
					t.Errorf("GetItemURL() = %q, want suffix %q", got, tt.wantSuffix)
				}
			}
		})
	}
}

func TestGetAllPostRoutePatterns(t *testing.T) {
	tests := []struct {
		name     string
		setup    func()
		wantLen  int
		wantHas  []string
		wantNot  []string
	}{
		{
			name: "single post route",
			setup: func() {
				viper.Reset()
				viper.Set("routes.posts.handler", "posts")
				viper.Set("routes.posts.path", "/posts/{slug}")
				viper.Set("routes.posts.out.posts.repo", "posts")
				viper.Set("routes.posts.out.posts.slug", "{slug}")
			},
			wantLen: 1,
			wantHas: []string{"/posts/{slug}"},
		},
		{
			name: "multiple post routes",
			setup: func() {
				viper.Reset()
				viper.Set("routes.posts.handler", "posts")
				viper.Set("routes.posts.path", "/posts/{slug}")
				viper.Set("routes.posts.out.posts.repo", "posts")
				viper.Set("routes.posts.out.posts.slug", "{slug}")
				viper.Set("routes.pages.handler", "posts")
				viper.Set("routes.pages.path", "/pages/{pageslug}")
				viper.Set("routes.pages.out.pages.repo", "pages")
				viper.Set("routes.pages.out.pages.slug", "{pageslug}")
			},
			wantLen: 2,
			wantHas: []string{"/posts/{slug}", "/pages/{pageslug}"},
		},
		{
			name: "excludes list routes (no slug param)",
			setup: func() {
				viper.Reset()
				viper.Set("routes.index.handler", "posts")
				viper.Set("routes.index.path", "/")
				viper.Set("routes.index.out.posts.repo", "posts")
				// No slug param - this is a list route
				viper.Set("routes.posts.handler", "posts")
				viper.Set("routes.posts.path", "/posts/{slug}")
				viper.Set("routes.posts.out.posts.repo", "posts")
				viper.Set("routes.posts.out.posts.slug", "{slug}")
			},
			wantLen: 1,
			wantHas: []string{"/posts/{slug}"},
			wantNot: []string{"/"},
		},
		{
			name: "excludes non-posts handlers",
			setup: func() {
				viper.Reset()
				viper.Set("routes.static.handler", "static")
				viper.Set("routes.static.path", "/static")
				viper.Set("routes.posts.handler", "posts")
				viper.Set("routes.posts.path", "/posts/{slug}")
				viper.Set("routes.posts.out.posts.repo", "posts")
				viper.Set("routes.posts.out.posts.slug", "{slug}")
			},
			wantLen: 1,
			wantHas: []string{"/posts/{slug}"},
			wantNot: []string{"/static"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()

			patterns := GetAllPostRoutePatterns()

			if len(patterns) != tt.wantLen {
				t.Errorf("GetAllPostRoutePatterns() returned %d patterns, want %d", len(patterns), tt.wantLen)
			}

			for _, want := range tt.wantHas {
				found := false
				for _, got := range patterns {
					if got == want {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("GetAllPostRoutePatterns() missing pattern %q, got %v", want, patterns)
				}
			}

			for _, notWant := range tt.wantNot {
				for _, got := range patterns {
					if got == notWant {
						t.Errorf("GetAllPostRoutePatterns() should not include %q, got %v", notWant, patterns)
					}
				}
			}
		})
	}
}

func TestGetRoutePatternForRepo(t *testing.T) {
	tests := []struct {
		name    string
		setup   func()
		repo    string
		want    string
	}{
		{
			name: "finds matching repo",
			setup: func() {
				viper.Reset()
				viper.Set("routes.posts.handler", "posts")
				viper.Set("routes.posts.path", "/posts/{slug}")
				viper.Set("routes.posts.out.posts.repo", "posts")
				viper.Set("routes.posts.out.posts.slug", "{slug}")
			},
			repo: "posts",
			want: "/posts/{slug}",
		},
		{
			name: "returns empty for non-existent repo",
			setup: func() {
				viper.Reset()
				viper.Set("routes.posts.handler", "posts")
				viper.Set("routes.posts.path", "/posts/{slug}")
				viper.Set("routes.posts.out.posts.repo", "posts")
				viper.Set("routes.posts.out.posts.slug", "{slug}")
			},
			repo: "nonexistent",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()

			got := GetRoutePatternForRepo(tt.repo)

			if got != tt.want {
				t.Errorf("GetRoutePatternForRepo(%q) = %q, want %q", tt.repo, got, tt.want)
			}
		})
	}
}
