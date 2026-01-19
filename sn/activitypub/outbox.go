package activitypub

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/ringmaster/Sn/sn/util"
	"github.com/spf13/viper"
)

// OutboxService handles ActivityPub outbox operations
type OutboxService struct {
	storage      *Storage
	keyManager   *KeyManager
	actorService *ActorService
	inboxService *InboxService
	db           *sql.DB
}

// NewOutboxService creates a new outbox service
func NewOutboxService(storage *Storage, keyManager *KeyManager, actorService *ActorService, inboxService *InboxService, db *sql.DB) *OutboxService {
	return &OutboxService{
		storage:      storage,
		keyManager:   keyManager,
		actorService: actorService,
		inboxService: inboxService,
		db:           db,
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
	// Query database for total count of posts by this author from ActivityPub-enabled repos
	var count int

	// Build SQL to count items where:
	// 1. Author matches username
	// 2. Repo has ActivityPub enabled
	repos := viper.GetStringMap("repos")
	var activityPubRepos []string
	for repoName := range repos {
		if isActivityPubEnabledForRepo(repoName) {
			activityPubRepos = append(activityPubRepos, repoName)
		}
	}

	if len(activityPubRepos) == 0 {
		return 0
	}

	// Create placeholders for IN clause
	placeholders := strings.Repeat("?,", len(activityPubRepos))
	placeholders = placeholders[:len(placeholders)-1] // Remove trailing comma

	sql := fmt.Sprintf(`
		SELECT COUNT(DISTINCT items.id)
		FROM items
		LEFT JOIN items_authors ON items.id = items_authors.item_id
		LEFT JOIN authors ON authors.id = items_authors.author_id
		WHERE authors.author = ? AND items.repo IN (%s)
	`, placeholders)

	// Prepare arguments: username + repo names
	args := make([]interface{}, len(activityPubRepos)+1)
	args[0] = username
	for i, repo := range activityPubRepos {
		args[i+1] = repo
	}

	err := os.db.QueryRow(sql, args...).Scan(&count)
	if err != nil {
		slog.Error("Failed to count published activities", "username", username, "error", err)
		return 0
	}

	slog.Info("Counted published activities", "username", username, "count", count)
	return count
}

func (os *OutboxService) getActivitiesForPage(username string, pageNum int) []interface{} {
	const itemsPerPage = 20
	offset := (pageNum - 1) * itemsPerPage

	// Query database for posts by this author from ActivityPub-enabled repos
	repos := viper.GetStringMap("repos")
	var activityPubRepos []string
	for repoName := range repos {
		if isActivityPubEnabledForRepo(repoName) {
			activityPubRepos = append(activityPubRepos, repoName)
		}
	}

	if len(activityPubRepos) == 0 {
		return []interface{}{}
	}

	// Create placeholders for IN clause
	placeholders := strings.Repeat("?,", len(activityPubRepos))
	placeholders = placeholders[:len(placeholders)-1] // Remove trailing comma

	sql := fmt.Sprintf(`
		SELECT DISTINCT items.id, items.repo, items.title, items.slug, items.publishedon, items.html, items.source
		FROM items
		LEFT JOIN items_authors ON items.id = items_authors.item_id
		LEFT JOIN authors ON authors.id = items_authors.author_id
		WHERE authors.author = ? AND items.repo IN (%s)
		ORDER BY items.publishedon DESC
		LIMIT ? OFFSET ?
	`, placeholders)

	// Prepare arguments: username + repo names + limit + offset
	args := make([]interface{}, len(activityPubRepos)+3)
	args[0] = username
	for i, repo := range activityPubRepos {
		args[i+1] = repo
	}
	args[len(activityPubRepos)+1] = itemsPerPage
	args[len(activityPubRepos)+2] = offset

	rows, err := os.db.Query(sql, args...)
	if err != nil {
		slog.Error("Failed to query published activities", "username", username, "error", err)
		return []interface{}{}
	}
	defer rows.Close()

	var activities []interface{}
	baseURL := getBaseURL()

	for rows.Next() {
		var id int64
		var repo, title, slug, publishedon, html, source string

		err := rows.Scan(&id, &repo, &title, &slug, &publishedon, &html, &source)
		if err != nil {
			slog.Error("Failed to scan activity row", "error", err)
			continue
		}

		// Get authors for this post
		authorRows, err := os.db.Query("SELECT author FROM authors INNER JOIN items_authors ON items_authors.author_id = authors.id WHERE items_authors.item_id = ?", id)
		if err != nil {
			slog.Error("Failed to query post authors", "postId", id, "error", err)
			continue
		}

		var authors []string
		for authorRows.Next() {
			var author string
			if err := authorRows.Scan(&author); err == nil {
				authors = append(authors, author)
			}
		}
		authorRows.Close()

		// Get categories/tags for this post
		tagRows, err := os.db.Query("SELECT category FROM categories INNER JOIN items_categories ON items_categories.category_id = categories.id WHERE items_categories.item_id = ?", id)
		if err != nil {
			slog.Error("Failed to query post tags", "postId", id, "error", err)
			continue
		}

		var tags []string
		for tagRows.Next() {
			var tag string
			if err := tagRows.Scan(&tag); err == nil {
				tags = append(tags, tag)
			}
		}
		tagRows.Close()

		// Parse published date
		publishedTime, err := time.Parse("2006-01-02 15:04:05", publishedon)
		if err != nil {
			slog.Warn("Failed to parse published date", "date", publishedon, "error", err)
			publishedTime = time.Now()
		}

		// Create ActivityPub Article object
		postURL := util.GetItemURL(struct {
			Slug string
			Repo string
			Date time.Time
		}{slug, repo, publishedTime})
		actorURL := fmt.Sprintf("%s/@%s", baseURL, username)

		// Build attribution for multiple authors
		var attribution interface{}
		if len(authors) == 1 {
			attribution = fmt.Sprintf("%s/@%s", baseURL, authors[0])
		} else if len(authors) > 1 {
			var authorURLs []string
			for _, author := range authors {
				authorURLs = append(authorURLs, fmt.Sprintf("%s/@%s", baseURL, author))
			}
			attribution = authorURLs
		} else {
			attribution = actorURL
		}

		// Create a temporary Item to use existing summary logic
		tempItem := struct {
			Html        string
			Frontmatter map[string]string
		}{
			Html:        html,
			Frontmatter: make(map[string]string),
		}

		// Get frontmatter for this item to extract summary
		frontmatterRows, err := os.db.Query("SELECT fieldname, value FROM frontmatter WHERE item_id = ?", id)
		if err == nil {
			for frontmatterRows.Next() {
				var fieldname, value string
				if err := frontmatterRows.Scan(&fieldname, &value); err == nil {
					tempItem.Frontmatter[fieldname] = value
				}
			}
			frontmatterRows.Close()
		}

		// Generate summary using same logic as ConvertItemToBlogPost
		summary := ""
		if summaryVal, exists := tempItem.Frontmatter["summary"]; exists {
			summary = summaryVal
		} else if descVal, exists := tempItem.Frontmatter["description"]; exists {
			summary = descVal
		} else {
			// Auto-generate summary from HTML content
			summary = util.GenerateSummaryFromHTML(tempItem.Html)
		}

		article := map[string]interface{}{
			"@context":     ActivityPubContext,
			"id":           postURL,
			"type":         "Article",
			"name":         title,
			"content":      html,
			"attributedTo": attribution,
			"published":    publishedTime.Format(time.RFC3339),
			"url":          postURL,
			"tag":          tags,
		}

		// Add summary if it exists (non-empty)
		if summary != "" {
			article["summary"] = summary
		}

		// Create Create activity wrapping the Article
		createActivity := map[string]interface{}{
			"@context":  ActivityPubContext,
			"id":        fmt.Sprintf("%s/activities/%s", baseURL, slug),
			"type":      "Create",
			"actor":     actorURL,
			"object":    article,
			"published": publishedTime.Format(time.RFC3339),
			"to":        []string{"https://www.w3.org/ns/activitystreams#Public"},
			"cc":        []string{fmt.Sprintf("%s/@%s/followers", baseURL, username)},
		}

		activities = append(activities, createActivity)
	}

	slog.Info("Retrieved published activities", "username", username, "page", pageNum, "count", len(activities))
	return activities
}

// HandleServerOutbox handles server-wide outbox collection requests for all ActivityPub-enabled repos
func (os *OutboxService) HandleServerOutbox(w http.ResponseWriter, r *http.Request) {
	slog.Info("Server outbox request received", "path", r.URL.Path, "remote_addr", r.RemoteAddr)

	// Check if ActivityPub is enabled
	if !isActivityPubEnabled() {
		http.Error(w, "ActivityPub not enabled", http.StatusNotFound)
		return
	}

	baseURL := GetBaseURL(r)
	outboxURL := baseURL + "/outbox"

	// Check for page parameter
	page := r.URL.Query().Get("page")
	if page == "" {
		// Return server outbox collection summary
		os.handleServerOutboxCollection(w, outboxURL)
		return
	}

	// Parse page number
	pageNum, err := strconv.Atoi(page)
	if err != nil || pageNum < 1 {
		http.Error(w, "Invalid page number", http.StatusBadRequest)
		return
	}

	os.handleServerOutboxPage(w, outboxURL, pageNum)
}

// handleServerOutboxCollection returns the server outbox collection summary
func (os *OutboxService) handleServerOutboxCollection(w http.ResponseWriter, outboxURL string) {
	// Get total count of published activities across all ActivityPub-enabled repos
	totalItems := os.getTotalServerActivities()

	collection := &Collection{
		Context:    ActivityPubContext,
		ID:         outboxURL,
		Type:       TypeOrderedCollection,
		TotalItems: totalItems,
		First:      outboxURL + "?page=1",
	}

	w.Header().Set("Content-Type", "application/activity+json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(collection)
	slog.Info("Server outbox collection request handled", "totalItems", totalItems)
}

// handleServerOutboxPage returns a specific page of server outbox items
func (os *OutboxService) handleServerOutboxPage(w http.ResponseWriter, outboxURL string, pageNum int) {
	// Get activities for this page across all ActivityPub-enabled repos
	activities := os.getServerActivitiesForPage(pageNum)

	pageURL := fmt.Sprintf("%s?page=%d", outboxURL, pageNum)

	page := &CollectionPage{
		Context:      ActivityPubContext,
		ID:           pageURL,
		Type:         TypeOrderedCollectionPage,
		PartOf:       outboxURL,
		OrderedItems: activities,
	}

	// Add navigation links if needed
	if pageNum > 1 {
		page.Prev = fmt.Sprintf("%s?page=%d", outboxURL, pageNum-1)
	}
	if len(activities) >= 20 { // Assuming 20 items per page
		page.Next = fmt.Sprintf("%s?page=%d", outboxURL, pageNum+1)
	}

	w.Header().Set("Content-Type", "application/activity+json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(page)
	slog.Info("Server outbox page request handled", "page", pageNum, "items", len(activities))
}

// getTotalServerActivities returns the total count of published activities across all ActivityPub-enabled repos
func (os *OutboxService) getTotalServerActivities() int {
	// TODO: Implement actual counting logic that:
	// 1. Iterates through all configured repos
	// 2. Checks if each repo has ActivityPub enabled
	// 3. Counts published articles/activities from those repos
	// For now, return 0 as placeholder
	slog.Info("Getting total server activities count", "placeholder", true)
	return 0
}

// getServerActivitiesForPage returns activities for a page across all ActivityPub-enabled repos
func (os *OutboxService) getServerActivitiesForPage(pageNum int) []interface{} {
	// TODO: Implement actual query logic that:
	// 1. Gets all repos with ActivityPub enabled
	// 2. Queries published articles from those repos
	// 3. Converts them to ActivityPub Article objects
	// 4. Returns the appropriate page of results
	// For now, return empty slice as placeholder
	slog.Info("Getting server activities for page", "page", pageNum, "placeholder", true)
	return []interface{}{}
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

// HandlePostObject handles requests for a post's ActivityPub object representation
func (os *OutboxService) HandlePostObject(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	slug := vars["slug"]

	if slug == "" {
		http.Error(w, "Missing slug", http.StatusBadRequest)
		return
	}

	baseURL := GetBaseURL(r)

	// Query the post from database
	row := os.db.QueryRow(`
		SELECT i.id, i.title, i.html, i.repo, i.publishedon
		FROM items i
		WHERE i.slug = ?
		LIMIT 1`, slug)

	var id int64
	var title, html, repo, publishedon string
	err := row.Scan(&id, &title, &html, &repo, &publishedon)
	if err != nil {
		slog.Warn("Post not found for ActivityPub", "slug", slug, "error", err)
		http.Error(w, "Post not found", http.StatusNotFound)
		return
	}

	// Check if ActivityPub is enabled for this repo
	if !isActivityPubEnabledForRepo(repo) {
		http.Error(w, "ActivityPub not enabled for this content", http.StatusNotFound)
		return
	}

	// Parse published date for URL building and later use
	publishedTime, err := time.Parse("2006-01-02 15:04:05", publishedon)
	if err != nil {
		publishedTime = time.Now()
	}

	// Build post URL using route config
	postURL := util.GetItemURL(struct {
		Slug string
		Repo string
		Date time.Time
	}{slug, repo, publishedTime})

	// Get authors for this post
	var authors []string
	authorRows, err := os.db.Query(`
		SELECT a.author FROM authors a
		INNER JOIN items_authors ia ON ia.author_id = a.id
		WHERE ia.item_id = ?`, id)
	if err == nil {
		for authorRows.Next() {
			var author string
			if err := authorRows.Scan(&author); err == nil {
				authors = append(authors, author)
			}
		}
		authorRows.Close()
	}

	// Get tags for this post
	var tags []string
	tagRows, err := os.db.Query(`
		SELECT c.category FROM categories c
		INNER JOIN items_categories ic ON ic.category_id = c.id
		WHERE ic.item_id = ?`, id)
	if err == nil {
		for tagRows.Next() {
			var tag string
			if err := tagRows.Scan(&tag); err == nil {
				tags = append(tags, tag)
			}
		}
		tagRows.Close()
	}

	// Build attribution
	var attribution interface{}
	if len(authors) == 1 {
		attribution = fmt.Sprintf("%s/@%s", baseURL, authors[0])
	} else if len(authors) > 1 {
		var authorURLs []string
		for _, author := range authors {
			authorURLs = append(authorURLs, fmt.Sprintf("%s/@%s", baseURL, author))
		}
		attribution = authorURLs
	} else {
		// Fallback to repo owner
		owner := getRepoOwner(repo)
		if owner != "" {
			attribution = fmt.Sprintf("%s/@%s", baseURL, owner)
		} else {
			attribution = baseURL
		}
	}

	// Get summary from frontmatter
	summary := ""
	frontmatterRows, err := os.db.Query("SELECT fieldname, value FROM frontmatter WHERE item_id = ?", id)
	if err == nil {
		for frontmatterRows.Next() {
			var fieldname, value string
			if err := frontmatterRows.Scan(&fieldname, &value); err == nil {
				if fieldname == "summary" || fieldname == "description" {
					summary = value
					break
				}
			}
		}
		frontmatterRows.Close()
	}

	// If no summary, auto-generate
	if summary == "" {
		summary = util.GenerateSummaryFromHTML(html)
	}

	// Build the Article object
	article := map[string]interface{}{
		"@context":     ActivityPubContext,
		"id":           postURL,
		"type":         "Article",
		"name":         title,
		"content":      html,
		"attributedTo": attribution,
		"published":    publishedTime.Format(time.RFC3339),
		"url":          postURL,
	}

	if summary != "" {
		article["summary"] = summary
	}

	if len(tags) > 0 {
		article["tag"] = convertTagsToActivityPub(tags)
	}

	// Set response headers
	w.Header().Set("Content-Type", ContentTypeActivityJSON)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(article)

	slog.Info("Served ActivityPub post object", "slug", slug, "title", title)
}
