package orchestrator

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// HTTPAIClient calls a JSON HTTP endpoint to fetch AI explanations.
type HTTPAIClient struct {
	Endpoint   string
	Token      string
	HTTPClient *http.Client
}

func (c *HTTPAIClient) ExplainFailure(ctx context.Context, req AIRequest) (AIResponse, error) {
	endpoint := strings.TrimSpace(c.Endpoint)
	if endpoint == "" {
		return AIResponse{}, ErrAIUnavailable
	}

	payload := map[string]string{
		"provider":       req.Provider,
		"model":          req.Model,
		"prompt_version": req.PromptVersion,
		"prompt":         req.Prompt,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return AIResponse{}, err
	}

	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return AIResponse{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	if token := strings.TrimSpace(c.Token); token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := httpClient.Do(request)
	if err != nil {
		return AIResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return AIResponse{}, fmt.Errorf("ai endpoint status %d", resp.StatusCode)
	}

	var decoded struct {
		Provider string `json:"provider"`
		Model    string `json:"model"`
		Summary  string `json:"summary"`
		Details  string `json:"details"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return AIResponse{}, err
	}
	if strings.TrimSpace(decoded.Summary) == "" {
		return AIResponse{}, errors.New("ai response missing summary")
	}
	return AIResponse{
		Provider: decoded.Provider,
		Model:    decoded.Model,
		Summary:  decoded.Summary,
		Details:  decoded.Details,
	}, nil
}
