package api

import (
	"net/url"
	"regexp"
	"strings"
)

// GetServerIdFromCloneURL returns the server id from a clone / remote url
func GetServerIdFromCloneURL(repo string) string {
	u, err := url.Parse(repo)
	if err != nil || u.Hostname() == "" {
		return "unknown"
	}

	parts := strings.Split(u.Hostname(), ".")
	if len(parts) >= 2 {
		return Slugify(parts[len(parts)-2] + "." + parts[len(parts)-1])
	}
	return Slugify(u.Hostname())
}

func Slugify(s string) string {
	regExp := regexp.MustCompile("[^a-z0-9]+")
	s = strings.ToLower(strings.TrimSpace(s))
	s = regExp.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}
