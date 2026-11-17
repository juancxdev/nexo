package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	auth "nexo-backend/internal/auth/domain"
	"nexo-backend/internal/platform/memory"
)

func TestRegisterLoginTenantAndSidebar(t *testing.T) {
	server := NewServer(memory.NewStore())
	handler := server.Handler()
	register := httptest.NewRecorder()
	handler.ServeHTTP(register, httptest.NewRequest(http.MethodPost, "/v1/auth/register", bytes.NewBufferString(`{"email":"owner@example.com","name":"Owner","password":"Strong!123"}`)))
	if register.Code != 201 {
		t.Fatalf("register status=%d", register.Code)
	}
	login := httptest.NewRecorder()
	handler.ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/v1/auth/login", bytes.NewBufferString(`{"email":"owner@example.com","password":"Strong!123"}`)))
	if login.Code != 200 {
		t.Fatalf("login status=%d", login.Code)
	}
	var payload struct {
		AccessToken string `json:"access_token"`
	}
	_ = json.NewDecoder(login.Body).Decode(&payload)
	if payload.AccessToken == "" {
		t.Fatal("missing access token")
	}
	create := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/tenants", bytes.NewBufferString(`{"name":"IncubaUNAS"}`))
	req.Header.Set("Authorization", "Bearer "+payload.AccessToken)
	handler.ServeHTTP(create, req)
	if create.Code != 201 {
		t.Fatalf("tenant status=%d body=%s", create.Code, create.Body.String())
	}
	sidebar := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/navigation/sidebar", nil)
	req.Header.Set("Authorization", "Bearer "+payload.AccessToken)
	handler.ServeHTTP(sidebar, req)
	if sidebar.Code != 200 {
		t.Fatalf("sidebar status=%d", sidebar.Code)
	}
}

func TestTenantContextGrantsEventScopesForOwner(t *testing.T) {
	server := NewServer(memory.NewStore())
	handler := server.Handler()

	register := httptest.NewRecorder()
	handler.ServeHTTP(register, httptest.NewRequest(http.MethodPost, "/v1/auth/register", bytes.NewBufferString(`{"email":"events-owner@example.com","name":"Events Owner","password":"Strong!123"}`)))
	login := httptest.NewRecorder()
	handler.ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/v1/auth/login", bytes.NewBufferString(`{"email":"events-owner@example.com","password":"Strong!123"}`)))
	var loginPayload struct {
		AccessToken string `json:"access_token"`
	}
	_ = json.NewDecoder(login.Body).Decode(&loginPayload)

	create := httptest.NewRecorder()
	createRequest := httptest.NewRequest(http.MethodPost, "/v1/tenants", bytes.NewBufferString(`{"name":"Eventos UNAS"}`))
	createRequest.Header.Set("Authorization", "Bearer "+loginPayload.AccessToken)
	handler.ServeHTTP(create, createRequest)
	var created struct {
		Tenant struct {
			ID string `json:"id"`
		} `json:"tenant"`
	}
	_ = json.NewDecoder(create.Body).Decode(&created)

	contextResponse := httptest.NewRecorder()
	contextRequest := httptest.NewRequest(http.MethodPost, "/v1/auth/tenant-context", bytes.NewBufferString(`{"tenant_id":"`+created.Tenant.ID+`"}`))
	contextRequest.Header.Set("Authorization", "Bearer "+loginPayload.AccessToken)
	handler.ServeHTTP(contextResponse, contextRequest)
	if contextResponse.Code != http.StatusOK {
		t.Fatalf("tenant context status=%d body=%s", contextResponse.Code, contextResponse.Body.String())
	}
	var payload struct {
		AccessToken string   `json:"access_token"`
		Scopes      []string `json:"scopes"`
	}
	_ = json.NewDecoder(contextResponse.Body).Decode(&payload)
	for _, required := range []string{"events:read", "events:write", "events:publish"} {
		found := false
		for _, scope := range payload.Scopes {
			if scope == required {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing %s in tenant-context response: %v", required, payload.Scopes)
		}
	}
	var claims auth.Claims
	if !server.verify(payload.AccessToken, &claims) {
		t.Fatal("tenant context access token is not a valid Core token")
	}
	if claims.TenantID != created.Tenant.ID {
		t.Fatalf("token tenant=%q want=%q", claims.TenantID, created.Tenant.ID)
	}
}

func TestUnknownRoleDoesNotReceiveEventScopes(t *testing.T) {
	for _, scope := range scopesForRole("future_role") {
		if len(scope) >= len("events:") && scope[:len("events:")] == "events:" {
			t.Fatalf("unknown role must not receive event scope: %s", scope)
		}
	}
}
