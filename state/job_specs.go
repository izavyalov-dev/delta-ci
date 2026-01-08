package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// RecordJobSpec stores the job specification JSON for recovery.
func (s *Store) RecordJobSpec(ctx context.Context, jobID string, specJSON []byte) error {
	if jobID == "" {
		return errors.New("job id required")
	}
	if len(specJSON) == 0 {
		return errors.New("spec json required")
	}

	_, err := s.db.ExecContext(ctx, `
INSERT INTO job_specs (job_id, spec_json)
VALUES ($1, $2)
ON CONFLICT (job_id) DO NOTHING
`, jobID, specJSON)
	return err
}

// GetJobSpec fetches the job specification JSON.
func (s *Store) GetJobSpec(ctx context.Context, jobID string) ([]byte, error) {
	var spec []byte
	err := s.db.QueryRowContext(ctx, `
SELECT spec_json
FROM job_specs
WHERE job_id = $1
`, jobID).Scan(&spec)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: job spec for job %s", ErrNotFound, jobID)
		}
		return nil, err
	}
	return spec, nil
}
