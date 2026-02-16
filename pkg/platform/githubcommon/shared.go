package githubcommon

import (
	"context"
	"fmt"

	"github.com/cidverse/go-vcsapp/pkg/platform/api"
	"github.com/google/go-github/v83/github"
)

func Variables(repo api.Repository, githubClient *github.Client) ([]api.CIVariable, error) {
	var result []api.CIVariable

	// env
	var envVariables []*github.ActionsVariable
	opts := github.ListOptions{PerPage: PageSize}
	for {
		data, resp, err := githubClient.Actions.ListOrgVariables(context.Background(), repo.Namespace, &opts)
		if err != nil {
			return result, fmt.Errorf("failed to list environment variables: %w", err)
		}
		envVariables = append(envVariables, data.Variables...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	opts = github.ListOptions{PerPage: PageSize}
	for {
		data, resp, err := githubClient.Actions.ListRepoVariables(context.Background(), repo.Namespace, repo.Name, &opts)
		if err != nil {
			return result, fmt.Errorf("failed to list environments variables: %w", err)
		}
		envVariables = append(envVariables, data.Variables...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	for _, v := range envVariables {
		result = append(result, api.CIVariable{
			Name:      v.Name,
			Value:     v.Value,
			IsSecret:  false,
			CreatedAt: v.CreatedAt.GetTime(),
			UpdatedAt: v.UpdatedAt.GetTime(),
		})
	}

	// secrets
	var envSecrets []*github.Secret
	for {
		data, resp, err := githubClient.Actions.ListOrgSecrets(context.Background(), repo.Namespace, &opts)
		if err != nil {
			return result, fmt.Errorf("failed to list merge requests: %w", err)
		}
		envSecrets = append(envSecrets, data.Secrets...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	opts = github.ListOptions{PerPage: PageSize}
	for {
		data, resp, err := githubClient.Actions.ListRepoSecrets(context.Background(), repo.Namespace, repo.Name, &opts)
		if err != nil {
			return result, fmt.Errorf("failed to list merge requests: %w", err)
		}
		envSecrets = append(envSecrets, data.Secrets...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	for _, v := range envSecrets {
		result = append(result, api.CIVariable{
			Name:      v.Name,
			Value:     "",
			IsSecret:  true,
			CreatedAt: v.CreatedAt.GetTime(),
			UpdatedAt: v.UpdatedAt.GetTime(),
		})
	}

	return result, nil
}

func Environments(repo api.Repository, githubClient *github.Client) ([]api.CIEnvironment, error) {
	var result []api.CIEnvironment
	opts := github.ListOptions{PerPage: PageSize}

	var environments []*github.Environment
	for {
		data, resp, err := githubClient.Repositories.ListEnvironments(context.Background(), repo.Namespace, repo.Name, &github.EnvironmentListOptions{ListOptions: opts})
		if err != nil {
			return result, fmt.Errorf("failed to list environments: %w", err)
		}
		environments = append(environments, data.Environments...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	for _, v := range environments {
		result = append(result, api.CIEnvironment{
			ID:          v.GetID(),
			Name:        v.GetName(),
			Description: "",
			Tier:        v.GetEnvironmentName(),
			CreatedAt:   v.CreatedAt.GetTime(),
			UpdatedAt:   v.UpdatedAt.GetTime(),
		})
	}

	return result, nil
}
