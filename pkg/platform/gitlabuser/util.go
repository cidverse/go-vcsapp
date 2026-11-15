package gitlabuser

import (
	"fmt"
	"net/http"
	"time"

	"github.com/cidverse/go-ptr"
	"github.com/cidverse/go-vcsapp/pkg/platform/api"
	"gitlab.com/gitlab-org/api/client-go/v3"
)

// branchSliceToNameSlice converts a slice of branches to a slice of branch names
func branchSliceToNameSlice(branches []*gitlab.Branch) []string {
	var branchNames []string
	for _, branch := range branches {
		branchNames = append(branchNames, branch.Name)
	}

	return branchNames
}

func toMergeRequestLabels(labels gitlab.Labels) []string {
	var result []string

	for _, l := range labels {
		result = append(result, l)
	}

	return result
}

func toMergeRequestState(state string) api.MergeRequestState {
	if state == "opened" {
		return api.MergeRequestStateOpen
	}

	return api.MergeRequestStateClosed
}

func toPipelineState(state string) api.PipelineState {
	switch state {
	case "created":
		return api.PipelineStateCreated
	case "waiting_for_resource":
		return api.PipelineStateWaitingForResource
	case "preparing":
		return api.PipelineStatePreparing
	case "pending":
		return api.PipelineStatePending
	case "running":
		return api.PipelineStateRunning
	case "success":
		return api.PipelineStateSuccess
	case "failed":
		return api.PipelineStateFailed
	case "canceled":
		return api.PipelineStateCanceled
	case "skipped":
		return api.PipelineStateSkipped
	case "manual":
		return api.PipelineStateManual
	case "scheduled":
		return api.PipelineStateScheduled
	default:
		return api.PipelineStateUnknown
	}
}

func toUser(user *gitlab.BasicUser) api.User {
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

func toMergeRequest(pr *gitlab.BasicMergeRequest, repo api.Repository) api.MergeRequest {
	return api.MergeRequest{
		Id:           pr.ID,
		Number:       int(pr.IID),
		Title:        pr.Title,
		Description:  pr.Description,
		Labels:       toMergeRequestLabels(pr.Labels),
		HeadSHA:      pr.SHA,
		SourceBranch: pr.SourceBranch,
		TargetBranch: pr.TargetBranch,
		State:        toMergeRequestState(pr.State),
		IsMerged:     pr.MergedAt != nil,
		IsLocked:     pr.DiscussionLocked,
		IsDraft:      pr.Draft,
		HasConflicts: pr.HasConflicts,
		CanMerge:     pr.DetailedMergeStatus == "mergeable", // see https://docs.gitlab.com/ee/api/merge_requests.html#merge-status
		CreatedAt:    pr.CreatedAt,
		UpdatedAt:    pr.UpdatedAt,
		Author:       toUser(pr.Author),
		Repository:   repo,
	}
}

// wrapMergeError classifies a merge error by status code, see https://docs.gitlab.com/api/merge_requests/#merge-a-merge-request
func wrapMergeError(repo api.Repository, mergeRequest api.MergeRequest, resp *gitlab.Response, err error) error {
	if resp != nil {
		switch resp.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return fmt.Errorf("failed to merge merge request %s!%d: %w: %w", repo.Path, mergeRequest.Number, api.ErrMergeForbidden, err)
		case http.StatusMethodNotAllowed, http.StatusNotAcceptable, http.StatusConflict, http.StatusUnprocessableEntity:
			// 405 not mergeable (draft, closed, pipeline, approvals), 406 conflicts, 409 sha mismatch, 422 merge failed
			return fmt.Errorf("failed to merge merge request %s!%d: %w: %w", repo.Path, mergeRequest.Number, api.ErrNotMergeable, err)
		}
	}

	return fmt.Errorf("failed to merge merge request %s!%d: %w", repo.Path, mergeRequest.Number, err)
}
