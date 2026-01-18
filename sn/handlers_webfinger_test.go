package sn

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/spf13/viper"
)

func TestFingerHandler(t *testing.T) {
	// Setup viper with test users
	viper.Reset()
	viper.Set("users.alice.passwordhash", "test")
	viper.Set("users.bob.passwordhash", "test")

	tests := []struct {
		name           string
		resource       string
		host           string
		expectedStatus int
		checkJSON      bool
	}{
		{
			name:           "valid request",
			resource:       "acct:alice@example.com",
			host:           "example.com",
			expectedStatus: http.StatusOK,
			checkJSON:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/.well-known/webfinger", nil)
			if tt.resource != "" {
				q := req.URL.Query()
				q.Add("resource", tt.resource)
				req.URL.RawQuery = q.Encode()
			}
			req.Host = tt.host

			rr := httptest.NewRecorder()

			// Create router and register handler
			r := mux.NewRouter()
			r.HandleFunc("/.well-known/webfinger", fingerHandler).Name("webfinger")

			r.ServeHTTP(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("fingerHandler returned wrong status code: got %v want %v", rr.Code, tt.expectedStatus)
			}

			if tt.checkJSON {
				// Check content type header for valid requests
				contentType := rr.Header().Get("Content-Type")
				if contentType != "application/activity+json" {
					t.Errorf("fingerHandler wrong content type: got %v want application/activity+json", contentType)
				}

				// Check body contains expected data
				body := rr.Body.String()
				if !contains(body, "alice") {
					t.Errorf("fingerHandler response should contain 'alice', got %s", body)
				}
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr, 0))
}

func containsAt(s, substr string, start int) bool {
	for i := start; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestFingerHandlerSecurityHeaders(t *testing.T) {
	viper.Reset()
	viper.Set("users.alice.passwordhash", "test")

	req := httptest.NewRequest("GET", "/.well-known/webfinger?resource=acct:alice@example.com", nil)
	req.Host = "example.com"

	rr := httptest.NewRecorder()

	r := mux.NewRouter()
	r.HandleFunc("/.well-known/webfinger", fingerHandler).Name("webfinger")
	r.ServeHTTP(rr, req)

	// Check security headers
	securityHeaders := map[string]string{
		"Strict-Transport-Security": "max-age=31536000; includeSubDomains",
		"X-Frame-Options":           "SAMEORIGIN",
		"X-Content-Type-Options":    "nosniff",
	}

	for header, expected := range securityHeaders {
		if got := rr.Header().Get(header); got != expected {
			t.Errorf("Missing or wrong %s header: got %q want %q", header, got, expected)
		}
	}
}
