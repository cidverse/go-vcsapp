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
	"github.com/cidverse/vcs-app/pkg/platform/api"
	"github.com/go-git/go-git/v5"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/google/go-github/v54/github"
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

func (n Platform) Repositories() ([]api.Repository, error) {
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
			// query branches
			branchList, _, err := orgClient.Repositories.ListBranches(context.Background(), repo.GetOwner().GetLogin(), repo.GetName(), &github.BranchListOptions{})
			if err != nil {
				return result, fmt.Errorf("failed to list branches: %w", err)
			}

			r := api.Repository{
				Id:             repo.GetID(),
				Namespace:      repo.GetOwner().GetLogin(),
				Name:           repo.GetName(),
				Description:    repo.GetDescription(),
				Type:           "git",
				URL:            strings.TrimPrefix(repo.GetHTMLURL(), "https://"),
				CloneURL:       repo.GetCloneURL(),
				DefaultBranch:  repo.GetDefaultBranch(),
				Branches:       branchSliceToNameSlice(branchList),
				CreatedAt:      repo.CreatedAt.GetTime(),
				RoundTripper:   itr,
				InternalClient: orgClient,
			}
			if repo.GetLicense() != nil {
				r.LicenseName = repo.GetLicense().GetName()
				r.LicenseURL = fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/LICENSE", repo.GetOwner().GetLogin(), repo.GetName(), repo.GetDefaultBranch())
			}
			result = append(result, r)
		}
	}

	return result, nil
}

func (n Platform) MergeRequests(repo api.Repository, options api.MergeRequestSearchOptions) ([]api.MergeRequest, error) {
	var result []api.MergeRequest

	var pullRequests []*github.PullRequest
	opts := github.ListOptions{PerPage: pageSize}
	for {
		data, resp, err := repo.InternalClient.(*github.Client).PullRequests.List(context.Background(), repo.Namespace, repo.Name, &github.PullRequestListOptions{
			Head:        options.SourceBranch,
			Base:        options.TargetBranch,
			State:       options.State,
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
		result = append(result, api.MergeRequest{
			Id:           pr.GetID(),
			Title:        pr.GetTitle(),
			Description:  pr.GetBody(),
			SourceBranch: pr.GetHead().GetRef(),
			TargetBranch: pr.GetBase().GetRef(),
			State:        pr.GetState(),
		})
	}

	return result, nil
}

func (n Platform) AuthMethod(repo api.Repository) githttp.AuthMethod {
	token, err := roundTripperToAccessToken(repo.RoundTripper)
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
		// read file content
		content, readErr := os.ReadFile(filepath.Join(dir, file))
		if readErr != nil {
			return fmt.Errorf("failed to read file: %w", err)
		}

		// get permissions
		fileStats, statsErr := os.Stat(filepath.Join(dir, file))
		if statsErr != nil {
			return fmt.Errorf("failed to get file stats: %w", err)
		}
		mode := "100644"
		if fileStats.Mode()&os.FileMode(0111) != 0 {
			mode = "100744"
		}
		entries = append(entries, &github.TreeEntry{
			Path:    github.String(file),
			Type:    github.String("blob"),
			Content: github.String(string(content)),
			Mode:    github.String(mode),
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
	})
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
		Base:  github.String(repository.DefaultBranch),
		Head:  github.String(sourceBranch),
		Title: github.String(title),
		Body:  github.String(description),
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
			Title: github.String(title),
			Body:  github.String(description),
		})
		if updateErr != nil {
			return fmt.Errorf("failed to update pull request: %w", updateErr)
		}
	} else {
		log.Debug().Str("source_branch", sourceBranch).Str("target_branch", repository.DefaultBranch).Str("title", title).Msg("no existing pull request found, creating")
		_, _, createErr := client.PullRequests.Create(context.Background(), repository.Namespace, repository.Name, &github.NewPullRequest{
			Base:  github.String(repository.DefaultBranch),
			Head:  github.String(sourceBranch),
			Title: github.String(title),
			Body:  github.String(description),
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
