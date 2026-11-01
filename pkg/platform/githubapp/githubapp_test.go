package githubapp

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cidverse/go-vcsapp/pkg/platform/api"
	"github.com/google/go-github/v93/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testRepositories = `{"total_count":2,"repositories":[
	{"id":1,"name":"active","full_name":"cidverse/active","default_branch":"main","archived":false,"owner":{"login":"cidverse","type":"Organization"}},
	{"id":2,"name":"archived","full_name":"cidverse/archived","default_branch":"main","archived":true,"owner":{"login":"cidverse","type":"Organization"}}
]}`

func newTestPlatform(t *testing.T, handler http.HandlerFunc) Platform {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/app/installations":
			_, _ = w.Write([]byte(`[{"id":42,"account":{"login":"cidverse","type":"Organization"}}]`))
		case "/app/installations/42/access_tokens":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"token":"installation-token","expires_at":"2099-01-01T00:00:00Z"}`))
		default:
			handler(w, r)
		}
	}))
	t.Cleanup(server.Close)

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	privateKey := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})

	baseURL := server.URL + "/"
	client, err := github.NewClient(github.WithURLs(&baseURL, nil))
	require.NoError(t, err)

	return Platform{
		appId:      1,
		privateKey: string(privateKey),
		client:     client,
		baseURL:    baseURL,
	}
}

func TestRepositoriesSkipsArchivedByDefault(t *testing.T) {
	var archivedCalls int
	platform := newTestPlatform(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/installation/repositories":
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
	assert.Equal(t, []string{"main"}, repos[0].Branches)

	// archived repositories must be dropped before the per-repository lookups
	assert.Equal(t, 0, archivedCalls)
}

func TestRepositoriesIncludesArchivedWhenRequested(t *testing.T) {
	platform := newTestPlatform(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/installation/repositories":
			_, _ = w.Write([]byte(testRepositories))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	repos, err := platform.Repositories(api.RepositoryListOpts{IncludeArchived: true})
	require.NoError(t, err)
	require.Len(t, repos, 2)
	assert.False(t, repos[0].IsArchived)
	assert.Equal(t, "cidverse/archived", repos[1].Path)
	assert.True(t, repos[1].IsArchived)
}
