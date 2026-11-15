package githubapp

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/cidverse/go-ptr"
	"github.com/cidverse/go-vcsapp/pkg/platform/api"
	"github.com/cidverse/go-vcsapp/pkg/platform/githubcommon"
	"github.com/go-git/go-git/v5"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/google/go-github/v93/github"
	"github.com/rs/zerolog/log"
)

const pageSize = 100

var sharedTransport = http.DefaultTransport // shared transport to reuse TCP connections.

type Platform struct {
	appId      int64
	privateKey string
	client     *github.Client
	baseURL    string               // overrides the GitHub API base url (used in tests), defaults to https://api.github.com
	clients    *installationClients // cache of installation-scoped clients, may be nil (no caching)
}

type Config struct {
	AppId      int64  `yaml:"appId"`
	PrivateKey string `yaml:"privateKey"`
}

// installationClient is an installation (org / user) scoped client, the transport takes care of refreshing the installation token
type installationClient struct {
	transport *ghinstallation.Transport
	client    *github.Client
}

type installationClients struct {
	mu      sync.Mutex
	clients map[int64]installationClient
}

func newInstallationClients() *installationClients {
	return &installationClients{clients: map[int64]installationClient{}}
}

func githubClientFromRepository(repo api.Repository) (*github.Client, error) {
	if repo.InternalClient == nil {
		return nil, fmt.Errorf("missing internal github client for repository: %s", repo.Path)
	}

	client, ok := repo.InternalClient.(*github.Client)
	if !ok {
		return nil, fmt.Errorf("invalid internal github client type %T for repository: %s", repo.InternalClient, repo.Path)
	}

	return client, nil
}

// installationClient returns a client scoped to the given installation
func (n Platform) installationClient(installationId int64) (installationClient, error) {
	if n.clients != nil {
		n.clients.mu.Lock()
		defer n.clients.mu.Unlock()
		if c, ok := n.clients.clients[installationId]; ok {
			return c, nil
		}
	}

	itr, err := ghinstallation.New(sharedTransport, n.appId, installationId, []byte(n.privateKey))
	if err != nil {
		return installationClient{}, fmt.Errorf("failed to create installation transport: %w", err)
	}
	clientOpts := []github.ClientOptionsFunc{github.WithTransport(itr)}
	if n.baseURL != "" {
		itr.BaseURL = strings.TrimRight(n.baseURL, "/")
		clientOpts = append(clientOpts, github.WithURLs(&n.baseURL, nil))
	}
	client, err := github.NewClient(clientOpts...)
	if err != nil {
		return installationClient{}, fmt.Errorf("failed to create github client: %w", err)
	}

	c := installationClient{transport: itr, client: client}
	if n.clients != nil {
		n.clients.clients[installationId] = c
	}
	return c, nil
}

// installations returns all installations of the app
func (n Platform) installations() ([]*github.Installation, error) {
	var installations []*github.Installation
	opts := &github.ListOptions{PerPage: pageSize}
	for {
		data, resp, err := n.client.Apps.ListInstallations(context.Background(), opts)
		if err != nil {
			return nil, fmt.Errorf("failed to list installations: %w", err)
		}
		installations = append(installations, data...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return installations, nil
}

func convertRepository(repo *github.Repository, installation *github.Installation, ic installationClient) api.Repository {
	r := api.Repository{
		PlatformId:        api.GetServerIdFromCloneURL(repo.GetCloneURL()),
		PlatformType:      "github",
		Id:                repo.GetID(),
		Namespace:         repo.GetOwner().GetLogin(),
		Name:              repo.GetName(),
		Path:              repo.GetFullName(),
		Description:       repo.GetDescription(),
		Type:              "git",
		URL:               strings.TrimPrefix(repo.GetHTMLURL(), "https://"),
		CloneURL:          repo.GetCloneURL(),
		CloneSSH:          repo.GetSSHURL(),
		DefaultBranch:     repo.GetDefaultBranch(),
		IsFork:            repo.GetFork(),
		IsEmpty:           false,
		IsPersonalProject: strings.EqualFold(installation.GetAccount().GetType(), "user"),
		IsArchived:        repo.GetArchived(),
		Topics:            repo.Topics,
		CreatedAt:         repo.CreatedAt.GetTime(),
		RoundTripper:      ic.transport,
		InternalClient:    ic.client,
		InternalRepo:      repo,
	}
	if repo.GetLicense() != nil {
		r.LicenseName = repo.GetLicense().GetName()
		r.LicenseURL = fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/LICENSE", repo.GetOwner().GetLogin(), repo.GetName(), repo.GetDefaultBranch())
	}

	return r
}

func (n Platform) Name() string {
	return "GitHub"
}

func (n Platform) Slug() string {
	return "github"
}

func (n Platform) Repositories(opts api.RepositoryListOpts) ([]api.Repository, error) {
	var result []api.Repository

	installations, err := n.installations()
	if err != nil {
		return result, err
	}
	log.Info().Int("count", len(installations)).Msg("github platform - found app installations")

	for _, installation := range installations {
		ic, err := n.installationClient(installation.GetID())
		if err != nil {
			return result, err
		}
		orgClient := ic.client

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
			// github has no server-side filter for archived repositories, skip them before any further api calls
			if repo.GetArchived() && !opts.IncludeArchived {
				continue
			}

			r := convertRepository(repo, installation, ic)

			// commit
			if opts.IncludeCommitHash {
				commit, _, err := orgClient.Repositories.GetCommit(context.Background(), repo.GetOwner().GetLogin(), repo.GetName(), "heads/"+repo.GetDefaultBranch(), &github.ListOptions{})
				if err != nil {
					if !strings.Contains(err.Error(), "409 Git Repository is empty") && !strings.Contains(err.Error(), "404 Not Found") {
						return result, fmt.Errorf("failed to get commit: %w", err)
					}

					r.IsEmpty = true
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
					if !strings.Contains(err.Error(), "409 Git Repository is empty") && !strings.Contains(err.Error(), "404 Not Found") {
						return result, fmt.Errorf("failed to list branches: %w", err)
					}

					r.IsEmpty = true
				} else {
					r.Branches = githubcommon.BranchSliceToNameSlice(branchList)
				}
			}

			result = append(result, r)
		}
	}

	return result, nil
}

// FindRepository looks up the installation that covers the repository and fetches it with an installation-scoped client
func (n Platform) FindRepository(path string) (api.Repository, error) {
	owner, name, ok := strings.Cut(path, "/")
	if !ok || owner == "" || name == "" {
		return api.Repository{}, fmt.Errorf("invalid repository path %q, expected owner/name", path)
	}

	installation, resp, err := n.client.Apps.GetRepositoryInstallation(context.Background(), owner, name)
	if err != nil {
		return api.Repository{}, githubcommon.WrapGetRepositoryError(path, resp, err)
	}
	ic, err := n.installationClient(installation.GetID())
	if err != nil {
		return api.Repository{}, err
	}

	repo, resp, err := ic.client.Repositories.Get(context.Background(), owner, name)
	if err != nil {
		return api.Repository{}, githubcommon.WrapGetRepositoryError(path, resp, err)
	}

	return convertRepository(repo, installation, ic), nil
}

// FindRepositoryById fetches a repository by id.
// The installation is resolved via the undocumented GET /repositories/{id}/installation endpoint, if that fails with 404 every installation is tried until one has access.
func (n Platform) FindRepositoryById(id int64) (api.Repository, error) {
	installation, resp, err := n.client.Apps.GetRepositoryInstallationByID(context.Background(), id)
	if err == nil {
		repo, found, err := n.getRepositoryByIdWithInstallation(id, installation)
		if err != nil {
			return api.Repository{}, err
		}
		if found {
			return repo, nil
		}
	} else if !githubcommon.IsNotFound(resp) {
		return api.Repository{}, githubcommon.WrapGetRepositoryError(id, resp, err)
	}

	// fallback: scan all installations
	installations, err := n.installations()
	if err != nil {
		return api.Repository{}, err
	}
	for _, inst := range installations {
		repo, found, err := n.getRepositoryByIdWithInstallation(id, inst)
		if err != nil {
			return api.Repository{}, err
		}
		if found {
			return repo, nil
		}
	}

	return api.Repository{}, fmt.Errorf("failed to get repository %d: %w", id, api.ErrRepositoryNotFound)
}

// getRepositoryByIdWithInstallation fetches a repository by id using an installation-scoped client, found is false if the installation has no access
func (n Platform) getRepositoryByIdWithInstallation(id int64, installation *github.Installation) (repository api.Repository, found bool, err error) {
	ic, err := n.installationClient(installation.GetID())
	if err != nil {
		return api.Repository{}, false, err
	}

	repo, resp, err := ic.client.Repositories.GetByID(context.Background(), id)
	if err != nil {
		if githubcommon.IsNotFound(resp) {
			return api.Repository{}, false, nil
		}
		return api.Repository{}, false, githubcommon.WrapGetRepositoryError(id, resp, err)
	}

	return convertRepository(repo, installation, ic), true, nil
}

func (n Platform) MergeRequests(repo api.Repository, options api.MergeRequestSearchOptions) ([]api.MergeRequest, error) {
	client, err := githubClientFromRepository(repo)
	if err != nil {
		return nil, err
	}
	return githubcommon.ListMergeRequests(client, repo, options)
}

func (n Platform) MergeRequest(repo api.Repository, number int) (api.MergeRequest, error) {
	client, err := githubClientFromRepository(repo)
	if err != nil {
		return api.MergeRequest{}, err
	}
	return githubcommon.GetMergeRequest(client, repo, number)
}

func (n Platform) MergeRequestDiff(repo api.Repository, mergeRequest api.MergeRequest) (api.MergeRequestDiff, error) {
	client, err := githubClientFromRepository(repo)
	if err != nil {
		return api.MergeRequestDiff{}, err
	}
	return githubcommon.MergeRequestDiff(client, repo, mergeRequest)
}

func (n Platform) SubmitReview(repo api.Repository, mergeRequest api.MergeRequest, approved bool, message *string) error {
	client, err := githubClientFromRepository(repo)
	if err != nil {
		return err
	}
	return githubcommon.SubmitReview(client, repo, mergeRequest, approved, message)
}

func (n Platform) Merge(repo api.Repository, mergeRequest api.MergeRequest, mergeStrategy api.MergeStrategyOptions) error {
	client, err := githubClientFromRepository(repo)
	if err != nil {
		return err
	}
	return githubcommon.Merge(client, repo, mergeRequest, mergeStrategy)
}

func (n Platform) Languages(repo api.Repository) (map[string]int, error) {
	client, err := githubClientFromRepository(repo)
	if err != nil {
		return nil, err
	}

	data, _, err := client.Repositories.ListLanguages(context.Background(), repo.Namespace, repo.Name)
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
	client, err := githubClientFromRepository(repo)
	if err != nil {
		return err
	}

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
	commit, _, err := client.Git.CreateCommit(context.Background(), repo.Namespace, repo.Name, github.Commit{
		Message: ptr.Ptr(message),
		Tree:    tree,
		Parents: []*github.Commit{{SHA: github.Ptr(base)}},
	}, &github.CreateCommitOptions{})
	if err != nil {
		return fmt.Errorf("failed to create commit: %w", err)
	}

	// create or update remote reference
	_, _, getRefErr := client.Git.GetRef(context.Background(), repo.Namespace, repo.Name, "refs/heads/"+branch)
	if getRefErr != nil {
		_, _, createRefErr := client.Git.CreateRef(context.Background(), repo.Namespace, repo.Name, github.CreateRef{
			Ref: "refs/heads/" + branch,
			SHA: commit.GetSHA(),
		})
		if createRefErr != nil {
			return fmt.Errorf("failed to create remote branch reference: %w", createRefErr)
		}
	} else {
		_, _, refErr := client.Git.UpdateRef(context.Background(), repo.Namespace, repo.Name, "refs/heads/"+branch, github.UpdateRef{
			SHA:   commit.GetSHA(),
			Force: ptr.True(),
		})
		if refErr != nil {
			return fmt.Errorf("failed to update reference: %w", refErr)
		}
	}

	return nil
}

func (n Platform) CreateMergeRequest(repository api.Repository, sourceBranch string, title string, description string) error {
	client, err := githubClientFromRepository(repository)
	if err != nil {
		return err
	}

	_, _, err = client.PullRequests.Create(context.Background(), repository.Namespace, repository.Name, github.CreatePullRequest{
		Base:  repository.DefaultBranch,
		Head:  sourceBranch,
		Title: ptr.Ptr(title),
		Body:  ptr.Ptr(description),
	})
	if err != nil {
		return fmt.Errorf("failed to create merge request: %w", err)
	}

	return nil
}

func (n Platform) CreateOrUpdateMergeRequest(repository api.Repository, sourceBranch string, title string, description string, key string) error {
	client, err := githubClientFromRepository(repository)
	if err != nil {
		return err
	}
	description = fmt.Sprintf("%s\n\n<!--vcs-merge-request-key:%s-->", description, key)

	// search merge request
	prs, _, err := client.PullRequests.List(context.Background(), repository.Namespace, repository.Name, &github.PullRequestListOptions{
		Head:  githubcommon.HeadFilter(repository.Namespace, sourceBranch),
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
		_, _, createErr := client.PullRequests.Create(context.Background(), repository.Namespace, repository.Name, github.CreatePullRequest{
			Base:  repository.DefaultBranch,
			Head:  sourceBranch,
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
	client, err := githubClientFromRepository(repository)
	if err != nil {
		return "", err
	}

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
	var result []api.Tag
	client, err := githubClientFromRepository(repository)
	if err != nil {
		return result, err
	}

	refs, _, err := client.Git.ListMatchingRefs(context.Background(), repository.Namespace, repository.Name, "tags/")
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
	var result []api.Release
	client, err := githubClientFromRepository(repository)
	if err != nil {
		return result, err
	}

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
	client, err := githubClientFromRepository(repository)
	if err != nil {
		return err
	}

	// create tag
	tag, _, err := client.Git.CreateTag(context.Background(), repository.Namespace, repository.Name, github.CreateTag{
		Tag:     tagName,
		Message: message,
		Object:  commitHash,
		Type:    "commit",
	})
	if err != nil {
		return fmt.Errorf("failed to create tag: %w", err)
	}

	// create ref
	_, _, err = client.Git.CreateRef(context.Background(), repository.Namespace, repository.Name, github.CreateRef{
		Ref: "refs/tags/" + tagName,
		SHA: tag.GetObject().GetSHA(),
	})
	if err != nil {
		return fmt.Errorf("failed to create tag reference: %w", err)
	}

	return nil
}

func (n Platform) Variables(repo api.Repository) ([]api.CIVariable, error) {
	client, err := githubClientFromRepository(repo)
	if err != nil {
		return nil, err
	}

	return githubcommon.Variables(repo, client)
}

func (n Platform) Environments(repo api.Repository) ([]api.CIEnvironment, error) {
	client, err := githubClientFromRepository(repo)
	if err != nil {
		return nil, err
	}

	return githubcommon.Environments(repo, client)
}

func (n Platform) EnvironmentVariables(repo api.Repository, environmentName string) ([]api.CIVariable, error) {
	var result []api.CIVariable
	client, err := githubClientFromRepository(repo)
	if err != nil {
		return result, err
	}

	// env
	var envVariables []*github.ActionsVariable
	opts := github.ListOptions{PerPage: pageSize}
	for {
		data, resp, err := client.Actions.ListEnvVariables(context.Background(), repo.Namespace, repo.Name, environmentName, &opts)
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

	// secrets
	var envSecrets []*github.Secret
	opts = github.ListOptions{PerPage: pageSize}
	for {
		data, resp, err := client.Actions.ListEnvSecrets(context.Background(), repo.Namespace, repo.Name, environmentName, &opts)
		if err != nil {
			return result, fmt.Errorf("failed to list environment secrets: %w", err)
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
		return Platform{}, fmt.Errorf("failed to create transport: %w", err)
	}

	client, err := github.NewClient(github.WithTransport(tr))
	if err != nil {
		return Platform{}, fmt.Errorf("failed to create github client: %w", err)
	}

	platform := Platform{
		appId:      config.AppId,
		privateKey: config.PrivateKey,
		client:     client,
		clients:    newInstallationClients(),
	}

	return platform, nil
}
