package vcsapp

import (
	"fmt"
	"os"
	"strconv"

	"github.com/cidverse/go-vcsapp/pkg/platform/api"
	"github.com/cidverse/go-vcsapp/pkg/platform/githubapp"
	"github.com/cidverse/go-vcsapp/pkg/platform/gitlabuser"
)

const (
	AuthorName              = "VCSAPP_AUTHOR_NAME"
	AuthorEMail             = "VCSAPP_AUTHOR_EMAIL"
	GithubAppId             = "GITHUB_APP_ID"
	GithubAppPrivateKey     = "GITHUB_APP_PRIVATE_KEY"
	GithubAppPrivateKeyFile = "GITHUB_APP_PRIVATE_KEY_FILE"
	GitlabServer            = "GITLAB_SERVER"
	GitlabAccessToken       = "GITLAB_ACCESS_TOKEN"
)

// GetPlatformFromEnvironment returns a platform configured via environment variables.
func GetPlatformFromEnvironment() (api.Platform, error) {
	env := getEnvAsMap()

	// author
	author := api.Author{
		Name:  "vcs-app",
		Email: "vcs-app@localhost",
	}
	if mapHasKey(env, AuthorName) {
		author.Name = env[AuthorName]
	}
	if mapHasKey(env, AuthorEMail) {
		author.Email = env[AuthorEMail]
	}

	// GitHub - as application
	if mapHasKey(env, GithubAppId) && mapHasKey(env, GithubAppPrivateKey) {
		appId, _ := strconv.ParseInt(os.Getenv(GithubAppId), 10, 64)
		platform, err := githubapp.NewPlatform(githubapp.Config{
			AppId:      appId,
			PrivateKey: env[GithubAppPrivateKey],
		})
		return platform, err
	}
	if mapHasKey(env, GithubAppId) && mapHasKey(env, GithubAppPrivateKeyFile) {
		appId, _ := strconv.ParseInt(os.Getenv(GithubAppId), 10, 64)

		// read private key
		privateKey, err := os.ReadFile(env[GithubAppPrivateKeyFile])
		if err != nil {
			return nil, fmt.Errorf("failed to read private key file: %w", err)
		}

		platform, err := githubapp.NewPlatform(githubapp.Config{
			AppId:      appId,
			PrivateKey: string(privateKey),
		})
		return platform, err
	}

	// GitLab - as user
	if mapHasKey(env, GitlabServer) && mapHasKey(env, GitlabAccessToken) {
		platform, err := gitlabuser.NewPlatform(gitlabuser.Config{
			Server:      env[GitlabServer],
			Author:      author,
			AccessToken: env[GitlabAccessToken],
		})
		return platform, err
	}

	return nil, fmt.Errorf("no platform found, please configure the platform environment variables according to the documentation")
}
