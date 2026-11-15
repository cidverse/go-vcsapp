package githubcommon

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/cidverse/go-ptr"
	"github.com/cidverse/go-vcsapp/pkg/platform/api"
	"github.com/google/go-github/v93/github"
	"github.com/rs/zerolog/log"
)

// HeadFilter returns the head filter for the pull request list endpoint, GitHub requires the format owner:branch (a bare branch name is ignored)
func HeadFilter(owner string, branch string) string {
	if branch == "" || strings.Contains(branch, ":") {
		return branch
	}
	return owner + ":" + branch
}

// CanMerge reports whether a pull request is mergeable, blocked / behind / dirty / draft / unknown states report false.
// The mergeability is only provided by the single pull request endpoint and computed asynchronously, nil is treated as "not mergeable yet".
func CanMerge(pr *github.PullRequest) bool {
	if pr.Mergeable == nil || !*pr.Mergeable {
		return false
	}

	switch pr.GetMergeableState() { // see https://docs.github.com/en/graphql/reference/enums#mergestatestatus
	case "clean", "has_hooks", "unstable":
		return true
	default:
		return false
	}
}

// ToStandardMergeRequest converts a GitHub pull request into an api.MergeRequest
func ToStandardMergeRequest(pr *github.PullRequest, repo api.Repository) api.MergeRequest {
	return api.MergeRequest{
		Id:            pr.GetID(),
		Number:        pr.GetNumber(),
		Title:         pr.GetTitle(),
		Description:   pr.GetBody(),
		Labels:        ToMergeRequestLabels(pr.Labels),
		HeadSHA:       pr.GetHead().GetSHA(),
		SourceBranch:  pr.GetHead().GetRef(),
		TargetBranch:  pr.GetBase().GetRef(),
		State:         ToStandardMergeRequestState(pr.GetState()),
		PipelineState: api.PipelineStateUnknown,
		IsMerged:      pr.GetMerged() || pr.MergedAt != nil, // list results only provide merged_at
		IsLocked:      pr.GetLocked(),
		IsDraft:       pr.GetDraft(),
		HasConflicts:  pr.GetMergeableState() == "dirty",
		CanMerge:      CanMerge(pr),
		CreatedAt:     pr.CreatedAt.GetTime(),
		UpdatedAt:     pr.UpdatedAt.GetTime(),
		Author:        ToStandardUser(pr.GetUser()),
		Repository:    repo,
	}
}

// ListMergeRequests lists all pull requests of a repository matching the search options
func ListMergeRequests(client *github.Client, repo api.Repository, options api.MergeRequestSearchOptions) ([]api.MergeRequest, error) {
	var result []api.MergeRequest

	searchState := "all"
	if options.State != nil && *options.State == api.MergeRequestStateOpen {
		searchState = "open"
	} else if options.State != nil && *options.State == api.MergeRequestStateClosed {
		searchState = "closed"
	}

	var pullRequests []*github.PullRequest
	opts := github.ListOptions{PerPage: PageSize}
	for {
		data, resp, err := client.PullRequests.List(context.Background(), repo.Namespace, repo.Name, &github.PullRequestListOptions{
			Head:        HeadFilter(repo.Namespace, options.SourceBranch),
			Base:        options.TargetBranch,
			State:       searchState,
			ListOptions: opts,
		})
		if err != nil {
			return result, fmt.Errorf("failed to list merge requests: %w", err)
		}
		pullRequests = append(pullRequests, data...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	for _, pr := range pullRequests {
		mr := ToStandardMergeRequest(pr, repo)
		if options.SourceBranch != "" && mr.SourceBranch != options.SourceBranch {
			continue
		}
		if options.IsDraft != nil && mr.IsDraft != ptr.Value(options.IsDraft) {
			continue
		}
		if options.IsMerged != nil && mr.IsMerged != ptr.Value(options.IsMerged) {
			continue
		}
		if options.AuthorId != nil && mr.Author.ID != ptr.Value(options.AuthorId) {
			continue
		}
		if options.AuthorUsername != nil && mr.Author.Username != ptr.Value(options.AuthorUsername) {
			continue
		}

		result = append(result, mr)
	}

	return result, nil
}

// GetMergeRequest fetches a single pull request by number, including its mergeability
func GetMergeRequest(client *github.Client, repo api.Repository, number int) (api.MergeRequest, error) {
	pr, resp, err := client.PullRequests.Get(context.Background(), repo.Namespace, repo.Name, number)
	if err != nil {
		if IsNotFound(resp) {
			return api.MergeRequest{}, fmt.Errorf("failed to get merge request %s#%d: %w", repo.Path, number, api.ErrMergeRequestNotFound)
		}
		return api.MergeRequest{}, fmt.Errorf("failed to get merge request %s#%d: %w", repo.Path, number, err)
	}

	return ToStandardMergeRequest(pr, repo), nil
}

// Merge merges a pull request by number
func Merge(client *github.Client, repo api.Repository, mergeRequest api.MergeRequest, mergeStrategy api.MergeStrategyOptions) error {
	_, resp, err := client.PullRequests.Merge(context.Background(), repo.Namespace, repo.Name, mergeRequest.Number, "", &github.PullRequestOptions{
		MergeMethod: ToMergeMethod(mergeStrategy),
		SHA:         ptr.ValueOrDefault(mergeStrategy.ExpectedHeadSHA, ""),
	})
	if err != nil {
		return WrapMergeError(repo, mergeRequest, resp, err)
	}

	// GitHub has no merge option to remove the source branch, delete it afterward (best effort, the merge already succeeded)
	if ptr.ValueOrDefault(mergeStrategy.RemoveSourceBranch, false) {
		deleteSourceBranch(client, repo, mergeRequest.Number)
	}

	return nil
}

// deleteSourceBranch deletes the head branch of a merged pull request, branches in forks are skipped
func deleteSourceBranch(client *github.Client, repo api.Repository, number int) {
	pr, _, err := client.PullRequests.Get(context.Background(), repo.Namespace, repo.Name, number)
	if err != nil {
		log.Warn().Err(err).Str("repo", repo.Path).Int("number", number).Msg("failed to get merge request, skipping source branch deletion")
		return
	}
	headRepo := pr.GetHead().GetRepo().GetFullName()
	branch := pr.GetHead().GetRef()
	if branch == "" || !strings.EqualFold(headRepo, pr.GetBase().GetRepo().GetFullName()) {
		log.Debug().Str("repo", repo.Path).Int("number", number).Str("head-repo", headRepo).Msg("source branch is not in the target repository, skipping source branch deletion")
		return
	}

	_, err = client.Git.DeleteRef(context.Background(), repo.Namespace, repo.Name, "heads/"+branch)
	if err != nil {
		log.Warn().Err(err).Str("repo", repo.Path).Str("branch", branch).Msg("failed to delete source branch after merge")
	}
}

// MergeRequestDiff returns all changed files of a pull request
func MergeRequestDiff(client *github.Client, repo api.Repository, mergeRequest api.MergeRequest) (api.MergeRequestDiff, error) {
	result := api.MergeRequestDiff{
		ChangedFiles: []api.MergeRequestFileDiff{},
	}

	opts := &github.ListOptions{PerPage: PageSize}
	for {
		files, resp, err := client.PullRequests.ListFiles(context.Background(), repo.Namespace, repo.Name, mergeRequest.Number, opts)
		if err != nil {
			return result, fmt.Errorf("failed to get diff: %w", err)
		}
		for _, d := range files {
			result.ChangedFiles = append(result.ChangedFiles, api.MergeRequestFileDiff{
				IsNew:     d.GetStatus() == "added",
				IsRenamed: d.GetStatus() == "renamed",
				IsDeleted: d.GetStatus() == "removed",
				OldPath:   d.GetPreviousFilename(),
				NewPath:   d.GetFilename(),
				Diff:      d.GetPatch(),
			})
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return result, nil
}

// SubmitReview approves or requests changes on a pull request
func SubmitReview(client *github.Client, repo api.Repository, mergeRequest api.MergeRequest, approved bool, message *string) error {
	event, action := "REQUEST_CHANGES", "request changes on"
	if approved {
		event, action = "APPROVE", "approve"
	}

	_, _, err := client.PullRequests.CreateReview(context.Background(), repo.Namespace, repo.Name, mergeRequest.Number, &github.PullRequestReviewRequest{
		Event: ptr.Ptr(event),
		Body:  message,
	})
	if err != nil {
		return fmt.Errorf("failed to %s merge request: %w", action, err)
	}

	return nil
}

// WrapMergeError classifies a merge error by status code, see https://docs.github.com/en/rest/pulls/pulls#merge-a-pull-request
func WrapMergeError(repo api.Repository, mergeRequest api.MergeRequest, resp *github.Response, err error) error {
	if resp != nil {
		switch resp.StatusCode {
		case http.StatusForbidden:
			return fmt.Errorf("failed to merge merge request %s#%d: %w: %w", repo.Path, mergeRequest.Number, api.ErrMergeForbidden, err)
		case http.StatusMethodNotAllowed, http.StatusConflict: // 405 not mergeable, 409 head sha mismatch
			return fmt.Errorf("failed to merge merge request %s#%d: %w: %w", repo.Path, mergeRequest.Number, api.ErrNotMergeable, err)
		}
	}

	return fmt.Errorf("failed to merge merge request %s#%d: %w", repo.Path, mergeRequest.Number, err)
}
