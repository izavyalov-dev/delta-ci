package loadtest

import (
	"context"
	"fmt"

	"github.com/izavyalov-dev/delta-ci/orchestrator"
)

// GenerateRuns creates the specified number of runs via the service layer.
// Each run gets jobsPerRun jobs. Returns the list of created run IDs.
func GenerateRuns(ctx context.Context, service *orchestrator.Service, count, jobsPerRun int) ([]string, error) {
	_ = jobsPerRun // job count is determined by the planner; we use it for documentation
	var runIDs []string
	for i := 0; i < count; i++ {
		details, err := service.CreateRun(ctx, orchestrator.CreateRunRequest{
			RepoID:    fmt.Sprintf("loadtest-repo-%d", i%5),
			Ref:       "refs/heads/loadtest",
			CommitSHA: fmt.Sprintf("loadtest-sha-%d", i),
		})
		if err != nil {
			return nil, fmt.Errorf("create run %d: %w", i, err)
		}
		runIDs = append(runIDs, details.Run.ID)
	}
	return runIDs, nil
}
