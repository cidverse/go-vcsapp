package githubuser

import (
	"fmt"
	"strings"

	"github.com/cidverse/go-vcsapp/pkg/platform/api"
	"github.com/google/go-github/v70/github"
)

func convertRepository(repo *github.Repository) api.Repository {
	r := api.Repository{
		PlatformId:    api.GetServerIdFromCloneURL(repo.GetCloneURL()),
		PlatformType:  "github",
		Id:            repo.GetID(),
		Namespace:     repo.GetOwner().GetLogin(),
		Name:          repo.GetName(),
		Description:   repo.GetDescription(),
		Type:          "git",
		URL:           strings.TrimPrefix(repo.GetHTMLURL(), "https://"),
		CloneURL:      repo.GetCloneURL(),
		CloneSSH:      repo.GetSSHURL(),
		DefaultBranch: repo.GetDefaultBranch(),
		IsFork:        repo.GetFork(),
		Topics:        repo.Topics,
		CreatedAt:     repo.CreatedAt.GetTime(),
		InternalRepo:  repo,
	}
	if repo.GetLicense() != nil {
		r.LicenseName = repo.GetLicense().GetName()
		r.LicenseURL = fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/LICENSE", repo.GetOwner().GetLogin(), repo.GetName(), repo.GetDefaultBranch())
	}

	return r
}
