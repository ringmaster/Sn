package sn

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	gitHttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/ringmaster/Sn/sn/activitypub"
	"github.com/spf13/afero"
)

func repoRestGetHandler(w http.ResponseWriter, r *http.Request) {
	// Implement your GET handler logic here
	w.Write([]byte("GET handler not implemented"))
}

func repoRestPostHandler(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Title   string `json:"title"`
		Slug    string `json:"slug"`
		Content string `json:"content"`
		Repo    string `json:"repo"`
		Tags    string `json:"tags"`
		Hero    string `json:"hero"`
		Date    string `json:"date"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, `{"error": "Invalid request payload"}`, http.StatusBadRequest)
		return
	}

	if payload.Title == "" || payload.Slug == "" || payload.Content == "" || payload.Repo == "" {
		http.Error(w, `{"error": "Missing required fields"}`, http.StatusBadRequest)
		return
	}

	repoPath := ConfigPath(fmt.Sprintf("repos.%s.path", payload.Repo))

	if exists, err := afero.DirExists(Vfs, repoPath); err != nil || !exists {
		http.Error(w, `{"error": "Repository not found"}`, http.StatusNotFound)
		return
	}

	if payload.Date == "" {
		payload.Date = time.Now().Format("2006-01-02 15:04:05")
	}

	session, _ := store.Get(r, "session")
	username := session.Values["username"].(string)

	var yamlTags []string
	if payload.Tags != "" {
		yamlTags = strings.Split(payload.Tags, ",")
		for i, tag := range yamlTags {
			yamlTags[i] = fmt.Sprintf("  - %s", strings.TrimSpace(tag))
		}
	}

	markdownContent := fmt.Sprintf("---\ntitle: %s\nslug: %s\ndate: %s\ntags:\n%s\nhero: %s\nauthors:\n  - %s\n---\n\n%s", payload.Title, payload.Slug, payload.Date, strings.Join(yamlTags, "\n"), payload.Hero, username, payload.Content)
	markdownFilePath := filepath.Join(repoPath, payload.Slug+".md")

	if err := afero.WriteFile(Vfs, markdownFilePath, []byte(markdownContent), 0644); err != nil {
		http.Error(w, `{"error": "Failed to write markdown file"}`, http.StatusInternalServerError)
		return
	}

	// Parse the date for ActivityPub
	publishedTime, err := time.Parse("2006-01-02 15:04:05", payload.Date)
	if err != nil {
		publishedTime = time.Now()
	}

	if snGitRepo := os.Getenv("SN_GIT_REPO"); snGitRepo != "" {
		// Retrieve username and password from environment variables
		gitusername := os.Getenv("SN_GIT_USERNAME")
		gitpassword := os.Getenv("SN_GIT_PASSWORD")

		// Get the Worktree
		worktree, err := Repo.Worktree()
		if err != nil {
			slog.Error("Failed to get worktree", slog.String("error", err.Error()))
			http.Error(w, `{"error": "Failed to get worktree"}`, http.StatusInternalServerError)
			return
		}

		// Stage the file (add it to the index)
		_, err = worktree.Add(markdownFilePath)
		if err != nil {
			slog.Error("Failed to add file to worktree", slog.String("filePath", markdownFilePath), slog.String("error", err.Error()))
			http.Error(w, `{"error": "Failed to add file to index"}`, http.StatusInternalServerError)
			return
		}

		// Commit the change
		commitHash, err := worktree.Commit("Updated file content", &git.CommitOptions{
			Author: &object.Signature{
				Name:  username,
				Email: "your-email@example.com",
				When:  time.Now(),
			},
		})
		if err != nil {
			slog.Error("Failed to commit changes", slog.String("error", err.Error()))
			http.Error(w, `{"error": "Failed to commit changes"}`, http.StatusInternalServerError)
			return
		}

		// Log the commit hash
		slog.Info("Commit successful", slog.String("commitHash", commitHash.String()))

		// Push the changes to the remote repository
		err = Repo.Push(&git.PushOptions{
			Auth: &gitHttp.BasicAuth{
				Username: gitusername,
				Password: gitpassword,
			},
		})
		if err != nil {
			slog.Error("Failed to push changes", slog.String("error", err.Error()))
			http.Error(w, `{"error": "Failed to push changes"}`, http.StatusInternalServerError)
			return
		}

		// Publish to ActivityPub after successful git operations
		if ActivityPubManager != nil && ActivityPubManager.IsEnabled() {
			// Build post URL
			scheme := "https"
			if r.TLS == nil {
				scheme = "http"
			}
			postURL := fmt.Sprintf("%s://%s/%s/%s", scheme, r.Host, payload.Repo, payload.Slug)

			// Parse tags
			var tags []string
			if payload.Tags != "" {
				tagList := strings.Split(payload.Tags, ",")
				for _, tag := range tagList {
					tags = append(tags, strings.TrimSpace(tag))
				}
			}

			// Create ActivityPub blog post with author from session
			blogPost := &activitypub.BlogPost{
				Title:           payload.Title,
				URL:             postURL,
				HTMLContent:     payload.Content, // TODO: Convert markdown to HTML
				MarkdownContent: payload.Content,
				Summary:         "", // TODO: Extract summary if needed
				PublishedAt:     publishedTime,
				Tags:            tags,
				Authors:         []string{username}, // Use session user as author
				Repo:            payload.Repo,
				Slug:            payload.Slug,
			}

			err = ActivityPubManager.PublishPost(blogPost)
			if err != nil {
				slog.Error("Failed to publish to ActivityPub", "error", err, "title", payload.Title)
				// Don't fail the entire operation, just log the error
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"message": "Markdown file created and pushed to remote repository successfully"}`))
	} else {
		// Publish to ActivityPub in local mode too
		if ActivityPubManager != nil && ActivityPubManager.IsEnabled() {
			// Build post URL
			scheme := "https"
			if r.TLS == nil {
				scheme = "http"
			}
			postURL := fmt.Sprintf("%s://%s/%s/%s", scheme, r.Host, payload.Repo, payload.Slug)

			// Parse tags
			var tags []string
			if payload.Tags != "" {
				tagList := strings.Split(payload.Tags, ",")
				for _, tag := range tagList {
					tags = append(tags, strings.TrimSpace(tag))
				}
			}

			// Create ActivityPub blog post with author from session
			blogPost := &activitypub.BlogPost{
				Title:           payload.Title,
				URL:             postURL,
				HTMLContent:     payload.Content, // TODO: Convert markdown to HTML
				MarkdownContent: payload.Content,
				Summary:         "", // TODO: Extract summary if needed
				PublishedAt:     publishedTime,
				Tags:            tags,
				Authors:         []string{username}, // Use session user as author
				Repo:            payload.Repo,
				Slug:            payload.Slug,
			}

			err = ActivityPubManager.PublishPost(blogPost)
			if err != nil {
				slog.Error("Failed to publish to ActivityPub", "error", err, "title", payload.Title)
				// Don't fail the entire operation, just log the error
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"message": "Markdown file created successfully"}`))
	}
}

func repoRestPutHandler(w http.ResponseWriter, r *http.Request) {
	// Implement your PUT handler logic here
	w.Write([]byte("PUT handler not implemented"))
}

func repoRestDeleteHandler(w http.ResponseWriter, r *http.Request) {
	// Implement your DELETE handler logic here
	w.Write([]byte("DELETE handler not implemented"))
}
