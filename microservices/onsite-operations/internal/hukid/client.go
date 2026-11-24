package hukid

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

var ErrNotConfigured = errors.New("hukid verifier not configured")

type Client struct {
	BaseURL       string
	OperatorToken string
	HTTP          *http.Client
	RequireTLS    bool
}

type HTTPError struct{ Status int }

func (e *HTTPError) Error() string { return fmt.Sprintf("hukid verification http %d", e.Status) }

type VerificationSessionCreated struct {
	SessionID string    `json:"sessionId"`
	LaunchURI string    `json:"launchUri"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type VerificationSession struct {
	SessionID    string    `json:"sessionId"`
	Status       string    `json:"status"`
	Result       string    `json:"result,omitempty"`
	Codes        []string  `json:"codes,omitempty"`
	ReportDigest string    `json:"reportDigest,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
	ExpiresAt    time.Time `json:"expiresAt"`
	CompletedAt  time.Time `json:"completedAt,omitempty"`
}

func NewFromEnv(getenv func(string) string) *Client {
	return &Client{BaseURL: strings.TrimRight(getenv("HUKID_BASE_URL"), "/"), OperatorToken: getenv("HUKID_OPERATOR_TOKEN"), RequireTLS: getenv("APP_ENV") == "production", HTTP: &http.Client{Timeout: 8 * time.Second}}
}

func (c *Client) configured() bool {
	return c != nil && c.BaseURL != "" && c.OperatorToken != "" && (!c.RequireTLS || strings.HasPrefix(strings.ToLower(c.BaseURL), "https://"))
}

func (c *Client) StartVerification(ctx context.Context, tenantID string, ttlSeconds int) (VerificationSessionCreated, error) {
	if !c.configured() {
		return VerificationSessionCreated{}, ErrNotConfigured
	}
	if ttlSeconds < 60 || ttlSeconds > 900 {
		ttlSeconds = 300
	}
	body, _ := json.Marshal(map[string]int{"ttlSeconds": ttlSeconds})
	var out VerificationSessionCreated
	if err := c.do(ctx, http.MethodPost, fmt.Sprintf("%s/v1/operator/tenants/%s/verification-sessions", c.BaseURL, tenantID), body, &out); err != nil {
		return VerificationSessionCreated{}, err
	}
	if out.SessionID == "" || out.LaunchURI == "" {
		return VerificationSessionCreated{}, errors.New("hukid invalid verification session response")
	}
	return out, nil
}

func (c *Client) GetVerification(ctx context.Context, tenantID, sessionID string) (VerificationSession, error) {
	if !c.configured() {
		return VerificationSession{}, ErrNotConfigured
	}
	var out VerificationSession
	if err := c.do(ctx, http.MethodGet, fmt.Sprintf("%s/v1/operator/tenants/%s/verification-sessions/%s", c.BaseURL, tenantID, sessionID), nil, &out); err != nil {
		return VerificationSession{}, err
	}
	return out, nil
}

func (c *Client) do(ctx context.Context, method, url string, body []byte, out any) error {
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.OperatorToken)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &HTTPError{Status: resp.StatusCode}
	}
	if out != nil && json.NewDecoder(resp.Body).Decode(out) != nil {
		return errors.New("hukid invalid response")
	}
	return nil
}
