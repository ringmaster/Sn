package activitypub

import (
	"testing"
	"time"
)

func TestActivityPubConstants(t *testing.T) {
	// Test type constants
	tests := []struct {
		name     string
		constant string
		expected string
	}{
		{"TypePerson", TypePerson, "Person"},
		{"TypeService", TypeService, "Service"},
		{"TypeApplication", TypeApplication, "Application"},
		{"TypeNote", TypeNote, "Note"},
		{"TypeArticle", TypeArticle, "Article"},
		{"TypeCreate", TypeCreate, "Create"},
		{"TypeUpdate", TypeUpdate, "Update"},
		{"TypeDelete", TypeDelete, "Delete"},
		{"TypeFollow", TypeFollow, "Follow"},
		{"TypeAccept", TypeAccept, "Accept"},
		{"TypeReject", TypeReject, "Reject"},
		{"TypeUndo", TypeUndo, "Undo"},
		{"TypeLike", TypeLike, "Like"},
		{"TypeAnnounce", TypeAnnounce, "Announce"},
		{"TypeCollection", TypeCollection, "Collection"},
		{"TypeOrderedCollection", TypeOrderedCollection, "OrderedCollection"},
		{"TypeCollectionPage", TypeCollectionPage, "CollectionPage"},
		{"TypeOrderedCollectionPage", TypeOrderedCollectionPage, "OrderedCollectionPage"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.constant != tt.expected {
				t.Errorf("%s = %q, expected %q", tt.name, tt.constant, tt.expected)
			}
		})
	}
}

func TestContentTypeConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant string
		expected string
	}{
		{"ContentTypeActivityJSON", ContentTypeActivityJSON, "application/activity+json"},
		{"ContentTypeLDJSON", ContentTypeLDJSON, "application/ld+json"},
		{"ContentTypeJSON", ContentTypeJSON, "application/json"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.constant != tt.expected {
				t.Errorf("%s = %q, expected %q", tt.name, tt.constant, tt.expected)
			}
		})
	}
}

func TestActivityPubContext(t *testing.T) {
	if len(ActivityPubContext) != 2 {
		t.Errorf("ActivityPubContext should have 2 URLs, got %d", len(ActivityPubContext))
	}

	expectedContexts := []string{
		"https://www.w3.org/ns/activitystreams",
		"https://w3id.org/security/v1",
	}

	for i, expected := range expectedContexts {
		if ActivityPubContext[i] != expected {
			t.Errorf("ActivityPubContext[%d] = %q, expected %q", i, ActivityPubContext[i], expected)
		}
	}
}

func TestActorStruct(t *testing.T) {
	actor := Actor{
		ID:                "https://example.com/@alice",
		Type:              TypePerson,
		PreferredUsername: "alice",
		Name:              "Alice",
		Summary:           "Hello, I'm Alice",
		Inbox:             "https://example.com/@alice/inbox",
		Outbox:            "https://example.com/@alice/outbox",
	}

	if actor.ID != "https://example.com/@alice" {
		t.Error("Actor.ID not set correctly")
	}
	if actor.Type != TypePerson {
		t.Error("Actor.Type not set correctly")
	}
	if actor.PreferredUsername != "alice" {
		t.Error("Actor.PreferredUsername not set correctly")
	}
}

func TestActivityStruct(t *testing.T) {
	activity := Activity{
		ID:        "https://example.com/activity/1",
		Type:      TypeCreate,
		Actor:     "https://example.com/@alice",
		Published: "2024-01-01T00:00:00Z",
		To:        []string{"https://www.w3.org/ns/activitystreams#Public"},
	}

	if activity.ID != "https://example.com/activity/1" {
		t.Error("Activity.ID not set correctly")
	}
	if activity.Type != TypeCreate {
		t.Error("Activity.Type not set correctly")
	}
}

func TestObjectStruct(t *testing.T) {
	obj := Object{
		ID:        "https://example.com/note/1",
		Type:      TypeNote,
		Content:   "Hello, world!",
		Published: "2024-01-01T00:00:00Z",
	}

	if obj.ID != "https://example.com/note/1" {
		t.Error("Object.ID not set correctly")
	}
	if obj.Type != TypeNote {
		t.Error("Object.Type not set correctly")
	}
	if obj.Content != "Hello, world!" {
		t.Error("Object.Content not set correctly")
	}
}

func TestFollowerStruct(t *testing.T) {
	now := time.Now()
	follower := Follower{
		ActorID:    "https://other.example/@bob",
		InboxURL:   "https://other.example/@bob/inbox",
		AcceptedAt: now,
		Domain:     "other.example",
		Username:   "bob",
	}

	if follower.ActorID != "https://other.example/@bob" {
		t.Error("Follower.ActorID not set correctly")
	}
	if follower.Domain != "other.example" {
		t.Error("Follower.Domain not set correctly")
	}
}

func TestKeyPairStruct(t *testing.T) {
	now := time.Now()
	kp := KeyPair{
		PrivateKeyPem: "-----BEGIN RSA PRIVATE KEY-----\ntest\n-----END RSA PRIVATE KEY-----",
		PublicKeyPem:  "-----BEGIN PUBLIC KEY-----\ntest\n-----END PUBLIC KEY-----",
		KeyID:         "https://example.com/@alice#main-key",
		CreatedAt:     now,
	}

	if kp.KeyID != "https://example.com/@alice#main-key" {
		t.Error("KeyPair.KeyID not set correctly")
	}
}

func TestBlogPostStructFields(t *testing.T) {
	now := time.Now()
	post := BlogPost{
		Title:           "Test Post",
		URL:             "https://example.com/posts/test",
		HTMLContent:     "<p>Hello</p>",
		MarkdownContent: "Hello",
		Summary:         "A test post",
		PublishedAt:     now,
		Tags:            []string{"test", "example"},
		Authors:         []string{"alice"},
		Repo:            "blog",
		Slug:            "test",
	}

	if post.Title != "Test Post" {
		t.Error("BlogPost.Title not set correctly")
	}
	if len(post.Tags) != 2 {
		t.Error("BlogPost.Tags should have 2 items")
	}
	if post.Tags[0] != "test" {
		t.Error("BlogPost.Tags[0] should be 'test'")
	}
}

func TestCollectionStruct(t *testing.T) {
	coll := Collection{
		ID:         "https://example.com/@alice/followers",
		Type:       TypeOrderedCollection,
		TotalItems: 42,
	}

	if coll.ID != "https://example.com/@alice/followers" {
		t.Error("Collection.ID not set correctly")
	}
	if coll.TotalItems != 42 {
		t.Error("Collection.TotalItems not set correctly")
	}
}

func TestTagStruct(t *testing.T) {
	tag := Tag{
		Type: "Hashtag",
		Name: "#golang",
		Href: "https://example.com/tags/golang",
	}

	if tag.Type != "Hashtag" {
		t.Error("Tag.Type not set correctly")
	}
	if tag.Name != "#golang" {
		t.Error("Tag.Name not set correctly")
	}
}

func TestCommentStruct(t *testing.T) {
	now := time.Now()
	comment := Comment{
		ID:          "comment-1",
		ActivityID:  "https://other.example/activity/1",
		InReplyTo:   "https://example.com/post/1",
		Author:      "https://other.example/@bob",
		AuthorName:  "Bob",
		Content:     "Great post!",
		ContentHTML: "<p>Great post!</p>",
		Published:   now,
		Verified:    true,
		Approved:    true,
		PostSlug:    "my-post",
		PostRepo:    "blog",
	}

	if comment.Author != "https://other.example/@bob" {
		t.Error("Comment.Author not set correctly")
	}
	if !comment.Verified {
		t.Error("Comment.Verified should be true")
	}
}

func TestPublicKeyStruct(t *testing.T) {
	pk := PublicKey{
		ID:           "https://example.com/@alice#main-key",
		Owner:        "https://example.com/@alice",
		PublicKeyPem: "-----BEGIN PUBLIC KEY-----\ntest\n-----END PUBLIC KEY-----",
	}

	if pk.ID != "https://example.com/@alice#main-key" {
		t.Error("PublicKey.ID not set correctly")
	}
	if pk.Owner != "https://example.com/@alice" {
		t.Error("PublicKey.Owner not set correctly")
	}
}

func TestImageStruct(t *testing.T) {
	img := Image{
		Type:      "Image",
		MediaType: "image/png",
		URL:       "https://example.com/avatar.png",
	}

	if img.Type != "Image" {
		t.Error("Image.Type not set correctly")
	}
	if img.MediaType != "image/png" {
		t.Error("Image.MediaType not set correctly")
	}
}
