package taskcli

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/cidverse/go-vcs"
	"github.com/cidverse/vcs-app/pkg/task/taskcommon"
	"github.com/rs/zerolog/log"
)

type CLITask struct {
	name                  string
	branchNameTemplate    string
	commitMessageTemplate string
	script                []string
}

// Name returns the name of the task
func (n CLITask) Name() string {
	return n.name
}

// Execute runs the task
func (n CLITask) Execute(ctx taskcommon.TaskContext) error {
	// clone repository
	vcsClient, err := vcs.GetVCSClientCloneRemote(ctx.Repository.CloneURL, ctx.Directory, ctx.Repository.DefaultBranch, ctx.Platform.AuthMethod(ctx.Repository))
	if err != nil {
		return fmt.Errorf("failed to get instantiate vcs client: %w", err)
	}

	// create and checkout new branch
	if err := vcsClient.CreateBranch(n.branchNameTemplate); err != nil {
		return fmt.Errorf("failed to create branch: %w", err)
	}

	// run tasks
	for _, line := range n.script {
		cmd := exec.Command("bash", "-c", line)
		cmd.Dir = ctx.Directory
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Printf("Error executing script: %v\n", err)
		}
	}

	// skip, if no files have changed
	isClean, err := vcsClient.IsClean()
	if err != nil {
		return fmt.Errorf("failed to check if repository is clean: %w", err)
	}
	if isClean {
		log.Info().Msg("no changes detected, skipping repository")
		return nil
	}

	// platform
	head, err := vcsClient.VCSHead()
	if err != nil {
		return fmt.Errorf("failed to get head: %w", err)
	}
	err = ctx.Platform.CommitAndPush(ctx.Repository, head.Hash, n.branchNameTemplate, n.commitMessageTemplate, ctx.Directory)
	if err != nil {
		return fmt.Errorf("failed to commit and push: %w", err)
	}

	// create merge request
	err = ctx.Platform.CreateMergeRequest(ctx.Repository, n.branchNameTemplate, n.commitMessageTemplate, "This is a test merge request.")
	if err != nil {
		return fmt.Errorf("failed to create merge request: %w", err)
	}

	return nil
}

// NewCLITask creates a new task
func NewCLITask(name string, script []string) CLITask {
	entity := CLITask{
		name:                  "My Task",
		branchNameTemplate:    "chore/test-app",
		commitMessageTemplate: "chore: test commit",
		script:                script,
	}

	return entity
}
