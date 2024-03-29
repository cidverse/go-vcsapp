package vcsapp

import (
	"fmt"
	"os"

	"github.com/cidverse/go-vcsapp/pkg/platform/api"
	"github.com/cidverse/go-vcsapp/pkg/task/taskcommon"
	"github.com/rs/zerolog/log"
)

func ExecuteTasks(platform api.Platform, tasks []taskcommon.Task) error {
	// list repositories
	repos, err := platform.Repositories(api.RepositoryListOpts{
		IncludeBranches:   true,
		IncludeCommitHash: true,
	})
	if err != nil {
		return fmt.Errorf("failed to list repositories: %w", err)
	}

	// log task names
	var taskNames []string
	for _, task := range tasks {
		taskNames = append(taskNames, task.Name())
	}
	log.Info().Int("repo_count", len(repos)).Strs("tasks", taskNames).Msg("executing tasks")

	// iterate over repositories and execute tasks
	for _, repo := range repos {
		for _, task := range tasks {
			err = ExecuteTask(platform, task, repo)
			if err != nil {
				log.Warn().Msg("failed to execute task: " + task.Name() + " for repository " + repo.Namespace + "/" + repo.Name + ": " + err.Error())
			}
		}
	}

	return nil
}

func ExecuteTask(platform api.Platform, task taskcommon.Task, repo api.Repository) error {
	// create temp directory
	tempDir, err := os.MkdirTemp("", "vcs-app-*")
	if err != nil {
		return fmt.Errorf("failed to prepare temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// execute task
	err = task.Execute(taskcommon.TaskContext{
		Directory:  tempDir,
		Platform:   platform,
		Repository: repo,
	})
	if err != nil {
		return err
	}

	return nil
}
