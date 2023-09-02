# VCS App

> A library to create apps for version control platforms, specifically to automate pull request creation.

## Examples

- [CID Workflow App](https://github.com/cidverse/cid-app)
- [PrimeLib Generator App](https://github.com/primelib/primelib-app)

## Usage

### Create Tasks

```go

type WorkflowTask struct {
}

// Name returns the name of the task
func (n WorkflowTask) Name() string {
    return "cid-workflow-update"
}

// Execute runs the task
func (n WorkflowTask) Execute(ctx taskcommon.TaskContext) error {
    helper := simpletask.New(ctx)

    // clone repository
    err := helper.Clone()
    if err != nil {
        return fmt.Errorf("failed to clone repository: %w", err)
    }

    // create and checkout new branch
    err = helper.CreateBranch("chore/my-branch-name")
    if err != nil {
        return fmt.Errorf("failed to create branch: %w", err)
    }

    // TODO: make file modifications here (in ctx.Directory, temp dir with a clean clone of the repository for every task)

    // commit, push and create merge request
    err = helper.CommitPushAndMergeRequest("chore: my change", "chore: my change", "my-description", "unique-key-to-prevent-duplicates")
    if err != nil {
        return fmt.Errorf("failed to commit push and create or update merge request: %w", err)
    }

    return nil
}
```

### Execute Tasks

```go
// platform
platform, err := vcsapp.GetPlatformFromEnvironment() // automatically configures the platform from environment variables, see below for details

// execute
err = vcsapp.ExecuteTasks(platform, []taskcommon.Task{
    WorkflowTask{},
})
```

## Configuration

You are *required* to have the environment variables for one platform set.

### GitHub App

Create a private key and store it in a file.

| Environment Variable          | Description                       |
|-------------------------------|-----------------------------------|
| `GITHUB_APP_ID`               | The ID of the GitHub App.         |
| `GITHUB_APP_PRIVATE_KEY_FILE` | The path to the private key file. |

### GitLab User

Create a GitLab user and generate a personal access token with the following permissions:

- api

| Environment Variable  | Description                |
|-----------------------|----------------------------|
| `GITLAB_SERVER`       | The GitLab server URL.     |
| `GITLAB_ACCESS_TOKEN` | The personal access token. |

## License

Released under the [MIT license](./LICENSE).
