package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultListen        = ":8090"
	defaultOpenAIURL     = "https://api.openai.com/v1/chat/completions"
	defaultOpenAIModel   = "gpt-4o-mini"
	defaultPromptMaxSize = 12_000
	defaultOutputMaxLen  = 512
)

type explainRequest struct {
	Provider      string `json:"provider"`
	Model         string `json:"model"`
	PromptVersion string `json:"prompt_version"`
	Prompt        string `json:"prompt"`
}

type explainResponse struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Summary  string `json:"summary"`
	Details  string `json:"details,omitempty"`
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func main() {
	flags := flag.NewFlagSet("ai-proxy", flag.ExitOnError)
	listen := flags.String("listen", envString("AI_PROXY_LISTEN", defaultListen), "Listen address")
	openAIURL := flags.String("openai-url", envString("OPENAI_API_URL", defaultOpenAIURL), "OpenAI API URL")
	openAIModel := flags.String("openai-model", envString("OPENAI_MODEL", defaultOpenAIModel), "Default OpenAI model")
	openAIKey := flags.String("openai-api-key", envString("OPENAI_API_KEY", ""), "OpenAI API key")
	httpTimeout := flags.Duration("http-timeout", envDuration("AI_PROXY_HTTP_TIMEOUT", 5*time.Second), "HTTP timeout")
	maxPromptBytes := flags.Int("max-prompt-bytes", envInt("AI_PROXY_MAX_PROMPT_BYTES", defaultPromptMaxSize), "Max prompt bytes")
	maxOutputLen := flags.Int("max-output-len", envInt("AI_PROXY_MAX_OUTPUT_LEN", defaultOutputMaxLen), "Max output length")
	_ = flags.Parse(os.Args[1:])

	if strings.TrimSpace(*openAIKey) == "" {
		fmt.Fprintln(os.Stderr, "openai-api-key or OPENAI_API_KEY required")
		os.Exit(1)
	}

	client := &http.Client{Timeout: *httpTimeout}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{}))
	handler := newProxyHandler(client, *openAIURL, *openAIModel, *openAIKey, *maxPromptBytes, *maxOutputLen, logger)
	mux := http.NewServeMux()
	mux.Handle("/v1/explain", handler)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	server := &http.Server{
		Addr:              *listen,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	logger.Info("ai proxy started", "event", "ai_proxy_started", "listen", *listen)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("ai proxy failed", "event", "ai_proxy_failed", "error", err)
		os.Exit(1)
	}
}

func newProxyHandler(client *http.Client, apiURL, defaultModel, apiKey string, maxPromptBytes, maxOutputLen int, logger *slog.Logger) http.Handler {
	if client == nil {
		client = http.DefaultClient
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req explainRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		req.Prompt = strings.TrimSpace(req.Prompt)
		if req.Prompt == "" {
			writeError(w, http.StatusBadRequest, errors.New("prompt required"))
			return
		}
		prompt := truncateBytes(req.Prompt, maxPromptBytes)
		model := strings.TrimSpace(req.Model)
		if model == "" {
			model = defaultModel
		}
		if model == "" {
			writeError(w, http.StatusBadRequest, errors.New("model required"))
			return
		}

		payload := chatRequest{
			Model: model,
			Messages: []chatMessage{
				{Role: "user", Content: prompt},
			},
			Temperature: 0.2,
			MaxTokens:   200,
		}
		body, err := json.Marshal(payload)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		ctx := r.Context()
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(body))
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Authorization", "Bearer "+strings.TrimSpace(apiKey))

		resp, err := client.Do(httpReq)
		if err != nil {
			logger.Warn("ai proxy request failed", "event", "ai_proxy_request_failed", "error", err)
			writeError(w, http.StatusBadGateway, err)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
			logger.Warn("ai proxy request rejected", "event", "ai_proxy_rejected", "status", resp.StatusCode)
			writeError(w, http.StatusBadGateway, fmt.Errorf("ai provider status %d", resp.StatusCode))
			return
		}

		var decoded chatResponse
		if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		if len(decoded.Choices) == 0 {
			writeError(w, http.StatusBadGateway, errors.New("ai provider returned no choices"))
			return
		}

		content := strings.TrimSpace(decoded.Choices[0].Message.Content)
		content = sanitizeText(content, maxOutputLen)
		if content == "" {
			writeError(w, http.StatusBadGateway, errors.New("ai provider returned empty response"))
			return
		}

		provider := strings.TrimSpace(req.Provider)
		if provider == "" {
			provider = "openai"
		}

		writeJSON(w, http.StatusOK, explainResponse{
			Provider: provider,
			Model:    decoded.Model,
			Summary:  content,
		})
	})
}

func decodeJSON(r *http.Request, target any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(target)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func truncateBytes(value string, maxBytes int) string {
	if maxBytes <= 0 || len(value) <= maxBytes {
		return value
	}
	if maxBytes <= 3 {
		return value[:maxBytes]
	}
	return value[:maxBytes-3] + "..."
}

func sanitizeText(value string, maxLen int) string {
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.TrimSpace(value)
	value = strings.Join(strings.Fields(value), " ")
	if maxLen <= 0 || len(value) <= maxLen {
		return value
	}
	if maxLen <= 3 {
		return value[:maxLen]
	}
	return value[:maxLen-3] + "..."
}

func envString(name, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
}

func envInt(name string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envDuration(name string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}
