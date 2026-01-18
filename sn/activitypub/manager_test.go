package activitypub

import (
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/spf13/afero"
	"github.com/spf13/viper"
)

// TestManagerIsEnabled tests the IsEnabled method
func TestManagerIsEnabled(t *testing.T) {
	tests := []struct {
		name     string
		enabled  bool
		expected bool
	}{
		{"enabled", true, true},
		{"disabled", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Manager{enabled: tt.enabled}
			if m.IsEnabled() != tt.expected {
				t.Errorf("IsEnabled() = %v, want %v", m.IsEnabled(), tt.expected)
			}
		})
	}
}

// TestNewManager_Disabled tests manager creation when ActivityPub is disabled
func TestNewManager_Disabled(t *testing.T) {
	viper.Reset()
	viper.Set("activitypub.enabled", false)

	mockFs := afero.NewMemMapFs()
	manager, err := NewManager(mockFs, nil)

	if err != nil {
		t.Fatalf("NewManager should not error when disabled: %v", err)
	}

	if manager == nil {
		t.Fatal("NewManager should return a manager when disabled")
	}

	if manager.IsEnabled() {
		t.Error("Manager should not be enabled when ActivityPub is disabled")
	}
}

// TestNewManager_NoMasterKey tests manager creation without master key
func TestNewManager_NoMasterKey(t *testing.T) {
	viper.Reset()
	viper.Set("activitypub.enabled", true)
	// Note: no master_key set

	mockFs := afero.NewMemMapFs()
	_, err := NewManager(mockFs, nil)

	if err == nil {
		t.Error("NewManager should error when master_key is not set")
	}
}

// TestNewManager_NoUsers tests manager creation without configured users
func TestNewManager_NoUsers(t *testing.T) {
	viper.Reset()
	viper.Set("activitypub.enabled", true)
	viper.Set("activitypub.master_key", "test-key-for-encryption")
	// Note: no users configured

	mockFs := afero.NewMemMapFs()
	mockFs.MkdirAll(".activitypub", 0755)

	_, err := NewManager(mockFs, nil)

	if err == nil {
		t.Error("NewManager should error when no users are configured")
	}
}

// TestManagerRegisterRoutes_Disabled tests route registration when disabled
func TestManagerRegisterRoutes_Disabled(t *testing.T) {
	m := &Manager{enabled: false}
	router := mux.NewRouter()

	// Should not panic and should do nothing
	m.RegisterRoutes(router)

	// Verify no routes were registered
	req := httptest.NewRequest("GET", "/.well-known/webfinger", nil)
	match := &mux.RouteMatch{}
	if router.Match(req, match) {
		t.Error("No routes should be registered when disabled")
	}
}

// TestGetBaseURL_Manager tests the base URL helper
func TestGetBaseURL_Manager(t *testing.T) {
	viper.Reset()
	viper.Set("rooturl", "https://example.com/")

	result := getBaseURL()

	// Should not have trailing slash
	if result == "" {
		t.Error("getBaseURL should return a value")
	}
	if result[len(result)-1] == '/' {
		t.Errorf("getBaseURL should not end with slash, got %q", result)
	}
}

// TestGetBaseURL_Manager_ActivityPubOverride tests ActivityPub-specific URL override
func TestGetBaseURL_Manager_ActivityPubOverride(t *testing.T) {
	viper.Reset()
	viper.Set("rooturl", "https://example.com")
	viper.Set("activitypub.rooturl", "https://activitypub.example.com")

	result := getBaseURL()

	if result != "https://activitypub.example.com" {
		t.Errorf("getBaseURL should use activitypub.rooturl, got %q", result)
	}
}

// TestGetPrimaryUser tests the primary user lookup
func TestGetPrimaryUser(t *testing.T) {
	tests := []struct {
		name     string
		setup    func()
		expected string
	}{
		{
			name: "explicit primary_user",
			setup: func() {
				viper.Reset()
				viper.Set("activitypub.primary_user", "alice")
				viper.Set("users.alice.passwordhash", "hash")
			},
			expected: "alice",
		},
		{
			name: "fallback to first user",
			setup: func() {
				viper.Reset()
				viper.Set("users.bob.passwordhash", "hash")
			},
			expected: "bob",
		},
		{
			name: "no users",
			setup: func() {
				viper.Reset()
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()
			result := getPrimaryUser()
			if result != tt.expected {
				t.Errorf("getPrimaryUser() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// TestIsActivityPubEnabled_Manager tests the global enable check
func TestIsActivityPubEnabled_Manager(t *testing.T) {
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
			name: "not set defaults to false",
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

// TestUserExists_Manager tests the user existence check
func TestUserExists_Manager(t *testing.T) {
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

// TestSetSecurityHeaders_Manager tests that security headers are set correctly
func TestSetSecurityHeaders_Manager(t *testing.T) {
	w := httptest.NewRecorder()
	setSecurityHeaders(w)

	expectedHeaders := []string{
		"Strict-Transport-Security",
		"X-Frame-Options",
		"X-Content-Type-Options",
	}

	for _, header := range expectedHeaders {
		if w.Header().Get(header) == "" {
			t.Errorf("Security header %q not set", header)
		}
	}
}

// TestKeyManagerStruct tests KeyManager struct initialization
func TestKeyManagerStruct(t *testing.T) {
	storage := &Storage{}
	km := NewKeyManager(storage)

	if km == nil {
		t.Fatal("NewKeyManager should not return nil")
	}

	if km.storage != storage {
		t.Error("KeyManager storage not set correctly")
	}
}

// TestManagerPublishPost_Disabled tests that PublishPost does nothing when disabled
func TestManagerPublishPost_Disabled(t *testing.T) {
	m := &Manager{enabled: false}

	err := m.PublishPost(&BlogPost{Title: "Test"})

	if err != nil {
		t.Errorf("PublishPost should not error when disabled: %v", err)
	}
}

// TestManagerUpdatePost_Disabled tests that UpdatePost does nothing when disabled
func TestManagerUpdatePost_Disabled(t *testing.T) {
	m := &Manager{enabled: false}

	err := m.UpdatePost(&BlogPost{Title: "Test"})

	if err != nil {
		t.Errorf("UpdatePost should not error when disabled: %v", err)
	}
}

// TestManagerDeletePost_Disabled tests that DeletePost does nothing when disabled
func TestManagerDeletePost_Disabled(t *testing.T) {
	m := &Manager{enabled: false}

	err := m.DeletePost("https://example.com/post/1", "blog")

	if err != nil {
		t.Errorf("DeletePost should not error when disabled: %v", err)
	}
}
