package taskcommon

import (
	"github.com/cidverse/go-vcsapp/pkg/platform/api"
)

type TaskContext struct {
	Directory  string
	Platform   api.Platform
	Repository api.Repository
}

// Task provides a interface to implement tasks
type Task interface {
	Name() string
	Execute(ctx TaskContext) error
}
