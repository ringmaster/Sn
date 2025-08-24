package activitypub

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// ActorService handles ActivityPub actor operations
type ActorService struct {
	storage    *Storage
	keyManager *KeyManager
}

// NewActorService creates a new actor service
func NewActorService(storage *Storage, keyManager *KeyManager) *ActorService {
	return &ActorService{
		storage:    storage,
		keyManager: keyManager,
	}
}

// HandleWebfinger handles WebFinger requests for actor discovery
func (as *ActorService) HandleWebfinger(w http.ResponseWriter, r *http.Request) {
	// Set security headers
	w.Header().Set("Content-Type", "application/jrd+json")
	w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
	w.Header().Set("X-Frame-Options", "SAMEORIGIN")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

	resource := r.URL.Query().Get("resource")
	if resource == "" {
		http.Error(w, "Missing resource parameter", http.StatusBadRequest)
		return
	}

	// Parse resource parameter
	parts := strings.SplitN(resource, ":", 2)
	if len(parts) != 2 || parts[0] != "acct" {
		http.Error(w, "Invalid resource format", http.StatusBadRequest)
		return
	}

	accountName := parts[1]
	atIndex := strings.LastIndex(accountName, "@")
	if atIndex == -1 {
		http.Error(w, "Invalid account format", http.StatusBadRequest)
		return
	}

	username := accountName[:atIndex]
	domain := accountName[atIndex+1:]

	// Validate domain matches request host or configured domain
	expectedDomain := getDomainFromConfig()
	if domain != r.Host && domain != expectedDomain {
		http.Error(w, "Invalid domain", http.StatusBadRequest)
		return
	}

	// Validate user exists in config
	users := viper.GetStringMap("users")
	if _, exists := users[username]; !exists {
		http.Error(w, "Account not found", http.StatusNotFound)
		return
	}

	// Build URLs
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}

	actorURL := fmt.Sprintf("%s://%s/@%s", scheme, domain, username)
	profileURL := fmt.Sprintf("%s://%s/", scheme, domain)

	// Create WebFinger response
	webfingerResponse := map[string]interface{}{
		"subject": resource,
		"aliases": []string{actorURL},
		"links": []map[string]interface{}{
			{
				"rel":  "self",
				"type": ContentTypeActivityJSON,
				"href": actorURL,
			},
			{
				"rel":  "http://webfinger.net/rel/profile-page",
				"type": "text/html",
				"href": profileURL,
			},
		},
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(webfingerResponse)
	slog.Info("WebFinger request handled", "resource", resource, "username", username)
}

// HandleActor handles ActivityPub actor profile requests
func (as *ActorService) HandleActor(w http.ResponseWriter, r *http.Request) {
	// Extract username from URL path
	// Expected format: /@username
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if len(pathParts) == 0 || !strings.HasPrefix(pathParts[0], "@") {
		http.Error(w, "Invalid actor path", http.StatusBadRequest)
		return
	}

	username := strings.TrimPrefix(pathParts[0], "@")
	if username == "" {
		http.Error(w, "Invalid username", http.StatusBadRequest)
		return
	}

	// Validate user exists in config
	users := viper.GetStringMap("users")
	userConfig, exists := users[username]
	if !exists {
		http.Error(w, "Actor not found", http.StatusNotFound)
		return
	}

	// Check if ActivityPub is enabled for this user/repo
	if !isActivityPubEnabled() {
		http.Error(w, "ActivityPub not enabled", http.StatusNotFound)
		return
	}

	// Set content type based on Accept header
	accept := r.Header.Get("Accept")
	if strings.Contains(accept, ContentTypeActivityJSON) ||
		strings.Contains(accept, ContentTypeLDJSON) {
		w.Header().Set("Content-Type", ContentTypeActivityJSON)
	} else {
		w.Header().Set("Content-Type", ContentTypeActivityJSON)
	}

	// Set security headers
	w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
	w.Header().Set("X-Frame-Options", "SAMEORIGIN")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

	// Build base URLs
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	baseURL := fmt.Sprintf("%s://%s", scheme, r.Host)
	actorURL := fmt.Sprintf("%s/@%s", baseURL, username)

	// Get user display information
	displayName := username
	summary := ""

	if userMap, ok := userConfig.(map[string]interface{}); ok {
		if name, exists := userMap["displayName"].(string); exists && name != "" {
			displayName = name
		}
		if bio, exists := userMap["bio"].(string); exists && bio != "" {
			summary = bio
		}
	}

	// Create actor object
	actor := &Actor{
		Context:                   ActivityPubContext,
		ID:                        actorURL,
		Type:                      TypePerson,
		Name:                      displayName,
		PreferredUsername:         username,
		Summary:                   summary,
		URL:                       baseURL,
		ManuallyApprovesFollowers: false,
		Discoverable:              true,
		Published:                 time.Now().UTC().Format(time.RFC3339),
		PublicKey: &PublicKey{
			ID:           actorURL + "#main-key",
			Owner:        actorURL,
			PublicKeyPem: as.keyManager.GetPublicKeyPEM(),
		},
		Inbox:     actorURL + "/inbox",
		Outbox:    actorURL + "/outbox",
		Following: actorURL + "/following",
		Followers: actorURL + "/followers",
		Endpoints: &Endpoints{
			SharedInbox: baseURL + "/inbox",
		},
	}

	// Add icon if configured
	if iconURL := viper.GetString("activitypub.icon"); iconURL != "" {
		actor.Icon = &Image{
			Type: "Image",
			URL:  iconURL,
		}
	}

	// Add banner if configured
	if bannerURL := viper.GetString("activitypub.banner"); bannerURL != "" {
		actor.Image = &Image{
			Type: "Image",
			URL:  bannerURL,
		}
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(actor)
	slog.Info("Actor profile request handled", "username", username, "actorURL", actorURL)
}

// HandleFollowers handles followers collection requests
func (as *ActorService) HandleFollowers(w http.ResponseWriter, r *http.Request) {
	username := extractUsernameFromPath(r.URL.Path, "followers")
	if username == "" {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	// Validate user exists
	if !userExists(username) {
		http.Error(w, "Actor not found", http.StatusNotFound)
		return
	}

	// Check if ActivityPub is enabled
	if !isActivityPubEnabled() {
		http.Error(w, "ActivityPub not enabled", http.StatusNotFound)
		return
	}

	// Set headers
	w.Header().Set("Content-Type", ContentTypeActivityJSON)
	setSecurityHeaders(w)

	// Build URLs
	scheme := getScheme(r)
	baseURL := fmt.Sprintf("%s://%s", scheme, r.Host)
	actorURL := fmt.Sprintf("%s/@%s", baseURL, username)
	followersURL := actorURL + "/followers"

	// Load followers for this user
	followers, err := as.storage.LoadFollowers(username)
	if err != nil {
		slog.Error("Failed to load followers", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Create followers collection
	followerList := make([]string, 0, len(followers))
	for actorID := range followers {
		followerList = append(followerList, actorID)
	}

	collection := &Collection{
		Context:      ActivityPubContext,
		ID:           followersURL,
		Type:         TypeOrderedCollection,
		TotalItems:   len(followerList),
		OrderedItems: followerList,
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(collection)
	slog.Info("Followers collection request handled", "username", username, "count", len(followerList))
}

// HandleFollowing handles following collection requests
func (as *ActorService) HandleFollowing(w http.ResponseWriter, r *http.Request) {
	username := extractUsernameFromPath(r.URL.Path, "following")
	if username == "" {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	// Validate user exists
	if !userExists(username) {
		http.Error(w, "Actor not found", http.StatusNotFound)
		return
	}

	// Check if ActivityPub is enabled
	if !isActivityPubEnabled() {
		http.Error(w, "ActivityPub not enabled", http.StatusNotFound)
		return
	}

	// Set headers
	w.Header().Set("Content-Type", ContentTypeActivityJSON)
	setSecurityHeaders(w)

	// Build URLs
	scheme := getScheme(r)
	baseURL := fmt.Sprintf("%s://%s", scheme, r.Host)
	actorURL := fmt.Sprintf("%s/@%s", baseURL, username)
	followingURL := actorURL + "/following"

	// Load following for this user
	following, err := as.storage.LoadFollowing(username)
	if err != nil {
		slog.Error("Failed to load following", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Create following collection
	followingList := make([]string, 0, len(following))
	for actorID := range following {
		followingList = append(followingList, actorID)
	}

	collection := &Collection{
		Context:      ActivityPubContext,
		ID:           followingURL,
		Type:         TypeOrderedCollection,
		TotalItems:   len(followingList),
		OrderedItems: followingList,
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(collection)
	slog.Info("Following collection request handled", "username", username, "count", len(followingList))
}

// Helper functions

func extractUsernameFromPath(path, suffix string) string {
	// Expected format: /@username/suffix
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) < 2 {
		return ""
	}

	if !strings.HasPrefix(parts[0], "@") {
		return ""
	}

	if parts[1] != suffix {
		return ""
	}

	return strings.TrimPrefix(parts[0], "@")
}

func userExists(username string) bool {
	users := viper.GetStringMap("users")
	_, exists := users[username]
	return exists
}

func isActivityPubEnabled() bool {
	return viper.GetBool("activitypub.enabled")
}

func getScheme(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

func setSecurityHeaders(w http.ResponseWriter) {
	w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
	w.Header().Set("X-Frame-Options", "SAMEORIGIN")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
}

// parseJSONResponse parses a JSON response into the provided interface
func parseJSONResponse(resp *http.Response, v interface{}) error {
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(v)
}

// GetActorURL builds the actor URL for a given username and request
func GetActorURL(r *http.Request, username string) string {
	scheme := getScheme(r)
	return fmt.Sprintf("%s://%s/@%s", scheme, r.Host, username)
}

// GetBaseURL builds the base URL from a request
func GetBaseURL(r *http.Request) string {
	scheme := getScheme(r)
	return fmt.Sprintf("%s://%s", scheme, r.Host)
}
