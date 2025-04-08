package gitlabuser

import (
	"encoding/base64"
	"fmt"
	"time"

	"github.com/cidverse/go-ptr"
	"github.com/cidverse/go-vcsapp/pkg/platform/api"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/rs/zerolog/log"
	"gitlab.com/gitlab-org/api/client-go"
)

const pageSize = 100

type Platform struct {
	accessToken string
	author      api.GitAuthor
	client      *gitlab.Client
}

type Config struct {
	Server      string
	Username    string
	AccessToken string
	Author      api.GitAuthor
}

func (n Platform) Name() string {
	return "GitLab"
}

func (n Platform) Slug() string {
	return "gitlab"
}

func (n Platform) Repositories(opts api.RepositoryListOpts) ([]api.Repository, error) {
	var result []api.Repository

	// query repositories
	var repositories []*gitlab.Project
	repositoryOpts := &gitlab.ListProjectsOptions{
		MinAccessLevel: ptr.Ptr(gitlab.MaintainerPermissions),
		Membership:     ptr.True(),
		Archived:       ptr.False(),
		ListOptions: gitlab.ListOptions{
			PerPage: pageSize,
		},
	}
	for {
		data, resp, err := n.client.Projects.ListProjects(repositoryOpts, nil)
		if err != nil {
			return result, fmt.Errorf("failed to list repos: %w", err)
		}
		repositories = append(repositories, data...)
		if resp.NextPage == 0 {
			break
		}
		repositoryOpts.Page = resp.NextPage
	}
	log.Debug().Int("count", len(repositories)).Msg("gitlab platform - found repositories")

	for _, repo := range repositories {
		r := convertRepository(repo)

		// commit
		if opts.IncludeCommitHash {
			commit, _, err := n.client.Commits.GetCommit(repo.ID, repo.DefaultBranch, &gitlab.GetCommitOptions{})
			if err != nil {
				return result, fmt.Errorf("failed to get commit: %w", err)
			}

			r.CommitHash = commit.ID
			r.CommitDate = commit.CommittedDate
		}

		// branches
		if opts.IncludeBranches {
			branchList, _, err := n.client.Branches.ListBranches(repo.ID, &gitlab.ListBranchesOptions{})
			if err != nil {
				return result, fmt.Errorf("failed to list branches: %w", err)
			}

			r.Branches = branchSliceToNameSlice(branchList)
		}

		result = append(result, r)
	}

	return result, nil
}

func (n Platform) FindRepository(path string) (api.Repository, error) {
	repo, _, err := n.client.Projects.GetProject(path, &gitlab.GetProjectOptions{License: gitlab.Ptr(true)})
	if err != nil {
		return api.Repository{}, fmt.Errorf("failed to get repository: %w", err)
	}

	return convertRepository(repo), nil
}

func (n Platform) MergeRequests(repo api.Repository, options api.MergeRequestSearchOptions) ([]api.MergeRequest, error) {
	var result []api.MergeRequest

	searchState := "all"
	if options.IsMerged != nil && *options.IsMerged {
		searchState = "merged"
	} else if options.State != nil && *options.State == api.MergeRequestStateOpen {
		searchState = "opened"
	} else if options.State != nil && *options.State == api.MergeRequestStateClosed {
		searchState = "closed"
	}

	var mergeRequests []*gitlab.BasicMergeRequest
	opts := &gitlab.ListProjectMergeRequestsOptions{
		SourceBranch:   ptr.Ptr(options.SourceBranch),
		TargetBranch:   ptr.Ptr(options.TargetBranch),
		State:          ptr.Ptr(searchState),
		Draft:          options.IsDraft,
		AuthorID:       ptr.Int64ToInt(options.AuthorId),
		AuthorUsername: options.AuthorUsername,
		ListOptions: gitlab.ListOptions{
			PerPage: pageSize,
		},
	}
	for {
		data, resp, err := n.client.MergeRequests.ListProjectMergeRequests(repo.Id, opts)
		if err != nil {
			return result, fmt.Errorf("failed to list merge requests: %w", err)
		}
		mergeRequests = append(mergeRequests, data...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	for _, pr := range mergeRequests {
		entry := api.MergeRequest{
			Id:           int64(pr.ID),
			Title:        pr.Title,
			Description:  pr.Description,
			SourceBranch: pr.SourceBranch,
			TargetBranch: pr.TargetBranch,
			State:        toMergeRequestState(pr.State),
			IsMerged:     pr.MergedAt != nil,
			IsLocked:     pr.DiscussionLocked,
			IsDraft:      pr.Draft,
			HasConflicts: pr.HasConflicts,
			CanMerge:     pr.DetailedMergeStatus == "mergeable", // see https://docs.gitlab.com/ee/api/merge_requests.html#merge-status
			Author:       toUser(pr.Author),
		}
		entry.PipelineState = api.PipelineStateUnknown // list request does not provide pipeline status in Pipeline.Status

		result = append(result, entry)
	}

	return result, nil
}

func (n Platform) MergeRequestDiff(repo api.Repository, mergeRequest api.MergeRequest) (api.MergeRequestDiff, error) {
	result := api.MergeRequestDiff{
		ChangedFiles: []api.MergeRequestFileDiff{},
	}

	diff, _, err := n.client.MergeRequests.ListMergeRequestDiffs(repo.Id, int(mergeRequest.Id), &gitlab.ListMergeRequestDiffsOptions{
		Unidiff: ptr.True(),
	})
	if err != nil {
		return result, fmt.Errorf("failed to get diff: %w", err)
	}

	for _, d := range diff {
		result.ChangedFiles = append(result.ChangedFiles, api.MergeRequestFileDiff{
			IsNew:     d.NewFile,
			IsRenamed: d.RenamedFile,
			IsDeleted: d.DeletedFile,
			OldPath:   d.OldPath,
			NewPath:   d.NewPath,
			OldMode:   d.AMode,
			NewMode:   d.BMode,
			Diff:      d.Diff,
		})
	}

	return result, nil
}

func (n Platform) SubmitReview(repo api.Repository, mergeRequest api.MergeRequest, approved bool, message *string) error {
	if message != nil {
		_, _, err := n.client.Notes.CreateMergeRequestNote(repo.Id, int(mergeRequest.Id), &gitlab.CreateMergeRequestNoteOptions{
			Body: message,
		})
		if err != nil {
			return fmt.Errorf("failed to create note: %w", err)
		}
	}

	if approved {
		_, _, err := n.client.MergeRequestApprovals.ApproveMergeRequest(repo.Id, int(mergeRequest.Id), &gitlab.ApproveMergeRequestOptions{})
		if err != nil {
			return fmt.Errorf("failed to approve merge request: %w", err)
		}
	} else {
		_, err := n.client.MergeRequestApprovals.UnapproveMergeRequest(repo.Id, int(mergeRequest.Id))
		if err != nil {
			return fmt.Errorf("failed to unapprove merge request: %w", err)
		}
	}

	return nil
}

func (n Platform) Merge(repo api.Repository, mergeRequest api.MergeRequest, mergeStrategy api.MergeStrategyOptions) error {
	_, _, err := n.client.MergeRequests.AcceptMergeRequest(repo.Id, int(mergeRequest.Id), &gitlab.AcceptMergeRequestOptions{
		Squash:                   mergeStrategy.Squash,
		ShouldRemoveSourceBranch: mergeStrategy.RemoveSourceBranch,
	})
	if err != nil {
		return fmt.Errorf("failed to resolve merge request: %w", err)
	}

	return nil
}

func (n Platform) Languages(repo api.Repository) (map[string]int, error) {
	result := make(map[string]int)

	languages, _, err := n.client.Projects.GetProjectLanguages(repo.Id, nil)
	if err != nil {
		return result, fmt.Errorf("failed to get languages: %w", err)
	}
	for language, lines := range *languages {
		result[language] = int(lines)
	}

	return result, nil
}

func (n Platform) CommitAndPush(repo api.Repository, base string, branch string, message string, dir string) error {
	// open repo
	r, err := git.PlainOpen(dir)
	if err != nil {
		return fmt.Errorf("failed to open repository: %w", err)
	}
	w, err := r.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	// track files and create commit
	err = w.AddGlob("*")
	if err != nil {
		return fmt.Errorf("failed to add files: %w", err)
	}
	_, err = w.Commit(message, &git.CommitOptions{
		Author: &object.Signature{
			Name:  n.author.Name,
			Email: n.author.Email,
			When:  time.Now(),
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create commit: %w", err)
	}

	// push changes
	err = r.Push(&git.PushOptions{
		RemoteURL: repo.CloneURL,
		Auth:      n.AuthMethod(repo),
		Force:     true,
	})
	if err != nil {
		return fmt.Errorf("failed to push changes: %w", err)
	}

	return nil
}

func (n Platform) CreateMergeRequest(repository api.Repository, sourceBranch string, title string, description string) error {
	_, _, err := n.client.MergeRequests.CreateMergeRequest(int(repository.Id), &gitlab.CreateMergeRequestOptions{
		Title:              ptr.Ptr(title),
		Description:        ptr.Ptr(description),
		SourceBranch:       ptr.Ptr(sourceBranch),
		TargetBranch:       ptr.Ptr(repository.DefaultBranch),
		RemoveSourceBranch: ptr.True(),
		Squash:             ptr.True(),
	})
	if err != nil {
		return fmt.Errorf("failed to create merge request: %w", err)
	}

	return nil
}

func (n Platform) CreateOrUpdateMergeRequest(repository api.Repository, sourceBranch string, title string, description string, key string) error {
	description = fmt.Sprintf("%s\n\n<!--vcs-merge-request-key:%s-->", description, key)

	// Search for an existing merge request with the same source branch
	mrs, _, err := n.client.MergeRequests.ListProjectMergeRequests(int(repository.Id), &gitlab.ListProjectMergeRequestsOptions{
		SourceBranch: ptr.Ptr(sourceBranch),
		TargetBranch: ptr.Ptr(repository.DefaultBranch),
		State:        ptr.Ptr("opened"),
	})
	if err != nil {
		return fmt.Errorf("failed to list merge requests: %w", err)
	}
	var existingMR *gitlab.BasicMergeRequest
	for _, mr := range mrs {
		existingMR = mr
		break
	}

	if existingMR != nil {
		_, _, updateErr := n.client.MergeRequests.UpdateMergeRequest(int(repository.Id), existingMR.IID, &gitlab.UpdateMergeRequestOptions{
			Title:       &title,
			Description: &description,
		})
		if updateErr != nil {
			return fmt.Errorf("failed to update merge request: %w", updateErr)
		}
	} else {
		_, _, createErr := n.client.MergeRequests.CreateMergeRequest(int(repository.Id), &gitlab.CreateMergeRequestOptions{
			Title:              ptr.Ptr(title),
			Description:        ptr.Ptr(description),
			SourceBranch:       ptr.Ptr(sourceBranch),
			TargetBranch:       ptr.Ptr(repository.DefaultBranch),
			RemoveSourceBranch: ptr.True(),
			Squash:             ptr.True(),
		})
		if createErr != nil {
			return fmt.Errorf("failed to create merge request: %w", createErr)
		}
	}

	return nil
}

func (n Platform) AuthMethod(repo api.Repository) githttp.AuthMethod {
	return &githttp.BasicAuth{
		Username: "oauth2",
		Password: n.accessToken,
	}
}

func (n Platform) FileContent(repository api.Repository, branch string, path string) (string, error) {
	// query file
	file, _, err := n.client.RepositoryFiles.GetFile(int(repository.Id), path, &gitlab.GetFileOptions{
		Ref: gitlab.Ptr(branch),
	})
	if err != nil {
		return "", fmt.Errorf("failed to get file: %w", err)
	}

	if file.Encoding == "base64" {
		decoded, err := base64.StdEncoding.DecodeString(file.Content)
		if err != nil {
			return "", fmt.Errorf("failed to decode file %s with encoding %s: %w", path, file.Encoding, err)
		}

		return string(decoded), nil
	} else if file.Encoding == "text" {
		return file.Content, nil
	}

	return "", fmt.Errorf("unknown encoding %s for file %s", file.Encoding, path)
}

func (n Platform) Tags(repository api.Repository, limit int) ([]api.Tag, error) {
	var result []api.Tag

	tagList, _, err := n.client.Tags.ListTags(int(repository.Id), &gitlab.ListTagsOptions{
		ListOptions: gitlab.ListOptions{
			PerPage: limit,
		},
	})
	if err != nil {
		return result, fmt.Errorf("failed to list tags: %w", err)
	}

	for _, r := range tagList {
		result = append(result, api.Tag{
			Name:       r.Name,
			CommitHash: r.Commit.ID,
		})
	}

	return result, nil
}

func (n Platform) Releases(repository api.Repository, limit int) ([]api.Release, error) {
	var result []api.Release

	releaseList, _, err := n.client.Releases.ListReleases(int(repository.Id), &gitlab.ListReleasesOptions{
		ListOptions: gitlab.ListOptions{
			PerPage: limit,
		},
	})
	if err != nil {
		return result, fmt.Errorf("failed to list releases: %w", err)
	}
	for _, r := range releaseList {
		result = append(result, api.Release{
			Name:        r.Name,
			TagName:     r.TagName,
			Description: r.Description,
			CommitHash:  r.Commit.ID,
			CreatedAt:   r.CreatedAt,
		})
	}

	return result, nil
}

func (n Platform) CreateTag(repository api.Repository, tag string, commitHash string, message string) error {
	_, _, err := n.client.Tags.CreateTag(int(repository.Id), &gitlab.CreateTagOptions{
		TagName: gitlab.Ptr(tag),
		Ref:     gitlab.Ptr(commitHash),
		Message: gitlab.Ptr(message),
	})
	if err != nil {
		return fmt.Errorf("failed to create tag: %w", err)
	}

	return nil
}

func (n Platform) Environments(repo api.Repository) ([]api.CIEnvironment, error) {
	var result []api.CIEnvironment

	environments, _, err := n.client.Environments.ListEnvironments(repo.Id, &gitlab.ListEnvironmentsOptions{
		ListOptions: gitlab.ListOptions{
			PerPage: pageSize,
		},
	})
	if err != nil {
		return result, fmt.Errorf("failed to list environments: %w", err)
	}

	for _, v := range environments {
		result = append(result, api.CIEnvironment{
			ID:          int64(v.ID),
			Name:        v.Name,
			Description: v.Description,
			Tier:        v.Tier,
			CreatedAt:   v.CreatedAt,
			UpdatedAt:   v.UpdatedAt,
		})
	}

	return result, nil
}

func (n Platform) EnvironmentVariables(repo api.Repository, environmentName string) ([]api.CIVariable, error) {
	var result []api.CIVariable

	variables, _, err := n.client.ProjectVariables.ListVariables(repo.Id, &gitlab.ListProjectVariablesOptions{
		PerPage: pageSize,
	})
	if err != nil {
		return result, fmt.Errorf("failed to list environment variables: %w", err)
	}

	for _, v := range variables {
		if v.EnvironmentScope != environmentName {
			continue
		}

		result = append(result, api.CIVariable{
			Name:      v.Key,
			Value:     v.Value,
			IsSecret:  v.Protected || v.Masked || v.Hidden,
			CreatedAt: nil,
			UpdatedAt: nil,
		})
	}

	return result, nil
}

// NewPlatform creates a GitLab platform
func NewPlatform(config Config) (Platform, error) {
	client, err := gitlab.NewClient(config.AccessToken, gitlab.WithBaseURL(config.Server+"/api/v4"))
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create gitlab client")
	}

	return Platform{
		accessToken: config.AccessToken,
		author:      config.Author,
		client:      client,
	}, nil
}
