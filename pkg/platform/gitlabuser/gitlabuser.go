package gitlabuser

import (
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/cidverse/go-vcsapp/pkg/platform/api"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/rs/zerolog/log"
	"github.com/xanzy/go-gitlab"
)

const pageSize = 100

type Platform struct {
	accessToken string
	author      api.Author
	client      *gitlab.Client
}

type Config struct {
	Server      string
	Username    string
	AccessToken string
	Author      api.Author
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
		MinAccessLevel: gitlab.AccessLevel(gitlab.MaintainerPermissions),
		Membership:     gitlab.Bool(true),
		Archived:       gitlab.Bool(false),
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
		r := api.Repository{
			PlatformId:    api.GetServerIdFromCloneURL(repo.HTTPURLToRepo),
			PlatformType:  "gitlab",
			Id:            int64(repo.ID),
			Namespace:     repo.Namespace.FullPath,
			Name:          repo.Name,
			Description:   repo.Description,
			Type:          "git",
			URL:           strings.TrimPrefix(repo.WebURL, "https://"),
			CloneURL:      repo.HTTPURLToRepo,
			DefaultBranch: repo.DefaultBranch,
			Topics:        repo.Topics,
			LicenseURL:    repo.LicenseURL,
			CreatedAt:     repo.CreatedAt,
			InternalRepo:  repo,
		}
		if repo.License != nil {
			r.LicenseName = repo.License.Name
		}

		// commit
		if opts.IncludeCommitHash {
			commit, _, err := n.client.Commits.GetCommit(repo.ID, repo.DefaultBranch)
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

func (n Platform) MergeRequests(repo api.Repository, options api.MergeRequestSearchOptions) ([]api.MergeRequest, error) {
	var result []api.MergeRequest

	var mergeRequests []*gitlab.MergeRequest
	opts := &gitlab.ListProjectMergeRequestsOptions{
		SourceBranch: gitlab.String(options.SourceBranch),
		TargetBranch: gitlab.String(options.TargetBranch),
		State:        gitlab.String(options.State),
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
		result = append(result, api.MergeRequest{
			Id:           int64(pr.ID),
			Title:        pr.Title,
			Description:  pr.Description,
			SourceBranch: pr.SourceBranch,
			TargetBranch: pr.TargetBranch,
			State:        pr.State,
		})
	}

	return result, nil
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
		// SignKey:
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
		Title:              gitlab.String(title),
		Description:        gitlab.String(description),
		SourceBranch:       gitlab.String(sourceBranch),
		TargetBranch:       gitlab.String(repository.DefaultBranch),
		RemoveSourceBranch: gitlab.Bool(true),
		Squash:             gitlab.Bool(true),
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
		SourceBranch: gitlab.String(sourceBranch),
		TargetBranch: gitlab.String(repository.DefaultBranch),
		State:        gitlab.String("opened"),
	})
	if err != nil {
		return fmt.Errorf("failed to list merge requests: %w", err)
	}
	var existingMR *gitlab.MergeRequest
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
			Title:              gitlab.String(title),
			Description:        gitlab.String(description),
			SourceBranch:       gitlab.String(sourceBranch),
			TargetBranch:       gitlab.String(repository.DefaultBranch),
			RemoveSourceBranch: gitlab.Bool(true),
			Squash:             gitlab.Bool(true),
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
		Ref: gitlab.String(branch),
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
		TagName: gitlab.String(tag),
		Ref:     gitlab.String(commitHash),
		Message: gitlab.String(message),
	})
	if err != nil {
		return fmt.Errorf("failed to create tag: %w", err)
	}

	return nil
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
