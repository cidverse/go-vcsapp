package gitlabuser

import (
	"time"

	"github.com/cidverse/go-ptr"
	"github.com/cidverse/go-vcsapp/pkg/platform/api"
	"github.com/xanzy/go-gitlab"
)

// branchSliceToNameSlice converts a slice of branches to a slice of branch names
func branchSliceToNameSlice(branches []*gitlab.Branch) []string {
	var branchNames []string
	for _, branch := range branches {
		branchNames = append(branchNames, branch.Name)
	}

	return branchNames
}

func toStandardMergeRequestState(state string) api.MergeRequestState {
	if state == "opened" {
		return api.MergeRequestStateOpen
	}

	return api.MergeRequestStateClosed
}

func toStandardUser(user *gitlab.BasicUser) api.User {
	if user == nil {
		return api.User{}
	}

	state := api.UserStateActive
	var suspendedAt *time.Time
	if user.State == "blocked" {
		state = api.UserStateSuspended
		suspendedAt = ptr.Ptr(time.Now())
	}

	return api.User{
		ID:                  int64(user.ID),
		Username:            user.Username,
		Name:                user.Name,
		Type:                api.UserTypeUser,
		State:               state,
		AvatarURL:           user.AvatarURL,
		CreatedAt:           user.CreatedAt,
		SuspendedAt:         suspendedAt,
		GlobalAdministrator: false,
	}
}
