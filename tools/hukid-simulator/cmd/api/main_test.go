package main

import (
	"bytes"
	"net/http"
	"testing"
	"time"
)

func TestSimulatorRequiresIssuedBearerToken(t *testing.T) {
	s := &server{requests: map[string]issuance{}, tokens: map[string]time.Time{}}
	app := newApp(s)
	req, _ := http.NewRequest(http.MethodGet, "/v1/issuance-requests/ir_missing", nil)
	req.Header.Set("Authorization", "Bearer forged")
	res, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", res.StatusCode, http.StatusUnauthorized)
	}
}

func TestSimulatorReplaysIdempotentRequest(t *testing.T) {
	s := &server{requests: map[string]issuance{}, tokens: map[string]time.Time{}}
	app := newApp(s)
	token := issueTestToken(s)
	body := []byte(`{"issuerId":"issuer","credentialDefinition":"definition","claims":{"certificate":{"id":"cert-1"}},"delivery":{"expiresInSeconds":300}}`)
	request := func() *http.Response {
		req, _ := http.NewRequest(http.MethodPost, "/v1/issuance-requests", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "nexo-test-key-0001")
		res, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	first, second := request(), request()
	if first.StatusCode != http.StatusCreated || second.StatusCode != http.StatusOK {
		t.Fatalf("statuses = %d, %d", first.StatusCode, second.StatusCode)
	}
}

func issueTestToken(s *server) string {
	token := "sim-test-token"
	s.tokens[token] = time.Now().Add(time.Minute)
	return token
}
