package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"os"
	"strings"
	"sync"
	"time"
)

type issuance struct {
	ID                        string    `json:"id"`
	OfferState                string    `json:"offerState"`
	IssuanceState             string    `json:"issuanceState"`
	IssuerID                  string    `json:"issuerId"`
	CredentialConfigurationID string    `json:"credentialConfigurationId"`
	CredentialOfferURI        string    `json:"credentialOfferUri"`
	ExpiresAt                 time.Time `json:"expiresAt"`
	CredentialID              string    `json:"credentialId,omitempty"`
	CreatedAt                 time.Time `json:"createdAt"`
	RequestHash               string    `json:"-"`
	IdempotencyKey            string    `json:"-"`
}
type server struct {
	mu           sync.Mutex
	requests     map[string]issuance
	tokens       map[string]time.Time
	verification map[string]verification
	faults       map[string]faultConfig
}
type verification struct {
	SessionID    string    `json:"sessionId"`
	Status       string    `json:"status"`
	Result       string    `json:"result,omitempty"`
	Codes        []string  `json:"codes,omitempty"`
	ReportDigest string    `json:"reportDigest,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
	ExpiresAt    time.Time `json:"expiresAt"`
	CompletedAt  time.Time `json:"completedAt,omitempty"`
}
type faultConfig struct {
	Status    int
	Remaining int
}

func main() {
	s := &server{requests: map[string]issuance{}, tokens: map[string]time.Time{}, verification: map[string]verification{}, faults: map[string]faultConfig{}}
	a := newApp(s)
	addr := os.Getenv("HUKID_SIMULATOR_ADDR")
	if addr == "" {
		addr = ":8090"
	}
	if err := a.Listen(addr); err != nil {
		panic(err)
	}
}
func newApp(s *server) *fiber.App {
	a := fiber.New(fiber.Config{AppName: "HukID Integration API Simulator"})
	a.Get("/health/live", func(c fiber.Ctx) error { return c.JSON(fiber.Map{"status": "ok"}) })
	a.Post("/oauth2/token", s.token)
	v1 := a.Group("/v1", s.auth)
	v1.Post("/issuance-requests", s.create)
	v1.Get("/issuance-requests/:id", s.get)
	v1.Post("/issuance-requests/:id/cancellations", s.cancel)
	v1.Post("/operator/tenants/:tenant/verification-sessions", s.startVerification)
	v1.Get("/operator/tenants/:tenant/verification-sessions/:id", s.getVerification)
	t := a.Group("/__test")
	t.Post("/reset", s.reset)
	t.Post("/issuance-requests/:id/redeem", s.redeem)
	t.Post("/issuance-requests/:id/issue", s.issue)
	t.Post("/faults", s.setFault)
	t.Post("/verification/:id/pass", s.passVerification)
	t.Post("/verification/:id/fail", s.failVerification)
	t.Post("/verification/:id/expire", s.expireVerification)
	return a
}
func (s *server) token(c fiber.Ctx) error {
	if status := s.consumeFault("token"); status != 0 {
		return problem(c, status, "simulated_failure", "fallo configurado para pruebas")
	}
	if c.FormValue("grant_type") != "client_credentials" {
		return problem(c, 400, "unsupported_grant_type", "solo se admite client_credentials")
	}
	token := "sim-" + uuid.NewString()
	s.mu.Lock()
	s.tokens[token] = time.Now().UTC().Add(5 * time.Minute)
	s.mu.Unlock()
	return c.JSON(fiber.Map{"access_token": token, "token_type": "Bearer", "expires_in": 300, "scope": c.FormValue("scope")})
}
func (s *server) auth(c fiber.Ctx) error {
	header := c.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		return problem(c, 401, "invalid_token", "token ausente")
	}
	token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
	if token == "dev-operator" && (os.Getenv("APP_ENV") == "development" || os.Getenv("APP_ENV") == "test") {
		return c.Next()
	}
	s.mu.Lock()
	expires, ok := s.tokens[token]
	s.mu.Unlock()
	if !ok || time.Now().UTC().After(expires) {
		return problem(c, 401, "invalid_token", "token inválido o vencido")
	}
	return c.Next()
}
func (s *server) startVerification(c fiber.Ctx) error {
	if strings.TrimSpace(c.Get("Authorization")) != "Bearer dev-operator" {
		return problem(c, 401, "invalid_token", "token inválido")
	}
	var in struct {
		TTLSeconds int `json:"ttlSeconds"`
	}
	_ = json.Unmarshal(c.Body(), &in)
	if in.TTLSeconds < 60 || in.TTLSeconds > 900 {
		in.TTLSeconds = 300
	}
	now := time.Now().UTC()
	v := verification{SessionID: "vs_" + uuid.NewString(), Status: "pending", CreatedAt: now, ExpiresAt: now.Add(time.Duration(in.TTLSeconds) * time.Second)}
	s.mu.Lock()
	s.verification[v.SessionID] = v
	s.mu.Unlock()
	return c.Status(201).JSON(fiber.Map{"sessionId": v.SessionID, "launchUri": "openid4vp://verify/" + v.SessionID, "expiresAt": v.ExpiresAt})
}
func (s *server) getVerification(c fiber.Ctx) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.verification[c.Params("id")]
	if !ok {
		return problem(c, 404, "resource_not_found", "sesión no encontrada")
	}
	if time.Now().UTC().After(v.ExpiresAt) && v.Status == "pending" {
		v.Status = "expired"
		s.verification[v.SessionID] = v
	}
	return c.JSON(v)
}
func (s *server) setVerificationResult(c fiber.Ctx, result string) error {
	if os.Getenv("APP_ENV") != "development" && os.Getenv("APP_ENV") != "test" {
		return problem(c, 404, "not_found", "ruta no disponible")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.verification[c.Params("id")]
	if !ok {
		return problem(c, 404, "resource_not_found", "sesión no encontrada")
	}
	if v.Status == "expired" {
		return problem(c, 409, "invalid_state_transition", "sesión expirada")
	}
	v.Status = "completed"
	v.Result = result
	v.CompletedAt = time.Now().UTC()
	digest := sha256.Sum256([]byte(v.SessionID + "|" + result + "|" + v.CompletedAt.UTC().Format(time.RFC3339Nano)))
	v.ReportDigest = hex.EncodeToString(digest[:])
	s.verification[v.SessionID] = v
	return c.JSON(v)
}
func (s *server) passVerification(c fiber.Ctx) error { return s.setVerificationResult(c, "pass") }
func (s *server) failVerification(c fiber.Ctx) error { return s.setVerificationResult(c, "fail") }
func (s *server) expireVerification(c fiber.Ctx) error {
	if os.Getenv("APP_ENV") != "development" && os.Getenv("APP_ENV") != "test" {
		return problem(c, 404, "not_found", "ruta no disponible")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.verification[c.Params("id")]
	if !ok {
		return problem(c, 404, "resource_not_found", "sesión no encontrada")
	}
	v.Status = "expired"
	s.verification[v.SessionID] = v
	return c.JSON(v)
}
func (s *server) create(c fiber.Ctx) error {
	key := c.Get("Idempotency-Key")
	if len(key) < 16 {
		return problem(c, 400, "invalid_idempotency_key", "Idempotency-Key es obligatorio")
	}
	body := c.Body()
	sum := sha256.Sum256(body)
	hash := hex.EncodeToString(sum[:])
	s.mu.Lock()
	defer s.mu.Unlock()
	if f := s.faults["create"]; f.Remaining > 0 {
		s.faults["create"] = faultConfig{Status: f.Status, Remaining: f.Remaining - 1}
		return problem(c, f.Status, "simulated_failure", "fallo configurado para pruebas")
	}
	for _, v := range s.requests {
		if v.IdempotencyKey == key {
			if v.RequestHash != hash {
				return problem(c, 409, "idempotency_key_reused", "la clave fue usada con otro payload")
			}
			c.Status(200)
			return c.JSON(v)
		}
	}
	var in struct {
		IssuerID             string         `json:"issuerId"`
		CredentialDefinition string         `json:"credentialDefinition"`
		Claims               map[string]any `json:"claims"`
		Delivery             struct {
			ExpiresInSeconds int    `json:"expiresInSeconds"`
			Flow             string `json:"flow"`
			TxCodeRequired   bool   `json:"txCodeRequired"`
		} `json:"delivery"`
	}
	if json.Unmarshal(body, &in) != nil || in.IssuerID == "" || in.CredentialDefinition == "" || in.Claims == nil {
		return problem(c, 422, "claim_validation_failed", "issuerId, credentialDefinition y claims son obligatorios")
	}
	if in.Delivery.Flow != "" && in.Delivery.Flow != "pre-authorized_code" || in.Delivery.TxCodeRequired {
		return problem(c, 422, "unsupported_delivery", "solo se admite pre-authorized_code sin tx_code")
	}
	now := time.Now().UTC()
	ttl := in.Delivery.ExpiresInSeconds
	if ttl < 60 || ttl > 86400 {
		ttl = 300
	}
	item := issuance{ID: "ir_" + uuid.NewString(), OfferState: "ready", IssuanceState: "prepared", IssuerID: in.IssuerID, CredentialConfigurationID: in.CredentialDefinition, CredentialOfferURI: "openid-credential-offer://?credential_offer_uri=https%3A%2F%2Fissuer.hukid.local%2Foffers%2F" + uuid.NewString(), ExpiresAt: now.Add(time.Duration(ttl) * time.Second), CreatedAt: now, RequestHash: hash, IdempotencyKey: key}
	s.requests[item.ID] = item
	c.Set("Location", "/v1/issuance-requests/"+item.ID)
	return c.Status(201).JSON(item)
}
func (s *server) get(c fiber.Ctx) error {
	if status := s.consumeFault("get"); status != 0 {
		return problem(c, status, "simulated_failure", "fallo configurado para pruebas")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.requests[c.Params("id")]
	if !ok {
		return problem(c, 404, "resource_not_found", "solicitud no encontrada")
	}
	if time.Now().UTC().After(v.ExpiresAt) && v.IssuanceState != "issued" && v.IssuanceState != "cancelled" {
		v.OfferState = "expired"
		v.IssuanceState = "failed"
		s.requests[v.ID] = v
	}
	return c.JSON(v)
}
func (s *server) cancel(c fiber.Ctx) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.requests[c.Params("id")]
	if !ok {
		return problem(c, 404, "resource_not_found", "solicitud no encontrada")
	}
	if v.IssuanceState == "issued" || v.IssuanceState == "cancelled" {
		return problem(c, 409, "invalid_state_transition", "la solicitud ya fue emitida")
	}
	v.OfferState = "cancelled"
	v.IssuanceState = "cancelled"
	s.requests[v.ID] = v
	return c.SendStatus(204)
}
func (s *server) redeem(c fiber.Ctx) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.requests[c.Params("id")]
	if !ok {
		return problem(c, 404, "resource_not_found", "solicitud no encontrada")
	}
	if v.IssuanceState == "issued" || v.IssuanceState == "cancelled" {
		return problem(c, 409, "invalid_state_transition", "estado terminal")
	}
	v.OfferState = "redeemed"
	s.requests[v.ID] = v
	return c.JSON(v)
}
func (s *server) issue(c fiber.Ctx) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.requests[c.Params("id")]
	if !ok {
		return problem(c, 404, "resource_not_found", "solicitud no encontrada")
	}
	if v.IssuanceState == "issued" || v.IssuanceState == "cancelled" {
		return problem(c, 409, "invalid_state_transition", "estado terminal")
	}
	v.OfferState = "redeemed"
	v.IssuanceState = "issued"
	v.CredentialID = "cred_" + uuid.NewString()
	s.requests[v.ID] = v
	return c.JSON(v)
}
func (s *server) reset(c fiber.Ctx) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = map[string]issuance{}
	s.tokens = map[string]time.Time{}
	s.verification = map[string]verification{}
	s.faults = map[string]faultConfig{}
	return c.SendStatus(204)
}
func (s *server) setFault(c fiber.Ctx) error {
	var b struct {
		Status int    `json:"status"`
		Route  string `json:"route"`
		Count  int    `json:"count"`
	}
	if json.Unmarshal(c.Body(), &b) != nil || b.Status < 400 || (b.Route != "" && b.Route != "token" && b.Route != "create" && b.Route != "get") {
		return problem(c, 400, "invalid_fault", "status inválido")
	}
	if b.Route == "" {
		b.Route = "create"
	}
	if b.Count < 1 {
		b.Count = 1
	}
	s.mu.Lock()
	s.faults[b.Route] = faultConfig{Status: b.Status, Remaining: b.Count}
	s.mu.Unlock()
	return c.SendStatus(204)
}
func (s *server) consumeFault(route string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	f := s.faults[route]
	if f.Remaining < 1 {
		return 0
	}
	f.Remaining--
	s.faults[route] = f
	return f.Status
}
func problem(c fiber.Ctx, status int, code, detail string) error {
	return c.Status(status).Type("application/problem+json").JSON(fiber.Map{"type": "https://docs.hukid.local/problems/" + code, "title": code, "status": status, "code": code, "detail": detail, "correlationId": c.Get("X-Correlation-Id")})
}
