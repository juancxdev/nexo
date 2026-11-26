package hukid

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var ErrNotConfigured = errors.New("hukid verifier not configured")

type Client struct {
	BaseURL       string
	OAuthURL      string
	ClientID      string
	ClientSecret  string
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
	baseURL := getenv("HUKID_PASS_BASE_URL")
	if baseURL == "" {
		baseURL = getenv("HUKID_BASE_URL")
	}
	oauthURL := getenv("HUKID_PASS_OAUTH_URL")
	if oauthURL == "" {
		oauthURL = getenv("HUKID_OAUTH_URL")
	}
	clientID := getenv("HUKID_PASS_CLIENT_ID")
	if clientID == "" {
		clientID = getenv("HUKID_CLIENT_ID")
	}
	clientSecret := getenv("HUKID_PASS_CLIENT_SECRET")
	if clientSecret == "" {
		clientSecret = getenv("HUKID_CLIENT_SECRET")
	}
	baseURL = strings.TrimRight(baseURL, "/")
	return &Client{
		BaseURL: baseURL, OAuthURL: oauthURL, ClientID: clientID,
		ClientSecret: clientSecret, OperatorToken: getenv("HUKID_OPERATOR_TOKEN"),
		RequireTLS: getenv("APP_ENV") == "production", HTTP: &http.Client{Timeout: 8 * time.Second},
	}
}

type PassVerification struct {
	Result               string `json:"result"`
	Code                 string `json:"code,omitempty"`
	RegistrationID       string `json:"registrationId,omitempty"`
	EventID              string `json:"eventId,omitempty"`
	CredentialID         string `json:"credentialId,omitempty"`
	CredentialDefinition string `json:"credentialDefinition,omitempty"`
}

func (c *Client) passConfigured() bool {
	return c != nil && c.BaseURL != "" && c.OAuthURL != "" && c.ClientID != "" && c.ClientSecret != "" &&
		(!c.RequireTLS || (strings.HasPrefix(strings.ToLower(c.BaseURL), "https://") && strings.HasPrefix(strings.ToLower(c.OAuthURL), "https://")))
}

func (c *Client) integrationToken(ctx context.Context) (string, error) {
	if !c.passConfigured() {
		return "", ErrNotConfigured
	}
	form := url.Values{"grant_type": {"client_credentials"}, "scope": {"pass:verify"}, "client_id": {c.ClientID}, "client_secret": {c.ClientSecret}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.OAuthURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", &HTTPError{Status: resp.StatusCode}
	}
	var out struct {
		AccessToken string `json:"access_token"`
	}
	if json.NewDecoder(resp.Body).Decode(&out) != nil || out.AccessToken == "" {
		return "", errors.New("hukid invalid token response")
	}
	return out.AccessToken, nil
}

func (c *Client) VerifyPass(ctx context.Context, code, eventID string) (PassVerification, error) {
	if strings.TrimSpace(code) == "" || strings.TrimSpace(eventID) == "" {
		return PassVerification{}, errors.New("hukid invalid pass request")
	}
	token, err := c.integrationToken(ctx)
	if err != nil {
		return PassVerification{}, err
	}
	body, _ := json.Marshal(map[string]string{"code": code, "eventId": eventID})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/pass-verifications", bytes.NewReader(body))
	if err != nil {
		return PassVerification{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return PassVerification{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return PassVerification{}, &HTTPError{Status: resp.StatusCode}
	}
	var out PassVerification
	if json.NewDecoder(resp.Body).Decode(&out) != nil || out.Result == "" {
		return PassVerification{}, errors.New("hukid invalid pass response")
	}
	return out, nil
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
