package sn

import (
	"fmt"
	"maps"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/feeds"
	"github.com/gorilla/mux"
	"github.com/spf13/viper"
)

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
