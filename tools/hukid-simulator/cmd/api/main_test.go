package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestSimulatorRequiresIssuedBearerToken(t *testing.T) {
	s := &server{requests: map[string]issuance{}, tokens: map[string]time.Time{}, faults: map[string]faultConfig{}}
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
	s := &server{requests: map[string]issuance{}, tokens: map[string]time.Time{}, faults: map[string]faultConfig{}}
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

func TestSimulatorSupportsFaultsPerEndpoint(t *testing.T) {
	s := &server{requests: map[string]issuance{}, tokens: map[string]time.Time{}, faults: map[string]faultConfig{}}
	app := newApp(s)
	body, _ := json.Marshal(map[string]any{"route": "token", "status": 503})
	req := httptest.NewRequest(http.MethodPost, "/__test/faults", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if resp, err := app.Test(req); err != nil || resp.StatusCode != http.StatusNoContent {
		t.Fatalf("configure fault: %v status=%d", err, resp.StatusCode)
	}
	if resp, _ := app.Test(httptest.NewRequest(http.MethodPost, "/oauth2/token", nil)); resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("token fault status=%d", resp.StatusCode)
	}
}

func TestSimulatorExpiresOffersOnRead(t *testing.T) {
	now := time.Now().UTC()
	s := &server{requests: map[string]issuance{"ir_expired": {ID: "ir_expired", OfferState: "ready", IssuanceState: "prepared", ExpiresAt: now.Add(-time.Minute)}}, tokens: map[string]time.Time{}, faults: map[string]faultConfig{}}
	app := newApp(s)
	token := issueTestToken(s)
	req := httptest.NewRequest(http.MethodGet, "/v1/issuance-requests/ir_expired", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := app.Test(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("get expired: %v status=%d", err, resp.StatusCode)
	}
	var got issuance
	if json.NewDecoder(resp.Body).Decode(&got) != nil || got.OfferState != "expired" || got.IssuanceState != "failed" {
		t.Fatalf("unexpected expired state: %+v", got)
	}
}

func TestSimulatorCoversCanonicalErrorStatuses(t *testing.T) {
	s := &server{requests: map[string]issuance{}, tokens: map[string]time.Time{}, faults: map[string]faultConfig{}}
	app := newApp(s)
	token := issueTestToken(s)
	request := func(body string, key string) int {
		req := httptest.NewRequest(http.MethodPost, "/v1/issuance-requests", bytes.NewBufferString(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Idempotency-Key", key)
		resp, _ := app.Test(req)
		return resp.StatusCode
	}
	if got := request(`{"issuerId":"","credentialDefinition":"x","claims":{}}`, "hukid-test-422-0001"); got != 422 {
		t.Fatalf("422 status=%d", got)
	}
	valid := `{"issuerId":"issuer","credentialDefinition":"definition","claims":{"certificate":{"id":"cert-error"}}}`
	if got := request(valid, "hukid-test-409-0001"); got != 201 {
		t.Fatalf("create status=%d", got)
	}
	if got := request(`{"issuerId":"issuer","credentialDefinition":"definition","claims":{"certificate":{"id":"different"}}}`, "hukid-test-409-0001"); got != 409 {
		t.Fatalf("409 status=%d", got)
	}
	for _, status := range []int{429, 503} {
		body, _ := json.Marshal(map[string]any{"route": "create", "status": status})
		req := httptest.NewRequest(http.MethodPost, "/__test/faults", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, _ := app.Test(req)
		if resp.StatusCode != 204 {
			t.Fatal("fault setup failed")
		}
		if got := request(valid, fmt.Sprintf("hukid-test-%d-0001", status)); got != status {
			t.Fatalf("status=%d got=%d", status, got)
		}
	}
}

func TestSimulatorRejectsUnsupportedTxCodeDelivery(t *testing.T) {
	s := &server{requests: map[string]issuance{}, tokens: map[string]time.Time{}, faults: map[string]faultConfig{}}
	app := newApp(s)
	token := issueTestToken(s)
	req := httptest.NewRequest(http.MethodPost, "/v1/issuance-requests", bytes.NewBufferString(`{"issuerId":"issuer","credentialDefinition":"definition","claims":{"certificate":{"id":"tx-code"}},"delivery":{"flow":"pre-authorized_code","txCodeRequired":true}}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Idempotency-Key", "hukid-tx-code-0001")
	resp, err := app.Test(req)
	if err != nil || resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status=%d err=%v, want 422", resp.StatusCode, err)
	}
}

func TestSimulatorConcurrentSameKeyIsIdempotent(t *testing.T) {
	s := &server{requests: map[string]issuance{}, tokens: map[string]time.Time{}, faults: map[string]faultConfig{}}
	app := newApp(s)
	token := issueTestToken(s)
	body := []byte(`{"issuerId":"issuer","credentialDefinition":"definition","claims":{"certificate":{"id":"concurrent"}}}`)
	statuses := make([]int, 12)
	var wg sync.WaitGroup
	for i := range statuses {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodPost, "/v1/issuance-requests", bytes.NewReader(body))
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("Idempotency-Key", "hukid-concurrent-0001")
			resp, err := app.Test(req)
			if err == nil {
				statuses[i] = resp.StatusCode
			}
		}(i)
	}
	wg.Wait()
	created, replayed := 0, 0
	for _, status := range statuses {
		if status == 201 {
			created++
		}
		if status == 200 {
			replayed++
		}
	}
	if created != 1 || replayed != len(statuses)-1 {
		t.Fatalf("statuses=%v created=%d replayed=%d", statuses, created, replayed)
	}
}

func issueTestToken(s *server) string {
	token := "sim-test-token"
	s.tokens[token] = time.Now().Add(time.Minute)
	return token
}
