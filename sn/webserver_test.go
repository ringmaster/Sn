package sn

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
)

// TestSpacesConfigStruct verifies SpacesConfig struct fields
func TestSpacesConfigStruct(t *testing.T) {
	config := SpacesConfig{
		SpaceName:   "mybucket",
		Endpoint:    "https://nyc3.digitaloceanspaces.com",
		AccessKeyID: "access123",
		SecretKey:   "secret456",
		Region:      "nyc3",
	}

	if config.SpaceName != "mybucket" {
		t.Errorf("SpaceName = %q, want mybucket", config.SpaceName)
	}
	if config.Endpoint != "https://nyc3.digitaloceanspaces.com" {
		t.Errorf("Endpoint = %q, want https://nyc3.digitaloceanspaces.com", config.Endpoint)
	}
	if config.AccessKeyID != "access123" {
		t.Errorf("AccessKeyID = %q, want access123", config.AccessKeyID)
	}
	if config.SecretKey != "secret456" {
		t.Errorf("SecretKey = %q, want secret456", config.SecretKey)
	}
	if config.Region != "nyc3" {
		t.Errorf("Region = %q, want nyc3", config.Region)
	}
}

// TestRouteStringValue tests parameter substitution in route strings
func TestRouteStringValue(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		template  string
		routeVars map[string]string
		query     string
		expected  string
	}{
		{
			name:      "simple route var",
			path:      "/post/my-post",
			template:  "post-{slug}",
			routeVars: map[string]string{"slug": "my-post"},
			query:     "",
			expected:  "post-my-post",
		},
		{
			name:      "query parameter with route vars",
			path:      "/search",
			template:  "search-{params.q}",
			routeVars: map[string]string{"_placeholder": ""}, // Need at least empty map for mux
			query:     "q=hello",
			expected:  "search-hello",
		},
		{
			name:      "no substitution needed",
			path:      "/about",
			template:  "about-page",
			routeVars: map[string]string{"_placeholder": ""},
			query:     "",
			expected:  "about-page",
		},
		{
			name:      "multiple vars",
			path:      "/blog/2024/post",
			template:  "{repo}-{year}-{slug}",
			routeVars: map[string]string{"repo": "blog", "year": "2024", "slug": "post"},
			query:     "",
			expected:  "blog-2024-post",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a request with the specified path and query
			url := "http://example.com" + tt.path
			if tt.query != "" {
				url += "?" + tt.query
			}
			req := httptest.NewRequest("GET", url, nil)

			// Always set mux vars (mux.Vars requires this to not return nil)
			req = mux.SetURLVars(req, tt.routeVars)

			result := routeStringValue(req, tt.template)
			if result != tt.expected {
				t.Errorf("routeStringValue() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// TestCtxKeyStruct tests the context key type
func TestCtxKeyStruct(t *testing.T) {
	// Verify ctxKey can be used as a context key
	key := ctxKey{}

	// ctxKey should be comparable (usable as map key)
	keys := make(map[ctxKey]string)
	keys[key] = "value"

	if keys[key] != "value" {
		t.Error("ctxKey should be usable as context key")
	}
}

// TestSlugPatternMatchesSubdirectories verifies that route patterns
// can match slugs containing slashes (subdirectory posts)
func TestSlugPatternMatchesSubdirectories(t *testing.T) {
	tests := []struct {
		name       string
		pattern    string
		path       string
		shouldMatch bool
		expectedSlug string
	}{
		{
			name:        "simple slug matches",
			pattern:     "/posts/{slug:.+}",
			path:        "/posts/my-post",
			shouldMatch: true,
			expectedSlug: "my-post",
		},
		{
			name:        "subdirectory slug matches",
			pattern:     "/posts/{slug:.+}",
			path:        "/posts/tests/path-test",
			shouldMatch: true,
			expectedSlug: "tests/path-test",
		},
		{
			name:        "deep subdirectory slug matches",
			pattern:     "/posts/{slug:.+}",
			path:        "/posts/2024/01/15/my-post",
			shouldMatch: true,
			expectedSlug: "2024/01/15/my-post",
		},
		{
			name:        "restrictive pattern rejects subdirectory",
			pattern:     "/posts/{slug:[^/]+}",
			path:        "/posts/tests/path-test",
			shouldMatch: false,
			expectedSlug: "",
		},
		{
			name:        "restrictive pattern accepts simple slug",
			pattern:     "/posts/{slug:[^/]+}",
			path:        "/posts/simple-post",
			shouldMatch: true,
			expectedSlug: "simple-post",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := mux.NewRouter()
			r.HandleFunc(tt.pattern, func(w http.ResponseWriter, r *http.Request) {
				vars := mux.Vars(r)
				w.Write([]byte(vars["slug"]))
			})

			req := httptest.NewRequest("GET", tt.path, nil)
			rr := httptest.NewRecorder()

			r.ServeHTTP(rr, req)

			if tt.shouldMatch {
				if rr.Code != http.StatusOK {
					t.Errorf("Expected route to match, got status %d", rr.Code)
				}
				if rr.Body.String() != tt.expectedSlug {
					t.Errorf("Expected slug %q, got %q", tt.expectedSlug, rr.Body.String())
				}
			} else {
				if rr.Code == http.StatusOK {
					t.Errorf("Expected route to NOT match, but it did with slug %q", rr.Body.String())
				}
			}
		})
	}
}
