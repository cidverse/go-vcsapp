package api

import (
	"net/http"

	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
)

// Platform provides a common interface to work with all platforms
type Platform interface {
	// Name returns the name of the platform
	Name() string
	// Slug returns the slug of the platform
	Slug() string
	// Repositories returns a list of all repositories we have access to
	Repositories() ([]Repository, error)
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
}

type Repository struct {
	Id             int64    // the id of the repository
	Namespace      string   // the namespace of the repository (e.g. organization or user)
	Name           string   // the name of the repository
	Type           string   // repository type - valid values: git
	CloneURL       string   // the clone url of the repository
	DefaultBranch  string   // the default branch of the repository
	Branches       []string // list of all branches
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
