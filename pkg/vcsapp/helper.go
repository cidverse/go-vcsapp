package vcsapp

import (
	"github.com/cidverse/go-ptr"
	"github.com/cidverse/go-vcsapp/pkg/platform/api"
	"github.com/rs/zerolog/log"
)

func ListMergeRequests(platform api.Platform) ([]api.MergeRequest, error) {
	var result []api.MergeRequest

	repos, err := platform.Repositories(api.RepositoryListOpts{})
	if err != nil {
		return nil, err
	}
	for _, repo := range repos {
		mrs, err := platform.MergeRequests(repo, api.MergeRequestSearchOptions{
			State:    ptr.Ptr(api.MergeRequestStateOpen),
			IsMerged: ptr.False(),
			IsDraft:  ptr.False(),
		})
		if err != nil {
			log.Warn().Err(err).Msgf("failed to get merge requests for repository %s", repo.Name)
			continue
		}

		for _, mr := range mrs {
			result = append(result, mr)
		}
	}

	return result, nil
}
