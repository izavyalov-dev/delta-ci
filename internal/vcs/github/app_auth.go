package github

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	appTokenGracePeriod = 2 * time.Minute
)

// TokenProvider returns a short-lived token for GitHub API calls.
type TokenProvider interface {
	Token(ctx context.Context) (string, error)
}

// AppTokenProvider uses a GitHub App installation token.
type AppTokenProvider struct {
	appID         string
	installation  string
	privateKey    *rsa.PrivateKey
	baseURL       string
	httpClient    *http.Client
	userAgent     string
	mu            sync.Mutex
	cachedToken   string
	cachedExpires time.Time
}

// NewAppTokenProvider builds a provider for GitHub App installation tokens.
func NewAppTokenProvider(appID, installationID string, privateKeyPEM []byte, baseURL string) (*AppTokenProvider, error) {
	if appID == "" || installationID == "" {
		return nil, errors.New("github app id and installation id are required")
	}
	key, err := parsePrivateKey(privateKeyPEM)
	if err != nil {
		return nil, err
	}
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return &AppTokenProvider{
		appID:        appID,
		installation: installationID,
		privateKey:   key,
		baseURL:      strings.TrimRight(baseURL, "/"),
		httpClient:   &http.Client{Timeout: 10 * time.Second},
		userAgent:    "delta-ci",
	}, nil
}

// Token returns a cached installation token or fetches a new one.
func (p *AppTokenProvider) Token(ctx context.Context) (string, error) {
	p.mu.Lock()
	if p.cachedToken != "" && time.Until(p.cachedExpires) > appTokenGracePeriod {
		token := p.cachedToken
		p.mu.Unlock()
		return token, nil
	}
	p.mu.Unlock()

	token, expiresAt, err := p.fetchInstallationToken(ctx)
	if err != nil {
		return "", err
	}

	p.mu.Lock()
	p.cachedToken = token
	p.cachedExpires = expiresAt
	p.mu.Unlock()

	return token, nil
}

type appTokenResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (p *AppTokenProvider) fetchInstallationToken(ctx context.Context) (string, time.Time, error) {
	jwt, err := p.signJWT()
	if err != nil {
		return "", time.Time{}, err
	}

	url := fmt.Sprintf("%s/app/installations/%s/access_tokens", p.baseURL, p.installation)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return "", time.Time{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+jwt)
	req.Header.Set("User-Agent", p.userAgent)

	client := p.httpClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", time.Time{}, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", time.Time{}, &APIError{StatusCode: resp.StatusCode, Message: string(body)}
	}

	var payload appTokenResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", time.Time{}, err
	}
	if payload.Token == "" {
		return "", time.Time{}, errors.New("github app token response missing token")
	}
	if payload.ExpiresAt.IsZero() {
		payload.ExpiresAt = time.Now().UTC().Add(30 * time.Minute)
	}
	return payload.Token, payload.ExpiresAt, nil
}

func (p *AppTokenProvider) signJWT() (string, error) {
	now := time.Now().UTC()
	claims := map[string]any{
		"iss": p.appID,
		"iat": now.Add(-60 * time.Second).Unix(),
		"exp": now.Add(9 * time.Minute).Unix(),
	}
	header := map[string]any{
		"alg": "RS256",
		"typ": "JWT",
	}

	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}

	enc := base64.RawURLEncoding
	signingInput := enc.EncodeToString(headerJSON) + "." + enc.EncodeToString(claimsJSON)
	sum := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, p.privateKey, crypto.SHA256, sum[:])
	if err != nil {
		return "", err
	}
	return signingInput + "." + enc.EncodeToString(sig), nil
}

func parsePrivateKey(pemBytes []byte) (*rsa.PrivateKey, error) {
	if len(pemBytes) == 0 {
		return nil, errors.New("github app private key required")
	}
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("failed to decode github app private key")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	rsaKey, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("github app private key is not RSA")
	}
	return rsaKey, nil
}
