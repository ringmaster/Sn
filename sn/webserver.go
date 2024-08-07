package sn

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
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
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/go-http-utils/etag"
	"github.com/gorilla/feeds"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/spf13/viper"
	"golang.org/x/crypto/acme/autocert"
	"golang.org/x/crypto/bcrypt"
)

var router *mux.Router

type SpacesConfig struct {
	SpaceName   string
	Endpoint    string
	AccessKeyID string
	SecretKey   string
	Region      string
}

func gitHandler(w http.ResponseWriter, r *http.Request) {
	routeName := mux.CurrentRoute(r).GetName()
	routeConfigLocation := fmt.Sprintf("routes.%s", routeName)

	path := ConfigPath(fmt.Sprintf("%s.dir", routeConfigLocation))
	remote := ConfigStringDefault(fmt.Sprintf("%s.remote", routeConfigLocation), "origin")

	var sshAuth *ssh.PublicKeys
	sshPath, err := filepath.Abs(ConfigPath(fmt.Sprintf("%s.keyfile", routeConfigLocation)))
	if err != nil {
		slog.Error(err.Error())
	}
	sshAuth, err = ssh.NewPublicKeysFromFile("git", sshPath, "")
	if err != nil {
		slog.Error(err.Error())
	}

	repo, err := git.PlainOpen(path)
	if err != nil {
		slog.Error(fmt.Sprintf("Git PlainOpen (%s): %#v\n", path, err))
	}
	worktree, err := repo.Worktree()
	if err != nil {
		slog.Error(fmt.Sprintf("Git Worktree: %#v\n", err))
	}
	err = worktree.Pull(&git.PullOptions{
		RemoteName: remote,
		Auth:       sshAuth,
	})
	if err != nil {
		slog.Error(fmt.Sprintf("Git PullOptions: %#v\n", err))
	}

	ref, _ := repo.Head()
	commit, _ := repo.CommitObject(ref.Hash())
	slog.Info("commit", "commit_text", commit, "commit_hash", ref.Hash(), "commit_path", path)

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

	// Check the password
	password := r.FormValue("password")
	if nil != bcrypt.CompareHashAndPassword([]byte(uploadPasswordHash), []byte(password)) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
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

	output := fmt.Sprintf("{\"cdn\": \"%s%s\"}", spaceConfData["cdn"], header.Filename)

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
		fmt.Println(err)
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
		case bool, int, string:
			routeParameters := mux.Vars(r)
			for param, param_value := range r.URL.Query() {
				routeParameters[fmt.Sprintf("params.%s", param)] = param_value[0]
			}
			temp := v
			for k1, v1 := range routeParameters {
				switch nv := temp.(type) {
				case string:
					temp = strings.ReplaceAll(nv, fmt.Sprintf("{%s}", k1), v1)
				}
			}

			context[outVarName] = temp
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
	context["http_status"] = 200

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
		if context["http_status"] == "200" {
			context["http_status"] = 302
		}
	}
	w.WriteHeader(context["http_status"].(int))

	w.Write([]byte(rendered))
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

	rendered := `{  
		"subject": "acct:ringmaster@asymptomatic.net",
		"aliases": [
		  "https://asymptomatic.net/@ringmaster"
		],
		"links": [
		  {
			"rel": "self",
			"type": "application/activity+json",
			"href": "https://asymptomatic.net/@ringmaster"
		  },
		  {
			"rel":"http://webfinger.net/rel/profile-page",
			"type":"text/html",
			"href":"https://asymptomatic.net/"
		  }
		]
	}`

	w.Write([]byte(rendered))
}

func customFileServer(fs http.FileSystem) http.Handler {
	fileServer := http.FileServer(fs)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		filePath := path.Clean(r.URL.Path)
		file, err := fs.Open(filePath) // Do not allow path traversals.
		if os.IsNotExist(err) {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprintf(w, "404: Cannot find %#v", filePath)
			return
		}
		defer file.Close()

		// Determine the MIME type based on the file extension
		ext := filepath.Ext(filePath)
		mimeType := mime.TypeByExtension(ext)
		if mimeType != "" {
			w.Header().Set("Content-Type", mimeType)
		} else {
			// Default to a binary stream if MIME type is unknown
			w.Header().Set("Content-Type", "application/octet-stream")
		}

		fileServer.ServeHTTP(w, r)
	})
}

func setRootUrl(r *http.Request) {
	protocol := "http"
	if r.TLS != nil {
		protocol = "https"
	}
	viper.SetDefault("rooturl", fmt.Sprintf("%s://%s/", protocol, r.Host))
}

func setupRoutes(router *mux.Router) {
	router.HandleFunc("/.well-known/webfinger", fingerHandler).Name("well-known-webfinger")

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
		case "static":
			if viper.IsSet(fmt.Sprintf("%s.file", routeConfigLocation)) {
				file := ConfigPath(fmt.Sprintf("%s.file", routeConfigLocation), OptionallyExist())
				router.HandleFunc(routePath, func(rw http.ResponseWriter, r *http.Request) {
					http.ServeFile(rw, r, file)
				}).Name(routeName)
			} else {
				dir := ConfigPath(fmt.Sprintf("%s.dir", routeConfigLocation))
				//router.PathPrefix(routePath).Handler(http.StripPrefix(routePath, http.FileServer(http.Dir(dir))))
				router.PathPrefix(routePath).Handler(http.StripPrefix(routePath, customFileServer(http.Dir(dir)))).Name(routeName)
				//router.PathPrefix(routePath).Handler(spaHandler{staticPath: http.Dir(dir), indexPath: "index.html"})
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
		default:
			router.HandleFunc(routePath, catchallHandler).Name(routeName)
		}
	}
}

func LogMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		logger := slog.Default()
		setRootUrl(r)

		req := r.WithContext(context.WithValue(r.Context(), "logger", logger))

		next.ServeHTTP(w, req)

		logger.Info("web request", "request_duration", fmt.Sprintf("%dms", time.Since(start).Milliseconds()),
			"route", mux.CurrentRoute(r).GetName(), "path", r.URL.Path)
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
