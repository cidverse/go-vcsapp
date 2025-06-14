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
	// MergeRequestDiff returns all changes of a merge request
	MergeRequestDiff(repo Repository, mergeRequest MergeRequest) (MergeRequestDiff, error)
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
	// Variables returns a list of all variables for a given repository (omitting secret values)
	Variables(repo Repository) ([]CIVariable, error)
	// Environments returns a list of all environments for a given repository
	Environments(repo Repository) ([]CIEnvironment, error)
	// EnvironmentVariables returns a list of all environment variables for a given repository and environment (omitting secret values)
	EnvironmentVariables(repo Repository, environmentName string) ([]CIVariable, error)
}

type Repository struct {
	PlatformId     string            // the platform id
	PlatformType   string            // the platform type
	Id             int64             // the id of the repository
	Namespace      string            // the namespace of the repository (e.g. organization or user)
	Name           string            // the name of the repository
	Path           string            // the path of the repository (e.g. organization/repo)
	Description    string            // the description of the repository
	Type           string            // repository type - valid values: git
	URL            string            // the url of the repository
	CloneURL       string            // the clone url of the repository
	CloneSSH       string            // the clone ssh url of the repository
	DefaultBranch  string            // the default branch of the repository
	IsFork         bool              // is this repository a fork
	IsEmpty        bool              // is this repository empty (no commits)
	Branches       []string          // list of all branches
	Topics         []string          // list of all topics
	Plan           string            // the plan of the repository (e.g. free, pro, etc. - directly using the platform-specific plan name)
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
	// Number is the project-specific number of the merge request (e.g. 42)
	Number int
	// Title is the title of the merge request
	Title string
	// Description is the description of the merge request
	Description string
	// Labels is a list of labels assigned to the merge request
	Labels []string
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
	// Repository is the repository of the merge request
	Repository Repository
}

type MergeRequestDiff struct {
	ChangedFiles []MergeRequestFileDiff
}

type MergeRequestFileDiff struct {
	IsNew     bool
	IsRenamed bool
	IsDeleted bool
	OldPath   string
	NewPath   string
	OldMode   string
	NewMode   string
	Diff      string
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
	IncludePlan       bool
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

type CIEnvironment struct {
	ID          int64      `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Tier        string     `json:"tier"` // e.g. "staging", "production"
	CreatedAt   *time.Time // the creation date of the environment
	UpdatedAt   *time.Time // the last update date of the environment
}

type CIVariable struct {
	Name      string     `json:"name"`
	Value     string     `json:"value"`
	IsSecret  bool       `json:"isSecret"`
	CreatedAt *time.Time // the creation date of the variable
	UpdatedAt *time.Time // the last update date of the variable
}
