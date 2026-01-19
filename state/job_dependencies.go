package state

import (
	"context"
	"errors"
)

func (s *Store) RecordJobDependency(ctx context.Context, jobID, dependsOnJobID string) error {
	if jobID == "" || dependsOnJobID == "" {
		return errors.New("job and dependency ids required")
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO job_dependencies (job_id, depends_on_job_id)
VALUES ($1, $2)
ON CONFLICT (job_id, depends_on_job_id) DO NOTHING
`, jobID, dependsOnJobID)
	return err
}

func (s *Store) ListJobDependents(ctx context.Context, jobID string) ([]string, error) {
	if jobID == "" {
		return nil, errors.New("job id required")
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT job_id
FROM job_dependencies
WHERE depends_on_job_id = $1
ORDER BY job_id ASC
`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var dependents []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		dependents = append(dependents, id)
	}
	return dependents, rows.Err()
}

func (s *Store) DependenciesSatisfied(ctx context.Context, jobID string) (bool, error) {
	if jobID == "" {
		return false, errors.New("job id required")
	}
	var remaining int
	if err := s.db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM job_dependencies d
JOIN jobs j ON j.id = d.depends_on_job_id
WHERE d.job_id = $1
  AND j.state <> 'SUCCEEDED'
`, jobID).Scan(&remaining); err != nil {
		return false, err
	}
	return remaining == 0, nil
}
