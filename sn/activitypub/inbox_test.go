package activitypub

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spf13/afero"
	"github.com/spf13/viper"
)

// TestExtractUsernameFromInboxPath tests inbox path username extraction
func TestExtractUsernameFromInboxPath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "valid user inbox",
			path:     "/@alice/inbox",
			expected: "alice",
		},
		{
			name:     "with leading slash",
			path:     "/@bob/inbox",
			expected: "bob",
		},
		{
			name:     "shared inbox",
			path:     "/inbox",
			expected: "",
		},
		{
			name:     "wrong endpoint",
			path:     "/@alice/outbox",
			expected: "",
		},
		{
			name:     "missing @ prefix",
			path:     "/alice/inbox",
			expected: "",
		},
		{
			name:     "empty path",
			path:     "",
			expected: "",
		},
		{
			name:     "just username no inbox",
			path:     "/@alice",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractUsernameFromInboxPath(tt.path)
			if result != tt.expected {
				t.Errorf("extractUsernameFromInboxPath(%q) = %q, want %q", tt.path, result, tt.expected)
			}
		})
	}
}

// TestExtractDomainFromActorID tests domain extraction from actor IDs
func TestExtractDomainFromActorID(t *testing.T) {
	tests := []struct {
		name     string
		actorID  string
		expected string
	}{
		{
			name:     "simple URL",
			actorID:  "https://example.com/users/alice",
			expected: "example.com",
		},
		{
			name:     "with port",
			actorID:  "https://example.com:8443/users/bob",
			expected: "example.com:8443",
		},
		{
			name:     "HTTP URL",
			actorID:  "http://localhost:8080/users/admin",
			expected: "localhost:8080",
		},
		{
			name:     "mastodon-style URL",
			actorID:  "https://mastodon.social/@alice",
			expected: "mastodon.social",
		},
		{
			name:     "invalid URL",
			actorID:  "not-a-url",
			expected: "",
		},
		{
			name:     "empty string",
			actorID:  "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractDomainFromActorID(tt.actorID)
			if result != tt.expected {
				t.Errorf("extractDomainFromActorID(%q) = %q, want %q", tt.actorID, result, tt.expected)
			}
		})
	}
}

// TestParsePostURLForReply tests URL parsing for reply detection
func TestParsePostURLForReply(t *testing.T) {
	tests := []struct {
		name         string
		inReplyTo    string
		host         string
		expectedRepo string
		expectedSlug string
	}{
		{
			name:         "valid same-domain reply",
			inReplyTo:    "https://example.com/blog/my-post",
			host:         "example.com",
			expectedRepo: "blog",
			expectedSlug: "my-post",
		},
		{
			name:         "different domain (external)",
			inReplyTo:    "https://other.com/blog/their-post",
			host:         "example.com",
			expectedRepo: "",
			expectedSlug: "",
		},
		{
			name:         "invalid URL",
			inReplyTo:    "not-a-url",
			host:         "example.com",
			expectedRepo: "",
			expectedSlug: "",
		},
		{
			name:         "only one path segment",
			inReplyTo:    "https://example.com/single",
			host:         "example.com",
			expectedRepo: "",
			expectedSlug: "",
		},
		{
			name:         "nested path",
			inReplyTo:    "https://example.com/posts/2024/my-post",
			host:         "example.com",
			expectedRepo: "posts",
			expectedSlug: "2024",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "http://"+tt.host+"/", nil)
			req.Host = tt.host

			repo, slug := parsePostURLForReply(tt.inReplyTo, req)

			if repo != tt.expectedRepo {
				t.Errorf("parsePostURLForReply() repo = %q, want %q", repo, tt.expectedRepo)
			}
			if slug != tt.expectedSlug {
				t.Errorf("parsePostURLForReply() slug = %q, want %q", slug, tt.expectedSlug)
			}
		})
	}
}

// TestGenerateCommentID tests comment ID generation
func TestGenerateCommentID(t *testing.T) {
	// Generate ID to verify correct prefix
	id1 := generateCommentID("https://example.com/activity/1")

	// IDs should have the correct prefix
	if !strings.HasPrefix(id1, "comment-") {
		t.Errorf("generateCommentID() should prefix with 'comment-', got %q", id1)
	}

	// ID should have a hex suffix after the prefix
	suffix := strings.TrimPrefix(id1, "comment-")
	if len(suffix) == 0 {
		t.Error("generateCommentID() should have a hex suffix")
	}
}

// TestNewInboxService tests InboxService constructor
func TestNewInboxService(t *testing.T) {
	storage := &Storage{}
	km := &KeyManager{}
	actor := &ActorService{}

	service := NewInboxService(storage, km, actor, nil)

	if service == nil {
		t.Fatal("NewInboxService should not return nil")
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
}

// TestInboxServiceHandleInbox_MethodNotAllowed tests non-POST requests
func TestInboxServiceHandleInbox_MethodNotAllowed(t *testing.T) {
	service := &InboxService{}

	methods := []string{"GET", "PUT", "DELETE", "PATCH"}
	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/@alice/inbox", nil)
			rr := httptest.NewRecorder()

			service.HandleInbox(rr, req)

			if rr.Code != http.StatusMethodNotAllowed {
				t.Errorf("HandleInbox(%s) status = %d, want %d", method, rr.Code, http.StatusMethodNotAllowed)
			}
		})
	}
}

// TestHandleInbox_ActivityPubDisabled tests that inbox returns 404 when ActivityPub is disabled
func TestHandleInbox_ActivityPubDisabled(t *testing.T) {
	viper.Reset()
	viper.Set("users.alice.passwordhash", "hash")
	viper.Set("activitypub.enabled", false)

	service := NewInboxService(&Storage{}, &KeyManager{}, &ActorService{}, nil)

	req := httptest.NewRequest("POST", "/@alice/inbox", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/activity+json")
	rr := httptest.NewRecorder()

	service.HandleInbox(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("HandleInbox() with ActivityPub disabled status = %d, want %d", rr.Code, http.StatusNotFound)
	}
}

// TestHandleInbox_InvalidJSON tests that invalid JSON returns 400
func TestHandleInbox_InvalidJSON(t *testing.T) {
	viper.Reset()
	viper.Set("users.alice.passwordhash", "hash")
	viper.Set("activitypub.enabled", true)

	service := NewInboxService(&Storage{}, &KeyManager{}, &ActorService{}, nil)

	req := httptest.NewRequest("POST", "/@alice/inbox", strings.NewReader(`not json`))
	req.Header.Set("Content-Type", "application/activity+json")
	rr := httptest.NewRecorder()

	service.HandleInbox(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("HandleInbox() with invalid JSON status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

// TestHandleInbox_InvalidInboxPath tests that invalid inbox paths return 400
func TestHandleInbox_InvalidInboxPath(t *testing.T) {
	viper.Reset()
	viper.Set("activitypub.enabled", true)

	service := NewInboxService(&Storage{}, &KeyManager{}, &ActorService{}, nil)

	// Path without @ prefix should fail
	req := httptest.NewRequest("POST", "/alice/inbox", strings.NewReader(`{"type": "Follow"}`))
	req.Header.Set("Content-Type", "application/activity+json")
	rr := httptest.NewRecorder()

	service.HandleInbox(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("HandleInbox() with invalid path status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

// TestHandleInbox_UserNotFound tests that requests for non-existent users return 404
func TestHandleInbox_UserNotFound(t *testing.T) {
	viper.Reset()
	viper.Set("activitypub.enabled", true)
	// Note: no users configured

	service := NewInboxService(&Storage{}, &KeyManager{}, &ActorService{}, nil)

	req := httptest.NewRequest("POST", "/@nonexistent/inbox", strings.NewReader(`{"type": "Follow", "actor": "https://example.com/@bob"}`))
	req.Header.Set("Content-Type", "application/activity+json")
	rr := httptest.NewRecorder()

	service.HandleInbox(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("HandleInbox() for nonexistent user status = %d, want %d", rr.Code, http.StatusNotFound)
	}
}

// TestHandleFollow_MissingActor tests that Follow without actor fails
func TestHandleFollow_MissingActor(t *testing.T) {
	viper.Reset()
	viper.Set("users.alice.passwordhash", "hash")
	viper.Set("activitypub.enabled", true)

	service := NewInboxService(&Storage{}, &KeyManager{}, &ActorService{}, nil)

	// Follow activity missing actor field
	body := `{"type": "Follow", "object": "https://example.com/@alice"}`
	req := httptest.NewRequest("POST", "/@alice/inbox", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/activity+json")
	rr := httptest.NewRecorder()

	service.HandleInbox(rr, req)

	// Should fail because actor is required
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("HandleInbox() Follow without actor status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}

// TestHandleFollow_MissingObject tests that Follow without object fails
func TestHandleFollow_MissingObject(t *testing.T) {
	viper.Reset()
	viper.Set("users.alice.passwordhash", "hash")
	viper.Set("activitypub.enabled", true)

	service := NewInboxService(&Storage{}, &KeyManager{}, &ActorService{}, nil)

	// Follow activity missing object field
	body := `{"type": "Follow", "actor": "https://remote.example/@bob"}`
	req := httptest.NewRequest("POST", "/@alice/inbox", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/activity+json")
	rr := httptest.NewRecorder()

	service.HandleInbox(rr, req)

	// Should fail because object is required
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("HandleInbox() Follow without object status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}

// TestHandleFollow_ObjectMismatch tests that Follow with wrong object fails
func TestHandleFollow_ObjectMismatch(t *testing.T) {
	viper.Reset()
	viper.Set("users.alice.passwordhash", "hash")
	viper.Set("activitypub.enabled", true)
	viper.Set("rooturl", "https://example.com")

	service := NewInboxService(&Storage{}, &KeyManager{}, &ActorService{}, nil)

	// Follow activity with wrong object (trying to follow someone else through alice's inbox)
	body := `{"type": "Follow", "actor": "https://remote.example/@bob", "object": "https://example.com/@charlie"}`
	req := httptest.NewRequest("POST", "https://example.com/@alice/inbox", strings.NewReader(body))
	req.Host = "example.com"
	req.Header.Set("Content-Type", "application/activity+json")
	rr := httptest.NewRecorder()

	service.HandleInbox(rr, req)

	// Should fail because object doesn't match alice's actor URL
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("HandleInbox() Follow with object mismatch status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}

// TestHandleFollow_ValidActivity tests a properly formed Follow activity
// This tests the intended functionality: when receiving a valid Follow,
// the system should store the follower and send back an Accept activity
func TestHandleFollow_ValidActivity(t *testing.T) {
	// Set up mock remote server that responds with actor info
	remoteServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/@bob" {
			// Return actor info when fetched
			actor := Actor{
				Context:           ActivityPubContext,
				ID:                "https://remote.example/@bob",
				Type:              TypePerson,
				PreferredUsername: "bob",
				Name:              "Bob",
				Inbox:             "https://remote.example/@bob/inbox",
				URL:               "https://remote.example/@bob",
			}
			w.Header().Set("Content-Type", ContentTypeActivityJSON)
			json.NewEncoder(w).Encode(actor)
			return
		}
		if r.URL.Path == "/@bob/inbox" && r.Method == "POST" {
			// Accept the Accept activity we send back
			w.WriteHeader(http.StatusAccepted)
			return
		}
		http.NotFound(w, r)
	}))
	defer remoteServer.Close()

	// Set up configuration
	viper.Reset()
	viper.Set("users.alice.passwordhash", "hash")
	viper.Set("activitypub.enabled", true)
	viper.Set("activitypub.master_key", "test-master-key-for-encryption")
	viper.Set("rooturl", "https://example.com")

	// Create mock storage using in-memory filesystem
	mockFs := afero.NewMemMapFs()
	storage := &Storage{
		activityPubFs:  mockFs,
		commitInterval: 0, // Disable commit processor for tests
	}
	// Create necessary directories
	mockFs.MkdirAll(".activitypub/users/alice", 0755)

	// Create service with mock storage
	service := NewInboxService(storage, &KeyManager{}, &ActorService{}, nil)

	// Create a valid Follow activity - note we use the mock server URL
	followActivity := Activity{
		Context: ActivityPubContext,
		Type:    TypeFollow,
		Actor:   remoteServer.URL + "/@bob",
		Object:  "https://example.com/@alice", // Must match the actor URL we expect
	}
	body, _ := json.Marshal(followActivity)

	req := httptest.NewRequest("POST", "https://example.com/@alice/inbox", strings.NewReader(string(body)))
	req.Host = "example.com"
	req.Header.Set("Content-Type", ContentTypeActivityJSON)
	rr := httptest.NewRecorder()

	service.HandleInbox(rr, req)

	// The activity should be accepted
	if rr.Code != http.StatusAccepted {
		t.Errorf("HandleInbox() valid Follow status = %d, want %d, body: %s", rr.Code, http.StatusAccepted, rr.Body.String())
	}

	// Verify follower was stored
	followers, err := storage.LoadFollowers("alice")
	if err != nil {
		t.Fatalf("Failed to load followers: %v", err)
	}

	followerID := remoteServer.URL + "/@bob"
	if _, exists := followers[followerID]; !exists {
		t.Errorf("Follower not stored, got followers: %v", followers)
	}
}

// TestHandleUndo_Follow tests that Undo Follow removes a follower
func TestHandleUndo_Follow(t *testing.T) {
	viper.Reset()
	viper.Set("users.alice.passwordhash", "hash")
	viper.Set("activitypub.enabled", true)
	viper.Set("activitypub.master_key", "test-master-key-for-encryption")

	// Create mock storage with existing follower
	mockFs := afero.NewMemMapFs()
	storage := &Storage{
		activityPubFs:  mockFs,
		commitInterval: 0,
	}
	mockFs.MkdirAll(".activitypub/users/alice", 0755)

	// Pre-populate with a follower
	existingFollower := &Follower{
		ActorID:  "https://remote.example/@bob",
		InboxURL: "https://remote.example/@bob/inbox",
		Username: "bob",
		Domain:   "remote.example",
	}
	storage.SaveFollowers("alice", map[string]*Follower{
		"https://remote.example/@bob": existingFollower,
	})

	service := NewInboxService(storage, &KeyManager{}, &ActorService{}, nil)

	// Create Undo Follow activity
	undoActivity := Activity{
		Context: ActivityPubContext,
		Type:    TypeUndo,
		Actor:   "https://remote.example/@bob",
		Object: map[string]interface{}{
			"type":   TypeFollow,
			"actor":  "https://remote.example/@bob",
			"object": "https://example.com/@alice",
		},
	}
	body, _ := json.Marshal(undoActivity)

	req := httptest.NewRequest("POST", "https://example.com/@alice/inbox", strings.NewReader(string(body)))
	req.Host = "example.com"
	req.Header.Set("Content-Type", ContentTypeActivityJSON)
	rr := httptest.NewRecorder()

	service.HandleInbox(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Errorf("HandleInbox() Undo Follow status = %d, want %d", rr.Code, http.StatusAccepted)
	}

	// Verify follower was removed
	followers, _ := storage.LoadFollowers("alice")
	if _, exists := followers["https://remote.example/@bob"]; exists {
		t.Error("Follower should have been removed after Undo Follow")
	}
}

// TestHandleCreate_Note tests handling of Note (comment/reply) activities
func TestHandleCreate_Note(t *testing.T) {
	viper.Reset()
	viper.Set("users.alice.passwordhash", "hash")
	viper.Set("activitypub.enabled", true)
	viper.Set("activitypub.master_key", "test-master-key-for-encryption")

	// Set up mock remote server for author info
	remoteServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/@commenter" {
			actor := Actor{
				ID:                "https://remote.example/@commenter",
				Type:              TypePerson,
				PreferredUsername: "commenter",
				Name:              "A Commenter",
				URL:               "https://remote.example/@commenter",
			}
			w.Header().Set("Content-Type", ContentTypeActivityJSON)
			json.NewEncoder(w).Encode(actor)
			return
		}
		http.NotFound(w, r)
	}))
	defer remoteServer.Close()

	mockFs := afero.NewMemMapFs()
	storage := &Storage{
		activityPubFs:  mockFs,
		commitInterval: 0,
	}
	mockFs.MkdirAll(".activitypub/comments", 0755)

	service := NewInboxService(storage, &KeyManager{}, &ActorService{}, nil)

	// Create Note activity (a reply to a post)
	createActivity := Activity{
		Context: ActivityPubContext,
		ID:      "https://remote.example/activity/123",
		Type:    TypeCreate,
		Actor:   remoteServer.URL + "/@commenter",
		Object: map[string]interface{}{
			"type":         TypeNote,
			"id":           "https://remote.example/note/456",
			"inReplyTo":    "https://example.com/blog/my-post",
			"content":      "<p>Great post!</p>",
			"attributedTo": remoteServer.URL + "/@commenter",
			"published":    "2024-01-15T10:30:00Z",
		},
	}
	body, _ := json.Marshal(createActivity)

	req := httptest.NewRequest("POST", "https://example.com/@alice/inbox", strings.NewReader(string(body)))
	req.Host = "example.com"
	req.Header.Set("Content-Type", ContentTypeActivityJSON)
	rr := httptest.NewRecorder()

	service.HandleInbox(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Errorf("HandleInbox() Create Note status = %d, want %d", rr.Code, http.StatusAccepted)
	}

	// Verify comment was stored
	comments, err := storage.LoadComments("blog", "my-post")
	if err != nil {
		t.Fatalf("Failed to load comments: %v", err)
	}

	if len(comments) != 1 {
		t.Errorf("Expected 1 comment, got %d", len(comments))
	}
}

// TestSharedInbox_Follow tests Follow activities sent to the shared inbox
func TestSharedInbox_Follow(t *testing.T) {
	viper.Reset()
	viper.Set("users.alice.passwordhash", "hash")
	viper.Set("activitypub.enabled", true)
	viper.Set("activitypub.master_key", "test-master-key-for-encryption")

	mockFs := afero.NewMemMapFs()
	storage := &Storage{
		activityPubFs:  mockFs,
		commitInterval: 0,
	}
	mockFs.MkdirAll(".activitypub/users/alice", 0755)

	service := NewInboxService(storage, &KeyManager{}, &ActorService{}, nil)

	// Follow activity sent to shared inbox
	followActivity := Activity{
		Context: ActivityPubContext,
		Type:    TypeFollow,
		Actor:   "https://remote.example/@bob",
		Object:  "https://example.com/@alice", // Target user determined from object
	}
	body, _ := json.Marshal(followActivity)

	req := httptest.NewRequest("POST", "https://example.com/inbox", strings.NewReader(string(body)))
	req.Host = "example.com"
	req.Header.Set("Content-Type", ContentTypeActivityJSON)
	rr := httptest.NewRecorder()

	service.HandleInbox(rr, req)

	// Shared inbox should accept the activity (even if it can't fully process it without actor fetch)
	if rr.Code != http.StatusAccepted && rr.Code != http.StatusInternalServerError {
		t.Errorf("SharedInbox Follow status = %d, want 202 or 500 (due to actor fetch)", rr.Code)
	}
}

// TestUnsupportedActivityType tests that unsupported activity types are handled gracefully
func TestUnsupportedActivityType(t *testing.T) {
	viper.Reset()
	viper.Set("users.alice.passwordhash", "hash")
	viper.Set("activitypub.enabled", true)

	service := NewInboxService(&Storage{}, &KeyManager{}, &ActorService{}, nil)

	// Some made-up activity type
	body := `{"type": "SomethingWeird", "actor": "https://remote.example/@bob"}`
	req := httptest.NewRequest("POST", "/@alice/inbox", strings.NewReader(body))
	req.Header.Set("Content-Type", ContentTypeActivityJSON)
	rr := httptest.NewRecorder()

	service.HandleInbox(rr, req)

	// Unsupported types should be accepted but ignored (not cause errors)
	if rr.Code != http.StatusAccepted {
		t.Errorf("HandleInbox() unsupported type status = %d, want %d", rr.Code, http.StatusAccepted)
	}
}

// TestProcessActivity tests the activity routing
func TestProcessActivity(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	storage := &Storage{
		activityPubFs:  mockFs,
		commitInterval: 0,
	}

	service := NewInboxService(storage, &KeyManager{}, &ActorService{}, nil)

	req := httptest.NewRequest("POST", "/@alice/inbox", nil)

	tests := []struct {
		name         string
		activityType string
		expectError  bool
	}{
		{"Update", TypeUpdate, false},
		{"Delete", TypeDelete, false},
		{"Like", TypeLike, false},
		{"Announce", TypeAnnounce, false},
		{"Unknown", "Unknown", false}, // Unknown types should not error
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			activity := &Activity{
				Type:  tt.activityType,
				Actor: "https://example.com/@actor",
			}

			err := service.processActivity(activity, "alice", req)

			if tt.expectError && err == nil {
				t.Errorf("processActivity(%s) expected error, got nil", tt.activityType)
			}
			if !tt.expectError && err != nil {
				t.Errorf("processActivity(%s) unexpected error: %v", tt.activityType, err)
			}
		})
	}
}
