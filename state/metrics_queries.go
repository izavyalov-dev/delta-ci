package state

import "context"

// QueueDepth returns the number of items currently in the dispatch queue.
func (s *Store) QueueDepth(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM job_queue`).Scan(&count)
	return count, err
}

// ActiveLeaseCount returns the number of leases in ACTIVE state.
func (s *Store) ActiveLeaseCount(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM leases WHERE state = 'ACTIVE'`).Scan(&count)
	return count, err
}
