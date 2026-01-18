package activitypub

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/spf13/viper"
)

// TestExtractUsernameFromPath tests the username extraction from URL paths
func TestExtractUsernameFromPath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		suffix   string
		expected string
	}{
		{
			name:     "valid inbox path",
			path:     "/@alice/inbox",
			suffix:   "inbox",
			expected: "alice",
		},
		{
			name:     "valid outbox path",
			path:     "/@bob/outbox",
			suffix:   "outbox",
			expected: "bob",
		},
		{
			name:     "valid followers path",
			path:     "/@charlie/followers",
			suffix:   "followers",
			expected: "charlie",
		},
		{
			name:     "with leading slash",
			path:     "/@dave/inbox",
			suffix:   "inbox",
			expected: "dave",
		},
		{
			name:     "wrong suffix",
			path:     "/@alice/inbox",
			suffix:   "outbox",
			expected: "",
		},
		{
			name:     "missing @ prefix",
			path:     "/alice/inbox",
			suffix:   "inbox",
			expected: "",
		},
		{
			name:     "empty path",
			path:     "",
			suffix:   "inbox",
			expected: "",
		},
		{
			name:     "just slash",
			path:     "/",
			suffix:   "inbox",
			expected: "",
		},
		{
			name:     "only username no suffix",
			path:     "/@alice",
			suffix:   "inbox",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractUsernameFromPath(tt.path, tt.suffix)
			if result != tt.expected {
				t.Errorf("extractUsernameFromPath(%q, %q) = %q, want %q", tt.path, tt.suffix, result, tt.expected)
			}
		})
	}
}

// TestUserExists tests user existence check
func TestUserExists(t *testing.T) {
	tests := []struct {
		name     string
		setup    func()
		username string
		expected bool
	}{
		{
			name: "user exists",
			setup: func() {
				viper.Reset()
				viper.Set("users.alice.passwordhash", "hash")
			},
			username: "alice",
			expected: true,
		},
		{
			name: "user does not exist",
			setup: func() {
				viper.Reset()
				viper.Set("users.alice.passwordhash", "hash")
			},
			username: "bob",
			expected: false,
		},
		{
			name: "no users configured",
			setup: func() {
				viper.Reset()
			},
			username: "anyone",
			expected: false,
		},
		{
			name: "multiple users, one matches",
			setup: func() {
				viper.Reset()
				viper.Set("users.alice.passwordhash", "hash")
				viper.Set("users.bob.passwordhash", "hash")
				viper.Set("users.charlie.passwordhash", "hash")
			},
			username: "bob",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()
			result := userExists(tt.username)
			if result != tt.expected {
				t.Errorf("userExists(%q) = %v, want %v", tt.username, result, tt.expected)
			}
		})
	}
}

// TestIsActivityPubEnabled tests the global ActivityPub toggle
func TestIsActivityPubEnabled(t *testing.T) {
	tests := []struct {
		name     string
		setup    func()
		expected bool
	}{
		{
			name: "enabled",
			setup: func() {
				viper.Reset()
				viper.Set("activitypub.enabled", true)
			},
			expected: true,
		},
		{
			name: "disabled",
			setup: func() {
				viper.Reset()
				viper.Set("activitypub.enabled", false)
			},
			expected: false,
		},
		{
			name: "not set (defaults to false)",
			setup: func() {
				viper.Reset()
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()
			result := isActivityPubEnabled()
			if result != tt.expected {
				t.Errorf("isActivityPubEnabled() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestGetScheme tests HTTP vs HTTPS detection
func TestGetScheme(t *testing.T) {
	tests := []struct {
		name     string
		request  *http.Request
		expected string
	}{
		{
			name:     "HTTP request",
			request:  httptest.NewRequest("GET", "http://example.com/", nil),
			expected: "http",
		},
		{
			name: "HTTPS request (TLS present)",
			request: func() *http.Request {
				req := httptest.NewRequest("GET", "https://example.com/", nil)
				req.TLS = &tls.ConnectionState{}
				return req
			}(),
			expected: "https",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getScheme(tt.request)
			if result != tt.expected {
				t.Errorf("getScheme() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// TestSetSecurityHeaders tests that all security headers are set
func TestSetSecurityHeaders(t *testing.T) {
	recorder := httptest.NewRecorder()
	setSecurityHeaders(recorder)

	expectedHeaders := map[string]string{
		"Strict-Transport-Security": "max-age=31536000; includeSubDomains",
		"X-Frame-Options":           "SAMEORIGIN",
		"X-Content-Type-Options":    "nosniff",
		"Referrer-Policy":           "strict-origin-when-cross-origin",
	}

	for header, expected := range expectedHeaders {
		got := recorder.Header().Get(header)
		if got != expected {
			t.Errorf("Header %q = %q, want %q", header, got, expected)
		}
	}
}

// TestGetActorURL tests actor URL construction
func TestGetActorURL(t *testing.T) {
	tests := []struct {
		name     string
		request  *http.Request
		username string
		expected string
	}{
		{
			name: "HTTP request",
			request: func() *http.Request {
				req := httptest.NewRequest("GET", "http://example.com/", nil)
				req.Host = "example.com"
				return req
			}(),
			username: "alice",
			expected: "http://example.com/@alice",
		},
		{
			name: "HTTPS request",
			request: func() *http.Request {
				req := httptest.NewRequest("GET", "https://example.com/", nil)
				req.Host = "example.com"
				req.TLS = &tls.ConnectionState{}
				return req
			}(),
			username: "bob",
			expected: "https://example.com/@bob",
		},
		{
			name: "with custom port",
			request: func() *http.Request {
				req := httptest.NewRequest("GET", "http://localhost:8080/", nil)
				req.Host = "localhost:8080"
				return req
			}(),
			username: "admin",
			expected: "http://localhost:8080/@admin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetActorURL(tt.request, tt.username)
			if result != tt.expected {
				t.Errorf("GetActorURL() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// TestGetBaseURL tests base URL construction
func TestGetBaseURL(t *testing.T) {
	tests := []struct {
		name     string
		request  *http.Request
		expected string
	}{
		{
			name: "HTTP request",
			request: func() *http.Request {
				req := httptest.NewRequest("GET", "http://example.com/", nil)
				req.Host = "example.com"
				return req
			}(),
			expected: "http://example.com",
		},
		{
			name: "HTTPS request",
			request: func() *http.Request {
				req := httptest.NewRequest("GET", "https://example.com/", nil)
				req.Host = "example.com"
				req.TLS = &tls.ConnectionState{}
				return req
			}(),
			expected: "https://example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetBaseURL(tt.request)
			if result != tt.expected {
				t.Errorf("GetBaseURL() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// TestNewActorService tests ActorService constructor
func TestNewActorService(t *testing.T) {
	storage := &Storage{}
	km := &KeyManager{}

	service := NewActorService(storage, km)

	if service == nil {
		t.Fatal("NewActorService should not return nil")
	}
	if service.storage != storage {
		t.Error("storage not set correctly")
	}
	if service.keyManager != km {
		t.Error("keyManager not set correctly")
	}
}

// TestHandleWebfinger tests the WebFinger endpoint
func TestHandleWebfinger(t *testing.T) {
	tests := []struct {
		name           string
		setup          func()
		resource       string
		host           string
		expectedStatus int
	}{
		{
			name: "valid request",
			setup: func() {
				viper.Reset()
				viper.Set("users.alice.passwordhash", "hash")
				viper.Set("rooturl", "https://example.com")
			},
			resource:       "acct:alice@example.com",
			host:           "example.com",
			expectedStatus: http.StatusOK,
		},
		{
			name: "missing resource parameter",
			setup: func() {
				viper.Reset()
			},
			resource:       "",
			host:           "example.com",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "invalid resource format - no colon",
			setup: func() {
				viper.Reset()
			},
			resource:       "alice@example.com",
			host:           "example.com",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "invalid resource format - wrong prefix",
			setup: func() {
				viper.Reset()
			},
			resource:       "user:alice@example.com",
			host:           "example.com",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "invalid account format - no @",
			setup: func() {
				viper.Reset()
			},
			resource:       "acct:aliceexample.com",
			host:           "example.com",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "user not found",
			setup: func() {
				viper.Reset()
				viper.Set("users.bob.passwordhash", "hash")
			},
			resource:       "acct:alice@example.com",
			host:           "example.com",
			expectedStatus: http.StatusNotFound,
		},
		{
			name: "wrong domain",
			setup: func() {
				viper.Reset()
				viper.Set("users.alice.passwordhash", "hash")
				viper.Set("rooturl", "https://example.com")
			},
			resource:       "acct:alice@other.com",
			host:           "example.com",
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()

			service := NewActorService(&Storage{}, &KeyManager{})

			url := "/.well-known/webfinger"
			if tt.resource != "" {
				url += "?resource=" + tt.resource
			}
			req := httptest.NewRequest("GET", url, nil)
			req.Host = tt.host
			rr := httptest.NewRecorder()

			service.HandleWebfinger(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("HandleWebfinger() status = %d, want %d, body: %s", rr.Code, tt.expectedStatus, rr.Body.String())
			}
		})
	}
}

// TestHandleActor tests the Actor endpoint
func TestHandleActor(t *testing.T) {
	tests := []struct {
		name           string
		setup          func()
		path           string
		host           string
		expectedStatus int
	}{
		{
			name: "valid actor request",
			setup: func() {
				viper.Reset()
				viper.Set("activitypub.enabled", true)
				viper.Set("users.alice.passwordhash", "hash")
				viper.Set("rooturl", "https://example.com")
			},
			path:           "/@alice",
			host:           "example.com",
			expectedStatus: http.StatusOK,
		},
		{
			name: "activitypub disabled",
			setup: func() {
				viper.Reset()
				viper.Set("activitypub.enabled", false)
			},
			path:           "/@alice",
			host:           "example.com",
			expectedStatus: http.StatusNotFound,
		},
		{
			name: "invalid path format",
			setup: func() {
				viper.Reset()
				viper.Set("activitypub.enabled", true)
			},
			path:           "/alice", // missing @
			host:           "example.com",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "user not found",
			setup: func() {
				viper.Reset()
				viper.Set("activitypub.enabled", true)
				viper.Set("users.bob.passwordhash", "hash")
			},
			path:           "/@alice",
			host:           "example.com",
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()

			km := &KeyManager{
				keyPair: &KeyPair{
					PublicKeyPem: "-----BEGIN PUBLIC KEY-----\ntest\n-----END PUBLIC KEY-----",
					KeyID:        "https://example.com/@alice#main-key",
				},
			}
			service := NewActorService(&Storage{}, km)

			req := httptest.NewRequest("GET", tt.path, nil)
			req.Host = tt.host
			rr := httptest.NewRecorder()

			service.HandleActor(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("HandleActor() status = %d, want %d, body: %s", rr.Code, tt.expectedStatus, rr.Body.String())
			}
		})
	}
}

// TestHandleFollowers tests the Followers endpoint
func TestHandleFollowers(t *testing.T) {
	tests := []struct {
		name           string
		setup          func()
		path           string
		host           string
		expectedStatus int
	}{
		{
			name: "activitypub disabled",
			setup: func() {
				viper.Reset()
				viper.Set("activitypub.enabled", false)
			},
			path:           "/@alice/followers",
			host:           "example.com",
			expectedStatus: http.StatusNotFound,
		},
		{
			name: "invalid path",
			setup: func() {
				viper.Reset()
				viper.Set("activitypub.enabled", true)
			},
			path:           "/alice/followers", // missing @
			host:           "example.com",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "user not found",
			setup: func() {
				viper.Reset()
				viper.Set("activitypub.enabled", true)
				viper.Set("users.bob.passwordhash", "hash")
			},
			path:           "/@alice/followers",
			host:           "example.com",
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()

			service := NewActorService(&Storage{}, &KeyManager{})

			req := httptest.NewRequest("GET", tt.path, nil)
			req.Host = tt.host
			rr := httptest.NewRecorder()

			service.HandleFollowers(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("HandleFollowers() status = %d, want %d", rr.Code, tt.expectedStatus)
			}
		})
	}
}

// TestHandleFollowing tests the Following endpoint
func TestHandleFollowing(t *testing.T) {
	tests := []struct {
		name           string
		setup          func()
		path           string
		host           string
		expectedStatus int
	}{
		{
			name: "activitypub disabled",
			setup: func() {
				viper.Reset()
				viper.Set("activitypub.enabled", false)
			},
			path:           "/@alice/following",
			host:           "example.com",
			expectedStatus: http.StatusNotFound,
		},
		{
			name: "invalid path",
			setup: func() {
				viper.Reset()
				viper.Set("activitypub.enabled", true)
			},
			path:           "/alice/following",
			host:           "example.com",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "user not found",
			setup: func() {
				viper.Reset()
				viper.Set("activitypub.enabled", true)
				viper.Set("users.bob.passwordhash", "hash")
			},
			path:           "/@alice/following",
			host:           "example.com",
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()

			service := NewActorService(&Storage{}, &KeyManager{})

			req := httptest.NewRequest("GET", tt.path, nil)
			req.Host = tt.host
			rr := httptest.NewRecorder()

			service.HandleFollowing(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("HandleFollowing() status = %d, want %d", rr.Code, tt.expectedStatus)
			}
		})
	}
}
