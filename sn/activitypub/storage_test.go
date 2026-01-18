package activitypub

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/spf13/afero"
)

// TestStorage_SaveAndLoadFollowers tests follower persistence
func TestStorage_SaveAndLoadFollowers(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	storage := &Storage{
		activityPubFs:  mockFs,
		commitInterval: 0, // Disable background commits
	}

	// Create necessary directories
	mockFs.MkdirAll(".activitypub/users/alice", 0755)

	// Create test followers
	followers := map[string]*Follower{
		"https://example.com/@bob": {
			ActorID:     "https://example.com/@bob",
			InboxURL:    "https://example.com/@bob/inbox",
			Username:    "bob",
			Domain:      "example.com",
			AcceptedAt:  time.Now(),
			SharedInbox: "https://example.com/inbox",
		},
		"https://other.com/@charlie": {
			ActorID:  "https://other.com/@charlie",
			InboxURL: "https://other.com/@charlie/inbox",
			Username: "charlie",
			Domain:   "other.com",
		},
	}

	// Save followers
	err := storage.SaveFollowers("alice", followers)
	if err != nil {
		t.Fatalf("SaveFollowers failed: %v", err)
	}

	// Load followers
	loaded, err := storage.LoadFollowers("alice")
	if err != nil {
		t.Fatalf("LoadFollowers failed: %v", err)
	}

	if len(loaded) != 2 {
		t.Errorf("Expected 2 followers, got %d", len(loaded))
	}

	if loaded["https://example.com/@bob"].Username != "bob" {
		t.Errorf("Expected bob, got %s", loaded["https://example.com/@bob"].Username)
	}

	if loaded["https://other.com/@charlie"].Domain != "other.com" {
		t.Errorf("Expected other.com, got %s", loaded["https://other.com/@charlie"].Domain)
	}
}

// TestStorage_LoadFollowers_Empty tests loading when no followers exist
func TestStorage_LoadFollowers_Empty(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	storage := &Storage{
		activityPubFs:  mockFs,
		commitInterval: 0,
	}

	// Don't create any files - just the directory
	mockFs.MkdirAll(".activitypub/users/alice", 0755)

	followers, err := storage.LoadFollowers("alice")
	if err != nil {
		t.Fatalf("LoadFollowers failed: %v", err)
	}

	if len(followers) != 0 {
		t.Errorf("Expected 0 followers, got %d", len(followers))
	}
}

// TestStorage_SaveAndLoadFollowing tests following persistence
func TestStorage_SaveAndLoadFollowing(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	storage := &Storage{
		activityPubFs:  mockFs,
		commitInterval: 0,
	}

	mockFs.MkdirAll(".activitypub/users/alice", 0755)

	// Create test following
	following := map[string]*Following{
		"https://example.com/@bob": {
			ActorID:    "https://example.com/@bob",
			FollowedAt: time.Now(),
		},
	}

	// Save following
	err := storage.SaveFollowing("alice", following)
	if err != nil {
		t.Fatalf("SaveFollowing failed: %v", err)
	}

	// Load following
	loaded, err := storage.LoadFollowing("alice")
	if err != nil {
		t.Fatalf("LoadFollowing failed: %v", err)
	}

	if len(loaded) != 1 {
		t.Errorf("Expected 1 following, got %d", len(loaded))
	}

	if _, exists := loaded["https://example.com/@bob"]; !exists {
		t.Error("Expected to find following entry")
	}
}

// TestStorage_LoadFollowing_Empty tests loading when no following exist
func TestStorage_LoadFollowing_Empty(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	storage := &Storage{
		activityPubFs:  mockFs,
		commitInterval: 0,
	}

	mockFs.MkdirAll(".activitypub/users/alice", 0755)

	following, err := storage.LoadFollowing("alice")
	if err != nil {
		t.Fatalf("LoadFollowing failed: %v", err)
	}

	if len(following) != 0 {
		t.Errorf("Expected 0 following, got %d", len(following))
	}
}

// TestStorage_SaveAndLoadComment tests comment persistence
func TestStorage_SaveAndLoadComment(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	storage := &Storage{
		activityPubFs:  mockFs,
		commitInterval: 0,
	}

	mockFs.MkdirAll(".activitypub/comments", 0755)

	// Create test comment
	comment := &Comment{
		ID:          "comment-123",
		ActivityID:  "https://example.com/activity/456",
		InReplyTo:   "https://mysite.com/blog/my-post",
		Author:      "https://example.com/@commenter",
		AuthorName:  "Commenter",
		AuthorURL:   "https://example.com/@commenter",
		Content:     "Great post!",
		ContentHTML: "<p>Great post!</p>",
		Published:   time.Now(),
		Verified:    true,
		Approved:    true,
		Hidden:      false,
		PostSlug:    "my-post",
		PostRepo:    "blog",
		Metadata:    map[string]string{"source": "activitypub"},
	}

	// Save comment
	err := storage.SaveComment(comment)
	if err != nil {
		t.Fatalf("SaveComment failed: %v", err)
	}

	// Load comments
	comments, err := storage.LoadComments("blog", "my-post")
	if err != nil {
		t.Fatalf("LoadComments failed: %v", err)
	}

	if len(comments) != 1 {
		t.Errorf("Expected 1 comment, got %d", len(comments))
	}

	if comments[0].ID != "comment-123" {
		t.Errorf("Expected comment ID comment-123, got %s", comments[0].ID)
	}

	if comments[0].AuthorName != "Commenter" {
		t.Errorf("Expected author Commenter, got %s", comments[0].AuthorName)
	}
}

// TestStorage_LoadComments_Empty tests loading when no comments exist
func TestStorage_LoadComments_Empty(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	storage := &Storage{
		activityPubFs:  mockFs,
		commitInterval: 0,
	}

	mockFs.MkdirAll(".activitypub/comments", 0755)

	comments, err := storage.LoadComments("blog", "nonexistent-post")
	if err != nil {
		t.Fatalf("LoadComments failed: %v", err)
	}

	if len(comments) != 0 {
		t.Errorf("Expected 0 comments, got %d", len(comments))
	}
}

// TestStorage_SaveAndLoadMetadata tests metadata persistence
func TestStorage_SaveAndLoadMetadata(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	storage := &Storage{
		activityPubFs:  mockFs,
		commitInterval: 0,
	}

	mockFs.MkdirAll(".activitypub", 0755)

	metadata := &FederationMetadata{
		InstanceName:        "My Blog",
		InstanceDescription: "A personal blog",
		AdminEmail:          "admin@mysite.com",
		Settings:            map[string]string{"key": "value"},
	}

	err := storage.SaveMetadata(metadata)
	if err != nil {
		t.Fatalf("SaveMetadata failed: %v", err)
	}

	loaded, err := storage.LoadMetadata()
	if err != nil {
		t.Fatalf("LoadMetadata failed: %v", err)
	}

	if loaded.InstanceName != "My Blog" {
		t.Errorf("Expected instance name 'My Blog', got %s", loaded.InstanceName)
	}

	if loaded.AdminEmail != "admin@mysite.com" {
		t.Errorf("Expected email 'admin@mysite.com', got %s", loaded.AdminEmail)
	}
}

// TestStorage_LoadMetadata_NotExists tests loading when metadata doesn't exist
func TestStorage_LoadMetadata_NotExists(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	storage := &Storage{
		activityPubFs:  mockFs,
		commitInterval: 0,
	}

	mockFs.MkdirAll(".activitypub", 0755)

	metadata, err := storage.LoadMetadata()
	if err != nil {
		t.Fatalf("LoadMetadata failed: %v", err)
	}

	if metadata != nil {
		t.Error("Expected nil metadata when file doesn't exist")
	}
}

// TestStorage_EnsureDirectories tests directory creation
func TestStorage_EnsureDirectories(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	storage := &Storage{
		activityPubFs:  mockFs,
		commitInterval: 0,
	}

	err := storage.ensureDirectories()
	if err != nil {
		t.Fatalf("ensureDirectories failed: %v", err)
	}

	// Check that directories were created
	dirs := []string{
		".activitypub",
		".activitypub/queue",
		".activitypub/comments",
		".activitypub/users",
	}

	for _, dir := range dirs {
		exists, err := afero.DirExists(mockFs, dir)
		if err != nil {
			t.Fatalf("Error checking directory %s: %v", dir, err)
		}
		if !exists {
			t.Errorf("Directory %s should exist", dir)
		}
	}
}

// TestStorage_MarkPendingChanges tests the pending changes flag
func TestStorage_MarkPendingChanges(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	storage := &Storage{
		activityPubFs:  mockFs,
		commitInterval: 0, // Immediate commits when 0
	}

	if storage.pendingChanges {
		t.Error("pendingChanges should be false initially")
	}

	storage.markPendingChanges()

	// With commitInterval=0, changes are committed immediately in a goroutine
	// The flag may be reset quickly, but the function should not error
}

// TestStorage_Encryption tests the encrypt/decrypt cycle
func TestStorage_Encryption(t *testing.T) {
	// Create a test master key (32 bytes after SHA-256)
	mockFs := afero.NewMemMapFs()
	storage := &Storage{
		activityPubFs:  mockFs,
		masterKey:      make([]byte, 32), // Use zeroed key for testing
		commitInterval: 0,
	}

	// Fill with test key
	for i := range storage.masterKey {
		storage.masterKey[i] = byte(i)
	}

	testData := []byte("This is a secret message that should be encrypted")

	// Encrypt
	encrypted, err := storage.encrypt(testData)
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}

	// Encrypted data should be different from original
	if string(encrypted) == string(testData) {
		t.Error("Encrypted data should not equal original")
	}

	// Decrypt
	decrypted, err := storage.decrypt(encrypted)
	if err != nil {
		t.Fatalf("decrypt failed: %v", err)
	}

	// Decrypted should match original
	if string(decrypted) != string(testData) {
		t.Errorf("Decrypted data doesn't match original. Got %q, want %q", string(decrypted), string(testData))
	}
}

// TestStorage_Decrypt_InvalidData tests decryption with invalid data
func TestStorage_Decrypt_InvalidData(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	storage := &Storage{
		activityPubFs:  mockFs,
		masterKey:      make([]byte, 32),
		commitInterval: 0,
	}

	// Try to decrypt non-base64 data
	_, err := storage.decrypt([]byte("not valid base64!!!"))
	if err == nil {
		t.Error("decrypt should fail with invalid base64")
	}

	// Try to decrypt empty data
	_, err = storage.decrypt([]byte(""))
	if err == nil {
		t.Error("decrypt should fail with empty data")
	}
}

// TestStorage_CommitChanges_NoRepo tests commit with no git repo
func TestStorage_CommitChanges_NoRepo(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	storage := &Storage{
		activityPubFs:     mockFs,
		activityPubRepo:   nil, // No git repo
		pendingChanges:    true,
		commitInterval:    0,
	}

	err := storage.commitChanges()
	if err != nil {
		t.Fatalf("commitChanges should not error when no repo: %v", err)
	}

	// pendingChanges should be reset
	if storage.pendingChanges {
		t.Error("pendingChanges should be false after commitChanges")
	}
}

// TestFollowerStruct_Storage tests Follower struct JSON serialization
func TestFollowerStruct_Storage(t *testing.T) {
	follower := &Follower{
		ActorID:     "https://example.com/@bob",
		InboxURL:    "https://example.com/@bob/inbox",
		SharedInbox: "https://example.com/inbox",
		AcceptedAt:  time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
		Domain:      "example.com",
		Username:    "bob",
	}

	// Test JSON marshaling
	data, err := json.Marshal(follower)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	// Unmarshal back
	var decoded Follower
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.ActorID != follower.ActorID {
		t.Errorf("ActorID mismatch: got %s, want %s", decoded.ActorID, follower.ActorID)
	}
}

// TestFollowingStruct_Storage tests Following struct JSON serialization
func TestFollowingStruct_Storage(t *testing.T) {
	following := &Following{
		ActorID:    "https://example.com/@bob",
		FollowedAt: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
	}

	data, err := json.Marshal(following)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded Following
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.ActorID != following.ActorID {
		t.Errorf("ActorID mismatch: got %s, want %s", decoded.ActorID, following.ActorID)
	}
}

// TestCommentStruct_Storage tests Comment struct JSON serialization
func TestCommentStruct_Storage(t *testing.T) {
	comment := &Comment{
		ID:          "comment-123",
		ActivityID:  "https://example.com/activity/456",
		InReplyTo:   "https://mysite.com/blog/my-post",
		Author:      "https://example.com/@bob",
		AuthorName:  "Bob",
		AuthorURL:   "https://example.com/@bob",
		Content:     "Test content",
		ContentHTML: "<p>Test content</p>",
		Published:   time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
		Verified:    true,
		Approved:    true,
		Hidden:      false,
		PostSlug:    "my-post",
		PostRepo:    "blog",
		Metadata:    map[string]string{"key": "value"},
	}

	data, err := json.Marshal(comment)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded Comment
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.ID != comment.ID {
		t.Errorf("ID mismatch: got %s, want %s", decoded.ID, comment.ID)
	}
	if decoded.AuthorName != comment.AuthorName {
		t.Errorf("AuthorName mismatch: got %s, want %s", decoded.AuthorName, comment.AuthorName)
	}
}

// TestFederationMetadataStruct_Storage tests FederationMetadata struct
func TestFederationMetadataStruct_Storage(t *testing.T) {
	metadata := &FederationMetadata{
		InstanceName:        "My Instance",
		InstanceDescription: "A test instance",
		AdminEmail:          "admin@example.com",
		Settings:            map[string]string{"key": "value"},
		UpdatedAt:           time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
	}

	data, err := json.Marshal(metadata)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded FederationMetadata
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.InstanceName != metadata.InstanceName {
		t.Errorf("InstanceName mismatch: got %s, want %s", decoded.InstanceName, metadata.InstanceName)
	}
}

// TestStorage_MultipleComments tests loading multiple comments
func TestStorage_MultipleComments(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	storage := &Storage{
		activityPubFs:  mockFs,
		commitInterval: 0,
	}

	mockFs.MkdirAll(".activitypub/comments", 0755)

	// Save multiple comments
	comments := []*Comment{
		{
			ID:          "comment-1",
			ActivityID:  "activity-1",
			Author:      "https://example.com/@alice",
			AuthorName:  "Alice",
			Content:     "First comment",
			PostSlug:    "test-post",
			PostRepo:    "blog",
			Published:   time.Now(),
			Metadata:    make(map[string]string),
		},
		{
			ID:          "comment-2",
			ActivityID:  "activity-2",
			Author:      "https://example.com/@bob",
			AuthorName:  "Bob",
			Content:     "Second comment",
			PostSlug:    "test-post",
			PostRepo:    "blog",
			Published:   time.Now(),
			Metadata:    make(map[string]string),
		},
	}

	for _, c := range comments {
		err := storage.SaveComment(c)
		if err != nil {
			t.Fatalf("SaveComment failed: %v", err)
		}
	}

	// Load all comments
	loaded, err := storage.LoadComments("blog", "test-post")
	if err != nil {
		t.Fatalf("LoadComments failed: %v", err)
	}

	if len(loaded) != 2 {
		t.Errorf("Expected 2 comments, got %d", len(loaded))
	}
}
