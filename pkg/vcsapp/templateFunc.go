package vcsapp

import (
	"regexp"
)

var vcsPattern = regexp.MustCompile(`\s+\(#\d+\)$`)
var jiraPattern = regexp.MustCompile(`\s+\([A-Z]+-\d+\)$`)

func RemoveIssueMentionFromMessage(text string) string {
	text = vcsPattern.ReplaceAllString(text, "")
	text = jiraPattern.ReplaceAllString(text, "")

	return text
}
