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
	return c.post(ctx, "/api/v1/internal/ack-lease", ack, nil)
}

func (c *HTTPClient) Heartbeat(ctx context.Context, hb protocol.Heartbeat) (protocol.HeartbeatAck, error) {
	var ack protocol.HeartbeatAck
	if err := c.post(ctx, "/api/v1/internal/heartbeat", hb, &ack); err != nil {
		return protocol.HeartbeatAck{}, err
	}
	return ack, nil
}

func (c *HTTPClient) Complete(ctx context.Context, complete protocol.Complete) error {
	return c.post(ctx, "/api/v1/internal/complete", complete, nil)
}

func (c *HTTPClient) CancelAck(ctx context.Context, cancel protocol.CancelAck) error {
	return c.post(ctx, "/api/v1/internal/cancel-ack", cancel, nil)
}

func (c *HTTPClient) post(ctx context.Context, path string, payload any, out any) error {
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
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
