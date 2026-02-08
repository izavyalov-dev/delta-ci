package orchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/izavyalov-dev/delta-ci/protocol"
	"github.com/izavyalov-dev/delta-ci/state"
)

const fixValidationTestCommand = "go test ./..."

// CreateFixSuggestionRequest describes an advisory patch candidate.
type CreateFixSuggestionRequest struct {
	JobID            string
	Provider         string
	Model            string
	PromptVersion    string
	Title            string
	Summary          string
	PatchUnifiedDiff string
	ValidateNow      bool
}

// CreateFixSuggestion stores an advisory fix patch for the latest job attempt.
func (s *Service) CreateFixSuggestion(ctx context.Context, req CreateFixSuggestionRequest) (state.FixSuggestion, error) {
	if strings.TrimSpace(req.JobID) == "" {
		return state.FixSuggestion{}, errors.New("job_id is required")
	}
	if strings.TrimSpace(req.Provider) == "" {
		return state.FixSuggestion{}, errors.New("provider is required")
	}
	if strings.TrimSpace(req.PromptVersion) == "" {
		return state.FixSuggestion{}, errors.New("prompt_version is required")
	}

	attempt, err := s.store.GetLatestJobAttempt(ctx, req.JobID)
	if err != nil {
		return state.FixSuggestion{}, err
	}

	suggestion, err := s.store.RecordFixSuggestion(ctx, state.FixSuggestion{
		JobAttemptID:     attempt.ID,
		Provider:         strings.TrimSpace(req.Provider),
		Model:            strings.TrimSpace(req.Model),
		PromptVersion:    strings.TrimSpace(req.PromptVersion),
		Title:            strings.TrimSpace(req.Title),
		Summary:          strings.TrimSpace(req.Summary),
		PatchFormat:      state.FixSuggestionPatchFormatUnifiedDiff,
		PatchUnifiedDiff: req.PatchUnifiedDiff,
		RequiresApproval: true,
		ValidationStatus: state.FixSuggestionValidationPending,
	})
	if err != nil {
		return state.FixSuggestion{}, err
	}

	if req.ValidateNow {
		return s.QueueFixSuggestionValidation(ctx, suggestion.ID)
	}
	return suggestion, nil
}

// QueueFixSuggestionValidation creates an isolated validation run for a fix suggestion.
func (s *Service) QueueFixSuggestionValidation(ctx context.Context, suggestionID int64) (state.FixSuggestion, error) {
	if suggestionID <= 0 {
		return state.FixSuggestion{}, errors.New("suggestion id must be > 0")
	}

	suggestion, err := s.store.GetFixSuggestion(ctx, suggestionID)
	if err != nil {
		return state.FixSuggestion{}, err
	}
	if suggestion.ValidationJobID != nil && suggestion.ValidationRunID != nil {
		return suggestion, nil
	}

	sourceAttempt, err := s.store.GetJobAttempt(ctx, suggestion.JobAttemptID)
	if err != nil {
		return state.FixSuggestion{}, err
	}
	sourceJob, err := s.store.GetJob(ctx, sourceAttempt.JobID)
	if err != nil {
		return state.FixSuggestion{}, err
	}
	sourceRun, err := s.store.GetRun(ctx, sourceJob.RunID)
	if err != nil {
		return state.FixSuggestion{}, err
	}

	validationRunID := s.ids.RunID()
	validationRun, err := s.store.CreateRun(ctx, state.Run{
		ID:        validationRunID,
		RepoID:    sourceRun.RepoID,
		Ref:       sourceRun.Ref,
		CommitSHA: sourceRun.CommitSHA,
		State:     state.RunStateCreated,
	})
	if err != nil {
		return state.FixSuggestion{}, err
	}
	if err := s.store.TransitionRunState(ctx, validationRun.ID, state.RunStatePlanning); err != nil {
		return state.FixSuggestion{}, err
	}

	validationJobID := s.ids.JobID()
	validationJobName := fmt.Sprintf("validate-fix-%d", suggestion.ID)
	validationJob, err := s.store.CreateJob(ctx, state.Job{
		ID:       validationJobID,
		RunID:    validationRun.ID,
		Name:     validationJobName,
		Required: true,
		State:    state.JobStateCreated,
		Reason:   fmt.Sprintf("validation run for fix suggestion #%d", suggestion.ID),
	})
	if err != nil {
		return state.FixSuggestion{}, err
	}

	spec := protocol.JobSpec{
		Name:    validationJobName,
		Workdir: ".",
		Steps: []string{
			buildFixValidationStep(suggestion.PatchUnifiedDiff),
		},
	}
	specJSON, err := json.Marshal(spec)
	if err != nil {
		return state.FixSuggestion{}, err
	}
	if err := s.store.RecordJobSpec(ctx, validationJob.ID, specJSON); err != nil {
		return state.FixSuggestion{}, err
	}

	validationAttemptID := s.ids.JobAttemptID()
	validationAttempt, err := s.store.CreateJobAttempt(ctx, state.JobAttempt{
		ID:            validationAttemptID,
		JobID:         validationJob.ID,
		AttemptNumber: 1,
		State:         state.JobStateCreated,
	})
	if err != nil {
		return state.FixSuggestion{}, err
	}

	if err := s.queueJobAttempt(ctx, &validationJob, &validationAttempt, nil); err != nil {
		return state.FixSuggestion{}, err
	}
	if err := s.store.TransitionRunState(ctx, validationRun.ID, state.RunStateQueued); err != nil {
		return state.FixSuggestion{}, err
	}
	if err := s.store.AttachFixSuggestionValidation(ctx, suggestion.ID, validationRun.ID, validationJob.ID); err != nil {
		return state.FixSuggestion{}, err
	}

	updated, err := s.store.GetFixSuggestion(ctx, suggestion.ID)
	if err != nil {
		return state.FixSuggestion{}, err
	}
	return updated, nil
}

func buildFixValidationStep(unifiedDiff string) string {
	patch := strings.TrimSpace(unifiedDiff)
	if patch == "" {
		patch = "--- a/empty\n+++ b/empty\n@@ -0,0 +1 @@\n+"
	}
	if !strings.HasSuffix(patch, "\n") {
		patch += "\n"
	}

	delim := fixPatchDelimiter(patch)
	var b strings.Builder
	b.WriteString("set -eu\n")
	b.WriteString("mkdir -p .delta-ci\n")
	fmt.Fprintf(&b, "cat > .delta-ci/fix.patch <<'%s'\n", delim)
	b.WriteString(patch)
	fmt.Fprintf(&b, "%s\n", delim)
	b.WriteString("git apply --check .delta-ci/fix.patch\n")
	b.WriteString("git apply .delta-ci/fix.patch\n")
	b.WriteString(fixValidationTestCommand)
	return b.String()
}

func fixPatchDelimiter(patch string) string {
	sum := sha256.Sum256([]byte(patch))
	base := "DELTA_CI_PATCH_" + strings.ToUpper(hex.EncodeToString(sum[:4]))
	delim := base
	counter := 0
	for containsDelimiterLine(patch, delim) {
		counter++
		delim = fmt.Sprintf("%s_%d", base, counter)
	}
	return delim
}

func containsDelimiterLine(body, delim string) bool {
	line := "\n" + delim + "\n"
	return strings.Contains("\n"+body+"\n", line)
}
