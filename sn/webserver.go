package sn

import (
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/go-http-utils/etag"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/spf13/viper"
	"golang.org/x/crypto/acme/autocert"
)

var router *mux.Router

func gitHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("Rendering git handler\n")

	routeName := mux.CurrentRoute(r).GetName()
	routeConfigLocation := fmt.Sprintf("routes.%s", routeName)

	path := ConfigPath(fmt.Sprintf("%s.dir", routeConfigLocation))
	remote := ConfigStringDefault(fmt.Sprintf("%s.remote", routeConfigLocation), "origin")

	var sshAuth *ssh.PublicKeys
	sshPath, err := filepath.Abs(ConfigPath(fmt.Sprintf("%s.keyfile", routeConfigLocation)))
	if err != nil {
		fmt.Println(err)
	}
	sshAuth, err = ssh.NewPublicKeysFromFile("git", sshPath, "")
	if err != nil {
		fmt.Println(err)
	}

	repo, err := git.PlainOpen(path)
	if err != nil {
		fmt.Printf("Git PlainOpen (%s): %#v\n", path, err)
	}
	worktree, err := repo.Worktree()
	if err != nil {
		fmt.Printf("Git Worktree: %#v\n", err)
	}
	err = worktree.Pull(&git.PullOptions{
		RemoteName: remote,
		Auth:       sshAuth,
	})
	if err != nil {
		fmt.Printf("Git PullOptions: %#v\n", err)
	}

	ref, _ := repo.Head()
	commit, _ := repo.CommitObject(ref.Hash())
	fmt.Printf("Current commit hash on %s: %s\n%s\n", path, ref.Hash(), commit)

	w.Header().Add("Content-Type", "text/plain")
	w.Header().Add("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
	w.Header().Add("X-Frame-Options", "SAMEORIGIN")
	w.Header().Add("X-Content-Type-Options", "nosniff")
	w.Header().Add("Upgrade-Insecure-Requests", "1")
	w.Header().Add("Referrer-Policy", "strict-origin-when-cross-origin")
	w.Header().Add("Permissions-Policy", "geolocation=(self), microphone=()")

	w.Write([]byte(commit.Hash.String() + ": " + commit.Message))
}

func debugHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("Rendering debug handler\n")

	setRootUrl(r)
	routeName := mux.CurrentRoute(r).GetName()
	routeConfigLocation := fmt.Sprintf("routes.%s", routeName)

	output := "*** Sn DEBUG INFO ***\n"
	output += "\n"
	output += fmt.Sprintf("Config File: %s\n", viper.ConfigFileUsed())
	output += fmt.Sprintf("routeName: %s\nrouteConfigLocation: %s\n", routeName, routeConfigLocation)
	output += "\n"

	router.Walk(func(route *mux.Route, router *mux.Router, ancestors []*mux.Route) error {
		pathTemplate, err := route.GetPathTemplate()
		if err == nil {
			output += fmt.Sprintln("ROUTE: ", pathTemplate)
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

func postHandler(w http.ResponseWriter, r *http.Request) {
	routeName := mux.CurrentRoute(r).GetName()
	fmt.Printf("Rendering posts handler for route %s\n", routeName)
	routeConfigLocation := fmt.Sprintf("routes.%s", routeName)
	fmt.Printf("  Config location: %s\n", routeConfigLocation)

	templateConfigLocation := fmt.Sprintf("%s.templates", routeConfigLocation)
	templateFiles := GetTemplateFilesFromConfig(templateConfigLocation)
	fmt.Printf("  Rendering templates from %s: %#v\n", templateConfigLocation, templateFiles)

	context := viper.GetStringMap(routeConfigLocation)
	setRootUrl(r)
	context["config"] = CopyMap(viper.AllSettings())
	context["pathvars"] = mux.Vars(r)
	context["params"] = r.URL.Query()
	context["post"] = nil

	pathvars := context["pathvars"]
	fmt.Printf("  Pathvars: %+v\n", pathvars)

	// Find the itemquery instances, loop over, assign results to context
	for outVarName := range viper.GetStringMap(fmt.Sprintf("%s.out", routeConfigLocation)) {
		fmt.Printf("  Fetching data for %s\n", outVarName)
		qlocation := fmt.Sprintf("%s.out.%s", routeConfigLocation, outVarName)
		outvals := viper.GetStringMap(qlocation)
		itemResult := ItemsFromOutvals(outvals, context)

		context[outVarName] = itemResult
	}

	context["mime"] = "text/html"
	if viper.IsSet(fmt.Sprintf("%s.content-type", routeConfigLocation)) {
		context["mime"] = viper.GetString(fmt.Sprintf("%s.content-type", routeConfigLocation))
	}

	rendered, err := RenderTemplateFiles(templateFiles, context)
	if err != nil {
		fmt.Printf("  Error rendering template: %s\n", err)
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

	w.Write([]byte(rendered))
}

func catchallHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("Rendering default handler\n")
	routeName := mux.CurrentRoute(r).GetName()
	routeConfigLocation := fmt.Sprintf("routes.%s", routeName)

	templateConfigLocation := fmt.Sprintf("%s.templates", routeConfigLocation)
	templateFiles := GetTemplateFilesFromConfig(templateConfigLocation)
	fmt.Printf("  Rendering templates from %s: %#v\n", templateConfigLocation, templateFiles)

	setRootUrl(r)
	context := CopyMap(viper.GetStringMap(routeConfigLocation))
	context["config"] = CopyMap(viper.AllSettings())
	context["pathvars"] = mux.Vars(r)
	context["params"] = r.URL.Query()
	context["post"] = nil
	output, _ := RenderTemplateFiles(templateFiles, context)

	if viper.IsSet(fmt.Sprintf("%s.status", routeConfigLocation)) {
		fmt.Printf("Setting custom status: %d\n", viper.GetInt(fmt.Sprintf("%s.status", routeConfigLocation)))
		w.WriteHeader(viper.GetInt(fmt.Sprintf("%s.status", routeConfigLocation)))
	}

	if viper.IsSet(fmt.Sprintf("%s.content-type", routeConfigLocation)) {
		fmt.Printf("Setting custom content-type: %s\n", viper.GetString(fmt.Sprintf("%s.content-type", routeConfigLocation)))
		w.Header().Add("Content-Type", viper.GetString(fmt.Sprintf("%s.content-type", routeConfigLocation)))
	} else {
		w.Header().Add("Content-Type", "text/html")
	}
	w.Header().Add("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
	w.Header().Add("X-Frame-Options", "SAMEORIGIN")
	w.Header().Add("X-Content-Type-Options", "nosniff")
	w.Header().Add("Upgrade-Insecure-Requests", "1")
	w.Header().Add("Referrer-Policy", "strict-origin-when-cross-origin")
	w.Header().Add("Permissions-Policy", "geolocation=(self), microphone=()")
	w.Write([]byte(output))
}

func customFileServer(fs http.FileSystem) http.Handler {
	fileServer := http.FileServer(fs)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := fs.Open(path.Clean(r.URL.Path)) // Do not allow path traversals.
		if os.IsNotExist(err) {
			fmt.Fprintf(w, "404: Cannot find %#v", path.Clean(r.URL.Path))

			return
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
				})
			} else {
				dir := ConfigPath(fmt.Sprintf("%s.dir", routeConfigLocation))
				fmt.Printf("    Static route at %s rooted at %s\n", routePath, dir)
				//router.PathPrefix(routePath).Handler(http.StripPrefix(routePath, http.FileServer(http.Dir(dir))))
				router.PathPrefix(routePath).Handler(http.StripPrefix(routePath, customFileServer(http.Dir(dir))))
				//router.PathPrefix(routePath).Handler(spaHandler{staticPath: http.Dir(dir), indexPath: "index.html"})
			}
		case "git":
			router.HandleFunc(routePath, gitHandler).Name(routeName)
		case "debug":
			router.HandleFunc(routePath, debugHandler).Name(routeName)
		case "redirect":
		default:
			router.PathPrefix("/").HandlerFunc(catchallHandler)
		}
	}
}

func WebserverStart() {
	router = mux.NewRouter()
	setupRoutes(router)
	http.Handle("/", etag.Handler(handlers.CompressHandler(router), false))

	if viper.IsSet("ssldomains") {
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
		fmt.Printf("Starting TLS HTTPS server on localhost, and HTTP server for LetsEncrypt.\n")
		log.Fatal(server.ListenAndServeTLS("", ""))
	} else {
		fmt.Printf("Starting HTTP server on http://localhost:%d\n", viper.GetInt("port"))
		log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", viper.GetInt("port")), nil))
	}
}
