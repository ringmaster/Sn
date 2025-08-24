package activitypub

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/spf13/afero"
	"github.com/spf13/viper"
)

// Storage handles ActivityPub data persistence using dual-checkout strategy
type Storage struct {
	mainFs          afero.Fs        // Main content filesystem
	activityPubFs   afero.Fs        // ActivityPub data filesystem
	activityPubRepo *git.Repository // ActivityPub branch repository
	activityPubPath string          // Path to ActivityPub checkout
	pendingChanges  bool            // Tracks if we have uncommitted changes
	lastCommitTime  time.Time       // Last time we committed changes
	commitInterval  time.Duration   // How long to wait before committing
	gitAuth         *http.BasicAuth // Git authentication
	masterKey       []byte          // Master key for encrypting sensitive data
}

// NewStorage creates a new ActivityPub storage instance
func NewStorage(mainFs afero.Fs) (*Storage, error) {
	// Get configuration
	activityPubBranch := viper.GetString("activitypub.branch")
	if activityPubBranch == "" {
		activityPubBranch = "activitypub-data"
	}

	commitIntervalMinutes := viper.GetInt("activitypub.commit_interval_minutes")
	if commitIntervalMinutes == 0 {
		commitIntervalMinutes = 10
	}

	// Get master key for encryption
	masterKeyStr := viper.GetString("activitypub.master_key")
	if masterKeyStr == "" {
		return nil, fmt.Errorf("activitypub.master_key is required when ActivityPub is enabled - set via config or SN_ACTIVITYPUB__MASTER_KEY environment variable")
	}

	// Derive a 32-byte key from the master key string using SHA-256
	hash := sha256.Sum256([]byte(masterKeyStr))
	masterKey := hash[:]

	storage := &Storage{
		mainFs:         mainFs,
		commitInterval: time.Duration(commitIntervalMinutes) * time.Minute,
		masterKey:      masterKey,
	}

	// Set up git authentication if available
	username := os.Getenv("SN_GIT_USERNAME")
	password := os.Getenv("SN_GIT_PASSWORD")
	if username != "" && password != "" {
		storage.gitAuth = &http.BasicAuth{
			Username: username,
			Password: password,
		}
	}

	// Only set up dual checkout in git mode
	if gitRepo := os.Getenv("SN_GIT_REPO"); gitRepo != "" {
		err := storage.setupDualCheckout(gitRepo, activityPubBranch)
		if err != nil {
			return nil, fmt.Errorf("failed to setup dual checkout: %w", err)
		}
	} else {
		// In local mode, create ActivityPub directory in main filesystem
		err := storage.setupLocalMode()
		if err != nil {
			return nil, fmt.Errorf("failed to setup local mode: %w", err)
		}
	}

	// Ensure ActivityPub directories exist
	err := storage.ensureDirectories()
	if err != nil {
		return nil, fmt.Errorf("failed to create ActivityPub directories: %w", err)
	}

	// Start background commit processor
	go storage.commitProcessor()

	return storage, nil
}

// setupDualCheckout creates a separate checkout of the repository on the ActivityPub branch
func (s *Storage) setupDualCheckout(gitRepo, branch string) error {
	// Create temporary directory for ActivityPub checkout
	tempDir, err := os.MkdirTemp("", "sn-activitypub-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}

	s.activityPubPath = tempDir
	slog.Info("Created ActivityPub checkout directory", "path", tempDir)

	// Clone repository to the temporary directory
	cloneOptions := &git.CloneOptions{
		URL:           gitRepo,
		ReferenceName: plumbing.NewBranchReferenceName(branch),
		SingleBranch:  false, // We need all branches to merge from main
	}

	if s.gitAuth != nil {
		cloneOptions.Auth = s.gitAuth
		slog.Info("Using git authentication for ActivityPub checkout")
	}

	repo, err := git.PlainClone(tempDir, false, cloneOptions)
	if err != nil {
		// If branch doesn't exist, clone main and create the branch
		if err == git.ErrBranchNotFound || err.Error() == "reference not found" {
			slog.Info("ActivityPub branch doesn't exist, creating it", "branch", branch)

			// Clone main branch first
			cloneOptions.ReferenceName = plumbing.NewBranchReferenceName("main")
			repo, err = git.PlainClone(tempDir, false, cloneOptions)
			if err != nil {
				return fmt.Errorf("failed to clone main branch: %w", err)
			}

			// Create and checkout the ActivityPub branch
			err = s.createActivityPubBranch(repo, branch)
			if err != nil {
				return fmt.Errorf("failed to create ActivityPub branch: %w", err)
			}
		} else {
			return fmt.Errorf("failed to clone ActivityPub branch: %w", err)
		}
	}

	s.activityPubRepo = repo
	s.activityPubFs = afero.NewBasePathFs(afero.NewOsFs(), tempDir)

	slog.Info("ActivityPub dual checkout setup complete", "branch", branch, "path", tempDir)
	return nil
}

// createActivityPubBranch creates a new ActivityPub branch from main
func (s *Storage) createActivityPubBranch(repo *git.Repository, branchName string) error {
	worktree, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	// Get HEAD reference (should be main)
	headRef, err := repo.Head()
	if err != nil {
		return fmt.Errorf("failed to get HEAD: %w", err)
	}

	// Create new branch reference
	branchRefName := plumbing.NewBranchReferenceName(branchName)
	branchRef := plumbing.NewHashReference(branchRefName, headRef.Hash())

	// Create the reference
	err = repo.Storer.SetReference(branchRef)
	if err != nil {
		return fmt.Errorf("failed to create branch reference: %w", err)
	}

	// Checkout the new branch
	err = worktree.Checkout(&git.CheckoutOptions{
		Branch: branchRefName,
	})
	if err != nil {
		return fmt.Errorf("failed to checkout new branch: %w", err)
	}

	// Create initial .activitypub directory and commit
	apDir := filepath.Join(s.activityPubPath, ".activitypub")
	err = os.MkdirAll(apDir, 0755)
	if err != nil {
		return fmt.Errorf("failed to create .activitypub directory: %w", err)
	}

	// Create initial empty files
	initialFiles := []string{"followers.json", "following.json", "keys.json", "metadata.json"}
	for _, filename := range initialFiles {
		filePath := filepath.Join(apDir, filename)
		err = os.WriteFile(filePath, []byte("{}"), 0644)
		if err != nil {
			return fmt.Errorf("failed to create initial file %s: %w", filename, err)
		}
	}

	// Add and commit initial files
	_, err = worktree.Add(".activitypub/")
	if err != nil {
		return fmt.Errorf("failed to add .activitypub directory: %w", err)
	}

	_, err = worktree.Commit("Initialize ActivityPub data branch", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Sn ActivityPub",
			Email: "activitypub@sn.local",
			When:  time.Now(),
		},
	})
	if err != nil {
		return fmt.Errorf("failed to commit initial ActivityPub files: %w", err)
	}

	// Push the new branch
	if s.gitAuth != nil {
		err = repo.Push(&git.PushOptions{
			RemoteName: "origin",
			RefSpecs:   []config.RefSpec{config.RefSpec(branchRefName + ":" + branchRefName)},
			Auth:       s.gitAuth,
		})
		if err != nil {
			slog.Warn("Failed to push new ActivityPub branch", "error", err)
		}
	}

	return nil
}

// setupLocalMode sets up ActivityPub storage in local filesystem mode
func (s *Storage) setupLocalMode() error {
	s.activityPubFs = s.mainFs
	slog.Info("ActivityPub storage setup in local mode")
	return nil
}

// ensureDirectories creates necessary ActivityPub directories
func (s *Storage) ensureDirectories() error {
	dirs := []string{
		".activitypub",
		".activitypub/queue",
		".activitypub/comments",
		".activitypub/users",
	}

	for _, dir := range dirs {
		err := s.activityPubFs.MkdirAll(dir, 0755)
		if err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	return nil
}

// SaveFollowers saves the followers list for a specific user to storage
func (s *Storage) SaveFollowers(username string, followers map[string]*Follower) error {
	data, err := json.MarshalIndent(followers, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal followers: %w", err)
	}

	// Create user-specific followers directory
	userDir := path.Join(".activitypub/users", username)
	err = s.activityPubFs.MkdirAll(userDir, 0755)
	if err != nil {
		return fmt.Errorf("failed to create user directory: %w", err)
	}

	err = afero.WriteFile(s.activityPubFs, path.Join(userDir, "followers.json"), data, 0644)
	if err != nil {
		return fmt.Errorf("failed to write followers file: %w", err)
	}

	s.markPendingChanges()
	return nil
}

// LoadFollowers loads the followers list for a specific user from storage
func (s *Storage) LoadFollowers(username string) (map[string]*Follower, error) {
	followers := make(map[string]*Follower)

	userFollowersFile := path.Join(".activitypub/users", username, "followers.json")
	exists, err := afero.Exists(s.activityPubFs, userFollowersFile)
	if err != nil {
		return followers, fmt.Errorf("failed to check if followers file exists: %w", err)
	}

	if !exists {
		return followers, nil
	}

	data, err := afero.ReadFile(s.activityPubFs, userFollowersFile)
	if err != nil {
		return followers, fmt.Errorf("failed to read followers file: %w", err)
	}

	err = json.Unmarshal(data, &followers)
	if err != nil {
		return followers, fmt.Errorf("failed to unmarshal followers: %w", err)
	}

	return followers, nil
}

// SaveFollowing saves the following list for a specific user to storage
func (s *Storage) SaveFollowing(username string, following map[string]*Following) error {
	data, err := json.MarshalIndent(following, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal following: %w", err)
	}

	// Create user-specific following directory
	userDir := path.Join(".activitypub/users", username)
	err = s.activityPubFs.MkdirAll(userDir, 0755)
	if err != nil {
		return fmt.Errorf("failed to create user directory: %w", err)
	}

	err = afero.WriteFile(s.activityPubFs, path.Join(userDir, "following.json"), data, 0644)
	if err != nil {
		return fmt.Errorf("failed to write following file: %w", err)
	}

	s.markPendingChanges()
	return nil
}

// LoadFollowing loads the following list for a specific user from storage
func (s *Storage) LoadFollowing(username string) (map[string]*Following, error) {
	following := make(map[string]*Following)

	userFollowingFile := path.Join(".activitypub/users", username, "following.json")
	exists, err := afero.Exists(s.activityPubFs, userFollowingFile)
	if err != nil {
		return following, fmt.Errorf("failed to check if following file exists: %w", err)
	}

	if !exists {
		return following, nil
	}

	data, err := afero.ReadFile(s.activityPubFs, userFollowingFile)
	if err != nil {
		return following, fmt.Errorf("failed to read following file: %w", err)
	}

	err = json.Unmarshal(data, &following)
	if err != nil {
		return following, fmt.Errorf("failed to unmarshal following: %w", err)
	}

	return following, nil
}

// SaveKeys saves the cryptographic keys to storage (encrypted)
func (s *Storage) SaveKeys(keys *KeyPair) error {
	data, err := json.MarshalIndent(keys, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal keys: %w", err)
	}

	// Encrypt the keys data
	encryptedData, err := s.encrypt(data)
	if err != nil {
		return fmt.Errorf("failed to encrypt keys: %w", err)
	}

	err = afero.WriteFile(s.activityPubFs, ".activitypub/keys.json", encryptedData, 0600)
	if err != nil {
		return fmt.Errorf("failed to write keys file: %w", err)
	}

	s.markPendingChanges()
	return nil
}

// LoadKeys loads the cryptographic keys from storage (decrypted)
func (s *Storage) LoadKeys() (*KeyPair, error) {
	exists, err := afero.Exists(s.activityPubFs, ".activitypub/keys.json")
	if err != nil {
		return nil, fmt.Errorf("failed to check if keys file exists: %w", err)
	}

	if !exists {
		return nil, nil
	}

	encryptedData, err := afero.ReadFile(s.activityPubFs, ".activitypub/keys.json")
	if err != nil {
		return nil, fmt.Errorf("failed to read keys file: %w", err)
	}

	// Decrypt the keys data
	data, err := s.decrypt(encryptedData)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt keys: %w", err)
	}

	var keys KeyPair
	err = json.Unmarshal(data, &keys)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal keys: %w", err)
	}

	return &keys, nil
}

// SaveMetadata saves federation metadata to storage
func (s *Storage) SaveMetadata(metadata *FederationMetadata) error {
	metadata.UpdatedAt = time.Now()

	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	err = afero.WriteFile(s.activityPubFs, ".activitypub/metadata.json", data, 0644)
	if err != nil {
		return fmt.Errorf("failed to write metadata file: %w", err)
	}

	s.markPendingChanges()
	return nil
}

// LoadMetadata loads federation metadata from storage
func (s *Storage) LoadMetadata() (*FederationMetadata, error) {
	exists, err := afero.Exists(s.activityPubFs, ".activitypub/metadata.json")
	if err != nil {
		return nil, fmt.Errorf("failed to check if metadata file exists: %w", err)
	}

	if !exists {
		return nil, nil
	}

	data, err := afero.ReadFile(s.activityPubFs, ".activitypub/metadata.json")
	if err != nil {
		return nil, fmt.Errorf("failed to read metadata file: %w", err)
	}

	var metadata FederationMetadata
	err = json.Unmarshal(data, &metadata)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
	}

	return &metadata, nil
}

// SaveComment saves a comment to storage
func (s *Storage) SaveComment(comment *Comment) error {
	// Create directory for the post's comments
	commentDir := path.Join(".activitypub/comments", comment.PostRepo, comment.PostSlug)
	err := s.activityPubFs.MkdirAll(commentDir, 0755)
	if err != nil {
		return fmt.Errorf("failed to create comment directory: %w", err)
	}

	// Save comment as individual JSON file
	filename := fmt.Sprintf("%s.json", comment.ID)
	filePath := path.Join(commentDir, filename)

	data, err := json.MarshalIndent(comment, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal comment: %w", err)
	}

	err = afero.WriteFile(s.activityPubFs, filePath, data, 0644)
	if err != nil {
		return fmt.Errorf("failed to write comment file: %w", err)
	}

	s.markPendingChanges()
	return nil
}

// LoadComments loads comments for a specific post
func (s *Storage) LoadComments(repo, slug string) ([]*Comment, error) {
	var comments []*Comment

	commentDir := path.Join(".activitypub/comments", repo, slug)
	exists, err := afero.DirExists(s.activityPubFs, commentDir)
	if err != nil {
		return comments, fmt.Errorf("failed to check comment directory: %w", err)
	}

	if !exists {
		return comments, nil
	}

	files, err := afero.ReadDir(s.activityPubFs, commentDir)
	if err != nil {
		return comments, fmt.Errorf("failed to read comment directory: %w", err)
	}

	for _, file := range files {
		if filepath.Ext(file.Name()) != ".json" {
			continue
		}

		filePath := path.Join(commentDir, file.Name())
		data, err := afero.ReadFile(s.activityPubFs, filePath)
		if err != nil {
			slog.Warn("Failed to read comment file", "file", filePath, "error", err)
			continue
		}

		var comment Comment
		err = json.Unmarshal(data, &comment)
		if err != nil {
			slog.Warn("Failed to unmarshal comment", "file", filePath, "error", err)
			continue
		}

		comments = append(comments, &comment)
	}

	return comments, nil
}

// markPendingChanges marks that we have uncommitted changes
// If commit interval is 0, commits immediately
func (s *Storage) markPendingChanges() {
	s.pendingChanges = true

	// If commit interval is 0, commit immediately (useful for testing)
	if s.commitInterval == 0 {
		go func() {
			err := s.commitChanges()
			if err != nil {
				slog.Error("Failed to commit ActivityPub changes immediately", "error", err)
			}
		}()
	}
}

// commitProcessor runs in background to commit ActivityPub changes periodically
// If commit interval is 0, this processor is effectively disabled since commits happen immediately
func (s *Storage) commitProcessor() {
	// If commit interval is 0, don't run periodic commits (they happen immediately)
	if s.commitInterval == 0 {
		slog.Info("ActivityPub commit processor disabled - using immediate commits")
		return
	}

	ticker := time.NewTicker(1 * time.Minute) // Check every minute
	defer ticker.Stop()

	for range ticker.C {
		if !s.pendingChanges {
			continue
		}

		// Check if enough time has passed since last commit
		if time.Since(s.lastCommitTime) < s.commitInterval {
			continue
		}

		err := s.commitChanges()
		if err != nil {
			slog.Error("Failed to commit ActivityPub changes", "error", err)
		}
	}
}

// commitChanges commits pending ActivityPub changes
func (s *Storage) commitChanges() error {
	// Only commit in git mode
	if s.activityPubRepo == nil {
		s.pendingChanges = false
		return nil
	}

	worktree, err := s.activityPubRepo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	// Merge changes from main branch first
	err = s.mergeFromMain()
	if err != nil {
		slog.Warn("Failed to merge from main branch", "error", err)
		// Continue with commit anyway
	}

	// Add all ActivityPub changes
	_, err = worktree.Add(".activitypub/")
	if err != nil {
		return fmt.Errorf("failed to add ActivityPub changes: %w", err)
	}

	// Check if there are any changes to commit
	status, err := worktree.Status()
	if err != nil {
		return fmt.Errorf("failed to get worktree status: %w", err)
	}

	if status.IsClean() {
		s.pendingChanges = false
		return nil
	}

	// Commit changes
	commitMsg := fmt.Sprintf("Update ActivityPub data - %s", time.Now().Format("2006-01-02 15:04:05"))
	_, err = worktree.Commit(commitMsg, &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Sn ActivityPub",
			Email: "activitypub@sn.local",
			When:  time.Now(),
		},
	})
	if err != nil {
		return fmt.Errorf("failed to commit ActivityPub changes: %w", err)
	}

	// Push changes
	if s.gitAuth != nil {
		err = s.activityPubRepo.Push(&git.PushOptions{
			RemoteName: "origin",
			Auth:       s.gitAuth,
		})
		if err != nil {
			slog.Warn("Failed to push ActivityPub changes", "error", err)
		} else {
			slog.Info("Successfully committed and pushed ActivityPub changes")
		}
	}

	s.pendingChanges = false
	s.lastCommitTime = time.Now()
	return nil
}

// encrypt encrypts data using AES-GCM with the master key
func (s *Storage) encrypt(data []byte) ([]byte, error) {
	block, err := aes.NewCipher(s.masterKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, data, nil)

	// Base64 encode for storage
	encoded := make([]byte, base64.StdEncoding.EncodedLen(len(ciphertext)))
	base64.StdEncoding.Encode(encoded, ciphertext)

	return encoded, nil
}

// decrypt decrypts data using AES-GCM with the master key
func (s *Storage) decrypt(encodedData []byte) ([]byte, error) {
	// Base64 decode
	ciphertext := make([]byte, base64.StdEncoding.DecodedLen(len(encodedData)))
	n, err := base64.StdEncoding.Decode(ciphertext, encodedData)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64: %w", err)
	}
	ciphertext = ciphertext[:n]

	block, err := aes.NewCipher(s.masterKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	data, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt: %w", err)
	}

	return data, nil
}

// mergeFromMain merges content changes from main branch
func (s *Storage) mergeFromMain() error {
	// Fetch latest changes
	err := s.activityPubRepo.Fetch(&git.FetchOptions{
		RemoteName: "origin",
		Auth:       s.gitAuth,
	})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return fmt.Errorf("failed to fetch: %w", err)
	}

	// For now, we'll skip automatic merging to avoid complexity
	// In a full implementation, you might want to implement merge logic
	// or use git merge commands via exec
	slog.Info("Skipping merge from main branch - not implemented yet")
	return nil
}

// Close cleans up the storage instance
func (s *Storage) Close() error {
	// Commit any pending changes
	if s.pendingChanges {
		err := s.commitChanges()
		if err != nil {
			slog.Error("Failed to commit pending changes on close", "error", err)
		}
	}

	// Clean up temporary directory if in git mode
	if s.activityPubPath != "" {
		err := os.RemoveAll(s.activityPubPath)
		if err != nil {
			slog.Warn("Failed to clean up ActivityPub checkout directory", "path", s.activityPubPath, "error", err)
		} else {
			slog.Info("Cleaned up ActivityPub checkout directory", "path", s.activityPubPath)
		}
	}

	return nil
}
