package githubcommon

import (
	"context"
	"fmt"
	"net/http"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/google/go-github/v60/github"
)

// BranchSliceToNameSlice converts a slice of branches to a slice of branch names
func BranchSliceToNameSlice(branches []*github.Branch) []string {
	var branchNames []string
	for _, branch := range branches {
		branchNames = append(branchNames, branch.GetName())
	}

	return branchNames
}

// RoundTripperToAccessToken takes a ghinstallation round-tripper and obtains a new access token
func RoundTripperToAccessToken(rt http.RoundTripper) (string, error) {
	if rt == nil {
		return "", fmt.Errorf("round tripper is nil")
	}

	if v, ok := rt.(*ghinstallation.Transport); ok {
		token, err := v.Token(context.Background())
		if err != nil {
			return "", fmt.Errorf("failed to get token: %w", err)
		}

		return token, nil
	}

	return "", fmt.Errorf("round tripper is not a ghinstallation.Transport")
}
