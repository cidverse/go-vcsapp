package gitlabuser

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cidverse/go-ptr"
	"github.com/cidverse/go-vcsapp/pkg/platform/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/gitlab-org/api/client-go/v3"
)

func newTestPlatform(t *testing.T, handler http.HandlerFunc) (Platform, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	platform, err := NewPlatform(Config{
		Server:      server.URL,
		AccessToken: "test-token",
	})
	require.NoError(t, err)

	return platform, server
}

func TestVariablesIncludesGroupVariables(t *testing.T) {
	platform, _ := newTestPlatform(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v4/groups/primelib/variables":
			_, _ = w.Write([]byte(`[
				{"key":"SHARED_KEY","value":"group-value","protected":false,"masked":false,"hidden":false,"raw":false,"environment_scope":""},
				{"key":"MASKED_KEY","value":"masked-value","protected":false,"masked":true,"hidden":false,"raw":false,"environment_scope":""}
			]`))
		case "/api/v4/projects/123/variables":
			_, _ = w.Write([]byte(`[
				{"key":"SHARED_KEY","value":"project-value","protected":false,"masked":false,"hidden":false,"environment_scope":""}
			]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	variables, err := platform.Variables(api.Repository{
		Id:        123,
		Namespace: "primelib",
	})
	require.NoError(t, err)

	byName := make(map[string]api.CIVariable, len(variables))
	for _, v := range variables {
		byName[v.Name] = v
	}

	// group vars are present and masked vars are flagged as secret
	assert.Contains(t, byName, "MASKED_KEY")
	assert.Equal(t, "masked-value", byName["MASKED_KEY"].Value)
	assert.True(t, byName["MASKED_KEY"].IsSecret)

	// project-level value wins for the duplicate key
	assert.Equal(t, "project-value", byName["SHARED_KEY"].Value)

	// project vars must come after group vars so downstream (later overrides earlier) works
	require.Len(t, variables, 3)
	assert.Equal(t, "SHARED_KEY", variables[len(variables)-1].Name)
	assert.Equal(t, "project-value", variables[len(variables)-1].Value)
}

func TestVariablesGroupFetchFailureDoesNotFail(t *testing.T) {
	platform, _ := newTestPlatform(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v4/groups/primelib/variables":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"404 Group Not found"}`))
		case "/api/v4/projects/123/variables":
			_, _ = w.Write([]byte(`[
				{"key":"PROJECT_KEY","value":"project-value","protected":true,"masked":false,"hidden":false,"environment_scope":""}
			]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	variables, err := platform.Variables(api.Repository{
		Id:        123,
		Namespace: "primelib",
	})
	require.NoError(t, err)
	require.Len(t, variables, 1)
	assert.Equal(t, "PROJECT_KEY", variables[0].Name)
	assert.True(t, variables[0].IsSecret)
}

func TestVariablesSkipsGroupVariablesForPersonalProjects(t *testing.T) {
	groupCalled := false
	platform, _ := newTestPlatform(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v4/groups/primelib/variables":
			groupCalled = true
			_, _ = w.Write([]byte(`[{"key":"GROUP_KEY","value":"group-value","environment_scope":""}]`))
		case "/api/v4/projects/123/variables":
			_, _ = w.Write([]byte(`[{"key":"PROJECT_KEY","value":"project-value","environment_scope":""}]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	variables, err := platform.Variables(api.Repository{
		Id:                123,
		Namespace:         "primelib",
		IsPersonalProject: true,
	})
	require.NoError(t, err)
	assert.False(t, groupCalled)
	require.Len(t, variables, 1)
	assert.Equal(t, "PROJECT_KEY", variables[0].Name)
}

func TestConvertRepositorySetsArchived(t *testing.T) {
	assert.True(t, convertRepository(&gitlab.Project{Archived: true, Namespace: &gitlab.ProjectNamespace{}}).IsArchived)
	assert.False(t, convertRepository(&gitlab.Project{Archived: false, Namespace: &gitlab.ProjectNamespace{}}).IsArchived)
}

func newProjectListPlatform(t *testing.T, archivedQuery *string) Platform {
	t.Helper()
	platform, _ := newTestPlatform(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v4/projects":
			*archivedQuery = r.URL.Query().Get("archived")
			// emulate the server-side filter
			if *archivedQuery == "false" {
				_, _ = w.Write([]byte(`[{"id":1,"path_with_namespace":"cidverse/active","archived":false,"namespace":{"full_path":"cidverse","kind":"group"}}]`))
				return
			}
			_, _ = w.Write([]byte(`[
				{"id":1,"path_with_namespace":"cidverse/active","archived":false,"namespace":{"full_path":"cidverse","kind":"group"}},
				{"id":2,"path_with_namespace":"cidverse/archived","archived":true,"namespace":{"full_path":"cidverse","kind":"group"}}
			]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	return platform
}

func TestRepositoriesSkipsArchivedByDefault(t *testing.T) {
	var archivedQuery string
	platform := newProjectListPlatform(t, &archivedQuery)

	repos, err := platform.Repositories(api.RepositoryListOpts{})
	require.NoError(t, err)
	assert.Equal(t, "false", archivedQuery)
	require.Len(t, repos, 1)
	assert.Equal(t, "cidverse/active", repos[0].Path)
	assert.False(t, repos[0].IsArchived)
}

func TestRepositoriesIncludesArchivedWhenRequested(t *testing.T) {
	var archivedQuery string
	platform := newProjectListPlatform(t, &archivedQuery)

	repos, err := platform.Repositories(api.RepositoryListOpts{IncludeArchived: true})
	require.NoError(t, err)
	assert.Empty(t, archivedQuery)
	require.Len(t, repos, 2)
	assert.Equal(t, "cidverse/archived", repos[1].Path)
	assert.True(t, repos[1].IsArchived)
}

func newProjectGetPlatform(t *testing.T) Platform {
	t.Helper()
	platform, _ := newTestPlatform(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.EscapedPath() {
		case "/api/v4/projects/1", "/api/v4/projects/cidverse%2Factive":
			_, _ = w.Write([]byte(`{"id":1,"path_with_namespace":"cidverse/active","http_url_to_repo":"https://gitlab.company.com/cidverse/active.git","namespace":{"full_path":"cidverse","kind":"group"}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	return platform
}

func TestFindRepository(t *testing.T) {
	platform := newProjectGetPlatform(t)

	repo, err := platform.FindRepository("cidverse/active")
	require.NoError(t, err)
	assert.Equal(t, int64(1), repo.Id)
	assert.Equal(t, "gitlab-company-com", repo.PlatformId)

	_, err = platform.FindRepository("cidverse/missing")
	assert.ErrorIs(t, err, api.ErrRepositoryNotFound)
}

func TestFindRepositoryById(t *testing.T) {
	platform := newProjectGetPlatform(t)

	repo, err := platform.FindRepositoryById(1)
	require.NoError(t, err)
	assert.Equal(t, "cidverse/active", repo.Path)

	_, err = platform.FindRepositoryById(99)
	assert.ErrorIs(t, err, api.ErrRepositoryNotFound)
}

var testMergeRepo = api.Repository{Id: 123, Path: "primelib/repo"}

func TestMergeRequestsTimestampsAndHeadSHA(t *testing.T) {
	platform, _ := newTestPlatform(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v4/projects/123/merge_requests", r.URL.Path)
		assert.Equal(t, "ci/cid-workflow-1.2.3", r.URL.Query().Get("source_branch"))
		_, _ = w.Write([]byte(`[{"id":9001,"iid":7,"state":"opened","sha":"abc","source_branch":"ci/cid-workflow-1.2.3","target_branch":"main","detailed_merge_status":"mergeable","created_at":"2026-01-02T03:04:05Z","updated_at":"2026-01-03T03:04:05Z"}]`))
	})

	mrs, err := platform.MergeRequests(testMergeRepo, api.MergeRequestSearchOptions{SourceBranch: "ci/cid-workflow-1.2.3"})
	require.NoError(t, err)
	require.Len(t, mrs, 1)
	assert.Equal(t, int64(9001), mrs[0].Id)
	assert.Equal(t, 7, mrs[0].Number)
	assert.Equal(t, "abc", mrs[0].HeadSHA)
	assert.True(t, mrs[0].CanMerge)
	require.NotNil(t, mrs[0].CreatedAt)
	require.NotNil(t, mrs[0].UpdatedAt)
	assert.Equal(t, 2026, mrs[0].CreatedAt.Year())
	assert.Equal(t, 3, mrs[0].UpdatedAt.Day())
}

func TestMergeRequestByIID(t *testing.T) {
	platform, _ := newTestPlatform(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v4/projects/123/merge_requests/7":
			_, _ = w.Write([]byte(`{"id":9001,"iid":7,"state":"opened","sha":"abc","detailed_merge_status":"mergeable","head_pipeline":{"status":"success"}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"404 Not found"}`))
		}
	})

	mr, err := platform.MergeRequest(testMergeRepo, 7)
	require.NoError(t, err)
	assert.Equal(t, 7, mr.Number)
	assert.True(t, mr.CanMerge)
	assert.Equal(t, api.PipelineStateSuccess, mr.PipelineState)

	_, err = platform.MergeRequest(testMergeRepo, 99)
	require.ErrorIs(t, err, api.ErrMergeRequestNotFound)
}

func TestMergeUsesIIDAndExpectedHeadSHA(t *testing.T) {
	var sha string
	platform, _ := newTestPlatform(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/api/v4/projects/123/merge_requests/7/merge" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"404 Not found"}`))
			return
		}
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		sha, _ = body["sha"].(string)
		_, _ = w.Write([]byte(`{"id":9001,"iid":7,"state":"merged"}`))
	})

	err := platform.Merge(testMergeRepo, api.MergeRequest{Id: 9001, Number: 7}, api.MergeStrategyOptions{ExpectedHeadSHA: ptr.Ptr("abc")})
	require.NoError(t, err)
	assert.Equal(t, "abc", sha)
}

func TestMergeErrorClassification(t *testing.T) {
	tests := []struct {
		status int
		want   error
	}{
		{http.StatusUnauthorized, api.ErrMergeForbidden},
		{http.StatusForbidden, api.ErrMergeForbidden},
		{http.StatusMethodNotAllowed, api.ErrNotMergeable},
		{http.StatusNotAcceptable, api.ErrNotMergeable},
		{http.StatusConflict, api.ErrNotMergeable},
		{http.StatusUnprocessableEntity, api.ErrNotMergeable},
		{http.StatusBadRequest, nil},
	}
	for _, tt := range tests {
		t.Run(http.StatusText(tt.status), func(t *testing.T) {
			platform, _ := newTestPlatform(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(`{"message":"nope"}`))
			})

			err := platform.Merge(testMergeRepo, api.MergeRequest{Number: 7}, api.MergeStrategyOptions{})
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

func TestToPipelineStateUnknown(t *testing.T) {
	assert.Equal(t, api.PipelineStateSuccess, toPipelineState("success"))
	assert.Equal(t, api.PipelineStateFailed, toPipelineState("failed"))
	assert.Equal(t, api.PipelineStateUnknown, toPipelineState("some_new_status"))
}
