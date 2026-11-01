package gitlabuser

import (
	"net/http"
	"net/http/httptest"
	"testing"

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
