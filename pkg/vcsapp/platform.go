package vcsapp

import (
	"fmt"
	"os"
	"strconv"

	"github.com/cidverse/go-vcsapp/pkg/platform/api"
	"github.com/cidverse/go-vcsapp/pkg/platform/githubapp"
	"github.com/cidverse/go-vcsapp/pkg/platform/githubuser"
	"github.com/cidverse/go-vcsapp/pkg/platform/gitlabuser"
)

const (
	AuthorName              = "VCSAPP_AUTHOR_NAME"
	AuthorEMail             = "VCSAPP_AUTHOR_EMAIL"
	GithubAppId             = "GITHUB_APP_ID"
	GithubAppPrivateKey     = "GITHUB_APP_PRIVATE_KEY"
	GithubAppPrivateKeyFile = "GITHUB_APP_PRIVATE_KEY_FILE"
	GithubUsername          = "GITHUB_USERNAME"
	GithubToken             = "GITHUB_TOKEN"
	GitlabServer            = "GITLAB_SERVER"
	GitlabAccessToken       = "GITLAB_ACCESS_TOKEN"
)

type PlatformConfig struct {
	GitHubAppId             string
	GitHubAppPrivateKey     string
	GitHubAppPrivateKeyFile string
	GitHubUsername          string
	GitHubToken             string
	GitLabServer            string
	GitLabAccessToken       string
	Author                  api.Author
}

func NewPlatform(platformConfig PlatformConfig) (api.Platform, error) {
	// GitLab - as user
	if platformConfig.GitLabServer != "" && platformConfig.GitLabAccessToken != "" {
		platform, err := gitlabuser.NewPlatform(gitlabuser.Config{
			Server:      platformConfig.GitLabServer,
			AccessToken: platformConfig.GitLabAccessToken,
			Author:      platformConfig.Author,
		})
		return platform, err
	}

	// GitHub - as application
	if platformConfig.GitHubAppId != "" && platformConfig.GitHubAppPrivateKey != "" {
		appId, _ := strconv.ParseInt(platformConfig.GitHubAppId, 10, 64)
		platform, err := githubapp.NewPlatform(githubapp.Config{
			AppId:      appId,
			PrivateKey: platformConfig.GitHubAppPrivateKey,
		})
		return platform, err
	}
	if platformConfig.GitHubAppId != "" && platformConfig.GitHubAppPrivateKeyFile != "" {
		appId, _ := strconv.ParseInt(platformConfig.GitHubAppId, 10, 64)

		// read private key
		privateKey, err := os.ReadFile(platformConfig.GitHubAppPrivateKeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read private key file: %w", err)
		}

		platform, err := githubapp.NewPlatform(githubapp.Config{
			AppId:      appId,
			PrivateKey: string(privateKey),
		})
		return platform, err
	}

	// GitHub - as user
	if platformConfig.GitHubUsername != "" && platformConfig.GitHubToken != "" {
		platform, err := githubuser.NewPlatform(githubuser.Config{
			Username:    platformConfig.GitHubUsername,
			AccessToken: platformConfig.GitHubToken,
		})
		return platform, err
	}

	return nil, fmt.Errorf("no valid platform found")
}

// GetPlatformFromEnvironment returns a platform configured via environment variables.
func GetPlatformFromEnvironment() (api.Platform, error) {
	env := getEnvAsMap()

	// author
	author := api.Author{
		Name:  "vcs-app",
		Email: "vcs-app@localhost",
	}
	if env[AuthorName] != "" {
		author.Name = env[AuthorName]
	}
	if env[AuthorEMail] != "" {
		author.Email = env[AuthorEMail]
	}

	// initialize platform
	p, err := NewPlatform(PlatformConfig{
		GitHubAppId:             env[GithubAppId],
		GitHubAppPrivateKey:     env[GithubAppPrivateKey],
		GitHubAppPrivateKeyFile: env[GithubAppPrivateKeyFile],
		GitHubUsername:          env[GithubUsername],
		GitHubToken:             env[GithubToken],
		GitLabServer:            env[GitlabServer],
		GitLabAccessToken:       env[GitlabAccessToken],
		Author:                  author,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize platform: %w. check the documentation and provide environment variables for at least one platform", err)
	}

	return p, nil
}
