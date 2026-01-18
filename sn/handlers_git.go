package sn

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"github.com/c4milo/afero2billy"
	"github.com/go-git/go-git/v5"
	gitHttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/gorilla/mux"
	"github.com/spf13/afero"
	"github.com/spf13/viper"
)

func gitHandler(w http.ResponseWriter, r *http.Request) {
	routeName := mux.CurrentRoute(r).GetName()
	routeConfigLocation := fmt.Sprintf("routes.%s", routeName)

	// remote := ConfigStringDefault(fmt.Sprintf("%s.remote", routeConfigLocation), "origin")
	var pullops *git.PullOptions
	var mechanism string

	gitUser := os.Getenv("SN_GIT_USERNAME")
	keyFileConfig := fmt.Sprintf("%s.keyfile", routeConfigLocation)
	if gitUser != "" {
		password := os.Getenv("SN_GIT_PASSWORD")
		pullops = &git.PullOptions{
			Auth: &gitHttp.BasicAuth{
				Username: gitUser,
				Password: password,
			},
		}
		mechanism = "basic auth"
	} else if viper.IsSet(keyFileConfig) {
		sshPath, err := filepath.Abs(ConfigPath(keyFileConfig))
		if err != nil {
			slog.Error(err.Error(), "key config", routeConfigLocation)
		}
		sshAuth, err := ssh.NewPublicKeysFromFile("git", sshPath, "")
		if err != nil {
			slog.Error(err.Error(), "key from file", sshPath)
		}
		pullops = &git.PullOptions{
			Auth: sshAuth,
		}
		mechanism = "ssh key"
	} else {
		slog.Error("Git webhook executed with no auth provided")
		return
	}

	slog.Info("Webhook route - git pull", "route", routeName, "mechanism", mechanism)

	var repo *git.Repository
	billyFs := afero2billy.New(Vfs)

	repo, err := git.Open(GitMemStorage, billyFs)
	if err != nil {
		slog.Error(fmt.Sprintf("Git Open: %#v\n", err))
	}
	worktree, err := repo.Worktree()
	if err != nil {
		slog.Error(fmt.Sprintf("Git Worktree: %#v\n", err))
	}

	// Get list of files BEFORE git pull to detect new files for ActivityPub
	existingFiles := make(map[string]bool)
	for repoName := range viper.GetStringMap("repos") {
		repoPath := ConfigPath(fmt.Sprintf("repos.%s.path", repoName))
		if exists, err := afero.DirExists(Vfs, repoPath); err == nil && exists {
			afero.Walk(Vfs, repoPath, func(path string, info os.FileInfo, _ error) error {
				if !info.IsDir() && filepath.Ext(path) == ".md" {
					existingFiles[path] = true
				}
				return nil
			})
		}
	}

	err = worktree.Pull(pullops)
	if err != nil {
		slog.Error(fmt.Sprintf("Git PullOptions: %#v\n", err))
	}

	ref, _ := repo.Head()
	commit, _ := repo.CommitObject(ref.Hash())
	slog.Info("commit", "commit_text", commit, "commit_hash", ref.Hash())

	// After git pull, reload repositories to pick up new files
	slog.Info("Webhook: reloading repositories after git pull")

	// Reload repositories
	DBLoadRepos()

	// Check for new files and trigger ActivityPub for them
	if ActivityPubManager != nil {
		for repoName := range viper.GetStringMap("repos") {
			repoPath := ConfigPath(fmt.Sprintf("repos.%s.path", repoName))
			if exists, err := afero.DirExists(Vfs, repoPath); err == nil && exists {
				afero.Walk(Vfs, repoPath, func(path string, info os.FileInfo, _ error) error {
					if !info.IsDir() && filepath.Ext(path) == ".md" {
						// If this file wasn't in our existing files list, it's new
						if !existingFiles[path] {
							slog.Info("Webhook detected new file", "file", path, "repo", repoName)
							// Load the item and publish to ActivityPub
							item, err := LoadItem(repoName, repoPath, path)
							if err == nil {
								blogPost := ConvertItemToBlogPost(item)
								if blogPost != nil {
									err := ActivityPubManager.PublishPost(blogPost)
									if err != nil {
										slog.Error("Failed to publish webhook file to ActivityPub", "error", err, "file", path)
									} else {
										slog.Info("Published webhook file to ActivityPub", "file", path, "title", item.Title)
									}
								}
							}
						}
					}
					return nil
				})
			}
		}
	}

	w.Header().Add("Content-Type", "text/plain")
	w.Header().Add("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
	w.Header().Add("X-Frame-Options", "SAMEORIGIN")
	w.Header().Add("X-Content-Type-Options", "nosniff")
	w.Header().Add("Upgrade-Insecure-Requests", "1")
	w.Header().Add("Referrer-Policy", "strict-origin-when-cross-origin")
	w.Header().Add("Permissions-Policy", "geolocation=(self), microphone=()")

	w.Write([]byte(commit.Hash.String() + ": " + commit.Message))
}
