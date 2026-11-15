package githubcommon

import (
	"testing"

	"github.com/google/go-github/v93/github"
	"github.com/stretchr/testify/assert"
)

func TestHeadFilter(t *testing.T) {
	assert.Equal(t, "", HeadFilter("cidverse", ""))
	assert.Equal(t, "cidverse:feature", HeadFilter("cidverse", "feature"))
	assert.Equal(t, "fork:feature", HeadFilter("cidverse", "fork:feature"))
}

func TestCanMerge(t *testing.T) {
	tests := []struct {
		mergeable *bool
		state     string
		want      bool
	}{
		{nil, "unknown", false}, // list result / not computed yet
		{github.Ptr(true), "clean", true},
		{github.Ptr(true), "has_hooks", true},
		{github.Ptr(true), "unstable", true},
		{github.Ptr(true), "blocked", false},
		{github.Ptr(true), "behind", false},
		{github.Ptr(true), "draft", false},
		{github.Ptr(false), "dirty", false},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, CanMerge(&github.PullRequest{Mergeable: tt.mergeable, MergeableState: github.Ptr(tt.state)}), tt.state)
	}
}
