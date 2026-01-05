package state

import (
	"errors"
	"fmt"
)

type RunState string

const (
	RunStateCreated         RunState = "CREATED"
	RunStatePlanning        RunState = "PLANNING"
	RunStatePlanFailed      RunState = "PLAN_FAILED"
	RunStateQueued          RunState = "QUEUED"
	RunStateRunning         RunState = "RUNNING"
	RunStateCancelRequested RunState = "CANCEL_REQUESTED"
	RunStateSuccess         RunState = "SUCCESS"
	RunStateFailed          RunState = "FAILED"
	RunStateCanceled        RunState = "CANCELED"
	RunStateReported        RunState = "REPORTED"
	RunStateTimeout         RunState = "TIMEOUT"
)

var runTransitions = map[RunState][]RunState{
	RunStateCreated:         {RunStateCreated, RunStatePlanning},
	RunStatePlanning:        {RunStatePlanning, RunStateQueued, RunStatePlanFailed},
	RunStatePlanFailed:      {RunStatePlanFailed, RunStateFailed},
	RunStateQueued:          {RunStateQueued, RunStateRunning},
	RunStateRunning:         {RunStateRunning, RunStateSuccess, RunStateFailed, RunStateCancelRequested, RunStateTimeout},
	RunStateCancelRequested: {RunStateCancelRequested, RunStateCanceled},
	RunStateSuccess:         {RunStateSuccess, RunStateReported},
	RunStateFailed:          {RunStateFailed, RunStateReported},
	RunStateCanceled:        {RunStateCanceled, RunStateReported},
	RunStateTimeout:         {RunStateTimeout, RunStateReported},
	RunStateReported:        {RunStateReported},
}

type JobState string

const (
	JobStateCreated         JobState = "CREATED"
	JobStateQueued          JobState = "QUEUED"
	JobStateLeased          JobState = "LEASED"
	JobStateStarting        JobState = "STARTING"
	JobStateRunning         JobState = "RUNNING"
	JobStateUploading       JobState = "UPLOADING"
	JobStateSucceeded       JobState = "SUCCEEDED"
	JobStateFailed          JobState = "FAILED"
	JobStateCancelRequested JobState = "CANCEL_REQUESTED"
	JobStateCanceled        JobState = "CANCELED"
	JobStateTimedOut        JobState = "TIMED_OUT"
	JobStateStale           JobState = "STALE"
)

var jobTransitions = map[JobState][]JobState{
	JobStateCreated:         {JobStateCreated, JobStateQueued},
	JobStateQueued:          {JobStateQueued, JobStateLeased, JobStateCancelRequested},
	JobStateLeased:          {JobStateLeased, JobStateStarting, JobStateQueued, JobStateCancelRequested, JobStateStale},
	JobStateStarting:        {JobStateStarting, JobStateRunning, JobStateQueued, JobStateCancelRequested, JobStateStale},
	JobStateRunning:         {JobStateRunning, JobStateUploading, JobStateTimedOut, JobStateQueued, JobStateCancelRequested, JobStateStale},
	JobStateUploading:       {JobStateUploading, JobStateSucceeded, JobStateFailed},
	JobStateSucceeded:       {JobStateSucceeded},
	JobStateFailed:          {JobStateFailed, JobStateQueued},
	JobStateCancelRequested: {JobStateCancelRequested, JobStateCanceled},
	JobStateCanceled:        {JobStateCanceled},
	JobStateTimedOut:        {JobStateTimedOut, JobStateQueued},
	JobStateStale:           {JobStateStale},
}

type LeaseState string

const (
	LeaseStateGranted   LeaseState = "GRANTED"
	LeaseStateActive    LeaseState = "ACTIVE"
	LeaseStateExpired   LeaseState = "EXPIRED"
	LeaseStateRevoked   LeaseState = "REVOKED"
	LeaseStateCompleted LeaseState = "COMPLETED"
	LeaseStateCanceled  LeaseState = "CANCELED"
)

var leaseTransitions = map[LeaseState][]LeaseState{
	LeaseStateGranted:   {LeaseStateGranted, LeaseStateActive, LeaseStateExpired, LeaseStateRevoked},
	LeaseStateActive:    {LeaseStateActive, LeaseStateExpired, LeaseStateCompleted, LeaseStateCanceled, LeaseStateRevoked},
	LeaseStateExpired:   {LeaseStateExpired},
	LeaseStateRevoked:   {LeaseStateRevoked},
	LeaseStateCompleted: {LeaseStateCompleted},
	LeaseStateCanceled:  {LeaseStateCanceled},
}

// TransitionError signals an illegal state transition detected in the persistence layer.
type TransitionError struct {
	Entity string
	ID     string
	From   string
	To     string
}

func (e TransitionError) Error() string {
	return fmt.Sprintf("%s %s: invalid transition from %s to %s", e.Entity, e.ID, e.From, e.To)
}

// UnknownStateError signals a state value that is not part of the documented state machine.
type UnknownStateError struct {
	Entity string
	State  string
}

func (e UnknownStateError) Error() string {
	return fmt.Sprintf("%s: unknown state %q", e.Entity, e.State)
}

func validateRunTransition(id string, from, to RunState) error {
	allowed, ok := runTransitions[from]
	if !ok {
		return UnknownStateError{Entity: "run", State: string(from)}
	}
	if !containsRunState(to) {
		return UnknownStateError{Entity: "run", State: string(to)}
	}
	if !containsRunStateValue(allowed, to) {
		return TransitionError{Entity: "run", ID: id, From: string(from), To: string(to)}
	}
	return nil
}

func validateJobTransition(id string, from, to JobState) error {
	allowed, ok := jobTransitions[from]
	if !ok {
		return UnknownStateError{Entity: "job", State: string(from)}
	}
	if !containsJobState(to) {
		return UnknownStateError{Entity: "job", State: string(to)}
	}
	if !containsJobStateValue(allowed, to) {
		return TransitionError{Entity: "job", ID: id, From: string(from), To: string(to)}
	}
	return nil
}

func validateLeaseTransition(id string, from, to LeaseState) error {
	allowed, ok := leaseTransitions[from]
	if !ok {
		return UnknownStateError{Entity: "lease", State: string(from)}
	}
	if !containsLeaseState(to) {
		return UnknownStateError{Entity: "lease", State: string(to)}
	}
	if !containsLeaseStateValue(allowed, to) {
		return TransitionError{Entity: "lease", ID: id, From: string(from), To: string(to)}
	}
	return nil
}

func containsRunStateValue(list []RunState, target RunState) bool {
	for _, candidate := range list {
		if candidate == target {
			return true
		}
	}
	return false
}

func containsJobStateValue(list []JobState, target JobState) bool {
	for _, candidate := range list {
		if candidate == target {
			return true
		}
	}
	return false
}

func containsLeaseStateValue(list []LeaseState, target LeaseState) bool {
	for _, candidate := range list {
		if candidate == target {
			return true
		}
	}
	return false
}

func containsRunState(s RunState) bool {
	_, ok := runTransitions[s]
	return ok
}

func containsJobState(s JobState) bool {
	_, ok := jobTransitions[s]
	return ok
}

func containsLeaseState(s LeaseState) bool {
	_, ok := leaseTransitions[s]
	return ok
}

func IsTransitionError(err error) bool {
	var te TransitionError
	return errors.As(err, &te)
}

func IsUnknownStateError(err error) bool {
	var ue UnknownStateError
	return errors.As(err, &ue)
}
