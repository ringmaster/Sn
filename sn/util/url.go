package util

import (
	"fmt"
	"log/slog"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// GetItemURL returns the full URL for an item based on route configuration.
// Accepts any struct with Slug, Repo, Title, and Date fields (e.g., sn.Item).
// Uses reflection to avoid circular imports between sn and util packages.
func GetItemURL(item interface{}) string {
	v := reflect.ValueOf(item)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	getString := func(name string) string {
		f := v.FieldByName(name)
		if f.IsValid() && f.Kind() == reflect.String {
			return f.String()
		}
		return ""
	}

	getTime := func(name string) time.Time {
		f := v.FieldByName(name)
		if f.IsValid() {
			if t, ok := f.Interface().(time.Time); ok {
				return t
			}
		}
		return time.Time{}
	}

	slug := getString("Slug")
	repo := getString("Repo")
	title := getString("Title")
	date := getTime("Date")

	baseURL := viper.GetString("rooturl")
	if activityPubURL := viper.GetString("activitypub.rooturl"); activityPubURL != "" {
		baseURL = activityPubURL
	}
	baseURL = strings.TrimSuffix(baseURL, "/")

	// Search through routes to find one that serves this repo
	routes := viper.GetStringMap("routes")
	for routeName := range routes {
		routeConfig := fmt.Sprintf("routes.%s", routeName)
		handler := viper.GetString(fmt.Sprintf("%s.handler", routeConfig))

		// Only check routes with "posts" handler
		if handler != "posts" {
			continue
		}

		// Check if any output query uses this repo
		outConfig := viper.GetStringMap(fmt.Sprintf("%s.out", routeConfig))
		for _, outVal := range outConfig {
			outMap, ok := outVal.(map[string]interface{})
			if !ok {
				continue
			}

			outRepo, hasRepo := outMap["repo"].(string)
			if !hasRepo || outRepo != repo {
				continue
			}

			// Check if this route has a slug parameter (single item route)
			_, hasSlug := outMap["slug"]
			if !hasSlug {
				continue
			}

			// Found a matching route - build URL from its path
			routePath := viper.GetString(fmt.Sprintf("%s.path", routeConfig))

			// Replace all {param} or {param:regex} patterns with item values
			re := regexp.MustCompile(`\{([^}:]+)(:[^}]*)?\}`)
			url := re.ReplaceAllStringFunc(routePath, func(match string) string {
				paramName := re.FindStringSubmatch(match)[1]

				switch paramName {
				case "slug":
					return slug
				case "repo":
					return repo
				case "title":
					return title
				case "year":
					if !date.IsZero() {
						return date.Format("2006")
					}
				case "month":
					if !date.IsZero() {
						return date.Format("01")
					}
				case "day":
					if !date.IsZero() {
						return date.Format("02")
					}
				default:
					// Handle slug variants (pageslug, postslug, etc.)
					if strings.HasSuffix(paramName, "slug") {
						return slug
					}
				}

				slog.Warn("URL pattern has unsubstituted parameter", "param", paramName, "repo", repo)
				return match
			})

			return baseURL + url
		}
	}

	// Fallback: use a simple /posts/{slug} pattern
	slog.Warn("No route found for repo, using fallback URL pattern", "repo", repo, "slug", slug)
	return fmt.Sprintf("%s/posts/%s", baseURL, slug)
}

// GetRoutePatternForRepo returns the URL pattern for routes that serve a given repo
// Returns empty string if no matching route found
func GetRoutePatternForRepo(repo string) string {
	routes := viper.GetStringMap("routes")
	for routeName := range routes {
		routeConfig := fmt.Sprintf("routes.%s", routeName)
		handler := viper.GetString(fmt.Sprintf("%s.handler", routeConfig))

		if handler != "posts" {
			continue
		}

		outConfig := viper.GetStringMap(fmt.Sprintf("%s.out", routeConfig))
		for _, outVal := range outConfig {
			outMap, ok := outVal.(map[string]interface{})
			if !ok {
				continue
			}

			outRepo, hasRepo := outMap["repo"].(string)
			if !hasRepo || outRepo != repo {
				continue
			}

			_, hasSlug := outMap["slug"]
			if !hasSlug {
				continue
			}

			return viper.GetString(fmt.Sprintf("%s.path", routeConfig))
		}
	}
	return ""
}

// GetAllPostRoutePatterns returns all URL patterns that serve posts with slugs
// Used for registering ActivityPub content negotiation routes
func GetAllPostRoutePatterns() []string {
	var patterns []string
	routes := viper.GetStringMap("routes")

	for routeName := range routes {
		routeConfig := fmt.Sprintf("routes.%s", routeName)
		handler := viper.GetString(fmt.Sprintf("%s.handler", routeConfig))

		if handler != "posts" {
			continue
		}

		outConfig := viper.GetStringMap(fmt.Sprintf("%s.out", routeConfig))
		for _, outVal := range outConfig {
			outMap, ok := outVal.(map[string]interface{})
			if !ok {
				continue
			}

			_, hasSlug := outMap["slug"]
			if hasSlug {
				routePath := viper.GetString(fmt.Sprintf("%s.path", routeConfig))
				patterns = append(patterns, routePath)
				break // Only add each route once
			}
		}
	}

	return patterns
}
