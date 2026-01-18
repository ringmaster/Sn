package sn

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"

	"github.com/go-git/go-git/plumbing/transport"
	"github.com/go-git/go-git/v5"
	gitHttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/spf13/viper"
	"golang.org/x/crypto/bcrypt"
)

func BasicAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session, _ := store.Get(r, "session")
		username, password, ok := r.BasicAuth()

		if ok {
			// Validate the username
			users := viper.GetStringMap("users")
			user, exists := users[username]
			if !exists {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				slog.Info("Basic Auth User Not Found", "username", username)
				return
			}

			passwordHash := user.(map[string]interface{})["passwordhash"].(string)
			if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)); err != nil {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				slog.Info("Basic Auth Password Incorrect", "username", username)
				return
			}

			// The username and password are correct, so set the session as authenticated
			session.Values["authenticated"] = true
			session.Values["username"] = username
			slog.Info("Basic Auth Logged In", "username", username)
			err := session.Save(r, w)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				slog.Error("Failed to save session", slog.String("error", err.Error()))
				return
			}
		}

		if session.Values["authenticated"] == true {
			// If the session is authenticated, serve the request
			next.ServeHTTP(w, r)
			return
		}

		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		slog.Info("Basic Auth Unauthorized", "username", username)
	})
}

func dataHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	session, _ := store.Get(r, "session")
	// Check if the user is authenticated
	if session.Values["authenticated"] != true {
		// Supply an abbreviated response to the frontend
		response := map[string]interface{}{
			"loggedIn": false,
			"username": nil,
			"repos":    []string{},
			"title":    viper.GetString("title"),
		}

		json.NewEncoder(w).Encode(response)
		//http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// User is authenticated, supply full response
	username := session.Values["username"].(string)
	repos := viper.GetStringMap("repos")
	gitCredentialsValid := true
	gitStatus := "Ok"

	if snGitRepo := os.Getenv("SN_GIT_REPO"); snGitRepo != "" {
		gitusername := os.Getenv("SN_GIT_USERNAME")
		gitpassword := os.Getenv("SN_GIT_PASSWORD")
		err := Repo.Push(&git.PushOptions{
			Auth: &gitHttp.BasicAuth{
				Username: gitusername,
				Password: gitpassword,
			},
		})
		switch err {
		case nil, git.NoErrAlreadyUpToDate:
			gitCredentialsValid = true
			gitStatus = "Commit and Push"
		case transport.ErrAuthorizationFailed:
			gitCredentialsValid = false
			gitStatus = "Authorization failed"
		default:
			gitStatus = err.Error()
			gitCredentialsValid = false
			slog.Error("Cannot push to the remote repository with current credentials", "error", err)
		}
	} else {
		gitCredentialsValid = true
		gitStatus = "Save to local"
	}

	response := map[string]interface{}{
		"loggedIn":            true,
		"username":            username,
		"repos":               repos,
		"slugPattern":         viper.GetString("slug_pattern"),
		"gitCredentialsValid": gitCredentialsValid,
		"gitStatus":           gitStatus,
	}

	json.NewEncoder(w).Encode(response)
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	session, _ := store.Get(r, "session")
	if session.Values["authenticated"] != true {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	} else {
		username := session.Values["username"].(string)
		response := map[string]interface{}{
			"loggedIn": true,
			"username": username,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}
}

func logoutHandler(w http.ResponseWriter, r *http.Request) {
	session, _ := store.Get(r, "session")
	session.Values["authenticated"] = false
	session.Values["username"] = ""
	session.Save(r, w)
	response := map[string]interface{}{
		"loggedIn": false,
		"username": nil,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
