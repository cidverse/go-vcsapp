package githubuser

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cidverse/go-ptr"
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

const testRepository = `{"id":1,"name":"active","full_name":"cidverse/active","default_branch":"main","owner":{"login":"cidverse","type":"Organization"}}`

func TestFindRepository(t *testing.T) {
	platform := newTestPlatform(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/cidverse/active":
			_, _ = w.Write([]byte(testRepository))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	repo, err := platform.FindRepository("cidverse/active")
	require.NoError(t, err)
	assert.Equal(t, int64(1), repo.Id)
	assert.NotNil(t, repo.InternalClient)

	_, err = platform.FindRepository("cidverse/missing")
	assert.ErrorIs(t, err, api.ErrRepositoryNotFound)

	_, err = platform.FindRepository("invalid")
	assert.Error(t, err)
}

func TestFindRepositoryById(t *testing.T) {
	platform := newTestPlatform(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repositories/1":
			_, _ = w.Write([]byte(testRepository))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	repo, err := platform.FindRepositoryById(1)
	require.NoError(t, err)
	assert.Equal(t, "cidverse/active", repo.Path)
	assert.NotNil(t, repo.InternalClient)

	_, err = platform.FindRepositoryById(99)
	assert.ErrorIs(t, err, api.ErrRepositoryNotFound)
}

var testMergeRepo = api.Repository{Id: 1, Namespace: "cidverse", Name: "active", Path: "cidverse/active"}

func TestMergeRequestsFiltersSourceBranch(t *testing.T) {
	var head string
	platform := newTestPlatform(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/repos/cidverse/active/pulls", r.URL.Path)
		head = r.URL.Query().Get("head")
		_, _ = w.Write([]byte(`[
			{"id":1001,"number":42,"state":"open","head":{"ref":"ci/cid-workflow-1.2.3","sha":"abc"},"base":{"ref":"main"},"created_at":"2026-01-02T03:04:05Z","updated_at":"2026-01-03T03:04:05Z"},
			{"id":1002,"number":43,"state":"open","head":{"ref":"other","sha":"def"},"base":{"ref":"main"}}
		]`))
	})

	mrs, err := platform.MergeRequests(testMergeRepo, api.MergeRequestSearchOptions{SourceBranch: "ci/cid-workflow-1.2.3"})
	require.NoError(t, err)
	assert.Equal(t, "cidverse:ci/cid-workflow-1.2.3", head)
	require.Len(t, mrs, 1)
	assert.Equal(t, int64(1001), mrs[0].Id)
	assert.Equal(t, 42, mrs[0].Number)
	assert.Equal(t, "abc", mrs[0].HeadSHA)
	require.NotNil(t, mrs[0].CreatedAt)
	require.NotNil(t, mrs[0].UpdatedAt)
	assert.Equal(t, "2026-01-02T03:04:05Z", mrs[0].CreatedAt.UTC().Format("2006-01-02T15:04:05Z"))
	assert.Equal(t, "2026-01-03T03:04:05Z", mrs[0].UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"))
}

func TestMergeRequestPopulatesCanMerge(t *testing.T) {
	platform := newTestPlatform(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/cidverse/active/pulls/42":
			_, _ = w.Write([]byte(`{"id":1001,"number":42,"state":"open","mergeable":true,"mergeable_state":"clean","head":{"ref":"feature","sha":"abc"},"base":{"ref":"main"}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	mr, err := platform.MergeRequest(testMergeRepo, 42)
	require.NoError(t, err)
	assert.Equal(t, 42, mr.Number)
	assert.True(t, mr.CanMerge)

	_, err = platform.MergeRequest(testMergeRepo, 99)
	require.ErrorIs(t, err, api.ErrMergeRequestNotFound)
}

func TestMergeUsesNumberAndExpectedHeadSHA(t *testing.T) {
	var body map[string]any
	platform := newTestPlatform(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/repos/cidverse/active/pulls/42/merge" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		_, _ = w.Write([]byte(`{"merged":true}`))
	})

	err := platform.Merge(testMergeRepo, api.MergeRequest{Id: 1001, Number: 42}, api.MergeStrategyOptions{Squash: ptr.True(), ExpectedHeadSHA: ptr.Ptr("abc")})
	require.NoError(t, err)
	assert.Equal(t, "abc", body["sha"])
	assert.Equal(t, "squash", body["merge_method"])
}

func TestMergeErrorClassification(t *testing.T) {
	tests := []struct {
		status int
		want   error
	}{
		{http.StatusForbidden, api.ErrMergeForbidden},
		{http.StatusMethodNotAllowed, api.ErrNotMergeable},
		{http.StatusConflict, api.ErrNotMergeable},
		{http.StatusUnprocessableEntity, nil},
	}
	for _, tt := range tests {
		t.Run(http.StatusText(tt.status), func(t *testing.T) {
			platform := newTestPlatform(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(`{"message":"nope"}`))
			})

			err := platform.Merge(testMergeRepo, api.MergeRequest{Number: 42}, api.MergeStrategyOptions{})
			require.Error(t, err)
			if tt.want == nil {
				assert.NotErrorIs(t, err, api.ErrMergeForbidden)
				assert.NotErrorIs(t, err, api.ErrNotMergeable)
			} else {
				assert.ErrorIs(t, err, tt.want)
			}
		})
	}
}

func TestMergeRequestsIsMergedFromMergedAt(t *testing.T) {
	platform := newTestPlatform(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[
			{"id":1,"number":1,"state":"closed","merged_at":"2026-01-02T03:04:05Z","head":{"ref":"a"},"base":{"ref":"main"}},
			{"id":2,"number":2,"state":"closed","head":{"ref":"b"},"base":{"ref":"main"}}
		]`))
	})

	mrs, err := platform.MergeRequests(testMergeRepo, api.MergeRequestSearchOptions{IsMerged: ptr.True()})
	require.NoError(t, err)
	require.Len(t, mrs, 1)
	assert.Equal(t, 1, mrs[0].Number)
	assert.True(t, mrs[0].IsMerged)
}

func TestMergeRemovesSourceBranch(t *testing.T) {
	for _, tt := range []struct {
		name     string
		headRepo string
		deleted  bool
	}{
		{"same repository", "cidverse/active", true},
		{"fork", "someone/active", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var deletedRef string
			platform := newTestPlatform(t, func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodPut && r.URL.Path == "/repos/cidverse/active/pulls/42/merge":
					_, _ = w.Write([]byte(`{"merged":true}`))
				case r.Method == http.MethodGet && r.URL.Path == "/repos/cidverse/active/pulls/42":
					_, _ = w.Write([]byte(`{"number":42,"head":{"ref":"feature","repo":{"full_name":"` + tt.headRepo + `"}},"base":{"ref":"main","repo":{"full_name":"cidverse/active"}}}`))
				case r.Method == http.MethodDelete:
					deletedRef = r.URL.Path
					w.WriteHeader(http.StatusNoContent)
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			})

			err := platform.Merge(testMergeRepo, api.MergeRequest{Number: 42}, api.MergeStrategyOptions{RemoveSourceBranch: ptr.True()})
			require.NoError(t, err)
			if tt.deleted {
				assert.Equal(t, "/repos/cidverse/active/git/refs/heads/feature", deletedRef)
			} else {
				assert.Empty(t, deletedRef)
			}
		})
	}
}

func TestMergeRequestDiffPaginates(t *testing.T) {
	platform := newTestPlatform(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") == "2" {
			_, _ = w.Write([]byte(`[{"filename":"b.go","status":"added"}]`))
			return
		}
		w.Header().Set("Link", `<http://`+r.Host+r.URL.Path+`?page=2>; rel="next"`)
		_, _ = w.Write([]byte(`[{"filename":"a.go","status":"modified"}]`))
	})

	diff, err := platform.MergeRequestDiff(testMergeRepo, api.MergeRequest{Number: 42})
	require.NoError(t, err)
	require.Len(t, diff.ChangedFiles, 2)
	assert.Equal(t, "b.go", diff.ChangedFiles[1].NewPath)
	assert.True(t, diff.ChangedFiles[1].IsNew)
}
