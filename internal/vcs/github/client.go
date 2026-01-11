package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const defaultBaseURL = "https://api.github.com"

// APIError captures non-2xx responses from GitHub.
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("github api error: status=%d message=%s", e.StatusCode, e.Message)
}

// Client is a minimal GitHub API client for checks and comments.
type Client struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
	UserAgent  string
}

// NewClient constructs a GitHub client.
func NewClient(token string) *Client {
	return &Client{
		BaseURL:    defaultBaseURL,
		Token:      token,
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
		UserAgent:  "delta-ci",
	}
}

// CheckRunRequest describes a check run payload.
type CheckRunRequest struct {
	Name        string    `json:"name"`
	HeadSHA     string    `json:"head_sha"`
	Status      string    `json:"status,omitempty"`
	Conclusion  string    `json:"conclusion,omitempty"`
	StartedAt   time.Time `json:"started_at,omitempty"`
	CompletedAt time.Time `json:"completed_at,omitempty"`
	Output      struct {
		Title   string `json:"title"`
		Summary string `json:"summary"`
	} `json:"output"`
}

// CheckRunResponse captures the check run ID.
type CheckRunResponse struct {
	ID int64 `json:"id"`
}

// CommentRequest describes an issue comment payload.
type CommentRequest struct {
	Body string `json:"body"`
}

// CommentResponse captures the comment ID.
type CommentResponse struct {
	ID int64 `json:"id"`
}

func (c *Client) CreateCheckRun(ctx context.Context, owner, repo string, payload CheckRunRequest) (CheckRunResponse, error) {
	path := fmt.Sprintf("/repos/%s/%s/check-runs", owner, repo)
	var resp CheckRunResponse
	if err := c.doJSON(ctx, http.MethodPost, path, payload, &resp); err != nil {
		return CheckRunResponse{}, err
	}
	return resp, nil
}

func (c *Client) UpdateCheckRun(ctx context.Context, owner, repo, checkRunID string, payload CheckRunRequest) (CheckRunResponse, error) {
	path := fmt.Sprintf("/repos/%s/%s/check-runs/%s", owner, repo, checkRunID)
	var resp CheckRunResponse
	if err := c.doJSON(ctx, http.MethodPatch, path, payload, &resp); err != nil {
		return CheckRunResponse{}, err
	}
	return resp, nil
}

func (c *Client) CreateComment(ctx context.Context, owner, repo string, prNumber int, body string) (CommentResponse, error) {
	path := fmt.Sprintf("/repos/%s/%s/issues/%d/comments", owner, repo, prNumber)
	var resp CommentResponse
	if err := c.doJSON(ctx, http.MethodPost, path, CommentRequest{Body: body}, &resp); err != nil {
		return CommentResponse{}, err
	}
	return resp, nil
}

func (c *Client) UpdateComment(ctx context.Context, owner, repo, commentID string, body string) (CommentResponse, error) {
	path := fmt.Sprintf("/repos/%s/%s/issues/comments/%s", owner, repo, commentID)
	var resp CommentResponse
	if err := c.doJSON(ctx, http.MethodPatch, path, CommentRequest{Body: body}, &resp); err != nil {
		return CommentResponse{}, err
	}
	return resp, nil
}

func (c *Client) doJSON(ctx context.Context, method, path string, payload any, out any) error {
	if c == nil {
		return errors.New("github client is nil")
	}
	if c.Token == "" {
		return errors.New("github token missing")
	}

	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}

	base := strings.TrimRight(c.BaseURL, "/")
	req, err := http.NewRequestWithContext(ctx, method, base+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("User-Agent", c.UserAgent)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &APIError{StatusCode: resp.StatusCode, Message: string(respBody)}
	}
	if out != nil {
		if err := json.Unmarshal(respBody, out); err != nil {
			return err
		}
	}
	return nil
}
