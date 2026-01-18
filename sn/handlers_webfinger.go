package sn

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/spf13/viper"
)

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
