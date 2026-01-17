package github

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const (
	EventPing        = "ping"
	EventPush        = "push"
	EventPullRequest = "pull_request"
)

// WebhookEvent captures normalized webhook data used to create runs.
type WebhookEvent struct {
	EventType string
	RepoID    string
	RepoOwner string
	RepoName  string
	Ref       string
	CommitSHA string
	PRNumber  *int
}

// VerifySignature checks a GitHub webhook signature header against the payload.
func VerifySignature(secret string, body []byte, signatureHeader string) (bool, error) {
	if secret == "" {
		return false, errors.New("webhook secret is empty")
	}
	if signatureHeader == "" {
		return false, errors.New("signature header missing")
	}

	parts := strings.SplitN(signatureHeader, "=", 2)
	if len(parts) != 2 {
		return false, errors.New("signature header malformed")
	}
	algo := parts[0]
	sigHex := parts[1]
	sigBytes, err := hex.DecodeString(sigHex)
	if err != nil {
		return false, fmt.Errorf("signature hex decode failed: %w", err)
	}

	var mac []byte
	switch algo {
	case "sha1":
		h := hmac.New(sha1.New, []byte(secret))
		_, _ = h.Write(body)
		mac = h.Sum(nil)
	case "sha256":
		h := hmac.New(sha256.New, []byte(secret))
		_, _ = h.Write(body)
		mac = h.Sum(nil)
	default:
		return false, fmt.Errorf("unsupported signature algorithm %q", algo)
	}

	return hmac.Equal(mac, sigBytes), nil
}

// NormalizeEvent parses a GitHub webhook payload into a normalized event.
// The boolean result indicates whether the event should trigger a run.
func NormalizeEvent(eventType string, body []byte) (WebhookEvent, bool, error) {
	switch eventType {
	case EventPing:
		return WebhookEvent{}, false, nil
	case EventPush:
		return normalizePush(body)
	case EventPullRequest:
		return normalizePullRequest(body)
	default:
		return WebhookEvent{}, false, nil
	}
}

// ComputeEventKey derives a deterministic idempotency key for webhook events.
func ComputeEventKey(repoID, commitSHA, eventType string, prNumber *int) (string, error) {
	if repoID == "" || commitSHA == "" || eventType == "" {
		return "", errors.New("repo_id, commit_sha, and event_type required")
	}
	prValue := ""
	if prNumber != nil {
		prValue = fmt.Sprintf("%d", *prNumber)
	}
	payload := fmt.Sprintf("%s|%s|%s|%s", repoID, commitSHA, eventType, prValue)
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:]), nil
}

type repoRef struct {
	FullName string `json:"full_name"`
	Name     string `json:"name"`
	Owner    struct {
		Login string `json:"login"`
	} `json:"owner"`
}

type pushEvent struct {
	Ref        string  `json:"ref"`
	After      string  `json:"after"`
	Deleted    bool    `json:"deleted"`
	Repository repoRef `json:"repository"`
}

func normalizePush(body []byte) (WebhookEvent, bool, error) {
	var evt pushEvent
	if err := json.Unmarshal(body, &evt); err != nil {
		return WebhookEvent{}, false, fmt.Errorf("decode push event: %w", err)
	}
	if evt.Deleted || evt.After == "" || evt.Ref == "" {
		return WebhookEvent{}, false, nil
	}
	owner, name, repoID := normalizeRepo(evt.Repository)
	if owner == "" || name == "" || repoID == "" {
		return WebhookEvent{}, false, errors.New("push event missing repository metadata")
	}
	return WebhookEvent{
		EventType: EventPush,
		RepoID:    repoID,
		RepoOwner: owner,
		RepoName:  name,
		Ref:       evt.Ref,
		CommitSHA: evt.After,
	}, true, nil
}

type pullRequestEvent struct {
	Action      string `json:"action"`
	Number      int    `json:"number"`
	PullRequest struct {
		Head struct {
			SHA string `json:"sha"`
		} `json:"head"`
	} `json:"pull_request"`
	Repository repoRef `json:"repository"`
}

func normalizePullRequest(body []byte) (WebhookEvent, bool, error) {
	var evt pullRequestEvent
	if err := json.Unmarshal(body, &evt); err != nil {
		return WebhookEvent{}, false, fmt.Errorf("decode pull_request event: %w", err)
	}
	if evt.Number <= 0 || evt.PullRequest.Head.SHA == "" {
		return WebhookEvent{}, false, nil
	}
	if !isSupportedPRAction(evt.Action) {
		return WebhookEvent{}, false, nil
	}
	owner, name, repoID := normalizeRepo(evt.Repository)
	if owner == "" || name == "" || repoID == "" {
		return WebhookEvent{}, false, errors.New("pull_request event missing repository metadata")
	}
	ref := fmt.Sprintf("refs/pull/%d/head", evt.Number)
	prNumber := evt.Number
	return WebhookEvent{
		EventType: EventPullRequest,
		RepoID:    repoID,
		RepoOwner: owner,
		RepoName:  name,
		Ref:       ref,
		CommitSHA: evt.PullRequest.Head.SHA,
		PRNumber:  &prNumber,
	}, true, nil
}

func isSupportedPRAction(action string) bool {
	switch action {
	case "opened", "synchronize", "reopened":
		return true
	default:
		return false
	}
}

func normalizeRepo(repo repoRef) (owner string, name string, repoID string) {
	owner = strings.TrimSpace(repo.Owner.Login)
	name = strings.TrimSpace(repo.Name)
	repoID = strings.TrimSpace(repo.FullName)
	if repoID == "" && owner != "" && name != "" {
		repoID = owner + "/" + name
	}
	if (owner == "" || name == "") && repoID != "" {
		parts := strings.SplitN(repoID, "/", 2)
		if len(parts) == 2 {
			if owner == "" {
				owner = parts[0]
			}
			if name == "" {
				name = parts[1]
			}
		}
	}
	return owner, name, repoID
}
