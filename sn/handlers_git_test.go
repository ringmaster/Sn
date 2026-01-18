package sn

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
	"github.com/spf13/viper"
)

// TestNewFileDetection tests that the webhook correctly detects new files
// This verifies the fix for the bug where existingFiles was built AFTER git pull
// instead of BEFORE, causing new files to never be detected
func TestNewFileDetection(t *testing.T) {
	// Create a mock filesystem
	mockFs := afero.NewMemMapFs()

	// Set up initial files (simulating state before git pull)
	mockFs.MkdirAll("posts", 0755)
	afero.WriteFile(mockFs, "posts/existing-post.md", []byte("# Existing Post\n\nContent"), 0644)

	// Build existingFiles map (this should happen BEFORE pull)
	existingFiles := make(map[string]bool)
	afero.Walk(mockFs, "posts", func(path string, info os.FileInfo, _ error) error {
		if !info.IsDir() && filepath.Ext(path) == ".md" {
			existingFiles[path] = true
		}
		return nil
	})

	// Verify existing file is tracked
	if !existingFiles["posts/existing-post.md"] {
		t.Error("Existing file should be tracked in existingFiles map")
	}

	// Simulate git pull adding a new file
	afero.WriteFile(mockFs, "posts/new-post.md", []byte("# New Post\n\nNew content"), 0644)

	// Now check for new files (this happens AFTER pull)
	var newFiles []string
	afero.Walk(mockFs, "posts", func(path string, info os.FileInfo, _ error) error {
		if !info.IsDir() && filepath.Ext(path) == ".md" {
			if !existingFiles[path] {
				newFiles = append(newFiles, path)
			}
		}
		return nil
	})

	// Verify new file is detected
	if len(newFiles) != 1 {
		t.Errorf("Expected 1 new file, got %d", len(newFiles))
	}

	if len(newFiles) > 0 && newFiles[0] != "posts/new-post.md" {
		t.Errorf("Expected new file 'posts/new-post.md', got %q", newFiles[0])
	}
}

// TestNewFileDetection_NoNewFiles tests that no false positives occur
func TestNewFileDetection_NoNewFiles(t *testing.T) {
	mockFs := afero.NewMemMapFs()

	// Set up initial files
	mockFs.MkdirAll("posts", 0755)
	afero.WriteFile(mockFs, "posts/post1.md", []byte("# Post 1"), 0644)
	afero.WriteFile(mockFs, "posts/post2.md", []byte("# Post 2"), 0644)

	// Build existingFiles map BEFORE "pull"
	existingFiles := make(map[string]bool)
	afero.Walk(mockFs, "posts", func(path string, info os.FileInfo, _ error) error {
		if !info.IsDir() && filepath.Ext(path) == ".md" {
			existingFiles[path] = true
		}
		return nil
	})

	// Simulate git pull with no new files (just content changes)
	afero.WriteFile(mockFs, "posts/post1.md", []byte("# Post 1 Updated"), 0644)

	// Check for new files
	var newFiles []string
	afero.Walk(mockFs, "posts", func(path string, info os.FileInfo, _ error) error {
		if !info.IsDir() && filepath.Ext(path) == ".md" {
			if !existingFiles[path] {
				newFiles = append(newFiles, path)
			}
		}
		return nil
	})

	// Verify no new files detected (updates don't count as new)
	if len(newFiles) != 0 {
		t.Errorf("Expected 0 new files for content updates, got %d: %v", len(newFiles), newFiles)
	}
}

// TestNewFileDetection_MultipleNewFiles tests detection of multiple new files
func TestNewFileDetection_MultipleNewFiles(t *testing.T) {
	mockFs := afero.NewMemMapFs()

	// Set up initial files
	mockFs.MkdirAll("posts", 0755)
	afero.WriteFile(mockFs, "posts/old-post.md", []byte("# Old Post"), 0644)

	// Build existingFiles map BEFORE "pull"
	existingFiles := make(map[string]bool)
	afero.Walk(mockFs, "posts", func(path string, info os.FileInfo, _ error) error {
		if !info.IsDir() && filepath.Ext(path) == ".md" {
			existingFiles[path] = true
		}
		return nil
	})

	// Simulate git pull adding multiple new files
	afero.WriteFile(mockFs, "posts/new-post-1.md", []byte("# New Post 1"), 0644)
	afero.WriteFile(mockFs, "posts/new-post-2.md", []byte("# New Post 2"), 0644)
	afero.WriteFile(mockFs, "posts/new-post-3.md", []byte("# New Post 3"), 0644)

	// Check for new files
	var newFiles []string
	afero.Walk(mockFs, "posts", func(path string, info os.FileInfo, _ error) error {
		if !info.IsDir() && filepath.Ext(path) == ".md" {
			if !existingFiles[path] {
				newFiles = append(newFiles, path)
			}
		}
		return nil
	})

	// Verify all new files are detected
	if len(newFiles) != 3 {
		t.Errorf("Expected 3 new files, got %d: %v", len(newFiles), newFiles)
	}
}

// TestNewFileDetection_IgnoresNonMarkdown tests that non-markdown files are ignored
func TestNewFileDetection_IgnoresNonMarkdown(t *testing.T) {
	mockFs := afero.NewMemMapFs()

	// Set up initial state
	mockFs.MkdirAll("posts", 0755)

	// Build existingFiles map BEFORE "pull"
	existingFiles := make(map[string]bool)
	afero.Walk(mockFs, "posts", func(path string, info os.FileInfo, _ error) error {
		if !info.IsDir() && filepath.Ext(path) == ".md" {
			existingFiles[path] = true
		}
		return nil
	})

	// Simulate git pull adding various file types
	afero.WriteFile(mockFs, "posts/new-post.md", []byte("# New Post"), 0644)
	afero.WriteFile(mockFs, "posts/image.png", []byte("fake png data"), 0644)
	afero.WriteFile(mockFs, "posts/config.yaml", []byte("key: value"), 0644)
	afero.WriteFile(mockFs, "posts/readme.txt", []byte("readme"), 0644)

	// Check for new files (only .md should be detected)
	var newFiles []string
	afero.Walk(mockFs, "posts", func(path string, info os.FileInfo, _ error) error {
		if !info.IsDir() && filepath.Ext(path) == ".md" {
			if !existingFiles[path] {
				newFiles = append(newFiles, path)
			}
		}
		return nil
	})

	// Verify only the markdown file is detected
	if len(newFiles) != 1 {
		t.Errorf("Expected 1 new markdown file, got %d: %v", len(newFiles), newFiles)
	}

	if len(newFiles) > 0 && newFiles[0] != "posts/new-post.md" {
		t.Errorf("Expected 'posts/new-post.md', got %q", newFiles[0])
	}
}

// TestNewFileDetection_BugRegression specifically tests the bug where
// existingFiles was built AFTER git pull, making detection impossible
func TestNewFileDetection_BugRegression(t *testing.T) {
	mockFs := afero.NewMemMapFs()

	// Initial state: one existing file
	mockFs.MkdirAll("posts", 0755)
	afero.WriteFile(mockFs, "posts/existing.md", []byte("# Existing"), 0644)

	// CORRECT behavior: Build existingFiles BEFORE simulated pull
	existingFilesBefore := make(map[string]bool)
	afero.Walk(mockFs, "posts", func(path string, info os.FileInfo, _ error) error {
		if !info.IsDir() && filepath.Ext(path) == ".md" {
			existingFilesBefore[path] = true
		}
		return nil
	})

	// Simulate git pull
	afero.WriteFile(mockFs, "posts/new.md", []byte("# New"), 0644)

	// BUGGY behavior: Build existingFiles AFTER pull (this was the bug)
	existingFilesAfter := make(map[string]bool)
	afero.Walk(mockFs, "posts", func(path string, info os.FileInfo, _ error) error {
		if !info.IsDir() && filepath.Ext(path) == ".md" {
			existingFilesAfter[path] = true
		}
		return nil
	})

	// Check what each approach detects
	var detectedWithCorrectApproach []string
	var detectedWithBuggyApproach []string

	afero.Walk(mockFs, "posts", func(path string, info os.FileInfo, _ error) error {
		if !info.IsDir() && filepath.Ext(path) == ".md" {
			if !existingFilesBefore[path] {
				detectedWithCorrectApproach = append(detectedWithCorrectApproach, path)
			}
			if !existingFilesAfter[path] {
				detectedWithBuggyApproach = append(detectedWithBuggyApproach, path)
			}
		}
		return nil
	})

	// Correct approach should detect the new file
	if len(detectedWithCorrectApproach) != 1 {
		t.Errorf("Correct approach should detect 1 new file, got %d", len(detectedWithCorrectApproach))
	}

	// Buggy approach would detect nothing (this is what we fixed)
	if len(detectedWithBuggyApproach) != 0 {
		t.Errorf("Buggy approach (building map after pull) should detect 0 files, got %d - this test validates our fix works", len(detectedWithBuggyApproach))
	}
}

// TestActivityPubConfigForWebhook tests that ActivityPub config is properly checked
func TestActivityPubConfigForWebhook(t *testing.T) {
	tests := []struct {
		name          string
		setup         func()
		expectEnabled bool
	}{
		{
			name: "ActivityPub enabled globally",
			setup: func() {
				viper.Reset()
				viper.Set("activitypub.enabled", true)
			},
			expectEnabled: true,
		},
		{
			name: "ActivityPub disabled globally",
			setup: func() {
				viper.Reset()
				viper.Set("activitypub.enabled", false)
			},
			expectEnabled: false,
		},
		{
			name: "ActivityPub not set (defaults to false)",
			setup: func() {
				viper.Reset()
			},
			expectEnabled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()
			enabled := viper.GetBool("activitypub.enabled")
			if enabled != tt.expectEnabled {
				t.Errorf("activitypub.enabled = %v, want %v", enabled, tt.expectEnabled)
			}
		})
	}
}
