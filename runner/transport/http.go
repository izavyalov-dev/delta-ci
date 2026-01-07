package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/izavyalov-dev/delta-ci/protocol"
)

type HTTPClient struct {
	baseURL string
	client  *http.Client
}

func NewHTTPClient(baseURL string) *HTTPClient {
	return &HTTPClient{
		baseURL: baseURL,
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (c *HTTPClient) AckLease(ctx context.Context, ack protocol.AckLease) error {
	return c.post(ctx, "/api/v1/internal/ack-lease", ack)
}

func (c *HTTPClient) Heartbeat(ctx context.Context, hb protocol.Heartbeat) error {
	return c.post(ctx, "/api/v1/internal/heartbeat", hb)
}

func (c *HTTPClient) Complete(ctx context.Context, complete protocol.Complete) error {
	return c.post(ctx, "/api/v1/internal/complete", complete)
}

func (c *HTTPClient) post(ctx context.Context, path string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status %s", resp.Status)
	}
	return nil
}
