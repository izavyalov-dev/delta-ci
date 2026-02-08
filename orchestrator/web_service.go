package orchestrator

import (
	"context"

	"github.com/izavyalov-dev/delta-ci/state"
)

// ListRuns returns a paginated, filterable list of run summaries.
func (s *Service) ListRuns(ctx context.Context, filter RunFilter) ([]RunSummary, int, error) {
	items, total, err := s.store.ListRuns(ctx, state.RunListFilter{
		Repo:   filter.Repo,
		State:  filter.State,
		Limit:  filter.Limit,
		Offset: filter.Offset,
	})
	if err != nil {
		return nil, 0, err
	}

	summaries := make([]RunSummary, 0, len(items))
	for _, item := range items {
		summaries = append(summaries, RunSummary{
			ID:        item.Run.ID,
			RepoID:    item.Run.RepoID,
			Ref:       item.Run.Ref,
			CommitSHA: item.Run.CommitSHA,
			State:     item.Run.State,
			JobCount:  item.JobCount,
			CreatedAt: item.Run.CreatedAt,
			UpdatedAt: item.Run.UpdatedAt,
		})
	}
	return summaries, total, nil
}

// GetSystemStats returns aggregate run counts for the dashboard.
func (s *Service) GetSystemStats(ctx context.Context) (SystemStats, error) {
	counts, err := s.store.CountRunsByState(ctx)
	if err != nil {
		return SystemStats{}, err
	}

	var stats SystemStats
	for stateVal, count := range counts {
		stats.Total += count
		switch stateVal {
		case state.RunStateRunning:
			stats.Running += count
		case state.RunStateQueued, state.RunStateCreated, state.RunStatePlanning:
			stats.Queued += count
		case state.RunStateSuccess, state.RunStateReported:
			stats.Succeeded += count
		case state.RunStateFailed, state.RunStatePlanFailed, state.RunStateTimeout:
			stats.Failed += count
		case state.RunStateCanceled, state.RunStateCancelRequested:
			stats.Canceled += count
		}
	}
	return stats, nil
}

// ListDistinctRepos returns all unique repository IDs.
func (s *Service) ListDistinctRepos(ctx context.Context) ([]string, error) {
	return s.store.ListDistinctRepos(ctx)
}
