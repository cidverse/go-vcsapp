package simpletask

import (
	"fmt"

	"github.com/cidverse/go-vcs"
	"github.com/cidverse/go-vcs/vcsapi"
	"github.com/cidverse/vcs-app/pkg/task/taskcommon"
	"github.com/rs/zerolog/log"
)

type SimpleTask struct {
	ctx        taskcommon.TaskContext
	VCSClient  vcsapi.Client
	BranchName string
}

// Clone clones the repository and initializes the vcs client
func (n *SimpleTask) Clone() error {
	// clone repository
	vcsClient, err := vcs.GetVCSClientCloneRemote(n.ctx.Repository.CloneURL, n.ctx.Directory, n.ctx.Repository.DefaultBranch, n.ctx.Platform.AuthMethod(n.ctx.Repository))
	if err != nil {
		return fmt.Errorf("failed to get instantiate vcs client: %w", err)
	}

	n.VCSClient = vcsClient
	return nil
}

// CreateBranch creates a new branch
func (n *SimpleTask) CreateBranch(branchName string) error {
	if n.VCSClient == nil {
		return fmt.Errorf("vcs client is nil, call Clone first to initialize the vcs client")
	}

	// create and checkout new branch
	if err := n.VCSClient.CreateBranch(branchName); err != nil {
		return fmt.Errorf("failed to create branch: %w", err)
	}
	n.BranchName = branchName

	return nil
}

// CommitPushAndMergeRequest commits and pushes the changes, additionally creates or updates the merge request
func (n *SimpleTask) CommitPushAndMergeRequest(commitMessage string, mergeRequestTitle string, mergeRequestDescription string, mergeRequestKey string) error {
	if n.VCSClient == nil {
		return fmt.Errorf("vcs client is nil, call Clone first to initialize the vcs client")
	}
	if n.BranchName == "" {
		return fmt.Errorf("branch name is empty, call CreateBranch first")
	}

	// commit and push if changes are present
	isClean, err := n.VCSClient.IsClean()
	if err != nil {
		return fmt.Errorf("failed to check if repository is clean: %w", err)
	}
	if !isClean {
		head, err := n.VCSClient.VCSHead()
		if err != nil {
			return fmt.Errorf("failed to get head: %w", err)
		}
		err = n.ctx.Platform.CommitAndPush(n.ctx.Repository, head.Hash, n.BranchName, commitMessage, n.ctx.Directory)
		if err != nil {
			return fmt.Errorf("failed to commit and push: %w", err)
		}
		log.Info().Str("branch", n.BranchName).Msg("pushed changes to remote")
	}

	// create or update merge request
	err = n.ctx.Platform.CreateOrUpdateMergeRequest(n.ctx.Repository, n.BranchName, mergeRequestTitle, mergeRequestDescription, mergeRequestKey)
	if err != nil {
		return err
	}
	log.Info().Msg("created / updated merge request")

	return nil
}

// New creates a new instance of the basic task helper
func New(ctx taskcommon.TaskContext) SimpleTask {
	entity := SimpleTask{
		ctx: ctx,
	}

	return entity
}
