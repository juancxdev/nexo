package httpapi

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
)

func testSecurityConfig() SecurityConfig {
	return SecurityConfig{
		RateWindow:           time.Minute,
		IPLimit:              10,
		EventLimit:           10,
		DeviceLimit:          10,
		ChallengeTTL:         time.Minute,
		ChallengeDifficulty:  14,
		ChallengeSecret:      []byte(strings.Repeat("s", 32)),
		DeviceCookieSameSite: http.SameSiteLaxMode,
	}
}

func TestRateLimitUsesIPEventAndDeviceBuckets(t *testing.T) {
	config := testSecurityConfig()
	config.IPLimit = 5
	config.EventLimit = 1
	config.DeviceLimit = 1
	state, err := NewSecurityState(config)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
	ipOne := netip.MustParseAddr("192.0.2.10")
	ipTwo := netip.MustParseAddr("192.0.2.11")

	if ok, _ := state.allowRequest(ipOne, "site", "event-a", "device-a", now); !ok {
		t.Fatal("first request was unexpectedly limited")
	}
	if ok, retry := state.allowRequest(ipOne, "site", "event-a", "device-a", now.Add(30*time.Second)); ok || retry != 30*time.Second {
		t.Fatalf("device limit = (%v, %s), want (false, 30s)", ok, retry)
	}
	if ok, _ := state.allowRequest(ipOne, "site", "event-b", "device-b", now.Add(30*time.Second)); !ok {
		t.Fatal("a different event and device should remain under the IP limit")
	}
	if ok, _ := state.allowRequest(ipTwo, "site", "event-a", "device-c", now.Add(30*time.Second)); ok {
		t.Fatal("event limit should apply across different IPs")
	}
	if ok, _ := state.allowRequest(ipTwo, "site", "event-c", "device-a", now.Add(30*time.Second)); ok {
		t.Fatal("device limit should apply across different IPs and events")
	}
}

func TestChallengeIsBoundExpiresAndCannotBeReplayed(t *testing.T) {
	state, err := NewSecurityState(testSecurityConfig())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
	token, expiresAt, err := state.issueChallenge("site", "event", "device-a", now)
	if err != nil {
		t.Fatal(err)
	}
	proof := solveTestProof(token, 14)

	if got := state.verifyChallenge(token, proof, "site", "event", "device-a", now); got != challengeValid {
		t.Fatalf("valid challenge result = %v", got)
	}
	if got := state.verifyChallenge(token, proof, "site", "event", "device-a", now); got != challengeReplay {
		t.Fatalf("replayed challenge result = %v", got)
	}
	if got := state.verifyChallenge(token, proof, "site", "other-event", "device-a", now); got != challengeInvalid {
		t.Fatalf("wrong-event challenge result = %v", got)
	}
	if got := state.verifyChallenge(token, proof, "site", "event", "device-b", now); got != challengeInvalid {
		t.Fatalf("wrong-device challenge result = %v", got)
	}

	expiredToken, _, err := state.issueChallenge("site", "event", "device-a", now)
	if err != nil {
		t.Fatal(err)
	}
	expiredProof := solveTestProof(expiredToken, 14)
	if got := state.verifyChallenge(expiredToken, expiredProof, "site", "event", "device-a", expiresAt); got != challengeExpired {
		t.Fatalf("expired challenge result = %v", got)
	}
}

func TestV2GuardRequiresChallengeAndRejectsReplay(t *testing.T) {
	server, err := NewWithSecurityConfig(nil, "", "", testSecurityConfig())
	if err != nil {
		t.Fatal(err)
	}
	app := fiber.New()
	app.Post("/sites/:site/events/:event/registrations", server.protectPublic(true, func(c fiber.Ctx) error {
		return c.SendStatus(http.StatusNoContent)
	}))

	missing := httptest.NewRequest(http.MethodPost, "/sites/site/events/event/registrations", nil)
	missingResponse, err := app.Test(missing)
	if err != nil {
		t.Fatal(err)
	}
	if missingResponse.StatusCode != http.StatusForbidden {
		t.Fatalf("missing challenge status = %d, want 403", missingResponse.StatusCode)
	}

	deviceID, err := randomURLToken(32)
	if err != nil {
		t.Fatal(err)
	}
	deviceCookie := server.security.makeDeviceCookie(deviceID)
	now := time.Now()
	challenge, _, err := server.security.issueChallenge("site", "event", deviceID, now)
	if err != nil {
		t.Fatal(err)
	}
	proof := solveTestProof(challenge, 14)
	newRequest := func() *http.Request {
		req := httptest.NewRequest(http.MethodPost, "/sites/site/events/event/registrations", nil)
		req.Header.Set("Cookie", deviceCookieName+"="+deviceCookie)
		req.Header.Set("X-Participation-Challenge", challenge)
		req.Header.Set("X-Participation-Proof", proof)
		return req
	}
	validResponse, err := app.Test(newRequest())
	if err != nil {
		t.Fatal(err)
	}
	if validResponse.StatusCode != http.StatusNoContent {
		t.Fatalf("valid challenge status = %d, want 204", validResponse.StatusCode)
	}
	replayResponse, err := app.Test(newRequest())
	if err != nil {
		t.Fatal(err)
	}
	if replayResponse.StatusCode != http.StatusConflict {
		t.Fatalf("replay status = %d, want 409", replayResponse.StatusCode)
	}
}

func TestConcurrentChallengeReplayHasOnlyOneWinner(t *testing.T) {
	state, err := NewSecurityState(testSecurityConfig())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	token, _, err := state.issueChallenge("site", "event", "device", now)
	if err != nil {
		t.Fatal(err)
	}
	proof := solveTestProof(token, 14)

	var wait sync.WaitGroup
	results := make(chan challengeResult, 16)
	for range 16 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			results <- state.verifyChallenge(token, proof, "site", "event", "device", now)
		}()
	}
	wait.Wait()
	close(results)
	valid, replayed := 0, 0
	for result := range results {
		switch result {
		case challengeValid:
			valid++
		case challengeReplay:
			replayed++
		default:
			t.Fatalf("unexpected concurrent verification result: %v", result)
		}
	}
	if valid != 1 || replayed != 15 {
		t.Fatalf("concurrent results: valid=%d replayed=%d, want 1 and 15", valid, replayed)
	}
}

func TestChallengeResponseIssuesCookieAcceptedByV2Guard(t *testing.T) {
	server, err := NewWithSecurityConfig(nil, "", "", testSecurityConfig())
	if err != nil {
		t.Fatal(err)
	}
	challengeApp := fiber.New()
	challengeApp.Get("/sites/:site/events/:event/challenge", server.issueChallengeResponse)
	response, err := challengeApp.Test(httptest.NewRequest(http.MethodGet, "/sites/site/events/event/challenge", nil))
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("challenge response status = %d, want 200", response.StatusCode)
	}
	var payload struct {
		Challenge  string    `json:"challenge"`
		Difficulty uint8     `json:"difficulty"`
		ExpiresAt  time.Time `json:"expires_at"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	var deviceCookie *http.Cookie
	for _, cookie := range response.Cookies() {
		if cookie.Name == deviceCookieName {
			deviceCookie = cookie
			break
		}
	}
	if deviceCookie == nil || !deviceCookie.HttpOnly || deviceCookie.Path != "/participation/public/v2" {
		t.Fatalf("device cookie = %#v", deviceCookie)
	}
	deviceID, validCookie := server.security.readDeviceCookie(deviceCookieName + "=" + deviceCookie.Value)
	if !validCookie {
		t.Fatal("server-issued device cookie did not verify")
	}
	proof := solveTestProof(payload.Challenge, int(payload.Difficulty))
	registerApp := fiber.New()
	registerApp.Post("/sites/:site/events/:event/registrations", server.protectPublic(true, func(c fiber.Ctx) error {
		return c.SendStatus(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodPost, "/sites/site/events/event/registrations", nil)
	request.AddCookie(deviceCookie)
	request.Header.Set("X-Participation-Challenge", payload.Challenge)
	request.Header.Set("X-Participation-Proof", proof)
	registered, err := registerApp.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	if registered.StatusCode != http.StatusNoContent {
		t.Fatalf("registration guard status = %d, want 204", registered.StatusCode)
	}
	if deviceID == "" || payload.ExpiresAt.IsZero() {
		t.Fatal("challenge omitted its device binding or expiry")
	}
}

func TestV2RegistrationRoutePassesValidChallengeToExistingHandler(t *testing.T) {
	server, err := NewWithSecurityConfig(nil, "", "", testSecurityConfig())
	if err != nil {
		t.Fatal(err)
	}
	deviceID, err := randomURLToken(32)
	if err != nil {
		t.Fatal(err)
	}
	challenge, _, err := server.security.issueChallenge("site", "event", deviceID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/participation/public/v2/sites/site/events/event/registrations", strings.NewReader(`{}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "compatibility-test-key")
	request.Header.Set("Cookie", deviceCookieName+"="+server.security.makeDeviceCookie(deviceID))
	request.Header.Set("X-Participation-Challenge", challenge)
	request.Header.Set("X-Participation-Proof", solveTestProof(challenge, 14))

	response, err := server.App.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusBadRequest || !strings.Contains(string(body), "invalid_registration") {
		t.Fatalf("v2 registration after valid proof = %d %s, want existing registration validation", response.StatusCode, body)
	}
}

func TestV2GuardSetsRetryAfterAndV1RemainsUngated(t *testing.T) {
	config := testSecurityConfig()
	config.IPLimit = 1
	server, err := NewWithSecurityConfig(nil, "", "", config)
	if err != nil {
		t.Fatal(err)
	}
	app := fiber.New()
	app.Get("/sites/:site/events/:event/form", server.protectPublic(false, func(c fiber.Ctx) error {
		return c.SendStatus(http.StatusNoContent)
	}))

	first, err := app.Test(httptest.NewRequest(http.MethodGet, "/sites/site/events/event/form", nil))
	if err != nil {
		t.Fatal(err)
	}
	if first.StatusCode != http.StatusNoContent {
		t.Fatalf("first request status = %d, want 204", first.StatusCode)
	}
	second, err := app.Test(httptest.NewRequest(http.MethodGet, "/sites/site/events/event/form", nil))
	if err != nil {
		t.Fatal(err)
	}
	if second.StatusCode != http.StatusTooManyRequests || second.Header.Get("Retry-After") == "" {
		t.Fatalf("limited response = %d, Retry-After=%q", second.StatusCode, second.Header.Get("Retry-After"))
	}

	legacy := httptest.NewRequest(http.MethodPost, "/participation/public/v1/sites/site/events/event/registrations", strings.NewReader(`{}`))
	legacyResponse, err := server.App.Test(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if legacyResponse.StatusCode != http.StatusBadRequest {
		t.Fatalf("v1 response = %d, want existing validation response 400", legacyResponse.StatusCode)
	}
}

func TestAllowlistBypassesV2Challenge(t *testing.T) {
	config := testSecurityConfig()
	config.Allowlist = []netip.Prefix{
		netip.MustParsePrefix("0.0.0.0/0"),
		netip.MustParsePrefix("::/0"),
	}
	server, err := NewWithSecurityConfig(nil, "", "", config)
	if err != nil {
		t.Fatal(err)
	}
	app := fiber.New()
	app.Post("/sites/:site/events/:event/registrations", server.protectPublic(true, func(c fiber.Ctx) error {
		return c.SendStatus(http.StatusNoContent)
	}))

	response, err := app.Test(httptest.NewRequest(http.MethodPost, "/sites/site/events/event/registrations", nil))
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("allowlisted request status = %d, want 204", response.StatusCode)
	}
	if got := server.security.metricsSnapshot().AllowlistedRequests; got != 1 {
		t.Fatalf("allowlisted metric = %d, want 1", got)
	}
}

func TestMetricsEndpointRequiresOperationalTokenAndReturnsAggregates(t *testing.T) {
	config := testSecurityConfig()
	config.MetricsToken = strings.Repeat("m", 32)
	server, err := NewWithSecurityConfig(nil, "", "", config)
	if err != nil {
		t.Fatal(err)
	}
	unauthorized, err := server.App.Test(httptest.NewRequest(http.MethodGet, "/participation/internal/v1/security/metrics", nil))
	if err != nil {
		t.Fatal(err)
	}
	if unauthorized.StatusCode != http.StatusUnauthorized {
		t.Fatalf("metrics without token status = %d, want 401", unauthorized.StatusCode)
	}
	request := httptest.NewRequest(http.MethodGet, "/participation/internal/v1/security/metrics", nil)
	request.Header.Set("Authorization", "Bearer "+config.MetricsToken)
	response, err := server.App.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("metrics status = %d, want 200", response.StatusCode)
	}
	var counters map[string]uint64
	if err := json.NewDecoder(response.Body).Decode(&counters); err != nil {
		t.Fatal(err)
	}
	if len(counters) != 7 || counters["public_requests"] != 0 {
		t.Fatalf("unexpected aggregate metrics: %v", counters)
	}
}

func TestProxyHeadersAreTrustedOnlyForConfiguredNetworks(t *testing.T) {
	config := testSecurityConfig()
	config.TrustedProxies = []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}
	config.Allowlist = []netip.Prefix{netip.MustParsePrefix("198.51.100.0/24")}
	state, err := NewSecurityState(config)
	if err != nil {
		t.Fatal(err)
	}

	forwarded := state.forwardedClientIP(netip.MustParseAddr("10.0.0.2"), "198.51.100.23, 10.1.0.4")
	if forwarded != netip.MustParseAddr("198.51.100.23") || !state.allowlisted(forwarded) {
		t.Fatalf("trusted proxy resolved client IP = %s", forwarded)
	}
	direct := state.forwardedClientIP(netip.MustParseAddr("203.0.113.4"), "198.51.100.23")
	if direct != netip.MustParseAddr("203.0.113.4") {
		t.Fatalf("untrusted X-Forwarded-For changed client IP to %s", direct)
	}
}

func solveTestProof(challenge string, difficulty int) string {
	for counter := uint32(0); ; counter++ {
		digest := sha256.Sum256([]byte(fmt.Sprintf("%s:%d", challenge, counter)))
		if hasLeadingZeroBits(digest, difficulty) {
			return fmt.Sprint(counter)
		}
	}
}
