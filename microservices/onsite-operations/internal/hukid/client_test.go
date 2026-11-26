package hukid

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientRedeemsCheckinPass(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth2/token":
			if err := r.ParseForm(); err != nil || r.Form.Get("scope") != "pass:verify" {
				t.Fatalf("invalid token form: %v", err)
			}
			if r.Form.Get("client_id") != "door-client" || r.Form.Get("client_secret") != "door-secret" {
				t.Fatal("missing client credentials")
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "pass-token", "token_type": "Bearer"})
		case "/v1/pass-verifications":
			if r.Header.Get("Authorization") != "Bearer pass-token" {
				t.Fatal("missing pass bearer")
			}
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["code"] != "opaque-code" || body["eventId"] != "event-1" {
				t.Fatalf("invalid pass body: %+v", body)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"result": "pass", "registrationId": "reg-1", "eventId": "event-1", "credentialId": "vc-1"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := &Client{BaseURL: server.URL + "/v1", OAuthURL: server.URL + "/oauth2/token", ClientID: "door-client", ClientSecret: "door-secret", HTTP: server.Client()}
	result, err := client.VerifyPass(context.Background(), "opaque-code", "event-1")
	if err != nil || result.Result != "pass" || result.RegistrationID != "reg-1" {
		t.Fatalf("verify pass: %+v %v", result, err)
	}
}

func TestClientStartsAndReadsVerificationSession(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer operator-token" {
			t.Fatal("missing bearer")
		}
		if r.Method == http.MethodPost {
			if r.URL.Path != "/v1/operator/tenants/tenant-1/verification-sessions" {
				t.Fatalf("path=%s", r.URL.Path)
			}
			if r.Header.Get("Content-Type") != "application/json" {
				t.Fatalf("content type=%s", r.Header.Get("Content-Type"))
			}
			body, _ := io.ReadAll(r.Body)
			if !bytes.Equal(bytes.TrimSpace(body), []byte(`{"ttlSeconds":300}`)) {
				t.Fatalf("body=%s", body)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"sessionId": "session-1", "launchUri": "openid4vp://request/1", "expiresAt": "2026-11-24T12:00:00Z"})
			return
		}
		if r.URL.Path != "/v1/operator/tenants/tenant-1/verification-sessions/session-1" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"sessionId": "session-1", "status": "completed", "result": "pass", "codes": []string{}, "reportDigest": "sha256:" + "0000000000000000000000000000000000000000000000000000000000000000", "createdAt": "2026-11-24T11:55:00Z", "expiresAt": "2026-11-24T12:00:00Z"})
	}))
	defer server.Close()
	client := &Client{BaseURL: server.URL, OperatorToken: "operator-token", HTTP: server.Client()}
	created, err := client.StartVerification(context.Background(), "tenant-1", 300)
	if err != nil || created.SessionID != "session-1" {
		t.Fatalf("create: %+v %v", created, err)
	}
	result, err := client.GetVerification(context.Background(), "tenant-1", created.SessionID)
	if err != nil || result.Result != "pass" {
		t.Fatalf("read: %+v %v", result, err)
	}
}
