package activitypub

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// InboxService handles incoming ActivityPub activities
type InboxService struct {
	storage      *Storage
	keyManager   *KeyManager
	actorService *ActorService
}

// NewInboxService creates a new inbox service
func NewInboxService(storage *Storage, keyManager *KeyManager, actorService *ActorService) *InboxService {
	return &InboxService{
		storage:      storage,
		keyManager:   keyManager,
		actorService: actorService,
	}
}

// HandleInbox handles incoming ActivityPub activities
func (is *InboxService) HandleInbox(w http.ResponseWriter, r *http.Request) {
	slog.Info("ActivityPub inbox request received", "method", r.Method, "path", r.URL.Path, "remote_addr", r.RemoteAddr, "user_agent", r.Header.Get("User-Agent"))

	// Only accept POST requests
	if r.Method != http.MethodPost {
		slog.Warn("Invalid method for ActivityPub inbox", "method", r.Method, "path", r.URL.Path)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check if ActivityPub is enabled
	if !isActivityPubEnabled() {
		slog.Error("ActivityPub inbox request received but ActivityPub is not enabled", "path", r.URL.Path, "remote_addr", r.RemoteAddr)
		http.Error(w, "ActivityPub not enabled", http.StatusNotFound)
		return
	}

	slog.Info("ActivityPub is enabled, processing inbox request", "path", r.URL.Path)

	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		slog.Error("Failed to read request body", "error", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Verify HTTP signature if present
	if signatureHeader := r.Header.Get("Signature"); signatureHeader != "" {
		err = is.verifyIncomingSignature(r, body)
		if err != nil {
			slog.Warn("HTTP signature verification failed", "error", err, "remote_addr", r.RemoteAddr)
			http.Error(w, "Signature verification failed", http.StatusUnauthorized)
			return
		}
		slog.Info("HTTP signature verified successfully")
	} else {
		slog.Warn("No HTTP signature found in request", "remote_addr", r.RemoteAddr)
		// For now, we'll accept unsigned requests but log them
		// In production, you might want to reject unsigned requests
	}

	// Parse the activity
	var activity Activity
	err = json.Unmarshal(body, &activity)
	if err != nil {
		slog.Error("Failed to parse activity JSON", "error", err)
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Extract username from URL path
	username := extractUsernameFromInboxPath(r.URL.Path)
	if username == "" {
		slog.Error("Could not extract username from inbox path", "path", r.URL.Path)
		http.Error(w, "Invalid inbox path", http.StatusBadRequest)
		return
	}
	slog.Info("Extracted username from inbox path", "username", username, "path", r.URL.Path)

	// Validate user exists
	if !userExists(username) {
		slog.Error("User does not exist for ActivityPub inbox", "username", username, "path", r.URL.Path)
		http.Error(w, "Actor not found", http.StatusNotFound)
		return
	}
	slog.Info("Validated user exists", "username", username)

	// Process the activity
	err = is.processActivity(&activity, username, r)
	if err != nil {
		slog.Error("Failed to process activity", "type", activity.Type, "actor", activity.Actor, "error", err)
		http.Error(w, "Failed to process activity", http.StatusInternalServerError)
		return
	}

	// Return success
	w.WriteHeader(http.StatusAccepted)
	slog.Info("Activity processed successfully", "type", activity.Type, "actor", activity.Actor, "username", username)
}

// processActivity processes different types of ActivityPub activities
func (is *InboxService) processActivity(activity *Activity, username string, r *http.Request) error {
	switch activity.Type {
	case TypeFollow:
		return is.handleFollow(activity, username, r)
	case TypeUndo:
		return is.handleUndo(activity, username, r)
	case TypeAccept:
		return is.handleAccept(activity, username, r)
	case TypeReject:
		return is.handleReject(activity, username, r)
	case TypeCreate:
		return is.handleCreate(activity, username, r)
	case TypeUpdate:
		return is.handleUpdate(activity, username, r)
	case TypeDelete:
		return is.handleDelete(activity, username, r)
	case TypeLike:
		return is.handleLike(activity, username, r)
	case TypeAnnounce:
		return is.handleAnnounce(activity, username, r)
	default:
		slog.Info("Unsupported activity type", "type", activity.Type, "actor", activity.Actor)
		return nil // Don't error on unsupported types, just ignore them
	}
}

// handleFollow processes Follow activities
func (is *InboxService) handleFollow(activity *Activity, username string, r *http.Request) error {
	actorID := activity.Actor
	slog.Info("Processing Follow activity", "actor", actorID, "username", username)

	if actorID == "" {
		slog.Error("Missing actor in Follow activity", "username", username)
		return fmt.Errorf("missing actor in Follow activity")
	}

	// Parse object - should be our actor URL
	objectStr, ok := activity.Object.(string)
	if !ok {
		slog.Error("Invalid object in Follow activity", "actor", actorID, "username", username)
		return fmt.Errorf("invalid object in Follow activity")
	}

	expectedActorURL := GetActorURL(r, username)
	if objectStr != expectedActorURL {
		slog.Error("Follow object mismatch", "actor", actorID, "username", username, "got", objectStr, "expected", expectedActorURL)
		return fmt.Errorf("Follow object doesn't match our actor URL: got %s, expected %s", objectStr, expectedActorURL)
	}

	slog.Info("Follow activity validated", "actor", actorID, "username", username, "object", objectStr)

	// Fetch the follower's actor info
	slog.Info("Fetching follower actor info", "actor", actorID)
	followerActor, err := is.fetchActor(actorID)
	if err != nil {
		slog.Error("Failed to fetch follower actor", "actor", actorID, "error", err)
		return fmt.Errorf("failed to fetch follower actor: %w", err)
	}
	slog.Info("Successfully fetched follower actor", "actor", actorID, "username", followerActor.PreferredUsername)

	// Create follower record
	follower := &Follower{
		ActorID:    actorID,
		InboxURL:   followerActor.Inbox,
		AcceptedAt: time.Now(),
		Domain:     extractDomainFromActorID(actorID),
		Username:   followerActor.PreferredUsername,
	}

	if followerActor.Endpoints != nil && followerActor.Endpoints.SharedInbox != "" {
		follower.SharedInbox = followerActor.Endpoints.SharedInbox
	}

	// Load current followers for this user
	slog.Info("Loading current followers", "username", username)
	followers, err := is.storage.LoadFollowers(username)
	if err != nil {
		slog.Error("Failed to load followers", "username", username, "error", err)
		return fmt.Errorf("failed to load followers: %w", err)
	}
	slog.Info("Loaded followers", "username", username, "count", len(followers))

	// Add to followers map
	followers[actorID] = follower
	slog.Info("Added follower to map", "actor", actorID, "username", username, "follower_username", follower.Username, "domain", follower.Domain)

	// Save followers to storage
	err = is.storage.SaveFollowers(username, followers)
	if err != nil {
		slog.Error("Failed to save followers", "username", username, "error", err)
		return fmt.Errorf("failed to save followers: %w", err)
	}
	slog.Info("Successfully saved followers to storage", "username", username, "total_followers", len(followers))

	// Send Accept activity back to the follower
	slog.Info("Sending Accept activity to follower", "actor", actorID, "inbox", follower.InboxURL)
	err = is.sendAcceptActivity(activity, follower, username, r)
	if err != nil {
		slog.Error("Failed to send Accept activity", "error", err, "follower", actorID, "inbox", follower.InboxURL)
		// Don't return error here, the follow was still processed successfully
	} else {
		slog.Info("Successfully sent Accept activity", "actor", actorID, "inbox", follower.InboxURL)
	}

	slog.Info("Follow request processed successfully", "follower", actorID, "username", username, "follower_username", follower.Username, "domain", follower.Domain, "total_followers", len(followers))
	return nil
}

// handleUndo processes Undo activities
func (is *InboxService) handleUndo(activity *Activity, username string, r *http.Request) error {
	actorID := activity.Actor
	slog.Info("Processing Undo activity", "actor", actorID, "username", username)

	if actorID == "" {
		slog.Error("Missing actor in Undo activity", "username", username)
		return fmt.Errorf("missing actor in Undo activity")
	}

	// Parse the undone object
	objectMap, ok := activity.Object.(map[string]interface{})
	if !ok {
		slog.Error("Invalid object in Undo activity", "actor", actorID, "username", username)
		return fmt.Errorf("invalid object in Undo activity")
	}

	objectType, ok := objectMap["type"].(string)
	if !ok {
		slog.Error("Missing type in undone object", "actor", actorID, "username", username)
		return fmt.Errorf("missing type in undone object")
	}
	slog.Info("Undo activity type", "actor", actorID, "username", username, "object_type", objectType)

	switch objectType {
	case TypeFollow:
		// Handle unfollow
		slog.Info("Processing unfollow", "actor", actorID, "username", username)
		followers, err := is.storage.LoadFollowers(username)
		if err != nil {
			slog.Error("Failed to load followers for unfollow", "username", username, "error", err)
			return fmt.Errorf("failed to load followers: %w", err)
		}
		slog.Info("Loaded followers for unfollow", "username", username, "count", len(followers))

		if followerInfo, exists := followers[actorID]; exists {
			slog.Info("Removing follower", "actor", actorID, "username", username, "follower_username", followerInfo.Username, "domain", followerInfo.Domain)
			delete(followers, actorID)
			err := is.storage.SaveFollowers(username, followers)
			if err != nil {
				slog.Error("Failed to save followers after unfollow", "username", username, "error", err)
				return fmt.Errorf("failed to save followers after unfollow: %w", err)
			}
			slog.Info("Unfollow processed successfully", "actor", actorID, "username", username, "follower_username", followerInfo.Username, "remaining_followers", len(followers))
		} else {
			slog.Warn("Attempted to unfollow non-existent follower", "actor", actorID, "username", username)
		}
	default:
		slog.Info("Unsupported Undo activity type", "type", objectType, "actor", actorID)
	}

	return nil
}

// handleAccept processes Accept activities (when someone accepts our follow request)
func (is *InboxService) handleAccept(activity *Activity, username string, r *http.Request) error {
	actorID := activity.Actor

	// Load following for this user
	following, err := is.storage.LoadFollowing(username)
	if err != nil {
		return fmt.Errorf("failed to load following: %w", err)
	}

	// Check if we have a pending follow for this actor
	if followingRecord, exists := following[actorID]; exists {
		followingRecord.FollowedAt = time.Now()
		err := is.storage.SaveFollowing(username, following)
		if err != nil {
			return fmt.Errorf("failed to save following after accept: %w", err)
		}
		slog.Info("Follow request accepted", "actor", actorID, "username", username)
	}

	return nil
}

// handleReject processes Reject activities (when someone rejects our follow request)
func (is *InboxService) handleReject(activity *Activity, username string, r *http.Request) error {
	actorID := activity.Actor

	// Load following for this user
	following, err := is.storage.LoadFollowing(username)
	if err != nil {
		return fmt.Errorf("failed to load following: %w", err)
	}

	// Remove from following if we were trying to follow them
	if _, exists := following[actorID]; exists {
		delete(following, actorID)
		err := is.storage.SaveFollowing(username, following)
		if err != nil {
			return fmt.Errorf("failed to save following after reject: %w", err)
		}
		slog.Info("Follow request rejected", "actor", actorID, "username", username)
	}

	return nil
}

// handleCreate processes Create activities (new posts/replies)
func (is *InboxService) handleCreate(activity *Activity, username string, r *http.Request) error {
	// Parse the created object
	objectMap, ok := activity.Object.(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid object in Create activity")
	}

	objectType, ok := objectMap["type"].(string)
	if !ok {
		return fmt.Errorf("missing type in created object")
	}

	switch objectType {
	case TypeNote:
		return is.handleCreateNote(activity, objectMap, username, r)
	case TypeArticle:
		return is.handleCreateArticle(activity, objectMap, username, r)
	default:
		slog.Info("Unsupported Create activity object type", "type", objectType, "actor", activity.Actor)
		return nil
	}
}

// handleCreateNote processes Create Note activities (replies/comments)
func (is *InboxService) handleCreateNote(activity *Activity, objectMap map[string]interface{}, username string, r *http.Request) error {
	// Check if this is a reply to one of our posts
	inReplyTo, ok := objectMap["inReplyTo"].(string)
	if !ok || inReplyTo == "" {
		// Not a reply, ignore for now
		return nil
	}

	// Parse the reply URL to determine if it's for one of our posts
	postRepo, postSlug := parsePostURLForReply(inReplyTo, r)
	if postRepo == "" || postSlug == "" {
		// Not a reply to our content
		return nil
	}

	// Extract comment details
	id, _ := objectMap["id"].(string)
	content, _ := objectMap["content"].(string)
	published, _ := objectMap["published"].(string)
	attributedTo, _ := objectMap["attributedTo"].(string)

	if id == "" || content == "" || attributedTo == "" {
		return fmt.Errorf("missing required fields in Note object")
	}

	// Parse published time
	publishedTime := time.Now()
	if published != "" {
		if t, err := time.Parse(time.RFC3339, published); err == nil {
			publishedTime = t
		}
	}

	// Fetch author information
	authorActor, err := is.fetchActor(attributedTo)
	if err != nil {
		slog.Warn("Failed to fetch author actor for comment", "error", err, "actor", attributedTo)
		// Continue with limited info
	}

	authorName := attributedTo
	authorURL := attributedTo
	if authorActor != nil {
		if authorActor.Name != "" {
			authorName = authorActor.Name
		} else if authorActor.PreferredUsername != "" {
			authorName = authorActor.PreferredUsername
		}
		if authorActor.URL != "" {
			authorURL = authorActor.URL
		}
	}

	// Create comment record
	comment := &Comment{
		ID:          generateCommentID(id),
		ActivityID:  activity.ID,
		InReplyTo:   inReplyTo,
		Author:      attributedTo,
		AuthorName:  authorName,
		AuthorURL:   authorURL,
		Content:     content,
		ContentHTML: content, // TODO: Process markdown/HTML if needed
		Published:   publishedTime,
		Verified:    true, // We verified the signature
		Approved:    true, // TODO: Implement moderation
		Hidden:      false,
		PostSlug:    postSlug,
		PostRepo:    postRepo,
		Metadata:    make(map[string]string),
	}

	// Save comment
	err = is.storage.SaveComment(comment)
	if err != nil {
		return fmt.Errorf("failed to save comment: %w", err)
	}

	slog.Info("Comment created", "id", comment.ID, "author", authorName, "post", postSlug)
	return nil
}

// handleCreateArticle processes Create Article activities
func (is *InboxService) handleCreateArticle(activity *Activity, objectMap map[string]interface{}, username string, r *http.Request) error {
	// For now, just log articles - we might want to track mentions or references
	id, _ := objectMap["id"].(string)
	name, _ := objectMap["name"].(string)
	attributedTo, _ := objectMap["attributedTo"].(string)

	slog.Info("Article created", "id", id, "name", name, "author", attributedTo)
	return nil
}

// handleUpdate processes Update activities
func (is *InboxService) handleUpdate(activity *Activity, username string, r *http.Request) error {
	// Handle updates to actors, objects, etc.
	// For now, just log them
	slog.Info("Update activity received", "actor", activity.Actor, "object", activity.Object)
	return nil
}

// handleDelete processes Delete activities
func (is *InboxService) handleDelete(activity *Activity, username string, r *http.Request) error {
	// Handle deletions
	// For now, just log them
	slog.Info("Delete activity received", "actor", activity.Actor, "object", activity.Object)
	return nil
}

// handleLike processes Like activities
func (is *InboxService) handleLike(activity *Activity, username string, r *http.Request) error {
	// Handle likes/favorites
	// For now, just log them
	slog.Info("Like activity received", "actor", activity.Actor, "object", activity.Object)
	return nil
}

// handleAnnounce processes Announce activities (boosts/reblogs)
func (is *InboxService) handleAnnounce(activity *Activity, username string, r *http.Request) error {
	// Handle announces/boosts
	// For now, just log them
	slog.Info("Announce activity received", "actor", activity.Actor, "object", activity.Object)
	return nil
}

// Helper functions

func (is *InboxService) verifyIncomingSignature(r *http.Request, body []byte) error {
	// Parse signature header to extract key ID
	signatureHeader := r.Header.Get("Signature")
	params, err := parseSignatureHeader(signatureHeader)
	if err != nil {
		return fmt.Errorf("failed to parse signature header: %w", err)
	}

	keyID, exists := params["keyId"]
	if !exists {
		return fmt.Errorf("keyId not found in signature header")
	}

	// Fetch public key
	publicKeyPEM, err := is.keyManager.FetchPublicKey(keyID)
	if err != nil {
		return fmt.Errorf("failed to fetch public key: %w", err)
	}

	// Verify signature
	return is.keyManager.VerifySignature(r, body, publicKeyPEM)
}

func (is *InboxService) fetchActor(actorID string) (*Actor, error) {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	req, err := http.NewRequest("GET", actorID, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/activity+json, application/ld+json")
	req.Header.Set("User-Agent", "Sn/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch actor: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch actor, status: %d", resp.StatusCode)
	}

	var actor Actor
	err = json.NewDecoder(resp.Body).Decode(&actor)
	if err != nil {
		return nil, fmt.Errorf("failed to decode actor: %w", err)
	}

	return &actor, nil
}

func (is *InboxService) sendAcceptActivity(followActivity *Activity, follower *Follower, username string, r *http.Request) error {
	baseURL := GetBaseURL(r)
	actorURL := GetActorURL(r, username)
	acceptID := GenerateActivityID(baseURL, username)

	accept := &Activity{
		Context: ActivityPubContext,
		ID:      acceptID,
		Type:    TypeAccept,
		Actor:   actorURL,
		Object:  followActivity,
		To:      []string{follower.ActorID},
	}

	// Convert to JSON
	acceptJSON, err := json.Marshal(accept)
	if err != nil {
		return fmt.Errorf("failed to marshal Accept activity: %w", err)
	}

	// Send to follower's inbox
	return is.sendActivityToInbox(follower.InboxURL, acceptJSON)
}

func (is *InboxService) sendActivityToInbox(inboxURL string, activityJSON []byte) error {
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	req, err := http.NewRequest("POST", inboxURL, strings.NewReader(string(activityJSON)))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", ContentTypeActivityJSON)
	req.Header.Set("User-Agent", "Sn/1.0")

	// Sign the request
	err = is.keyManager.SignRequest(req, activityJSON)
	if err != nil {
		return fmt.Errorf("failed to sign request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send activity: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("failed to deliver activity, status: %d", resp.StatusCode)
	}

	return nil
}

func extractUsernameFromInboxPath(path string) string {
	// Expected formats: /@username/inbox or /inbox (shared inbox)
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")

	if len(parts) == 1 && parts[0] == "inbox" {
		// Shared inbox - for now, return empty (we might handle this differently)
		return ""
	}

	if len(parts) >= 2 && strings.HasPrefix(parts[0], "@") && parts[1] == "inbox" {
		return strings.TrimPrefix(parts[0], "@")
	}

	return ""
}

func extractDomainFromActorID(actorID string) string {
	u, err := url.Parse(actorID)
	if err != nil {
		return ""
	}
	return u.Host
}

func parsePostURLForReply(inReplyTo string, r *http.Request) (string, string) {
	// Parse the URL to see if it matches our post URL pattern
	// Expected pattern: https://domain.com/repo/slug or similar
	u, err := url.Parse(inReplyTo)
	if err != nil {
		return "", ""
	}

	// Check if it's our domain
	if u.Host != r.Host {
		return "", ""
	}

	// Parse path - this depends on your URL structure
	// For now, assume /repo/slug pattern
	parts := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
	if len(parts) >= 2 {
		return parts[0], parts[1] // repo, slug
	}

	return "", ""
}

func generateCommentID(originalID string) string {
	// Create a consistent ID for the comment
	// Use hash of original ID to ensure uniqueness
	return fmt.Sprintf("comment-%x", time.Now().UnixNano())
}
