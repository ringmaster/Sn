package activitypub

import (
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

func TestParseSignatureHeader(t *testing.T) {
	tests := []struct {
		name     string
		header   string
		expected map[string]string
		wantErr  bool
	}{
		{
			name:   "valid signature header",
			header: `keyId="https://example.com/users/alice#main-key",algorithm="rsa-sha256",headers="(request-target) host date",signature="abc123"`,
			expected: map[string]string{
				"keyId":     "https://example.com/users/alice#main-key",
				"algorithm": "rsa-sha256",
				"headers":   "(request-target) host date",
				"signature": "abc123",
			},
			wantErr: false,
		},
		{
			name:   "with digest header",
			header: `keyId="https://example.com/key",algorithm="rsa-sha256",headers="(request-target) host date digest",signature="xyz789"`,
			expected: map[string]string{
				"keyId":     "https://example.com/key",
				"algorithm": "rsa-sha256",
				"headers":   "(request-target) host date digest",
				"signature": "xyz789",
			},
			wantErr: false,
		},
		{
			name:   "with spaces around equals",
			header: `keyId = "test",algorithm = "rsa-sha256",signature = "sig"`,
			expected: map[string]string{
				"keyId":     "test",
				"algorithm": "rsa-sha256",
				"signature": "sig",
			},
			wantErr: false,
		},
		{
			name:     "empty header",
			header:   "",
			expected: map[string]string{},
			wantErr:  false,
		},
		{
			name:    "invalid format - no equals",
			header:  `keyId"test"`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseSignatureHeader(tt.header)

			if tt.wantErr {
				if err == nil {
					t.Error("parseSignatureHeader should have returned an error")
				}
				return
			}

			if err != nil {
				t.Errorf("parseSignatureHeader returned unexpected error: %v", err)
				return
			}

			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("parseSignatureHeader() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestContainsString(t *testing.T) {
	tests := []struct {
		name     string
		slice    []string
		str      string
		expected bool
	}{
		{"empty slice", []string{}, "test", false},
		{"found at start", []string{"test", "other"}, "test", true},
		{"found at end", []string{"other", "test"}, "test", true},
		{"found in middle", []string{"a", "test", "b"}, "test", true},
		{"not found", []string{"a", "b", "c"}, "test", false},
		{"case sensitive", []string{"TEST"}, "test", false},
		{"single item found", []string{"test"}, "test", true},
		{"single item not found", []string{"other"}, "test", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := containsString(tt.slice, tt.str)
			if result != tt.expected {
				t.Errorf("containsString(%v, %q) = %v, expected %v", tt.slice, tt.str, result, tt.expected)
			}
		})
	}
}

func TestGenerateActivityID(t *testing.T) {
	baseURL := "https://example.com"
	username := "alice"

	id1 := GenerateActivityID(baseURL, username)

	// Should contain baseURL and username
	if !strings.Contains(id1, baseURL) {
		t.Errorf("GenerateActivityID should contain baseURL, got %s", id1)
	}

	if !strings.Contains(id1, "@"+username) {
		t.Errorf("GenerateActivityID should contain @username, got %s", id1)
	}

	if !strings.Contains(id1, "/activities/") {
		t.Errorf("GenerateActivityID should contain /activities/, got %s", id1)
	}

	// Should have a timestamp component (numeric suffix)
	parts := strings.Split(id1, "/")
	lastPart := parts[len(parts)-1]
	if len(lastPart) == 0 {
		t.Error("GenerateActivityID should have a timestamp suffix")
	}
}

func TestGenerateObjectID(t *testing.T) {
	baseURL := "https://example.com"
	username := "bob"

	id1 := GenerateObjectID(baseURL, username)

	// Should contain baseURL and username
	if !strings.Contains(id1, baseURL) {
		t.Errorf("GenerateObjectID should contain baseURL, got %s", id1)
	}

	if !strings.Contains(id1, "@"+username) {
		t.Errorf("GenerateObjectID should contain @username, got %s", id1)
	}

	if !strings.Contains(id1, "/objects/") {
		t.Errorf("GenerateObjectID should contain /objects/, got %s", id1)
	}

	// Should have a timestamp component (numeric suffix)
	parts := strings.Split(id1, "/")
	lastPart := parts[len(parts)-1]
	if len(lastPart) == 0 {
		t.Error("GenerateObjectID should have a timestamp suffix")
	}
}

func TestNormalizeHeaders(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "empty",
			input:    []string{},
			expected: []string{},
		},
		{
			name:     "already lowercase",
			input:    []string{"host", "date"},
			expected: []string{"date", "host"},
		},
		{
			name:     "uppercase",
			input:    []string{"HOST", "DATE"},
			expected: []string{"date", "host"},
		},
		{
			name:     "mixed case",
			input:    []string{"Host", "Date", "Digest"},
			expected: []string{"date", "digest", "host"},
		},
		{
			name:     "with whitespace",
			input:    []string{" host ", " date "},
			expected: []string{"date", "host"},
		},
		{
			name:     "special header",
			input:    []string{"(request-target)", "Host"},
			expected: []string{"(request-target)", "host"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeHeaders(tt.input)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("NormalizeHeaders(%v) = %v, expected %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestKeyManagerGetPublicKeyPEM(t *testing.T) {
	km := &KeyManager{}

	// Without key pair, should return empty
	if result := km.GetPublicKeyPEM(); result != "" {
		t.Errorf("GetPublicKeyPEM with nil keyPair should return empty, got %q", result)
	}

	// With key pair
	km.keyPair = &KeyPair{
		PublicKeyPem: "-----BEGIN PUBLIC KEY-----\ntest\n-----END PUBLIC KEY-----",
	}

	if result := km.GetPublicKeyPEM(); result != km.keyPair.PublicKeyPem {
		t.Errorf("GetPublicKeyPEM should return public key, got %q", result)
	}
}

func TestKeyManagerGetKeyID(t *testing.T) {
	km := &KeyManager{}

	// Without key pair, should return empty
	if result := km.GetKeyID(); result != "" {
		t.Errorf("GetKeyID with nil keyPair should return empty, got %q", result)
	}

	// With key pair
	km.keyPair = &KeyPair{
		KeyID: "https://example.com/users/alice#main-key",
	}

	if result := km.GetKeyID(); result != km.keyPair.KeyID {
		t.Errorf("GetKeyID should return key ID, got %q", result)
	}
}

func TestBuildSignatureString(t *testing.T) {
	km := &KeyManager{}

	// Create a test request
	reqURL, _ := url.Parse("https://example.com/inbox")
	req := &http.Request{
		Method: "POST",
		URL:    reqURL,
		Host:   "example.com",
		Header: http.Header{
			"Date":   []string{"Mon, 01 Jan 2024 00:00:00 GMT"},
			"Digest": []string{"SHA-256=abc123"},
		},
	}
	req.Header.Set("Host", "example.com")

	tests := []struct {
		name     string
		headers  []string
		contains []string
		wantErr  bool
	}{
		{
			name:     "request-target only",
			headers:  []string{"(request-target)"},
			contains: []string{"(request-target): post /inbox"},
			wantErr:  false,
		},
		{
			name:     "host header",
			headers:  []string{"host"},
			contains: []string{"host: example.com"},
			wantErr:  false,
		},
		{
			name:     "date header",
			headers:  []string{"date"},
			contains: []string{"date: Mon, 01 Jan 2024 00:00:00 GMT"},
			wantErr:  false,
		},
		{
			name:     "multiple headers",
			headers:  []string{"(request-target)", "host", "date"},
			contains: []string{"(request-target):", "host:", "date:"},
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := km.buildSignatureString(req, tt.headers)

			if tt.wantErr {
				if err == nil {
					t.Error("buildSignatureString should have returned an error")
				}
				return
			}

			if err != nil {
				t.Errorf("buildSignatureString returned unexpected error: %v", err)
				return
			}

			for _, expected := range tt.contains {
				if !strings.Contains(result, expected) {
					t.Errorf("buildSignatureString should contain %q, got %q", expected, result)
				}
			}
		})
	}
}

func TestNewKeyManager(t *testing.T) {
	storage := &Storage{}
	km := NewKeyManager(storage)

	if km == nil {
		t.Fatal("NewKeyManager should not return nil")
	}

	if km.storage != storage {
		t.Error("NewKeyManager should set storage")
	}

	if km.keyPair != nil {
		t.Error("NewKeyManager should have nil keyPair initially")
	}
}
