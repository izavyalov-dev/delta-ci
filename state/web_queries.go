package state

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// RunListFilter describes filtering and pagination for run lists.
type RunListFilter struct {
	Repo   string
	State  string
	Limit  int
	Offset int
}

// RunListItem is a lightweight run record with job count for list views.
type RunListItem struct {
	Run      Run
	JobCount int
}

// ListRuns returns paginated runs with job counts, filtered by optional repo and state.
func (s *Store) ListRuns(ctx context.Context, filter RunListFilter) ([]RunListItem, int, error) {
	if filter.Limit <= 0 {
		filter.Limit = 25
	}
	if filter.Limit > 100 {
		filter.Limit = 100
	}

	var conditions []string
	var args []any
	argIdx := 1

	if filter.Repo != "" {
		conditions = append(conditions, fmt.Sprintf("r.repo_id = $%d", argIdx))
		args = append(args, filter.Repo)
		argIdx++
	}
	if filter.State != "" {
		conditions = append(conditions, fmt.Sprintf("r.state = $%d", argIdx))
		args = append(args, filter.State)
		argIdx++
	}

	where := ""
	if len(conditions) > 0 {
		where = "WHERE " + strings.Join(conditions, " AND ")
	}

	// Count total
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM runs r %s", where)
	var total int
	if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Fetch runs with job counts
	query := fmt.Sprintf(`
SELECT r.id, r.repo_id, r.ref, r.commit_sha, r.state, r.created_at, r.updated_at,
       COALESCE(j.job_count, 0)
FROM runs r
LEFT JOIN (SELECT run_id, COUNT(*) AS job_count FROM jobs GROUP BY run_id) j ON j.run_id = r.id
%s
ORDER BY r.created_at DESC, r.id DESC
LIMIT $%d OFFSET $%d
`, where, argIdx, argIdx+1)
	args = append(args, filter.Limit, filter.Offset)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []RunListItem
	for rows.Next() {
		var item RunListItem
		if err := rows.Scan(
			&item.Run.ID, &item.Run.RepoID, &item.Run.Ref, &item.Run.CommitSHA,
			&item.Run.State, &item.Run.CreatedAt, &item.Run.UpdatedAt,
			&item.JobCount,
		); err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}

	return items, total, rows.Err()
}

// CountRunsByState returns aggregate run counts grouped by state.
func (s *Store) CountRunsByState(ctx context.Context) (map[RunState]int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT state, COUNT(*) FROM runs GROUP BY state`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[RunState]int)
	for rows.Next() {
		var stateVal RunState
		var count int
		if err := rows.Scan(&stateVal, &count); err != nil {
			return nil, err
		}
		counts[stateVal] = count
	}
	return counts, rows.Err()
}

// ListDistinctRepos returns all unique repo_id values.
func (s *Store) ListDistinctRepos(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT repo_id FROM runs ORDER BY repo_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var repos []string
	for rows.Next() {
		var repo string
		if err := rows.Scan(&repo); err != nil {
			return nil, err
		}
		repos = append(repos, repo)
	}
	return repos, rows.Err()
}

// CountActiveLeases returns the number of GRANTED or ACTIVE leases.
func (s *Store) CountActiveLeases(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `
SELECT COUNT(*) FROM leases WHERE state IN ($1, $2)
`, LeaseStateGranted, LeaseStateActive).Scan(&count)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, err
	}
	return count, nil
}
