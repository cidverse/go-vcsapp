package vcsapp

import (
	"testing"
)

func TestRemoveIssueMentionFromMessage(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"This is a test message (#123)", "This is a test message"},
		{"Another example (#4567)", "Another example"},
		{"No issue mentioned here", "No issue mentioned here"},
		{"Different Style (AB-1234)", "Different Style"},
	}

	for _, test := range tests {
		result := RemoveIssueMentionFromMessage(test.input)
		if result != test.expected {
			t.Errorf("For input '%s', expected '%s', but got '%s'", test.input, test.expected, result)
		}
	}
}
