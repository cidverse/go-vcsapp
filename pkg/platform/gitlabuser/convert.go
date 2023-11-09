package gitlabuser

import (
	"strings"

	"github.com/cidverse/go-vcsapp/pkg/platform/api"
	"github.com/xanzy/go-gitlab"
)

func convertRepository(repo *gitlab.Project) api.Repository {
	r := api.Repository{
		PlatformId:    api.GetServerIdFromCloneURL(repo.HTTPURLToRepo),
		PlatformType:  "gitlab",
		Id:            int64(repo.ID),
		Namespace:     repo.Namespace.FullPath,
		Name:          repo.Name,
		Description:   repo.Description,
		Type:          "git",
		URL:           strings.TrimPrefix(repo.WebURL, "https://"),
		CloneURL:      repo.HTTPURLToRepo,
		CloneSSH:      repo.SSHURLToRepo,
		DefaultBranch: repo.DefaultBranch,
		Topics:        repo.Topics,
		LicenseURL:    repo.LicenseURL,
		CreatedAt:     repo.CreatedAt,
		InternalRepo:  repo,
	}
	if repo.License != nil {
		r.LicenseName = repo.License.Name
	}

	return r
}
