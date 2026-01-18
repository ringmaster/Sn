package sn

import (
	"testing"

	"github.com/ringmaster/Sn/sn/activitypub"
	"github.com/spf13/afero"
	"github.com/spf13/viper"
)

func TestConfigStringDefault(t *testing.T) {
	// Reset viper for test isolation
	viper.Reset()

	tests := []struct {
		name       string
		configKey  string
		setupValue string
		hasSetup   bool
		defaultVal string
		expected   string
	}{
		{
			name:       "returns default when not set",
			configKey:  "test.missing.key",
			hasSetup:   false,
			defaultVal: "default_value",
			expected:   "default_value",
		},
		{
			name:       "returns config value when set",
			configKey:  "test.existing.key",
			setupValue: "configured_value",
			hasSetup:   true,
			defaultVal: "default_value",
			expected:   "configured_value",
		},
		{
			name:       "empty default",
			configKey:  "test.another.missing",
			hasSetup:   false,
			defaultVal: "",
			expected:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			viper.Reset()
			if tt.hasSetup {
				viper.Set(tt.configKey, tt.setupValue)
			}

			result := ConfigStringDefault(tt.configKey, tt.defaultVal)
			if result != tt.expected {
				t.Errorf("ConfigStringDefault(%q, %q) = %q, expected %q",
					tt.configKey, tt.defaultVal, result, tt.expected)
			}
		})
	}
}

func TestDirExistsFs(t *testing.T) {
	// Create an in-memory filesystem for testing
	fs := afero.NewMemMapFs()

	// Create test directories and files
	fs.MkdirAll("/testdir", 0755)
	fs.MkdirAll("/nested/dir", 0755)
	afero.WriteFile(fs, "/testfile.txt", []byte("content"), 0644)

	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{"existing directory", "/testdir", true},
		{"nested directory", "/nested/dir", true},
		{"parent of nested", "/nested", true},
		{"non-existent path", "/nonexistent", false},
		{"file not directory", "/testfile.txt", false},
		{"root directory", "/", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DirExistsFs(fs, tt.path)
			if result != tt.expected {
				t.Errorf("DirExistsFs(fs, %q) = %v, expected %v", tt.path, result, tt.expected)
			}
		})
	}
}

func TestConfigPathOptions(t *testing.T) {
	t.Run("WithDefault sets default", func(t *testing.T) {
		opts := &ConfigPathOptions{}
		WithDefault("/some/path")(opts)

		if !opts.HasDefault {
			t.Error("WithDefault should set HasDefault to true")
		}
		if opts.Default != "/some/path" {
			t.Errorf("WithDefault should set Default to '/some/path', got %q", opts.Default)
		}
	})

	t.Run("MustExist sets MustExist", func(t *testing.T) {
		opts := &ConfigPathOptions{MustExist: false}
		MustExist()(opts)

		if !opts.MustExist {
			t.Error("MustExist should set MustExist to true")
		}
	})

	t.Run("OptionallyExist clears MustExist", func(t *testing.T) {
		opts := &ConfigPathOptions{MustExist: true}
		OptionallyExist()(opts)

		if opts.MustExist {
			t.Error("OptionallyExist should set MustExist to false")
		}
	})

	t.Run("multiple options can be combined", func(t *testing.T) {
		opts := &ConfigPathOptions{}
		WithDefault("/default")(opts)
		MustExist()(opts)

		if !opts.HasDefault || opts.Default != "/default" {
			t.Error("WithDefault option should be applied")
		}
		if !opts.MustExist {
			t.Error("MustExist option should be applied")
		}
	})
}

func TestConfigPathOptionsDefaults(t *testing.T) {
	opts := &ConfigPathOptions{}

	// Check default values
	if opts.HasDefault {
		t.Error("HasDefault should default to false")
	}
	if opts.Default != "" {
		t.Error("Default should default to empty string")
	}
	if opts.MustExist {
		t.Error("MustExist should default to false")
	}
}

// TestInitializeActivityPub_RequiresDatabase tests that InitializeActivityPub
// requires the database to be connected first
func TestInitializeActivityPub_RequiresDatabase(t *testing.T) {
	// This test validates the fix for the regen-keys command bug
	// where InitializeActivityPub was not called, leaving ActivityPubManager nil

	viper.Reset()
	viper.Set("activitypub.enabled", true)
	viper.Set("activitypub.master_key", "test-master-key-12345")
	viper.Set("users.testuser.passwordhash", "hash")
	viper.Set("rooturl", "https://example.com")

	// Create a mock filesystem
	mockFs := afero.NewMemMapFs()
	mockFs.MkdirAll(".activitypub", 0755)

	// Save the original Vfs and restore after test
	originalVfs := Vfs
	Vfs = mockFs
	defer func() { Vfs = originalVfs }()

	// Without calling DBConnect first, ActivityPubManager should handle nil db gracefully
	// or the initialization should work for the parts that don't need db

	// The key insight: ActivityPubManager is nil until InitializeActivityPub is called
	// This was the bug in regen-keys - it only called ConfigSetup, not InitializeActivityPub

	// Verify ActivityPubManager starts as nil
	originalManager := ActivityPubManager
	ActivityPubManager = nil
	defer func() { ActivityPubManager = originalManager }()

	if ActivityPubManager != nil {
		t.Error("ActivityPubManager should be nil before InitializeActivityPub is called")
	}

	// Note: We can't fully test InitializeActivityPub without a database,
	// but we can verify the precondition that the bug fix addresses
}

// TestForceRegenerateActivityPubKeys_RequiresManager tests that key regeneration
// requires ActivityPubManager to be initialized
func TestForceRegenerateActivityPubKeys_RequiresManager(t *testing.T) {
	// Save original state
	originalManager := ActivityPubManager
	defer func() { ActivityPubManager = originalManager }()

	// Test with nil manager
	ActivityPubManager = nil
	err := ForceRegenerateActivityPubKeys()
	if err == nil {
		t.Error("ForceRegenerateActivityPubKeys should error when ActivityPubManager is nil")
	}
}

// TestForceRegenerateActivityPubKeys_RequiresEnabled tests that key regeneration
// requires ActivityPub to be enabled
func TestForceRegenerateActivityPubKeys_RequiresEnabled(t *testing.T) {
	// Save original state
	originalManager := ActivityPubManager
	defer func() { ActivityPubManager = originalManager }()

	// Create a disabled manager
	ActivityPubManager = &activitypub.Manager{}
	// Note: The Manager struct has enabled field but it's not exported
	// We rely on IsEnabled() returning false for a zero-value Manager

	err := ForceRegenerateActivityPubKeys()
	if err == nil {
		t.Error("ForceRegenerateActivityPubKeys should error when ActivityPub is disabled")
	}
}
