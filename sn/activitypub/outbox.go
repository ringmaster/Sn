package activitypub

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// OutboxService handles ActivityPub outbox operations
type OutboxService struct {
	storage      *Storage
	keyManager   *KeyManager
	actorService *ActorService
	inboxService *InboxService
}

// NewOutboxService creates a new outbox service
func NewOutboxService(storage *Storage, keyManager *KeyManager, actorService *ActorService, inboxService *InboxService) *OutboxService {
	return &OutboxService{
		storage:      storage,
		keyManager:   keyManager,
		actorService: actorService,
		inboxService: inboxService,
	}
}

// HandleOutbox handles outbox collection requests
func (os *OutboxService) HandleOutbox(w http.ResponseWriter, r *http.Request) {
	username := extractUsernameFromPath(r.URL.Path, "outbox")
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
	outboxURL := actorURL + "/outbox"

	// Get pagination parameters
	page := r.URL.Query().Get("page")
	if page == "" {
		// Return outbox collection summary
		os.handleOutboxCollection(w, outboxURL, username)
		return
	}

	// Return specific page
	pageNum, err := strconv.Atoi(page)
	if err != nil {
		http.Error(w, "Invalid page parameter", http.StatusBadRequest)
		return
	}

	os.handleOutboxPage(w, outboxURL, username, pageNum)
}

// handleOutboxCollection returns the outbox collection summary
func (os *OutboxService) handleOutboxCollection(w http.ResponseWriter, outboxURL, username string) {
	// For now, return a simple collection
	// In a full implementation, you'd get the actual count of published activities
	totalItems := os.getTotalPublishedActivities(username)

	collection := &Collection{
		Context:    ActivityPubContext,
		ID:         outboxURL,
		Type:       TypeOrderedCollection,
		TotalItems: totalItems,
		First:      outboxURL + "?page=1",
	}

	if totalItems > 0 {
		collection.Last = fmt.Sprintf("%s?page=%d", outboxURL, (totalItems/20)+1)
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(collection)
	slog.Info("Outbox collection request handled", "username", username, "totalItems", totalItems)
}

// handleOutboxPage returns a specific page of outbox items
func (os *OutboxService) handleOutboxPage(w http.ResponseWriter, outboxURL, username string, pageNum int) {
	// Get activities for this page
	activities := os.getActivitiesForPage(username, pageNum)

	pageURL := fmt.Sprintf("%s?page=%d", outboxURL, pageNum)

	page := &CollectionPage{
		Context:      ActivityPubContext,
		ID:           pageURL,
		Type:         TypeOrderedCollectionPage,
		PartOf:       outboxURL,
		OrderedItems: activities,
	}

	// Add next/prev links
	if pageNum > 1 {
		page.Prev = fmt.Sprintf("%s?page=%d", outboxURL, pageNum-1)
	}

	if len(activities) == 20 { // Full page, might have more
		page.Next = fmt.Sprintf("%s?page=%d", outboxURL, pageNum+1)
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(page)
	slog.Info("Outbox page request handled", "username", username, "page", pageNum, "items", len(activities))
}

// PublishPost publishes a blog post to ActivityPub followers
func (os *OutboxService) PublishPost(post *BlogPost, baseURL string) error {
	// Check if ActivityPub is enabled for this repo
	if !isActivityPubEnabledForRepo(post.Repo) {
		slog.Info("ActivityPub not enabled for repo, skipping publication", "repo", post.Repo)
		return nil
	}

	// Get the primary author for this post
	author := getPostPrimaryAuthor(post)
	if author == "" {
		return fmt.Errorf("no valid author found for post: %s", post.Title)
	}

	actorURL := fmt.Sprintf("%s/@%s", baseURL, author)

	// Build attribution - can be a single actor or array of actors for multi-author posts
	attribution := buildPostAttribution(post, baseURL)

	// Create the Article object
	article := &Article{
		Object: Object{
			Context:      ActivityPubContext,
			ID:           post.URL,
			Type:         TypeArticle,
			Name:         post.Title,
			Content:      post.HTMLContent,
			Summary:      post.Summary,
			URL:          post.URL,
			AttributedTo: attribution, // Can be string or []string
			Published:    post.PublishedAt.Format(time.RFC3339),
			To:           []string{"https://www.w3.org/ns/activitystreams#Public"},
			CC:           buildFollowersCC(post, baseURL), // Include all authors' followers
			Tag:          convertTagsToActivityPub(post.Tags),
		},
	}

	if post.MarkdownContent != "" {
		article.Source = &Source{
			Content:   post.MarkdownContent,
			MediaType: "text/markdown",
		}
	}

	// Create the Create activity
	activityID := GenerateActivityID(baseURL, author)
	createActivity := &Activity{
		Context:   ActivityPubContext,
		ID:        activityID,
		Type:      TypeCreate,
		Actor:     actorURL, // Primary author is the actor
		Object:    article,
		Published: post.PublishedAt.Format(time.RFC3339),
		To:        []string{"https://www.w3.org/ns/activitystreams#Public"},
		CC:        buildFollowersCC(post, baseURL), // Include all authors' followers
	}

	// Deliver to followers
	err := os.deliverToFollowers(createActivity, author)
	if err != nil {
		return fmt.Errorf("failed to deliver to followers: %w", err)
	}

	slog.Info("Blog post published to ActivityPub", "title", post.Title, "url", post.URL, "actor", actorURL, "author", author)
	return nil
}

// UpdatePost publishes an update activity for a modified blog post
func (os *OutboxService) UpdatePost(post *BlogPost, baseURL string) error {
	// Check if ActivityPub is enabled for this repo
	if !isActivityPubEnabledForRepo(post.Repo) {
		return nil
	}

	author := getPostPrimaryAuthor(post)
	if author == "" {
		return fmt.Errorf("no valid author found for post: %s", post.Title)
	}

	actorURL := fmt.Sprintf("%s/@%s", baseURL, author)

	// Build attribution for updated post
	attribution := buildPostAttribution(post, baseURL)

	// Create the updated Article object
	article := &Article{
		Object: Object{
			Context:      ActivityPubContext,
			ID:           post.URL,
			Type:         TypeArticle,
			Name:         post.Title,
			Content:      post.HTMLContent,
			Summary:      post.Summary,
			URL:          post.URL,
			AttributedTo: attribution, // Can be string or []string
			Published:    post.PublishedAt.Format(time.RFC3339),
			Updated:      time.Now().Format(time.RFC3339),
			To:           []string{"https://www.w3.org/ns/activitystreams#Public"},
			CC:           buildFollowersCC(post, baseURL), // Include all authors' followers
			Tag:          convertTagsToActivityPub(post.Tags),
		},
	}

	if post.MarkdownContent != "" {
		article.Source = &Source{
			Content:   post.MarkdownContent,
			MediaType: "text/markdown",
		}
	}

	// Create the Update activity
	activityID := GenerateActivityID(baseURL, author)
	updateActivity := &Activity{
		Context:   ActivityPubContext,
		ID:        activityID,
		Type:      TypeUpdate,
		Actor:     actorURL,
		Object:    article,
		Published: time.Now().Format(time.RFC3339),
		To:        []string{"https://www.w3.org/ns/activitystreams#Public"},
		CC:        buildFollowersCC(post, baseURL), // Include all authors' followers
	}

	// Deliver to followers
	err := os.deliverToFollowers(updateActivity, author)
	if err != nil {
		return fmt.Errorf("failed to deliver update to followers: %w", err)
	}

	slog.Info("Blog post update published to ActivityPub", "title", post.Title, "url", post.URL, "actor", actorURL, "author", author)
	return nil
}

// DeletePost publishes a delete activity for a removed blog post
func (os *OutboxService) DeletePost(postURL, repo, baseURL string) error {
	// Check if ActivityPub is enabled for this repo
	if !isActivityPubEnabledForRepo(repo) {
		return nil
	}

	// For deletions, we need to look up the original author or fall back to repo owner
	author := getRepoOwner(repo) // Fallback since we may not have the original post data
	if author == "" {
		return fmt.Errorf("no owner configured for repo: %s", repo)
	}

	actorURL := fmt.Sprintf("%s/@%s", baseURL, author)

	// Create the Delete activity
	activityID := GenerateActivityID(baseURL, author)
	deleteActivity := &Activity{
		Context:   ActivityPubContext,
		ID:        activityID,
		Type:      TypeDelete,
		Actor:     actorURL,
		Object:    postURL,
		Published: time.Now().Format(time.RFC3339),
		To:        []string{"https://www.w3.org/ns/activitystreams#Public"},
		CC:        []string{actorURL + "/followers"}, // For deletes, just use the fallback actor
	}

	// Deliver to followers
	err := os.deliverToFollowers(deleteActivity, author)
	if err != nil {
		return fmt.Errorf("failed to deliver delete to followers: %w", err)
	}

	slog.Info("Blog post deletion published to ActivityPub", "url", postURL, "actor", actorURL, "author", author)
	return nil
}

// deliverToFollowers sends an activity to all followers of the specified user
// For multi-author posts, this should be called for the primary author
func (os *OutboxService) deliverToFollowers(activity *Activity, username string) error {
	// For multi-author posts, we could potentially deliver to all authors' followers
	// For now, we deliver to the primary author's followers
	// TODO: Consider if we want to aggregate followers from all co-authors

	// Load followers for the specified user
	followers, err := os.storage.LoadFollowers(username)
	if err != nil {
		return fmt.Errorf("failed to load followers: %w", err)
	}

	if len(followers) == 0 {
		slog.Info("No followers to deliver to", "username", username)
		return nil
	}

	// Convert activity to JSON
	activityJSON, err := json.Marshal(activity)
	if err != nil {
		return fmt.Errorf("failed to marshal activity: %w", err)
	}

	// Group followers by shared inbox to reduce requests
	inboxGroups := make(map[string][]*Follower)
	for _, follower := range followers {
		inboxURL := follower.InboxURL
		if follower.SharedInbox != "" {
			inboxURL = follower.SharedInbox
		}
		inboxGroups[inboxURL] = append(inboxGroups[inboxURL], follower)
	}

	// Deliver to each inbox
	successCount := 0
	for inboxURL, inboxFollowers := range inboxGroups {
		err := os.deliverToInbox(inboxURL, activityJSON)
		if err != nil {
			slog.Error("Failed to deliver to inbox", "inbox", inboxURL, "error", err)
			// Continue with other inboxes
		} else {
			successCount += len(inboxFollowers)
			slog.Info("Successfully delivered to inbox", "inbox", inboxURL, "followers", len(inboxFollowers))
		}
	}

	slog.Info("Activity delivery completed", "username", username, "total_followers", len(followers), "successful_deliveries", successCount)
	return nil
}

// deliverToInbox sends an activity to a specific inbox
func (os *OutboxService) deliverToInbox(inboxURL string, activityJSON []byte) error {
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
	err = os.keyManager.SignRequest(req, activityJSON)
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

// Helper functions for outbox implementation

func (os *OutboxService) getTotalPublishedActivities(username string) int {
	// This would query your database/storage for the count of published activities
	// For now, return 0 as placeholder
	return 0
}

func (os *OutboxService) getActivitiesForPage(username string, pageNum int) []string {
	// This would query your database/storage for activities on this page
	// For now, return empty slice as placeholder
	return []string{}
}

// BlogPost represents a blog post for ActivityPub publishing
type BlogPost struct {
	Title           string
	URL             string
	HTMLContent     string
	MarkdownContent string
	Summary         string
	PublishedAt     time.Time
	Tags            []string
	Authors         []string // Post authors from frontmatter
	Repo            string
	Slug            string
}

func convertTagsToActivityPub(tags []string) []Tag {
	var apTags []Tag
	for _, tag := range tags {
		apTags = append(apTags, Tag{
			Type: "Hashtag",
			Name: "#" + tag,
			Href: "", // Could link to tag page if desired
		})
	}
	return apTags
}

func isActivityPubEnabledForRepo(repo string) bool {
	// Check if ActivityPub is enabled globally
	if !viper.GetBool("activitypub.enabled") {
		return false
	}

	// Check if this specific repo has ActivityPub enabled
	repoConfig := fmt.Sprintf("repos.%s.activitypub", repo)
	if viper.IsSet(repoConfig) {
		return viper.GetBool(repoConfig)
	}

	// Default to enabled if global ActivityPub is enabled
	return true
}

func getRepoOwner(repo string) string {
	// Get the owner/primary user for this repo (fallback for when we don't have post authors)
	ownerConfig := fmt.Sprintf("repos.%s.owner", repo)
	if viper.IsSet(ownerConfig) {
		return viper.GetString(ownerConfig)
	}

	// Fallback to primary ActivityPub user
	if primaryUser := viper.GetString("activitypub.primary_user"); primaryUser != "" {
		users := viper.GetStringMap("users")
		if _, exists := users[primaryUser]; exists {
			return primaryUser
		}
	}

	// Final fallback to first user in users config
	users := viper.GetStringMap("users")
	for username := range users {
		return username
	}

	return ""
}

// getPostPrimaryAuthor gets the primary author for a post, with fallbacks
func getPostPrimaryAuthor(post *BlogPost) string {
	// First, try to use the post's actual authors
	if len(post.Authors) > 0 {
		primaryAuthor := post.Authors[0] // Use first author as primary

		// Validate the author exists in the users config
		users := viper.GetStringMap("users")
		if _, exists := users[primaryAuthor]; exists {
			return primaryAuthor
		}

		// If first author doesn't exist, try other authors
		for _, author := range post.Authors {
			if _, exists := users[author]; exists {
				slog.Warn("Primary author not found, using alternate", "primary", primaryAuthor, "using", author, "post", post.Title)
				return author
			}
		}

		slog.Warn("No valid authors found in users config", "authors", post.Authors, "post", post.Title)
	}

	// Fallback to repo owner if no valid authors found
	fallback := getRepoOwner(post.Repo)
	if fallback != "" {
		slog.Warn("Using repo owner as fallback author", "repo", post.Repo, "owner", fallback, "post", post.Title)
		return fallback
	}

	return ""
}

// buildPostAttribution creates the attribution for a post based on its authors
func buildPostAttribution(post *BlogPost, baseURL string) interface{} {
	users := viper.GetStringMap("users")
	var validAuthors []string

	// Find all valid authors (those that exist in users config)
	for _, author := range post.Authors {
		if _, exists := users[author]; exists {
			validAuthors = append(validAuthors, fmt.Sprintf("%s/@%s", baseURL, author))
		}
	}

	// If no valid authors found, fall back to repo owner
	if len(validAuthors) == 0 {
		fallbackAuthor := getRepoOwner(post.Repo)
		if fallbackAuthor != "" {
			return fmt.Sprintf("%s/@%s", baseURL, fallbackAuthor)
		}
		return fmt.Sprintf("%s/@unknown", baseURL) // Last resort
	}

	// If single author, return as string (more common case)
	if len(validAuthors) == 1 {
		return validAuthors[0]
	}

	// Multiple authors, return as array
	return validAuthors
}

// buildFollowersCC creates the CC field including all authors' followers
func buildFollowersCC(post *BlogPost, baseURL string) []string {
	users := viper.GetStringMap("users")
	var cc []string

	// Add followers of each valid author
	for _, author := range post.Authors {
		if _, exists := users[author]; exists {
			cc = append(cc, fmt.Sprintf("%s/@%s/followers", baseURL, author))
		}
	}

	// If no valid authors found, fall back to repo owner's followers
	if len(cc) == 0 {
		fallbackAuthor := getRepoOwner(post.Repo)
		if fallbackAuthor != "" {
			cc = append(cc, fmt.Sprintf("%s/@%s/followers", baseURL, fallbackAuthor))
		}
	}

	return cc
}
