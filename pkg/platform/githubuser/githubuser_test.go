package githubuser

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cidverse/go-vcsapp/pkg/platform/api"
	"github.com/google/go-github/v93/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testRepositories = `[
	{"id":1,"name":"active","full_name":"cidverse/active","default_branch":"main","archived":false,"owner":{"login":"cidverse","type":"Organization"}},
	{"id":2,"name":"archived","full_name":"cidverse/archived","default_branch":"main","archived":true,"owner":{"login":"cidverse","type":"Organization"}}
]`

func newTestPlatform(t *testing.T, handler http.HandlerFunc) Platform {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	baseURL := server.URL + "/"
	client, err := github.NewClient(github.WithURLs(&baseURL, nil))
	require.NoError(t, err)

	return Platform{client: client}
}

func TestConvertRepositorySetsArchived(t *testing.T) {
	assert.True(t, convertRepository(&github.Repository{Archived: github.Ptr(true)}, nil).IsArchived)
	assert.False(t, convertRepository(&github.Repository{Archived: github.Ptr(false)}, nil).IsArchived)
	assert.False(t, convertRepository(&github.Repository{}, nil).IsArchived)
}

func TestRepositoriesSkipsArchivedByDefault(t *testing.T) {
	var archivedCalls int
	platform := newTestPlatform(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/user/repos":
			_, _ = w.Write([]byte(testRepositories))
		case "/repos/cidverse/active/commits/heads/main":
			_, _ = w.Write([]byte(`{"sha":"abc123"}`))
		case "/repos/cidverse/active/branches":
			_, _ = w.Write([]byte(`[{"name":"main"}]`))
		case "/repos/cidverse/archived/commits/heads/main", "/repos/cidverse/archived/branches":
			archivedCalls++
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	repos, err := platform.Repositories(api.RepositoryListOpts{IncludeBranches: true, IncludeCommitHash: true})
	require.NoError(t, err)
	require.Len(t, repos, 1)
	assert.Equal(t, "cidverse/active", repos[0].Path)
	assert.False(t, repos[0].IsArchived)
	assert.Equal(t, "abc123", repos[0].CommitHash)

	// archived repositories must be dropped before the per-repository lookups
	assert.Equal(t, 0, archivedCalls)
}

func TestRepositoriesIncludesArchivedWhenRequested(t *testing.T) {
	platform := newTestPlatform(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/user/repos":
			_, _ = w.Write([]byte(testRepositories))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	repos, err := platform.Repositories(api.RepositoryListOpts{IncludeArchived: true})
	require.NoError(t, err)
	require.Len(t, repos, 2)
	assert.Equal(t, "cidverse/archived", repos[1].Path)
	assert.True(t, repos[1].IsArchived)
}
