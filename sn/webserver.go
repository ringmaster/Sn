package sn

import (
	"context"
	"crypto/tls"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"log/slog"
	"maps"
	"net/http"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-http-utils/etag"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
	"github.com/spf13/afero"
	"github.com/spf13/viper"
	"golang.org/x/crypto/acme/autocert"
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

func postHandler(w http.ResponseWriter, r *http.Request) {
	routeName := mux.CurrentRoute(r).GetName()
	templateHandler(w, r, routeName)
}

func catchallHandler(w http.ResponseWriter, r *http.Request) {
	routeName := mux.CurrentRoute(r).GetName()
	templateHandler(w, r, routeName)
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
