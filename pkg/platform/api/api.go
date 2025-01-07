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
	// FindRepository returns one repository by its name
	FindRepository(name string) (Repository, error)
	// MergeRequests returns a list of all pull requests created by us
	MergeRequests(repository Repository, options MergeRequestSearchOptions) ([]MergeRequest, error)
	// SubmitReview submits a review result / approval for a merge request
	SubmitReview(repo Repository, mergeRequest MergeRequest, approved bool, message *string) error
	// Merge merges a merge request
	Merge(repo Repository, mergeRequest MergeRequest, mergeStrategy MergeStrategyOptions) error
	// Languages returns a map of used languages and their line count
	Languages(repository Repository) (map[string]int, error)
	// AuthMethod returns the authentication method used by the platform, required to push changes
	AuthMethod(repository Repository) githttp.AuthMethod
	// CommitAndPush creates a commit in the repository and pushes it to the remote
	CommitAndPush(repository Repository, base string, branch string, message string, dir string) error
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
	PlatformId     string            // the platform id
	PlatformType   string            // the platform type
	Id             int64             // the id of the repository
	Namespace      string            // the namespace of the repository (e.g. organization or user)
	Name           string            // the name of the repository
	Description    string            // the description of the repository
	Type           string            // repository type - valid values: git
	URL            string            // the url of the repository
	CloneURL       string            // the clone url of the repository
	CloneSSH       string            // the clone ssh url of the repository
	DefaultBranch  string            // the default branch of the repository
	IsFork         bool              // is this repository a fork
	Branches       []string          // list of all branches
	Topics         []string          // list of all topics
	LicenseName    string            // the name of the license
	LicenseURL     string            // the url of the license
	CommitHash     string            // the commit hash of the latest commit on the default branch
	CommitDate     *time.Time        // the commit date of the latest commit on the default branch
	CreatedAt      *time.Time        // the creation date of the repository
	RoundTripper   http.RoundTripper `json:"-" yaml:"-"` // this is a platform specific round tripper for the repository (GitHub apps require an org-scoped round tripper)
	InternalClient interface{}       `json:"-" yaml:"-"` // this is a platform specific client for the repository (GitHub apps require an org-scoped client)
	InternalRepo   interface{}       `json:"-" yaml:"-"` // this is the original repository object from the platform
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
	State MergeRequestState
	// PipelineState is the state of the pipeline
	PipelineState PipelineState
	// IsMerged is true if the merge request is merged
	IsMerged bool
	// IsLocked is true if the merge request is locked
	IsLocked bool
	// IsDraft is true if the merge request is a work in progress / not ready for review
	IsDraft bool
	// HasConflicts is true if the merge request has conflicts
	HasConflicts bool
	// CanMerge is true if the merge request can be merged (no conflicts, no unresolved discussions, no work in progress, pipeline passed)
	CanMerge bool
	// Author is the author of the merge request
	Author User
}

type MergeRequestSearchOptions struct {
	SourceBranch   string
	TargetBranch   string
	State          *MergeRequestState
	IsMerged       *bool   // Filter by merged status
	IsDraft        *bool   // Filter by draft status
	AuthorId       *int64  // Filter by author user id
	AuthorUsername *string // Filter by author username
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

type MergeStrategyOptions struct {
	Squash             *bool
	RemoveSourceBranch *bool
}

type RepositoryListOpts struct {
	IncludeBranches   bool
	IncludeCommitHash bool
}

type User struct {
	ID                  int64      `json:"id"`
	Username            string     `json:"username"`
	Name                string     `json:"name"`
	Type                UserType   `json:"type"`
	State               UserState  `json:"state"`
	CreatedAt           *time.Time `json:"created_at"`
	SuspendedAt         *time.Time `json:"suspended_at"`
	AvatarURL           string     `json:"avatar_url"`
	GlobalAdministrator bool       `json:"global_administrator"`
}

type GitAuthor struct {
	Name  string `yaml:"name"`
	Email string `yaml:"email"`
}
