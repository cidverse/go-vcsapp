package api

import (
	"testing"
)

func TestGetServerIdFromCloneURL(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{"https://github.com/username/repo.git", "github-com"},
		{"https://gitlab.com/username/repo.git", "gitlab-com"},
		{"https://bitbucket.org/username/repo.git", "bitbucket-org"},
		{"https://unknown-url.com/username/repo.git", "unknown-url-com"},
		{"invalid-url", "unknown"},
		{"ftp://invalid-url.com", "invalid-url-com"},
	}

	for _, tc := range testCases {
		result := GetServerIdFromCloneURL(tc.input)
		if result != tc.expected {
			t.Errorf("For input %s, expected %s, but got %s", tc.input, tc.expected, result)
		}
	}
}

func TestSlugify(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{"GitHub.com", "github-com"},
		{"Net_2Work", "net-2work"},
		{"Some Random Text!!!", "some-random-text"},
		{"", ""},
		{"   Spaces Before and After   ", "spaces-before-and-after"},
		{"123$#@SpecialChars", "123-specialchars"},
	}

	for _, tc := range testCases {
		result := Slugify(tc.input)
		if result != tc.expected {
			t.Errorf("For input %s, expected %s, but got %s", tc.input, tc.expected, result)
		}
	}
}
