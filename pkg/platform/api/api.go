package api

import (
	"net/http"
	"time"

	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
)

// Platform provides a common interface to work with all platforms
type Platform interface {
	// Name returns the name of the platform
	Name() string
	// Slug returns the slug of the platform
	Slug() string
	// Repositories returns a list of all repositories we have access to
	Repositories(opts RepositoryListOpts) ([]Repository, error)
	// MergeRequests returns a list of all pull requests created by us
	MergeRequests(repository Repository, options MergeRequestSearchOptions) ([]MergeRequest, error)
	// AuthMethod returns the authentication method used by the platform, required to push changes
	AuthMethod(repository Repository) githttp.AuthMethod
	// CommitAndPush creates a commit in the repository and pushes it to the remote
	CommitAndPush(repo Repository, base string, branch string, message string, dir string) error
	// CreateMergeRequest creates a merge request
	CreateMergeRequest(repository Repository, sourceBranch string, title string, description string) error
	// CreateOrUpdateMergeRequest creates a merge request
	CreateOrUpdateMergeRequest(repository Repository, sourceBranch string, title string, description string, key string) error
	// FileContent returns the content of a file
	FileContent(repository Repository, branch string, path string) (string, error)
	// Tags returns a list of all tags
	Tags(repository Repository, limit int) ([]Tag, error)
	// Releases returns a list of all releases
	Releases(repository Repository, limit int) ([]Release, error)
	// CreateTag creates a tag
	CreateTag(repository Repository, tag string, commitHash string, message string) error
}

type Repository struct {
	Id             int64      // the id of the repository
	Namespace      string     // the namespace of the repository (e.g. organization or user)
	Name           string     // the name of the repository
	Description    string     // the description of the repository
	Type           string     // repository type - valid values: git
	URL            string     // the url of the repository
	CloneURL       string     // the clone url of the repository
	DefaultBranch  string     // the default branch of the repository
	Branches       []string   // list of all branches
	LicenseName    string     // the name of the license
	LicenseURL     string     // the url of the license
	CommitHash     string     // the commit hash of the latest commit on the default branch
	CommitDate     *time.Time // the commit date of the latest commit on the default branch
	CreatedAt      *time.Time // the creation date of the repository
	RoundTripper   http.RoundTripper
	InternalClient interface{} // this is a platform specific client for the repository (GitHub apps require an org-scoped client)
}

type MergeRequest struct {
	// ID is the unique identifier of the merge request
	Id int64
	// Title is the title of the merge request
	Title string
	// Description is the description of the merge request
	Description string
	// SourceBranch is the source branch of the merge request
	SourceBranch string
	// TargetBranch is the target branch of the merge request
	TargetBranch string
	// State is the state of the merge request
	State string
}

type Author struct {
	Name  string `yaml:"name"`
	Email string `yaml:"email"`
}

type MergeRequestSearchOptions struct {
	SourceBranch string
	TargetBranch string
	State        string
}

type Tag struct {
	// Name is the name of the release
	Name string
	// CommitHash is the commit hash of the release
	CommitHash string
}

type Release struct {
	// Name is the name of the release
	Name string
	// TagName is the tag name of the release
	TagName string
	// Description is the description of the release
	Description string
	// CommitHash is the commit hash of the release
	CommitHash string
	// CreatedAt is the creation date of the release
	CreatedAt *time.Time
}

type RepositoryListOpts struct {
	IncludeBranches   bool
	IncludeCommitHash bool
}
