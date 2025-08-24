package sn

import (
	"bytes"
	"context"
	"crypto/tls"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"log/slog"
	"maps"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/c4milo/afero2billy"
	"github.com/go-git/go-git/plumbing/transport"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	gitHttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/go-http-utils/etag"
	"github.com/gorilla/feeds"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
	"github.com/ringmaster/Sn/sn/activitypub"
	"github.com/spf13/afero"
	"github.com/spf13/viper"
	"golang.org/x/crypto/acme/autocert"
	"golang.org/x/crypto/bcrypt"
)

//go:embed all:frontend
var frontend embed.FS

// FileSystem is an interface that includes the methods required by both embed.FS and http.FileSystem.
type FileSystem interface {
	Open(name string) (fs.File, error)
	ReadDir(name string) ([]fs.DirEntry, error)
}

var router *mux.Router

type SpacesConfig struct {
	SpaceName   string
	Endpoint    string
	AccessKeyID string
	SecretKey   string
	Region      string
}

type ctxKey struct{}

var store = sessions.NewCookieStore([]byte(`os.Getenv("SESSION_KEY")`))

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
	err = worktree.Pull(pullops)
	if err != nil {
		slog.Error(fmt.Sprintf("Git PullOptions: %#v\n", err))
	}

	ref, _ := repo.Head()
	commit, _ := repo.CommitObject(ref.Hash())
	slog.Info("commit", "commit_text", commit, "commit_hash", ref.Hash())

	w.Header().Add("Content-Type", "text/plain")
	w.Header().Add("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
	w.Header().Add("X-Frame-Options", "SAMEORIGIN")
	w.Header().Add("X-Content-Type-Options", "nosniff")
	w.Header().Add("Upgrade-Insecure-Requests", "1")
	w.Header().Add("Referrer-Policy", "strict-origin-when-cross-origin")
	w.Header().Add("Permissions-Policy", "geolocation=(self), microphone=()")

	w.Write([]byte(commit.Hash.String() + ": " + commit.Message))
}

func uploadFormHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	formHTML := `
        <!DOCTYPE html>
        <html>
        <head>
            <title>Upload File</title>
        </head>
        <body>
            <h1>Upload File</h1>
            <form method="post" enctype="multipart/form-data">
                <label for="password">Password:</label>
                <input type="password" id="password" name="password" required><br><br>
                <label for="file">File:</label>
                <input type="file" id="file" name="file" required><br><br>
                <input type="submit" value="Upload">
            </form>
        </body>
        </html>
    `
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, formHTML)
}

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		uploadFormHandler(w, r)
		return
	}

	routeName := mux.CurrentRoute(r).GetName()
	routeConfigLocation := fmt.Sprintf("routes.%s", routeName)

	uploadConfigLocation := fmt.Sprintf("%s.s3", routeConfigLocation)
	spaceConfigName := viper.GetString(uploadConfigLocation)
	spaceConfData := viper.GetStringMapString(fmt.Sprintf("s3.%s", spaceConfigName))

	uploadPasswordHash := viper.GetString(fmt.Sprintf("%s.passwordhash", routeConfigLocation))

	// Is the session authenticated?
	session, _ := store.Get(r, "session")
	if session.Values["authenticated"] != true {
		// Check the password
		password := r.FormValue("password")
		if nil != bcrypt.CompareHashAndPassword([]byte(uploadPasswordHash), []byte(password)) {
			http.Error(w, "Upload Unauthorized", http.StatusUnauthorized)
			return
		}
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Determine the content type
	contentType, err := determineContentType(file, header)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	spaceConf := SpacesConfig{
		SpaceName:   spaceConfData["spacename"],
		Endpoint:    spaceConfData["endpoint"],
		Region:      spaceConfData["region"],
		AccessKeyID: spaceConfData["accesskeyid"],
		SecretKey:   spaceConfData["secretkey"],
	}

	err = uploadToSpaces(file, header.Filename, spaceConf, contentType)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	output := fmt.Sprintf("{\"cdn\": \"%s%s\", \"s3\": \"s3://%s/%s\"}", spaceConfData["cdn"], header.Filename, spaceConfData["spacename"], header.Filename)

	w.Header().Add("Content-Type", "application/json")
	w.Write([]byte(output))
}

func uploadToSpaces(file io.ReadSeeker, filename string, spaceConf SpacesConfig, contentType string) error {
	s3Config := &aws.Config{
		Credentials:      credentials.NewStaticCredentials(spaceConf.AccessKeyID, spaceConf.SecretKey, ""), // Specifies your credentials.
		Endpoint:         aws.String(spaceConf.Endpoint),                                                   // Find your endpoint in the control panel, under Settings. Prepend "https://".
		S3ForcePathStyle: aws.Bool(false),                                                                  // // Configures to use subdomain/virtual calling format. Depending on your version, alternatively use o.UsePathStyle = false
		Region:           aws.String(spaceConf.Region),                                                     // Must be "us-east-1" when creating new Spaces. Otherwise, use the region in your endpoint, such as "nyc3".
	}

	// Step 3: The new session validates your request and directs it to your Space's specified endpoint using the AWS SDK.
	newSession, err := session.NewSession(s3Config)
	if err != nil {
		slog.Error("Could not create new S3 session", "error", err.Error())
		return err
	}
	s3Client := s3.New(newSession)

	// Step 4: Define the parameters of the object you want to upload.
	object := s3.PutObjectInput{
		Bucket:             &spaceConf.SpaceName,      // The path to the directory you want to upload the object to, starting with your Space name.
		Key:                &filename,                 // Object key, referenced whenever you want to access this file later.
		Body:               file,                      // The object's contents.
		ACL:                aws.String("public-read"), // Defines Access-control List (ACL) permissions, such as private or public.
		ContentType:        aws.String(contentType),
		ContentDisposition: aws.String("inline"),
		CacheControl:       aws.String("max-age=2592000,public"),
		Metadata: map[string]*string{ // Required. Defines metadata tags.
			"x-uploaded-by": aws.String("Sn"),
		},
	}

	// Step 5: Run the PutObject function with your parameters, catching for errors.
	_, err = s3Client.PutObject(&object)
	if err != nil {
		fmt.Println(err.Error())
		fmt.Println(s3Config)
		fmt.Println(object)
		return err
	}

	return nil
}

func determineContentType(file multipart.File, header *multipart.FileHeader) (string, error) {
	// Read a chunk to determine content type
	buf := make([]byte, 512)
	_, err := file.Read(buf)
	if err != nil {
		return "", fmt.Errorf("unable to read file to determine content type: %v", err)
	}

	// Reset the file pointer to the beginning
	_, err = file.Seek(0, io.SeekStart)
	if err != nil {
		return "", fmt.Errorf("unable to reset file pointer: %v", err)
	}

	// Detect the content type
	contentType := http.DetectContentType(buf)

	// Fallback to the extension-based content type
	if contentType == "application/octet-stream" {
		ext := filepath.Ext(header.Filename)
		mimeType := mime.TypeByExtension(ext)
		if mimeType != "" {
			contentType = mimeType
		}
	}

	return contentType, nil
}

func debugHandler(w http.ResponseWriter, r *http.Request) {
	routeName := mux.CurrentRoute(r).GetName()
	routeConfigLocation := fmt.Sprintf("routes.%s", routeName)

	output := "*** Sn DEBUG INFO ***\n"
	output += "\n"
	output += fmt.Sprintf("Config File: %s\n", viper.ConfigFileUsed())
	output += fmt.Sprintf("routeName: %s\nrouteConfigLocation: %s\n", routeName, routeConfigLocation)
	output += "\n"

	router.Walk(func(route *mux.Route, router *mux.Router, ancestors []*mux.Route) error {
		output += fmt.Sprintln("ROUTE: ", route.GetName())
		pathTemplate, err := route.GetPathTemplate()
		if err == nil {
			output += fmt.Sprintln("Path: ", pathTemplate)
		}
		pathRegexp, err := route.GetPathRegexp()
		if err == nil {
			output += fmt.Sprintln("Path regexp: ", pathRegexp)
		}
		queriesTemplates, err := route.GetQueriesTemplates()
		if err == nil {
			output += fmt.Sprintln("Queries templates: ", strings.Join(queriesTemplates, ","))
		}
		queriesRegexps, err := route.GetQueriesRegexp()
		if err == nil {
			output += fmt.Sprintln("Queries regexps: ", strings.Join(queriesRegexps, ","))
		}
		methods, err := route.GetMethods()
		if err == nil {
			output += fmt.Sprintln("Methods: ", strings.Join(methods, ","))
		}
		output += "\n"
		return nil
	})

	w.Header().Add("Content-Type", "text/plain")
	w.Write([]byte(output))
}

func rssHandler(w http.ResponseWriter, r *http.Request) {
	feed := feedHandler(r)

	w.Header().Add("Content-Type", "text/rss+xml")
	w.Header().Add("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
	w.Header().Add("X-Frame-Options", "SAMEORIGIN")
	w.Header().Add("X-Content-Type-Options", "nosniff")
	w.Header().Add("Upgrade-Insecure-Requests", "1")
	w.Header().Add("Referrer-Policy", "strict-origin-when-cross-origin")
	w.Header().Add("Permissions-Policy", "geolocation=(self), microphone=()")
	w.WriteHeader(200)

	response, _ := feed.ToRss()
	w.Write([]byte(response))
}

func atomHandler(w http.ResponseWriter, r *http.Request) {
	feed := feedHandler(r)

	w.Header().Add("Content-Type", "text/rss+xml")
	w.Header().Add("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
	w.Header().Add("X-Frame-Options", "SAMEORIGIN")
	w.Header().Add("X-Content-Type-Options", "nosniff")
	w.Header().Add("Upgrade-Insecure-Requests", "1")
	w.Header().Add("Referrer-Policy", "strict-origin-when-cross-origin")
	w.Header().Add("Permissions-Policy", "geolocation=(self), microphone=()")
	w.WriteHeader(200)

	response, _ := feed.ToAtom()
	w.Write([]byte(response))
}

func feedHandler(r *http.Request) *feeds.Feed {
	routeConfigLocation := fmt.Sprintf("routes.%s", mux.CurrentRoute(r).GetName())

	context := viper.GetStringMap(routeConfigLocation)
	context["config"] = CopyMap(viper.AllSettings())
	context["pathvars"] = mux.Vars(r)
	context["params"] = r.URL.Query()
	context["post"] = nil
	context["url"] = r.URL

	feedParams := maps.Clone(viper.GetStringMap(fmt.Sprintf("%s.feed", routeConfigLocation)))
	itemResult := ItemsFromOutvals(feedParams, context)

	now := time.Now()
	feed := &feeds.Feed{
		Title:       viper.GetString("title"),
		Link:        &feeds.Link{Href: viper.GetString("rooturl")},
		Description: viper.GetString(fmt.Sprintf("%s.feed.description", routeConfigLocation)),
		//Author:      &feeds.Author{Name: "Jason Moiron", Email: "jmoiron@jmoiron.net"},
		Created: now,
	}

	for _, item := range itemResult.Items {
		routeParameters := map[string]string{}
		routeParameters["slug"] = item.Slug
		url := viper.GetString(fmt.Sprintf("%s.feed.itemurl", routeConfigLocation))
		for k1, v1 := range routeParameters {
			url = strings.ReplaceAll(url, fmt.Sprintf("{%s}", k1), v1)
		}

		feed.Items = append(feed.Items, &feeds.Item{
			Title:       item.Title,
			Link:        &feeds.Link{Href: url},
			Description: item.Html,
			Created:     item.Date,
		})
	}

	return feed
}

func routeStringValue(r *http.Request, v string) string {
	routeParameters := mux.Vars(r)
	for param, param_value := range r.URL.Query() {
		routeParameters[fmt.Sprintf("params.%s", param)] = param_value[0]
	}
	temp := v
	for k1, v1 := range routeParameters {
		temp = strings.ReplaceAll(temp, fmt.Sprintf("{%s}", k1), v1)
	}
	return temp
}

func templateHandler(w http.ResponseWriter, r *http.Request, routeName string) {
	routeConfigLocation := fmt.Sprintf("routes.%s", routeName)

	templateConfigLocation := fmt.Sprintf("%s.templates", routeConfigLocation)
	templateFiles := GetTemplateFilesFromConfig(templateConfigLocation)

	context := viper.GetStringMap(routeConfigLocation)
	context["config"] = CopyMap(viper.AllSettings())
	context["pathvars"] = mux.Vars(r)
	context["params"] = r.URL.Query()
	context["post"] = nil
	context["url"] = r.URL

	// Find the itemquery instances, loop over, assign results to context
	for outVarName := range viper.GetStringMap(fmt.Sprintf("%s.out", routeConfigLocation)) {
		qlocation := fmt.Sprintf("%s.out.%s", routeConfigLocation, outVarName)

		outval := viper.Get(qlocation)
		switch v := outval.(type) {
		case bool:
			context[outVarName] = v
		case int:
			context[outVarName] = v
		case string:
			context[outVarName] = routeStringValue(r, v)
		default:
			outvals := maps.Clone(viper.GetStringMap(qlocation))
			itemResult := ItemsFromOutvals(outvals, context)
			context[outVarName] = itemResult
			if len(itemResult.Items) == 0 && outvals["404_on_empty"] != nil {
				templateHandler(w, r, outvals["404_on_empty"].(string))
				return
			}
		}
	}

	context["mime"] = "text/html"
	if viper.IsSet(fmt.Sprintf("%s.content-type", routeConfigLocation)) {
		context["mime"] = viper.GetString(fmt.Sprintf("%s.content-type", routeConfigLocation))
	}
	if context["http_status"] == nil {
		context["http_status"] = 200
	}

	rendered, err := RenderTemplateFiles(templateFiles, context)
	if err != nil {
		slog.Default().Error("error rendering template", "err", err)
		rendered = fmt.Sprintf("<div class=\"notification is-danger\">Error rendering template: %s</div>\n", err)
	}

	// May use context here to set additional headers, as defined by the handler
	w.Header().Add("Content-Type", context["mime"].(string))
	w.Header().Add("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
	w.Header().Add("X-Frame-Options", "SAMEORIGIN")
	w.Header().Add("X-Content-Type-Options", "nosniff")
	w.Header().Add("Upgrade-Insecure-Requests", "1")
	w.Header().Add("Referrer-Policy", "strict-origin-when-cross-origin")
	w.Header().Add("Permissions-Policy", "geolocation=(self), microphone=()")
	if viper.IsSet(fmt.Sprintf("%s.location", routeConfigLocation)) {
		context["location"] = viper.GetString(fmt.Sprintf("%s.location", routeConfigLocation))
		w.Header().Add("location", context["location"].(string))
		if context["http_status"] == 200 {
			context["http_status"] = 302
		}
	}
	// Ensure it's an int before writing header
	var statusCode int
	switch v := context["http_status"].(type) {
	case int:
		statusCode = v
	case float64: // In case it's a JSON number
		statusCode = int(v)
	case string:
		// Convert string to int
		if code, err := strconv.Atoi(v); err == nil {
			statusCode = code
		} else {
			// Default to 200 if conversion fails
			statusCode = 200
			slog.Default().Warn("Failed to convert http_status to integer", "value", v, "err", err)
		}
	default:
		// Default to 200 for any other types
		statusCode = 200
		slog.Default().Warn("Unexpected http_status type", "type", fmt.Sprintf("%T", v), "value", v)
	}

	w.WriteHeader(statusCode)

	w.Write([]byte(rendered))
}

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

func postHandler(w http.ResponseWriter, r *http.Request) {
	routeName := mux.CurrentRoute(r).GetName()
	templateHandler(w, r, routeName)
}

func catchallHandler(w http.ResponseWriter, r *http.Request) {
	routeName := mux.CurrentRoute(r).GetName()
	templateHandler(w, r, routeName)
}

func fingerHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", "application/activity+json")
	w.Header().Add("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
	w.Header().Add("X-Frame-Options", "SAMEORIGIN")
	w.Header().Add("X-Content-Type-Options", "nosniff")
	w.Header().Add("Upgrade-Insecure-Requests", "1")
	w.Header().Add("Referrer-Policy", "strict-origin-when-cross-origin")
	w.Header().Add("Permissions-Policy", "geolocation=(self), microphone=()")
	w.WriteHeader(200)

	resource := r.URL.Query().Get("resource")
	if resource == "" {
		http.Error(w, "Missing resource parameter", http.StatusBadRequest)
		return
	}

	parts := strings.SplitN(resource, ":", 2)
	if len(parts) != 2 || parts[0] != "acct" {
		http.Error(w, "Invalid resource format", http.StatusBadRequest)
		return
	}

	accountName := parts[1]
	username := strings.Split(accountName, "@")[0]

	// Validate the accountName against the users in the config and the domain the site runs on
	users := viper.GetStringMap("users")
	if _, exists := users[username]; !exists {
		http.Error(w, "Account not found", http.StatusNotFound)
		return
	}

	// extract `domain` from the request
	domain := r.Host
	if !strings.HasSuffix(accountName, "@"+domain) {
		http.Error(w, "Invalid domain", http.StatusBadRequest)
		return
	}

	schema := "http"
	if r.TLS != nil {
		schema = "https"
	}
	profileURL := fmt.Sprintf("%s://%s/@%s", schema, domain, username)
	homepageURL := fmt.Sprintf("%s://%s/", schema, domain)

	rendered := fmt.Sprintf(`{
		"subject": "acct:%s",
		"aliases": [
		  "%s"
		],
		"links": [
		  {
			"rel": "self",
			"type": "application/activity+json",
			"href": "%s"
		  },
		  {
			"rel":"http://webfinger.net/rel/profile-page",
			"type":"text/html",
			"href":"%s"
		  }
		]
	}`, accountName, profileURL, profileURL, homepageURL)

	w.Write([]byte(rendered))
}

func customFileServer(fs afero.Fs, file string) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		fileContents, err := fs.Open(file)
		if err != nil {
			http.Error(rw, "File not found", http.StatusNotFound)
			return
		}
		defer fileContents.Close()
		content, err := io.ReadAll(fileContents)
		if err != nil {
			http.Error(rw, "Error reading file", http.StatusInternalServerError)
			return
		}
		http.ServeContent(rw, r, path.Base(file), time.Now(), bytes.NewReader(content))
	})
}

func replaceBasePath(content []byte, basePath string) []byte {
	result := []byte(strings.ReplaceAll(string(content), "{{BASE_PATH}}", basePath))
	result = []byte(strings.ReplaceAll(string(result), "{{UNSPLASH}}", viper.GetString("unsplash")))
	return result
}

func customDirServer(fs afero.Fs, routeName string, prefix string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		slog.Info("serving file", "route", routeName, "path", r.URL.Path)
		upath := r.URL.Path
		if strings.HasSuffix(r.URL.Path, "/") {
			upath = path.Join(upath, "index.html")
		}

		filePath := filepath.Join(prefix, path.Clean(upath))
		file, err := fs.Open(filePath)
		var doNotFound = false
		var stat os.FileInfo
		// If the file doesn't exist, try to serve the index.html file
		if os.IsNotExist(err) {
			doNotFound = true
		} else {
			stat, err = file.Stat()
			if err != nil {
				doNotFound = true
			} else if stat.IsDir() {
				doNotFound = true
				filePath = filePath + "/"
			}
		}

		if doNotFound {
			// Try to find an index.html file in progressive directories from prefix up to filePath
			dirPath := filePath
			for {
				dirPath = filepath.Dir(dirPath)
				fmt.Printf("dirPath: %s\n", dirPath)
				if dirPath == "." || dirPath == "/" {
					break
				}
				indexFilePath := filepath.Join(dirPath, "index.html")
				indexFile, indexErr := fs.Open(indexFilePath)
				if indexErr == nil {
					defer indexFile.Close()
					indexContent, indexErr := io.ReadAll(indexFile)
					if indexErr != nil {
						http.Error(w, "Error reading index file", http.StatusInternalServerError)
						return
					}
					indexContent = replaceBasePath(indexContent, viper.GetString(fmt.Sprintf("routes.%s.path", routeName)))
					http.ServeContent(w, r, "index.html", time.Now(), bytes.NewReader(indexContent))
					return
				}
			}
			w.WriteHeader(http.StatusNotFound)
			http.Error(w, fmt.Sprintf("404.1: %s Cannot find an index.html between %#v and %#v", routeName, filePath, prefix), http.StatusNotFound)
			return
		}
		defer file.Close()

		content, err := io.ReadAll(file)
		if err != nil {
			http.Error(w, "Error reading file", http.StatusInternalServerError)
			return
		}
		content = replaceBasePath(content, viper.GetString(fmt.Sprintf("routes.%s.path", routeName)))
		reader := bytes.NewReader(content)
		http.ServeContent(w, r, stat.Name(), stat.ModTime(), reader)
	})
}

func setRootUrl(r *http.Request) {
	if !viper.IsSet("rooturl") {
		protocol := "http"
		if r.TLS != nil {
			protocol = "https"
		}
		viper.SetDefault("rooturl", fmt.Sprintf("%s://%s/", protocol, r.Host))
	}
}

func setupRoutes(router *mux.Router) {
	// Register ActivityPub routes
	if ActivityPubManager != nil && ActivityPubManager.IsEnabled() {
		ActivityPubManager.RegisterRoutes(router)
	}

	routelist := make([]string, 0, len(viper.GetStringMap("routes")))
	for key := range viper.GetStringMap("routes") {
		routelist = append(routelist, key)
	}
	sort.Strings(routelist)
	for _, routeName := range routelist {
		routeConfigLocation := fmt.Sprintf("routes.%s", routeName)
		routePath := viper.GetString(fmt.Sprintf("%s.path", routeConfigLocation))
		switch viper.GetString(fmt.Sprintf("%s.handler", routeConfigLocation)) {
		case "posts":
			router.HandleFunc(routePath, postHandler).Name(routeName)
		case "frontend":
			// This path is outside of auth middleware because it needs to supply basic API data to the frontend
			router.Path(path.Join(routePath, "api")).Methods("GET").HandlerFunc(dataHandler).Name(routeName + "_api")
			apiroutes := router.PathPrefix(path.Join(routePath, "api")).Subrouter()
			apiroutes.Use(BasicAuthMiddleware)
			reporest := apiroutes.PathPrefix("/repo").Subrouter()
			reporest.Path("/{repo:.+}/{slug:.+}").Methods("GET").HandlerFunc(repoRestGetHandler).Name(routeName + "_reporest_get")
			reporest.Path("/{repo:.+}/{slug:.+}").Methods("POST").HandlerFunc(repoRestPostHandler).Name(routeName + "_reporest_post")
			reporest.Path("/{repo:.+}/{slug:.+}").Methods("PUT").HandlerFunc(repoRestPutHandler).Name(routeName + "_reporest_put")
			reporest.Path("/{repo:.+}/{slug:.+}").Methods("DELETE").HandlerFunc(repoRestDeleteHandler).Name(routeName + "_reporest_delete")
			apiroutes.Path("/upload").Methods("POST").HandlerFunc(uploadHandler).Name(routeName + "_apiupload")
			apiroutes.Methods("POST").HandlerFunc(loginHandler).Name("000" + routeName + "_apilogin")
			apiroutes.Methods("DELETE").HandlerFunc(logoutHandler).Name("000" + routeName + "_apidelete")
			if viper.IsSet(fmt.Sprintf("%s.dir", routeConfigLocation)) {
				dir := ConfigPath(fmt.Sprintf("%s.dir", routeConfigLocation))
				router.PathPrefix(routePath).Handler(http.StripPrefix(routePath, customDirServer(Vfs, routeName, dir))).Name(routeName + "_dir_static")
			} else {
				router.PathPrefix(routePath).Handler(http.StripPrefix(routePath, customDirServer(afero.FromIOFS{FS: frontend}, routeName, "frontend"))).Name(routeName + "_dir")
			}
		case "static":
			if viper.IsSet(fmt.Sprintf("%s.file", routeConfigLocation)) {
				file := ConfigPath(fmt.Sprintf("%s.file", routeConfigLocation), OptionallyExist())
				router.Path(routePath).Handler(customFileServer(Vfs, file)).Name(routeName)
			} else {
				dir := ConfigPath(fmt.Sprintf("%s.dir", routeConfigLocation))
				router.PathPrefix(routePath).Handler(http.StripPrefix(routePath, customDirServer(Vfs, routeName, dir))).Name(routeName)
			}
		case "upload":
			router.HandleFunc(routePath, uploadHandler).Name(routeName)
		case "git":
			router.HandleFunc(routePath, gitHandler).Name(routeName)
		case "debug":
			router.HandleFunc(routePath, debugHandler).Name(routeName)
		case "feed":
			router.NewRoute().HeadersRegexp("Accept", "rss").Name(routeName).Path(routePath).HandlerFunc(rssHandler)
			router.NewRoute().HeadersRegexp("Accept", "atom").Name(routeName).Path(routePath).HandlerFunc(atomHandler)
			router.NewRoute().Name(routeName).Path(routePath).HandlerFunc(rssHandler) // I hate this
		case "redirect":
			router.HandleFunc(routePath, func(w http.ResponseWriter, r *http.Request) {
				to := viper.GetString(fmt.Sprintf("%s.to", routeConfigLocation))
				to = routeStringValue(r, to)
				http.Redirect(w, r, to, http.StatusTemporaryRedirect)
			}).Name(routeName)
		default:
			router.HandleFunc(routePath, catchallHandler).Name(routeName)
		}
	}

	// Legacy webfinger handler fallback (if ActivityPub is disabled)
	if ActivityPubManager == nil || !ActivityPubManager.IsEnabled() {
		router.HandleFunc("/.well-known/webfinger", fingerHandler).Name("well-known-webfinger")
	}
}

func LogMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		remoteIP := r.RemoteAddr
		if ip := r.Header.Get("X-Real-IP"); ip != "" {
			remoteIP = ip
		} else if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
			remoteIP = ip
		}
		logger := slog.Default()
		setRootUrl(r)

		req := r.WithContext(context.WithValue(r.Context(), ctxKey{}, logger))

		next.ServeHTTP(w, req)

		session, _ := store.Get(r, "session")
		username := ""
		if session.Values["authenticated"] == true {
			username = session.Values["username"].(string)
		}
		referrer := r.Referer()
		logger.Info("web request", "remote_ip", remoteIP, "request_duration", fmt.Sprintf("%dms", time.Since(start).Milliseconds()),
			"route", mux.CurrentRoute(r).GetName(), "path", r.URL.Path, "username", username, "referrer", referrer)
	})
}

func WebserverStart() {
	router = mux.NewRouter()
	router.Use(LogMiddleware)
	setupRoutes(router)
	http.Handle("/", etag.Handler(handlers.CompressHandler(router), false))

	if viper.IsSet("ssldomains") && viper.GetBool("use_ssl") {
		certManager := autocert.Manager{
			Prompt:     autocert.AcceptTOS,
			HostPolicy: autocert.HostWhitelist(viper.GetStringSlice("ssldomains")...), //Your domain here
			Cache:      autocert.DirCache("certs"),                                    //Folder for storing certificates
		}

		server := &http.Server{
			Addr: ":https",
			TLSConfig: &tls.Config{
				GetCertificate: certManager.GetCertificate,
			},
		}

		go http.ListenAndServe(":http", certManager.HTTPHandler(nil))
		slog.Default().Info("TLS HTTPS server started", "domains", viper.GetStringSlice("ssldomains"))
		log.Fatal(server.ListenAndServeTLS("", ""))
	} else {
		slog.Default().Info("HTTP server started", "port", viper.GetInt("port"),
			"host", fmt.Sprintf("http://localhost:%d", viper.GetInt("port")))
		log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", viper.GetInt("port")), nil))
	}
}
