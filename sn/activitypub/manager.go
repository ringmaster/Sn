package activitypub

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/gorilla/mux"
	"github.com/spf13/afero"
	"github.com/spf13/viper"
)

// Manager coordinates all ActivityPub services
type Manager struct {
	storage       *Storage
	keyManager    *KeyManager
	actorService  *ActorService
	inboxService  *InboxService
	outboxService *OutboxService
	enabled       bool
}

// NewManager creates a new ActivityPub manager
func NewManager(mainFs afero.Fs) (*Manager, error) {
	// Check if ActivityPub is enabled
	enabled := viper.GetBool("activitypub.enabled")
	if !enabled {
		slog.Info("ActivityPub is disabled")
		return &Manager{enabled: false}, nil
	}

	slog.Info("Initializing ActivityPub services")

	// Initialize storage
	storage, err := NewStorage(mainFs)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize ActivityPub storage: %w", err)
	}

	// Initialize key manager
	keyManager := NewKeyManager(storage)

	// Get the primary user for key initialization
	primaryUser := getPrimaryUser()
	if primaryUser == "" {
		return nil, fmt.Errorf("no users configured for ActivityPub")
	}

	// Build actor URL for key initialization
	baseURL := getBaseURL()
	actorURL := fmt.Sprintf("%s/@%s", baseURL, primaryUser)

	// Initialize keys
	err = keyManager.InitializeKeys(actorURL)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize ActivityPub keys: %w", err)
	}

	// Initialize services
	actorService := NewActorService(storage, keyManager)
	inboxService := NewInboxService(storage, keyManager, actorService)
	outboxService := NewOutboxService(storage, keyManager, actorService, inboxService)

	manager := &Manager{
		storage:       storage,
		keyManager:    keyManager,
		actorService:  actorService,
		inboxService:  inboxService,
		outboxService: outboxService,
		enabled:       true,
	}

	slog.Info("ActivityPub services initialized successfully")
	return manager, nil
}

// IsEnabled returns whether ActivityPub is enabled
func (m *Manager) IsEnabled() bool {
	return m.enabled
}

// RegisterRoutes registers ActivityPub routes with the provided router
func (m *Manager) RegisterRoutes(router *mux.Router) {
	if !m.enabled {
		return
	}

	slog.Info("Registering ActivityPub routes")

	// WebFinger endpoint
	router.HandleFunc("/.well-known/webfinger", m.actorService.HandleWebfinger).
		Methods("GET").
		Name("activitypub-webfinger")

	// Actor profile endpoints
	router.HandleFunc("/@{username}", m.actorService.HandleActor).
		Methods("GET").
		Headers("Accept", "application/activity+json").
		Name("activitypub-actor")

	router.HandleFunc("/@{username}", m.actorService.HandleActor).
		Methods("GET").
		Headers("Accept", "application/ld+json").
		Name("activitypub-actor-ld")

	// Fallback for actors without specific Accept header
	router.HandleFunc("/@{username}", m.handleActorWithContentNegotiation).
		Methods("GET").
		Name("activitypub-actor-fallback")

	// Inbox endpoints
	router.HandleFunc("/@{username}/inbox", m.inboxService.HandleInbox).
		Methods("POST").
		Name("activitypub-inbox")

	// Shared inbox
	router.HandleFunc("/inbox", m.inboxService.HandleInbox).
		Methods("POST").
		Name("activitypub-shared-inbox")

	// Outbox endpoints
	router.HandleFunc("/@{username}/outbox", m.outboxService.HandleOutbox).
		Methods("GET").
		Name("activitypub-outbox")

	// Followers collection
	router.HandleFunc("/@{username}/followers", m.actorService.HandleFollowers).
		Methods("GET").
		Name("activitypub-followers")

	// Following collection
	router.HandleFunc("/@{username}/following", m.actorService.HandleFollowing).
		Methods("GET").
		Name("activitypub-following")

	slog.Info("ActivityPub routes registered successfully")
}

// handleActorWithContentNegotiation handles actor requests with proper content negotiation
func (m *Manager) handleActorWithContentNegotiation(w http.ResponseWriter, r *http.Request) {
	accept := r.Header.Get("Accept")

	// Check if client accepts ActivityPub content types
	if containsActivityPubContentType(accept) {
		m.actorService.HandleActor(w, r)
		return
	}

	// For non-ActivityPub requests, we might want to redirect to the blog homepage
	// or return an HTML profile page. For now, return a simple message.
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)

	username := mux.Vars(r)["username"]
	fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head>
	<title>@%s</title>
	<meta name="viewport" content="width=device-width, initial-scale=1">
</head>
<body>
	<h1>@%s</h1>
	<p>This is an ActivityPub actor. To follow this user, search for @%s@%s in your ActivityPub client.</p>
	<p><a href="/">← Back to blog</a></p>
</body>
</html>`, username, username, username, r.Host)
}

// PublishPost publishes a blog post to ActivityPub
// For multi-author posts, publishes under each author that exists in the users config
func (m *Manager) PublishPost(post *BlogPost) error {
	if !m.enabled {
		return nil
	}

	baseURL := getBaseURL()

	// If post has multiple authors, we could publish from each one
	// For now, we'll publish from the primary author (handled in outboxService)
	// TODO: Consider if we want to publish the same post from multiple actors
	return m.outboxService.PublishPost(post, baseURL)
}

// UpdatePost publishes an update for a blog post to ActivityPub
// Updates are published from the same author as the original post
func (m *Manager) UpdatePost(post *BlogPost) error {
	if !m.enabled {
		return nil
	}

	baseURL := getBaseURL()
	return m.outboxService.UpdatePost(post, baseURL)
}

// DeletePost publishes a delete activity for a blog post to ActivityPub
// Since we may not have the original post data, falls back to repo owner
func (m *Manager) DeletePost(postURL, repo string) error {
	if !m.enabled {
		return nil
	}

	baseURL := getBaseURL()
	return m.outboxService.DeletePost(postURL, repo, baseURL)
}

// GetComments returns comments for a specific post
func (m *Manager) GetComments(repo, slug string) ([]*Comment, error) {
	if !m.enabled {
		return nil, nil
	}

	return m.storage.LoadComments(repo, slug)
}

// Close cleans up resources
func (m *Manager) Close() error {
	if !m.enabled {
		return nil
	}

	slog.Info("Shutting down ActivityPub services")
	return m.storage.Close()
}

// Helper functions

func containsActivityPubContentType(accept string) bool {
	return strings.Contains(accept, ContentTypeActivityJSON) ||
		strings.Contains(accept, ContentTypeLDJSON) ||
		strings.Contains(accept, "application/activity+json") ||
		strings.Contains(accept, "application/ld+json")
}

func getPrimaryUser() string {
	users := viper.GetStringMap("users")

	// Look for a designated primary user
	if primaryUser := viper.GetString("activitypub.primary_user"); primaryUser != "" {
		if _, exists := users[primaryUser]; exists {
			return primaryUser
		}
	}

	// Fall back to the first user
	for username := range users {
		return username
	}

	return ""
}

func getBaseURL() string {
	// First try ActivityPub-specific override
	if baseURL := viper.GetString("activitypub.rooturl"); baseURL != "" {
		return strings.TrimSuffix(baseURL, "/")
	}

	// Then try existing rooturl config
	if rootURL := viper.GetString("rooturl"); rootURL != "" {
		// Remove trailing slash if present
		return strings.TrimSuffix(rootURL, "/")
	}

	// Fall back to site.base_url if specified (legacy)
	if baseURL := viper.GetString("site.base_url"); baseURL != "" {
		return strings.TrimSuffix(baseURL, "/")
	}

	// Try to construct from domain
	domain := getDomainFromConfig()
	if domain != "localhost" {
		scheme := "https"
		if viper.GetBool("activitypub.insecure") || viper.GetBool("site.insecure") {
			scheme = "http"
		}
		return fmt.Sprintf("%s://%s", scheme, domain)
	}

	// Default fallback
	return "https://localhost"
}

// getDomainFromConfig extracts the domain from existing config
func getDomainFromConfig() string {
	// First try ActivityPub-specific domain override
	if domain := viper.GetString("activitypub.domain"); domain != "" {
		return domain
	}

	// Try to parse domain from ActivityPub rooturl override
	if rootURL := viper.GetString("activitypub.rooturl"); rootURL != "" {
		if u, err := url.Parse(rootURL); err == nil && u.Host != "" {
			return u.Host
		}
	}

	// Try to parse domain from main rooturl
	if rootURL := viper.GetString("rooturl"); rootURL != "" {
		if u, err := url.Parse(rootURL); err == nil && u.Host != "" {
			return u.Host
		}
	}

	// Legacy fallbacks
	if domain := viper.GetString("site.domain"); domain != "" {
		return domain
	}

	if baseURL := viper.GetString("site.base_url"); baseURL != "" {
		if u, err := url.Parse(baseURL); err == nil && u.Host != "" {
			return u.Host
		}
	}

	return "localhost"
}

// getSiteNameFromConfig gets the site name from existing config
func getSiteNameFromConfig() string {
	// First try ActivityPub-specific title override
	if title := viper.GetString("activitypub.title"); title != "" {
		return title
	}

	// Then try existing title
	if title := viper.GetString("title"); title != "" {
		return title
	}

	// Legacy fallback to site.name
	if siteName := viper.GetString("site.name"); siteName != "" {
		return siteName
	}

	return "Sn Blog"
}
