package orchestrator

import (
	"crypto/rand"
	"fmt"
	"time"
)

// IDGenerator produces opaque identifiers for runs, jobs, attempts, and leases.
type IDGenerator interface {
	RunID() string
	JobID() string
	JobAttemptID() string
	LeaseID() string
}

// RandomIDGenerator produces random, prefixed identifiers suitable for Phase 0.
type RandomIDGenerator struct{}

func (RandomIDGenerator) RunID() string        { return randomID("run") }
func (RandomIDGenerator) JobID() string        { return randomID("job") }
func (RandomIDGenerator) JobAttemptID() string { return randomID("attempt") }
func (RandomIDGenerator) LeaseID() string      { return randomID("lease") }

func randomID(prefix string) string {
	var b [10]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return fmt.Sprintf("%s_%x", prefix, b[:])
}
