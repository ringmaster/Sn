package activitypub

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

// TestConvertTagsToActivityPub tests the tag conversion function
func TestConvertTagsToActivityPub(t *testing.T) {
	tags := []string{"golang", "programming", "testing"}
	result := convertTagsToActivityPub(tags)

	if len(result) != 3 {
		t.Errorf("Expected 3 tags, got %d", len(result))
	}

	for i, tag := range result {
		if tag.Type != "Hashtag" {
			t.Errorf("Expected tag type 'Hashtag', got %q", tag.Type)
		}
		expectedName := "#" + tags[i]
		if tag.Name != expectedName {
			t.Errorf("Expected tag name %q, got %q", expectedName, tag.Name)
		}
	}
}

// TestConvertTagsToActivityPubEmpty tests empty tag slice
func TestConvertTagsToActivityPubEmpty(t *testing.T) {
	result := convertTagsToActivityPub([]string{})
	if result != nil && len(result) != 0 {
		t.Errorf("Expected empty/nil result for empty tags, got %v", result)
	}
}

// TestBlogPostStruct tests that BlogPost struct has expected fields
func TestBlogPostStruct(t *testing.T) {
	bp := BlogPost{
		Title:           "Test Title",
		URL:             "https://example.com/post",
		HTMLContent:     "<p>Content</p>",
		MarkdownContent: "Content",
		Summary:         "Test summary",
		Tags:            []string{"test"},
		Authors:         []string{"author1"},
		Repo:            "blog",
		Slug:            "test-post",
	}

	if bp.Title != "Test Title" {
		t.Errorf("Expected Title 'Test Title', got %q", bp.Title)
	}
	if bp.URL != "https://example.com/post" {
		t.Errorf("Expected URL 'https://example.com/post', got %q", bp.URL)
	}
	if len(bp.Tags) != 1 || bp.Tags[0] != "test" {
		t.Errorf("Expected Tags ['test'], got %v", bp.Tags)
	}
	if len(bp.Authors) != 1 || bp.Authors[0] != "author1" {
		t.Errorf("Expected Authors ['author1'], got %v", bp.Authors)
	}
}

// TestIsActivityPubEnabledForRepo tests the repo-level ActivityPub toggle
func TestIsActivityPubEnabledForRepo(t *testing.T) {
	tests := []struct {
		name         string
		setup        func()
		repo         string
		expected     bool
	}{
		{
			name: "globally disabled",
			setup: func() {
				viper.Reset()
				viper.Set("activitypub.enabled", false)
			},
			repo:     "blog",
			expected: false,
		},
		{
			name: "globally enabled, no repo config",
			setup: func() {
				viper.Reset()
				viper.Set("activitypub.enabled", true)
			},
			repo:     "blog",
			expected: true,
		},
		{
			name: "globally enabled, repo explicitly enabled",
			setup: func() {
				viper.Reset()
				viper.Set("activitypub.enabled", true)
				viper.Set("repos.blog.activitypub", true)
			},
			repo:     "blog",
			expected: true,
		},
		{
			name: "globally enabled, repo explicitly disabled",
			setup: func() {
				viper.Reset()
				viper.Set("activitypub.enabled", true)
				viper.Set("repos.private.activitypub", false)
			},
			repo:     "private",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()
			result := isActivityPubEnabledForRepo(tt.repo)
			if result != tt.expected {
				t.Errorf("isActivityPubEnabledForRepo(%q) = %v, want %v", tt.repo, result, tt.expected)
			}
		})
	}
}

// TestGetRepoOwner tests fallback chain for repo ownership
func TestGetRepoOwner(t *testing.T) {
	tests := []struct {
		name     string
		setup    func()
		repo     string
		expected string
	}{
		{
			name: "repo has explicit owner",
			setup: func() {
				viper.Reset()
				viper.Set("repos.blog.owner", "alice")
			},
			repo:     "blog",
			expected: "alice",
		},
		{
			name: "fallback to primary ActivityPub user",
			setup: func() {
				viper.Reset()
				viper.Set("activitypub.primary_user", "bob")
				viper.Set("users.bob.passwordhash", "hash")
			},
			repo:     "blog",
			expected: "bob",
		},
		{
			name: "fallback to first user",
			setup: func() {
				viper.Reset()
				viper.Set("users.charlie.passwordhash", "hash")
			},
			repo:     "blog",
			expected: "charlie",
		},
		{
			name: "no users configured",
			setup: func() {
				viper.Reset()
			},
			repo:     "blog",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()
			result := getRepoOwner(tt.repo)
			if result != tt.expected {
				t.Errorf("getRepoOwner(%q) = %q, want %q", tt.repo, result, tt.expected)
			}
		})
	}
}

// TestGetPostPrimaryAuthor tests author resolution with fallbacks
func TestGetPostPrimaryAuthor(t *testing.T) {
	tests := []struct {
		name     string
		setup    func()
		post     *BlogPost
		expected string
	}{
		{
			name: "valid first author",
			setup: func() {
				viper.Reset()
				viper.Set("users.alice.passwordhash", "hash")
				viper.Set("users.bob.passwordhash", "hash")
			},
			post:     &BlogPost{Authors: []string{"alice", "bob"}, Repo: "blog"},
			expected: "alice",
		},
		{
			name: "first author invalid, second valid",
			setup: func() {
				viper.Reset()
				viper.Set("users.bob.passwordhash", "hash")
			},
			post:     &BlogPost{Authors: []string{"unknown", "bob"}, Repo: "blog"},
			expected: "bob",
		},
		{
			name: "no valid authors, fallback to repo owner",
			setup: func() {
				viper.Reset()
				viper.Set("repos.blog.owner", "charlie")
				viper.Set("users.charlie.passwordhash", "hash")
			},
			post:     &BlogPost{Authors: []string{"unknown"}, Repo: "blog"},
			expected: "charlie",
		},
		{
			name: "empty authors, fallback to repo owner",
			setup: func() {
				viper.Reset()
				viper.Set("repos.blog.owner", "dave")
				viper.Set("users.dave.passwordhash", "hash")
			},
			post:     &BlogPost{Authors: []string{}, Repo: "blog"},
			expected: "dave",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()
			result := getPostPrimaryAuthor(tt.post)
			if result != tt.expected {
				t.Errorf("getPostPrimaryAuthor() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// TestBuildPostAttribution tests attribution URL generation
func TestBuildPostAttribution(t *testing.T) {
	tests := []struct {
		name        string
		setup       func()
		post        *BlogPost
		baseURL     string
		checkResult func(t *testing.T, result interface{})
	}{
		{
			name: "single valid author",
			setup: func() {
				viper.Reset()
				viper.Set("users.alice.passwordhash", "hash")
			},
			post:    &BlogPost{Authors: []string{"alice"}, Repo: "blog"},
			baseURL: "https://example.com",
			checkResult: func(t *testing.T, result interface{}) {
				str, ok := result.(string)
				if !ok {
					t.Error("Expected string result for single author")
					return
				}
				if str != "https://example.com/@alice" {
					t.Errorf("Got %q, want https://example.com/@alice", str)
				}
			},
		},
		{
			name: "multiple valid authors",
			setup: func() {
				viper.Reset()
				viper.Set("users.alice.passwordhash", "hash")
				viper.Set("users.bob.passwordhash", "hash")
			},
			post:    &BlogPost{Authors: []string{"alice", "bob"}, Repo: "blog"},
			baseURL: "https://example.com",
			checkResult: func(t *testing.T, result interface{}) {
				arr, ok := result.([]string)
				if !ok {
					t.Error("Expected []string result for multiple authors")
					return
				}
				if len(arr) != 2 {
					t.Errorf("Expected 2 authors, got %d", len(arr))
				}
			},
		},
		{
			name: "no valid authors, fallback to repo owner",
			setup: func() {
				viper.Reset()
				viper.Set("repos.blog.owner", "charlie")
				viper.Set("users.charlie.passwordhash", "hash")
			},
			post:    &BlogPost{Authors: []string{"unknown"}, Repo: "blog"},
			baseURL: "https://example.com",
			checkResult: func(t *testing.T, result interface{}) {
				str, ok := result.(string)
				if !ok {
					t.Error("Expected string result for fallback")
					return
				}
				if !strings.Contains(str, "@charlie") {
					t.Errorf("Expected fallback to charlie, got %q", str)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()
			result := buildPostAttribution(tt.post, tt.baseURL)
			tt.checkResult(t, result)
		})
	}
}

// TestBuildFollowersCC tests the CC field generation for followers
func TestBuildFollowersCC(t *testing.T) {
	tests := []struct {
		name          string
		setup         func()
		post          *BlogPost
		baseURL       string
		expectedCount int
		checkContains string
	}{
		{
			name: "single author",
			setup: func() {
				viper.Reset()
				viper.Set("users.alice.passwordhash", "hash")
			},
			post:          &BlogPost{Authors: []string{"alice"}, Repo: "blog"},
			baseURL:       "https://example.com",
			expectedCount: 1,
			checkContains: "@alice/followers",
		},
		{
			name: "multiple authors",
			setup: func() {
				viper.Reset()
				viper.Set("users.alice.passwordhash", "hash")
				viper.Set("users.bob.passwordhash", "hash")
			},
			post:          &BlogPost{Authors: []string{"alice", "bob"}, Repo: "blog"},
			baseURL:       "https://example.com",
			expectedCount: 2,
			checkContains: "/followers",
		},
		{
			name: "invalid authors, fallback to owner",
			setup: func() {
				viper.Reset()
				viper.Set("repos.blog.owner", "charlie")
				viper.Set("users.charlie.passwordhash", "hash")
			},
			post:          &BlogPost{Authors: []string{"unknown"}, Repo: "blog"},
			baseURL:       "https://example.com",
			expectedCount: 1,
			checkContains: "@charlie/followers",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()
			result := buildFollowersCC(tt.post, tt.baseURL)
			if len(result) != tt.expectedCount {
				t.Errorf("buildFollowersCC() returned %d items, want %d", len(result), tt.expectedCount)
			}
			found := false
			for _, cc := range result {
				if strings.Contains(cc, tt.checkContains) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Expected CC to contain %q, got %v", tt.checkContains, result)
			}
		})
	}
}

// TestNewOutboxService tests the OutboxService constructor
func TestNewOutboxService(t *testing.T) {
	storage := &Storage{}
	km := &KeyManager{}
	actor := &ActorService{}
	inbox := &InboxService{}

	service := NewOutboxService(storage, km, actor, inbox, nil)

	if service == nil {
		t.Fatal("NewOutboxService should not return nil")
	}
	if service.storage != storage {
		t.Error("storage not set correctly")
	}
	if service.keyManager != km {
		t.Error("keyManager not set correctly")
	}
	if service.actorService != actor {
		t.Error("actorService not set correctly")
	}
	if service.inboxService != inbox {
		t.Error("inboxService not set correctly")
	}
}

// TestHandleOutbox tests the Outbox endpoint
func TestHandleOutbox(t *testing.T) {
	tests := []struct {
		name           string
		setup          func()
		path           string
		query          string
		host           string
		expectedStatus int
	}{
		{
			name: "invalid path - missing @",
			setup: func() {
				viper.Reset()
			},
			path:           "/alice/outbox",
			host:           "example.com",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "user not found",
			setup: func() {
				viper.Reset()
				viper.Set("users.bob.passwordhash", "hash")
			},
			path:           "/@alice/outbox",
			host:           "example.com",
			expectedStatus: http.StatusNotFound,
		},
		{
			name: "activitypub disabled",
			setup: func() {
				viper.Reset()
				viper.Set("users.alice.passwordhash", "hash")
				viper.Set("activitypub.enabled", false)
			},
			path:           "/@alice/outbox",
			host:           "example.com",
			expectedStatus: http.StatusNotFound,
		},
		{
			name: "valid request - collection",
			setup: func() {
				viper.Reset()
				viper.Set("users.alice.passwordhash", "hash")
				viper.Set("activitypub.enabled", true)
			},
			path:           "/@alice/outbox",
			host:           "example.com",
			expectedStatus: http.StatusOK,
		},
		{
			name: "valid request - page",
			setup: func() {
				viper.Reset()
				viper.Set("users.alice.passwordhash", "hash")
				viper.Set("activitypub.enabled", true)
			},
			path:           "/@alice/outbox",
			query:          "page=1",
			host:           "example.com",
			expectedStatus: http.StatusOK,
		},
		{
			name: "invalid page parameter",
			setup: func() {
				viper.Reset()
				viper.Set("users.alice.passwordhash", "hash")
				viper.Set("activitypub.enabled", true)
			},
			path:           "/@alice/outbox",
			query:          "page=abc",
			host:           "example.com",
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()

			service := NewOutboxService(&Storage{}, &KeyManager{}, &ActorService{}, &InboxService{}, nil)

			url := tt.path
			if tt.query != "" {
				url += "?" + tt.query
			}
			req := httptest.NewRequest("GET", url, nil)
			req.Host = tt.host
			rr := httptest.NewRecorder()

			service.HandleOutbox(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("HandleOutbox() status = %d, want %d, body: %s", rr.Code, tt.expectedStatus, rr.Body.String())
			}
		})
	}
}

// TestHandleServerOutbox tests the server-wide Outbox endpoint
func TestHandleServerOutbox(t *testing.T) {
	tests := []struct {
		name           string
		setup          func()
		path           string
		query          string
		host           string
		expectedStatus int
	}{
		{
			name: "activitypub disabled",
			setup: func() {
				viper.Reset()
				viper.Set("activitypub.enabled", false)
			},
			path:           "/outbox",
			host:           "example.com",
			expectedStatus: http.StatusNotFound,
		},
		{
			name: "valid request - collection",
			setup: func() {
				viper.Reset()
				viper.Set("activitypub.enabled", true)
			},
			path:           "/outbox",
			host:           "example.com",
			expectedStatus: http.StatusOK,
		},
		{
			name: "valid request - page",
			setup: func() {
				viper.Reset()
				viper.Set("activitypub.enabled", true)
			},
			path:           "/outbox",
			query:          "page=1",
			host:           "example.com",
			expectedStatus: http.StatusOK,
		},
		{
			name: "invalid page parameter",
			setup: func() {
				viper.Reset()
				viper.Set("activitypub.enabled", true)
			},
			path:           "/outbox",
			query:          "page=abc",
			host:           "example.com",
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()

			service := NewOutboxService(&Storage{}, &KeyManager{}, &ActorService{}, &InboxService{}, nil)

			url := tt.path
			if tt.query != "" {
				url += "?" + tt.query
			}
			req := httptest.NewRequest("GET", url, nil)
			req.Host = tt.host
			rr := httptest.NewRecorder()

			service.HandleServerOutbox(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("HandleServerOutbox() status = %d, want %d, body: %s", rr.Code, tt.expectedStatus, rr.Body.String())
			}
		})
	}
}

