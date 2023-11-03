package githubapp

import (
	"context"
	"fmt"
	"net/http"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/google/go-github/v56/github"
)

// branchSliceToNameSlice converts a slice of branches to a slice of branch names
func branchSliceToNameSlice(branches []*github.Branch) []string {
	var branchNames []string
	for _, branch := range branches {
		branchNames = append(branchNames, branch.GetName())
	}

	return branchNames
}

// roundTripperToAccessToken takes a ghinstallation round-tripper and obtains a new access token
func roundTripperToAccessToken(rt http.RoundTripper) (string, error) {
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
