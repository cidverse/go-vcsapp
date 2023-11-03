package gitlabuser

import (
	"github.com/xanzy/go-gitlab"
)

// branchSliceToNameSlice converts a slice of branches to a slice of branch names
func branchSliceToNameSlice(branches []*gitlab.Branch) []string {
	var branchNames []string
	for _, branch := range branches {
		branchNames = append(branchNames, branch.Name)
	}

	return branchNames
}
