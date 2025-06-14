package githubcommon

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/cidverse/go-ptr"
	"github.com/cidverse/go-vcsapp/pkg/platform/api"
	"github.com/google/go-github/v72/github"
)

// BranchSliceToNameSlice converts a slice of branches to a slice of branch names
func BranchSliceToNameSlice(branches []*github.Branch) []string {
	var branchNames []string
	for _, branch := range branches {
		branchNames = append(branchNames, branch.GetName())
	}

	return branchNames
}

func ToMergeRequestLabels(labels []*github.Label) []string {
	var labelNames []string
	for _, label := range labels {
		labelNames = append(labelNames, label.GetName())
	}

	return labelNames
}

func ToStandardMergeRequestState(state string) api.MergeRequestState {
	if state == "open" {
		return api.MergeRequestStateOpen
	}

	return api.MergeRequestStateClosed
}

func ToMergeMethod(mergeStrategyOptions api.MergeStrategyOptions) string {
	if ptr.ValueOrDefault(mergeStrategyOptions.Squash, false) {
		return "squash"
	}

	return ""
}

func ToStandardUser(user *github.User) api.User {
	if user == nil {
		return api.User{}
	}

	state := api.UserStateActive
	if user.SuspendedAt != nil {
		state = api.UserStateSuspended
	}

	return api.User{
		ID:                  user.GetID(),
		Username:            user.GetLogin(),
		Name:                user.GetName(),
		Type:                api.UserType(strings.ToLower(user.GetType())),
		State:               state,
		AvatarURL:           user.GetAvatarURL(),
		CreatedAt:           user.CreatedAt.GetTime(),
		SuspendedAt:         user.SuspendedAt.GetTime(),
		GlobalAdministrator: user.GetSiteAdmin(),
	}
}

// RoundTripperToAccessToken takes a ghinstallation round-tripper and obtains a new access token
func RoundTripperToAccessToken(rt http.RoundTripper) (string, error) {
	if rt == nil {
		return "", fmt.Errorf("round tripper is nil")
	}

	if v, ok := rt.(*ghinstallation.Transport); ok {
		token, err := v.Token(context.Background())
		if err != nil {
			return "", fmt.Errorf("failed to get token: %w", err)
		}

		return token, nil
	}

	return "", fmt.Errorf("round tripper is not a ghinstallation.Transport")
}
