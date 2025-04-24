package githubapp

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/cidverse/go-ptr"
	"github.com/cidverse/go-vcsapp/pkg/platform/api"
	"github.com/cidverse/go-vcsapp/pkg/platform/githubcommon"
	"github.com/go-git/go-git/v5"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/google/go-github/v71/github"
	"github.com/rs/zerolog/log"
)

const pageSize = 100

var sharedTransport = http.DefaultTransport // shared transport to reuse TCP connections.

type Platform struct {
	appId      int64
	privateKey string
	client     *github.Client
}

type Config struct {
	AppId      int64  `yaml:"appId"`
	PrivateKey string `yaml:"privateKey"`
}

func (n Platform) Name() string {
	return "GitHub"
}

func (n Platform) Slug() string {
	return "github"
}

func (n Platform) Repositories(opts api.RepositoryListOpts) ([]api.Repository, error) {
	var result []api.Repository

	// query installations
	var installations []*github.Installation
	installationOpts := &github.ListOptions{PerPage: pageSize}
	for {
		data, resp, err := n.client.Apps.ListInstallations(context.Background(), installationOpts)
		if err != nil {
			log.Fatal().Err(err).Interface("opts", installationOpts).Msg("failed to list installations")
		}
		installations = append(installations, data...)
		if resp.NextPage == 0 {
			break
		}
		installationOpts.Page = resp.NextPage
	}
	log.Info().Int("count", len(installations)).Msg("github platform - found app installations")

	for _, installation := range installations {
		itr, err := ghinstallation.New(sharedTransport, n.appId, *installation.ID, []byte(n.privateKey))
		if err != nil {
			return result, fmt.Errorf("failed to create installation transport: %w", err)
		}
		orgClient := github.NewClient(&http.Client{Transport: itr})

		// query repositories
		var repositories []*github.Repository
		repositoryOpts := &github.ListOptions{PerPage: pageSize}
		for {
			data, resp, err := orgClient.Apps.ListRepos(context.Background(), repositoryOpts)
			if err != nil {
				return result, fmt.Errorf("failed to list repos: %w", err)
			}
			repositories = append(repositories, data.Repositories...)
			if resp.NextPage == 0 {
				break
			}
			repositoryOpts.Page = resp.NextPage
		}
		log.Debug().Str("org", installation.Account.GetLogin()).Int("count", len(repositories)).Msg("github platform - found repositories in organization")

		for _, repo := range repositories {
			r := api.Repository{
				PlatformId:     api.GetServerIdFromCloneURL(repo.GetCloneURL()),
				PlatformType:   "github",
				Id:             repo.GetID(),
				Namespace:      repo.GetOwner().GetLogin(),
				Name:           repo.GetName(),
				Description:    repo.GetDescription(),
				Type:           "git",
				URL:            strings.TrimPrefix(repo.GetHTMLURL(), "https://"),
				CloneURL:       repo.GetCloneURL(),
				CloneSSH:       repo.GetSSHURL(),
				DefaultBranch:  repo.GetDefaultBranch(),
				IsFork:         repo.GetFork(),
				IsEmpty:        false,
				Topics:         repo.Topics,
				CreatedAt:      repo.CreatedAt.GetTime(),
				RoundTripper:   itr,
				InternalClient: orgClient,
				InternalRepo:   repo,
			}
			if repo.GetLicense() != nil {
				r.LicenseName = repo.GetLicense().GetName()
				r.LicenseURL = fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/LICENSE", repo.GetOwner().GetLogin(), repo.GetName(), repo.GetDefaultBranch())
			}

			// commit
			if opts.IncludeCommitHash {
				commit, _, err := orgClient.Repositories.GetCommit(context.Background(), repo.GetOwner().GetLogin(), repo.GetName(), "heads/"+repo.GetDefaultBranch(), &github.ListOptions{})
				if err != nil {
					if !strings.Contains(err.Error(), "409 Git Repository is empty") {
						return result, fmt.Errorf("failed to get commit: %w", err)
					} else {
						r.IsEmpty = true
					}
				} else {
					r.CommitHash = commit.GetSHA()
					user := commit.GetCommitter()
					if user != nil {
						r.CommitDate = user.CreatedAt.GetTime()
					}
				}
			}

			// branches
			if opts.IncludeBranches {
				branchList, _, err := orgClient.Repositories.ListBranches(context.Background(), repo.GetOwner().GetLogin(), repo.GetName(), &github.BranchListOptions{})
				if err != nil {
					if !strings.Contains(err.Error(), "409 Git Repository is empty") {
						return result, fmt.Errorf("failed to list branches: %w", err)
					} else {
						r.IsEmpty = true
					}
				} else {
					r.Branches = githubcommon.BranchSliceToNameSlice(branchList)
				}
			}

			result = append(result, r)
		}
	}

	return result, nil
}

func (n Platform) FindRepository(path string) (api.Repository, error) {
	return api.Repository{}, fmt.Errorf("not implemented")
}

func (n Platform) MergeRequests(repo api.Repository, options api.MergeRequestSearchOptions) ([]api.MergeRequest, error) {
	var result []api.MergeRequest

	searchState := "all"
	if options.State != nil && *options.State == api.MergeRequestStateOpen {
		searchState = "open"
	} else if options.State != nil && *options.State == api.MergeRequestStateClosed {
		searchState = "closed"
	}

	var pullRequests []*github.PullRequest
	opts := github.ListOptions{PerPage: pageSize}
	for {
		data, resp, err := repo.InternalClient.(*github.Client).PullRequests.List(context.Background(), repo.Namespace, repo.Name, &github.PullRequestListOptions{
			Head:        options.SourceBranch,
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
		if options.IsDraft != nil && pr.GetDraft() != ptr.Value(options.IsDraft) {
			continue
		}
		if options.IsMerged != nil && pr.GetMerged() != ptr.Value(options.IsMerged) {
			continue
		}
		if options.AuthorId != nil && pr.GetUser().GetID() != ptr.Value(options.AuthorId) {
			continue
		}
		if options.AuthorUsername != nil && pr.GetUser().GetLogin() != ptr.Value(options.AuthorUsername) {
			continue
		}

		result = append(result, api.MergeRequest{
			Id:           pr.GetID(),
			Title:        pr.GetTitle(),
			Description:  pr.GetBody(),
			SourceBranch: pr.GetHead().GetRef(),
			TargetBranch: pr.GetBase().GetRef(),
			State:        githubcommon.ToStandardMergeRequestState(pr.GetState()),
			IsMerged:     pr.GetMerged(),
			IsLocked:     pr.GetLocked(),
			IsDraft:      pr.GetDraft(),
			HasConflicts: pr.GetMergeableState() == "dirty", // see https://docs.github.com/en/graphql/reference/enums#mergestatestatus
			CanMerge:     pr.GetMergeable(),
			Author:       githubcommon.ToStandardUser(pr.GetUser()),
		})
	}

	return result, nil
}

func (n Platform) MergeRequestDiff(repo api.Repository, mergeRequest api.MergeRequest) (api.MergeRequestDiff, error) {
	result := api.MergeRequestDiff{
		ChangedFiles: []api.MergeRequestFileDiff{},
	}

	diff, _, err := repo.InternalClient.(*github.Client).PullRequests.ListFiles(context.Background(), repo.Namespace, repo.Name, int(mergeRequest.Id), &github.ListOptions{})
	if err != nil {
		return result, fmt.Errorf("failed to get diff: %w", err)
	}

	for _, d := range diff {
		result.ChangedFiles = append(result.ChangedFiles, api.MergeRequestFileDiff{
			IsNew:     d.GetStatus() == "added",
			IsRenamed: d.GetStatus() == "renamed",
			IsDeleted: d.GetStatus() == "removed",
			OldPath:   d.GetPreviousFilename(),
			NewPath:   d.GetFilename(),
			OldMode:   "",
			NewMode:   "",
			Diff:      d.GetPatch(),
		})
	}

	return result, nil
}

func (n Platform) SubmitReview(repo api.Repository, mergeRequest api.MergeRequest, approved bool, message *string) error {
	if approved {
		_, _, err := repo.InternalClient.(*github.Client).PullRequests.CreateReview(context.Background(), repo.Namespace, repo.Name, int(mergeRequest.Id), &github.PullRequestReviewRequest{
			Event: ptr.Ptr("APPROVE"),
			Body:  message,
		})
		if err != nil {
			return fmt.Errorf("failed to approve merge request: %w", err)
		}
	} else {
		_, _, err := repo.InternalClient.(*github.Client).PullRequests.CreateReview(context.Background(), repo.Namespace, repo.Name, int(mergeRequest.Id), &github.PullRequestReviewRequest{
			Event: ptr.Ptr("REQUEST_CHANGES"),
			Body:  message,
		})
		if err != nil {
			return fmt.Errorf("failed to approve merge request: %w", err)
		}
	}

	return nil
}

func (n Platform) Merge(repo api.Repository, mergeRequest api.MergeRequest, mergeStrategy api.MergeStrategyOptions) error {
	_, _, err := repo.InternalClient.(*github.Client).PullRequests.Merge(context.Background(), repo.Namespace, repo.Name, int(mergeRequest.Id), "", &github.PullRequestOptions{
		MergeMethod: githubcommon.ToMergeMethod(mergeStrategy),
	})
	if err != nil {
		return fmt.Errorf("failed to merge merge request: %w", err)
	}

	return nil
}

func (n Platform) Languages(repo api.Repository) (map[string]int, error) {
	data, _, err := repo.InternalClient.(*github.Client).Repositories.ListLanguages(context.Background(), repo.Namespace, repo.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to list languages: %w", err)
	}

	return data, err
}

func (n Platform) AuthMethod(repo api.Repository) githttp.AuthMethod {
	token, err := githubcommon.RoundTripperToAccessToken(repo.RoundTripper)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to get access token")
	}

	return &githttp.BasicAuth{
		Username: strconv.FormatInt(n.appId, 10),
		Password: token,
	}
}

func (n Platform) CommitAndPush(repo api.Repository, base string, branch string, message string, dir string) error {
	client := repo.InternalClient.(*github.Client)

	// prepare tree
	var entries []*github.TreeEntry

	// get all changed files in directory
	r, err := git.PlainOpen(dir)
	if err != nil {
		return fmt.Errorf("failed to open repository: %w", err)
	}
	w, err := r.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	status, err := w.Status()
	if err != nil {
		return fmt.Errorf("failed to get status: %w", err)
	}
	for file := range status {
		filePath := filepath.Join(dir, file)

		// deleted file
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			entries = append(entries, &github.TreeEntry{
				Path: ptr.Ptr(file),
				Type: ptr.Ptr("blob"),
				Mode: ptr.Ptr("100644"),
				SHA:  nil,
			})
			continue
		}

		// read file content
		content, readErr := os.ReadFile(filePath)
		if readErr != nil {
			return fmt.Errorf("failed to read file: %w", readErr)
		}
		contentStr := api.UnifyLineEndings(string(content))

		// get permissions
		fileStats, statsErr := os.Stat(filePath)
		if statsErr != nil {
			return fmt.Errorf("failed to get file stats: %w", statsErr)
		}
		mode := "100644"
		if fileStats.Mode()&os.FileMode(0111) != 0 {
			mode = "100744" // executable files
		}
		entries = append(entries, &github.TreeEntry{
			Path:    ptr.Ptr(file),
			Type:    ptr.Ptr("blob"),
			Content: ptr.Ptr(contentStr),
			Mode:    ptr.Ptr(mode),
		})
	}

	// create tree
	tree, _, err := client.Git.CreateTree(context.Background(), repo.Namespace, repo.Name, base, entries)
	if err != nil {
		return fmt.Errorf("failed to create tree: %w", err)
	}

	// commit tree
	commit, _, err := client.Git.CreateCommit(context.Background(), repo.Namespace, repo.Name, &github.Commit{
		Message: github.String(message),
		Tree:    tree,
		Parents: []*github.Commit{{SHA: github.String(base)}},
	}, &github.CreateCommitOptions{})
	if err != nil {
		return fmt.Errorf("failed to create commit: %w", err)
	}

	// create or update remote reference
	_, _, getRefErr := client.Git.GetRef(context.Background(), repo.Namespace, repo.Name, "refs/heads/"+branch)
	if getRefErr != nil {
		_, _, createRefErr := client.Git.CreateRef(context.Background(), repo.Namespace, repo.Name, &github.Reference{
			Ref:    github.String("refs/heads/" + branch),
			Object: &github.GitObject{SHA: commit.SHA},
		})
		if createRefErr != nil {
			return fmt.Errorf("failed to create remote branch reference: %w", createRefErr)
		}
	} else {
		_, _, refErr := client.Git.UpdateRef(context.Background(), repo.Namespace, repo.Name, &github.Reference{
			Ref:    github.String("refs/heads/" + branch),
			Object: &github.GitObject{SHA: commit.SHA},
		}, true)
		if refErr != nil {
			return fmt.Errorf("failed to update reference: %w", refErr)
		}
	}

	return nil
}

func (n Platform) CreateMergeRequest(repository api.Repository, sourceBranch string, title string, description string) error {
	_, _, err := repository.InternalClient.(*github.Client).PullRequests.Create(context.Background(), repository.Namespace, repository.Name, &github.NewPullRequest{
		Base:  ptr.Ptr(repository.DefaultBranch),
		Head:  ptr.Ptr(sourceBranch),
		Title: ptr.Ptr(title),
		Body:  ptr.Ptr(description),
	})
	if err != nil {
		return fmt.Errorf("failed to create merge request: %w", err)
	}

	return nil
}

func (n Platform) CreateOrUpdateMergeRequest(repository api.Repository, sourceBranch string, title string, description string, key string) error {
	client := repository.InternalClient.(*github.Client)
	description = fmt.Sprintf("%s\n\n<!--vcs-merge-request-key:%s-->", description, key)

	// search merge request
	prs, _, err := client.PullRequests.List(context.Background(), repository.Namespace, repository.Name, &github.PullRequestListOptions{
		Head:  sourceBranch,
		Base:  repository.DefaultBranch,
		State: "open",
	})
	if err != nil {
		return fmt.Errorf("failed to list pull requests: %w", err)
	}
	var existingPR *github.PullRequest
	for _, pr := range prs {
		if sourceBranch != "" && pr.GetHead().GetRef() != sourceBranch {
			continue
		}
		if repository.DefaultBranch != "" && pr.GetBase().GetRef() != repository.DefaultBranch {
			continue
		}

		existingPR = pr
		break
	}

	if existingPR != nil {
		log.Debug().Int64("id", existingPR.GetID()).Int("number", existingPR.GetNumber()).Str("source-branch", sourceBranch).Str("target-branch", repository.DefaultBranch).Msg("found existing pull request, updating")
		_, _, updateErr := client.PullRequests.Edit(context.Background(), repository.Namespace, repository.Name, existingPR.GetNumber(), &github.PullRequest{
			Title: ptr.Ptr(title),
			Body:  ptr.Ptr(description),
		})
		if updateErr != nil {
			return fmt.Errorf("failed to update pull request: %w", updateErr)
		}
	} else {
		log.Debug().Str("source_branch", sourceBranch).Str("target_branch", repository.DefaultBranch).Str("title", title).Msg("no existing pull request found, creating")
		_, _, createErr := client.PullRequests.Create(context.Background(), repository.Namespace, repository.Name, &github.NewPullRequest{
			Base:  ptr.Ptr(repository.DefaultBranch),
			Head:  ptr.Ptr(sourceBranch),
			Title: ptr.Ptr(title),
			Body:  ptr.Ptr(description),
		})
		if createErr != nil {
			return fmt.Errorf("failed to create merge request: %w", createErr)
		}
	}

	return nil
}

func (n Platform) FileContent(repository api.Repository, branch string, path string) (string, error) {
	client := repository.InternalClient.(*github.Client)

	// get file content
	fileContent, _, _, err := client.Repositories.GetContents(context.Background(), repository.Namespace, repository.Name, path, &github.RepositoryContentGetOptions{
		Ref: branch,
	})
	if err != nil {
		return "", fmt.Errorf("failed to get file content: %w", err)
	}

	return fileContent.GetContent()
}

func (n Platform) Tags(repository api.Repository, limit int) ([]api.Tag, error) {
	client := repository.InternalClient.(*github.Client)
	var result []api.Tag

	refs, _, err := client.Git.ListMatchingRefs(context.Background(), repository.Namespace, repository.Name, &github.ReferenceListOptions{
		Ref: "tags/",
		ListOptions: github.ListOptions{
			PerPage: limit,
		},
	})
	if err != nil {
		return result, fmt.Errorf("failed to list tags: %w", err)
	}

	for _, r := range refs {
		result = append(result, api.Tag{
			Name:       strings.TrimPrefix(r.GetRef(), "refs/tags/"),
			CommitHash: r.Object.GetSHA(),
		})
	}

	return result, nil
}

func (n Platform) Releases(repository api.Repository, limit int) ([]api.Release, error) {
	client := repository.InternalClient.(*github.Client)
	var result []api.Release

	releaseList, _, err := client.Repositories.ListReleases(context.Background(), repository.Namespace, repository.Name, &github.ListOptions{
		PerPage: limit,
	})
	if err != nil {
		return result, fmt.Errorf("failed to list releases: %w", err)
	}
	for _, r := range releaseList {
		ref, _, err := client.Git.GetRef(context.Background(), repository.Namespace, repository.Name, "tags/"+r.GetTagName())
		if err != nil {
			return result, fmt.Errorf("failed to get tag: %w", err)
		}

		result = append(result, api.Release{
			Name:        r.GetName(),
			TagName:     r.GetTagName(),
			Description: r.GetBody(),
			CommitHash:  ref.GetObject().GetSHA(),
			CreatedAt:   r.CreatedAt.GetTime(),
		})
	}

	return result, nil
}

func (n Platform) CreateTag(repository api.Repository, tagName string, commitHash string, message string) error {
	client := repository.InternalClient.(*github.Client)

	// create tag
	tag, _, err := client.Git.CreateTag(context.Background(), repository.Namespace, repository.Name, &github.Tag{
		Tag:     ptr.Ptr(tagName),
		SHA:     ptr.Ptr(commitHash),
		Message: ptr.Ptr(message),
		Object:  &github.GitObject{Type: ptr.Ptr("commit"), SHA: ptr.Ptr(commitHash)},
	})
	if err != nil {
		return fmt.Errorf("failed to create tag: %w", err)
	}

	// create ref
	_, _, err = client.Git.CreateRef(context.Background(), repository.Namespace, repository.Name, &github.Reference{
		Ref:    ptr.Ptr("refs/tags/" + tagName),
		Object: tag.GetObject(),
	})
	if err != nil {
		return fmt.Errorf("failed to create tag reference: %w", err)
	}

	return nil
}

func (n Platform) Environments(repo api.Repository) ([]api.CIEnvironment, error) {
	var result []api.CIEnvironment
	opts := github.ListOptions{PerPage: pageSize}

	var environments []*github.Environment
	for {
		data, resp, err := repo.InternalClient.(*github.Client).Repositories.ListEnvironments(context.Background(), repo.Namespace, repo.Name, &github.EnvironmentListOptions{ListOptions: opts})
		if err != nil {
			return result, fmt.Errorf("failed to list environments: %w", err)
		}
		environments = append(environments, data.Environments...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	for _, v := range environments {
		result = append(result, api.CIEnvironment{
			ID:          v.GetID(),
			Name:        v.GetName(),
			Description: "",
			Tier:        v.GetEnvironmentName(),
			CreatedAt:   v.CreatedAt.GetTime(),
			UpdatedAt:   v.UpdatedAt.GetTime(),
		})
	}

	return result, nil
}

func (n Platform) EnvironmentVariables(repo api.Repository, environmentName string) ([]api.CIVariable, error) {
	var result []api.CIVariable
	opts := github.ListOptions{PerPage: pageSize}

	var envVariables []*github.ActionsVariable
	for {
		data, resp, err := repo.InternalClient.(*github.Client).Actions.ListEnvVariables(context.Background(), repo.Namespace, repo.Name, environmentName, &opts)
		if err != nil {
			return result, fmt.Errorf("failed to list environment variables: %w", err)
		}
		envVariables = append(envVariables, data.Variables...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	for _, v := range envVariables {
		result = append(result, api.CIVariable{
			Name:      v.Name,
			Value:     v.Value,
			IsSecret:  false,
			CreatedAt: v.CreatedAt.GetTime(),
			UpdatedAt: v.UpdatedAt.GetTime(),
		})
	}

	var envSecrets []*github.Secret
	for {
		data, resp, err := repo.InternalClient.(*github.Client).Actions.ListEnvSecrets(context.Background(), int(repo.Id), environmentName, &opts)
		if err != nil {
			return result, fmt.Errorf("failed to list merge requests: %w", err)
		}
		envSecrets = append(envSecrets, data.Secrets...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	for _, v := range envSecrets {
		result = append(result, api.CIVariable{
			Name:      v.Name,
			Value:     "",
			IsSecret:  true,
			CreatedAt: v.CreatedAt.GetTime(),
			UpdatedAt: v.UpdatedAt.GetTime(),
		})
	}

	return result, nil
}

// NewPlatform creates a GitHub platform
func NewPlatform(config Config) (Platform, error) {
	tr, err := ghinstallation.NewAppsTransport(sharedTransport, config.AppId, []byte(config.PrivateKey))
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create installation transport")
	}

	platform := Platform{
		appId:      config.AppId,
		privateKey: config.PrivateKey,
		client:     github.NewClient(&http.Client{Transport: tr}),
	}

	return platform, nil
}
