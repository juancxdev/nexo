package httpapi

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"io"
	"math/big"
	"net/http"
	"os"
	"strings"
	"time"

	"nexo-event-operations/internal/secureclaims"
)

type Server struct {
	App *fiber.App
	DB  *pgxpool.Pool
}

func New(db *pgxpool.Pool) *Server {
	s := &Server{DB: db, App: fiber.New(fiber.Config{AppName: "Nexo Event Operations"})}
	s.routes()
	return s
}
func (s *Server) routes() {
	a := s.App
	a.Use(func(c fiber.Ctx) error {
		if origin := allowedOrigin(c.Get("Origin")); origin != "" {
			c.Set("Access-Control-Allow-Origin", origin)
			c.Set("Vary", "Origin")
		}
		c.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Idempotency-Key, X-Mock-Tenant, X-Mock-Actor")
		c.Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
		if c.Method() == "OPTIONS" {
			return c.SendStatus(204)
		}
		return c.Next()
	})
	a.Get("/health/live", func(c fiber.Ctx) error { return c.JSON(fiber.Map{"status": "ok"}) })
	a.Get("/health/ready", func(c fiber.Ctx) error {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := s.DB.Ping(ctx); err != nil {
			return problem(c, 503, "database_unavailable", "base de datos no disponible")
		}
		return c.JSON(fiber.Map{"status": "ok"})
	})
	a.Post("/operations/v1/report-jobs", s.private("reports:read"), s.createReport)
	a.Get("/operations/v1/report-jobs/:id", s.private("reports:read"), s.getReport)
	a.Post("/operations/v1/report-jobs/:id/cancellations", s.private("reports:read"), s.cancelReport)
	a.Post("/operations/v1/certificate-jobs", s.private("certificates:issue"), s.issueCertificate)
	a.Get("/operations/v1/certificates/:id", s.private("certificates:issue"), s.getCertificate)
	a.Post("/operations/v1/certificates/:id/hukid-deliveries", s.private("certificates:issue"), s.createDelivery)
	a.Get("/operations/v1/certificates/:id/hukid-deliveries", s.private("certificates:issue"), s.getDelivery)
	a.Post("/operations/v1/hukid-deliveries/:id/cancellations", s.private("certificates:issue"), s.cancelDelivery)
	a.Post("/operations/v1/hukid-deliveries/:id/retries", s.private("certificates:issue"), s.retryDelivery)
	a.Get("/public/v1/certificates/:code", s.verifyCertificate)
}

func allowedOrigin(origin string) string {
	if origin == "" {
		return ""
	}
	configured := os.Getenv("CORS_ALLOWED_ORIGINS")
	if configured == "" && os.Getenv("APP_ENV") != "production" {
		configured = "http://localhost:5174,http://127.0.0.1:5174"
	}
	for _, candidate := range strings.Split(configured, ",") {
		if strings.TrimSpace(candidate) == origin {
			return origin
		}
	}
	return ""
}
func tenant(c fiber.Ctx) string {
	if v, ok := c.Locals("tenant_id").(string); ok && v != "" {
		return v
	}
	v := c.Get("X-Mock-Tenant")
	if v == "" {
		v = "00000000-0000-0000-0000-000000000001"
	}
	return v
}
func actor(c fiber.Ctx) string {
	if v, ok := c.Locals("actor_id").(string); ok && v != "" {
		return v
	}
	v := c.Get("X-Mock-Actor")
	if v == "" {
		v = "00000000-0000-0000-0000-000000000004"
	}
	return v
}
func (s *Server) private(scope string) fiber.Handler {
	return func(c fiber.Ctx) error {
		if auth := strings.TrimSpace(c.Get("Authorization")); auth != "" {
			claims, err := verifyBearer(auth)
			if err != nil {
				return problem(c, 401, "unauthorized", "token inválido o vencido")
			}
			if !hasScope(claims.Scopes, scope) {
				return problem(c, 403, "scope_forbidden", "el token no tiene el scope requerido")
			}
			c.Locals("tenant_id", claims.TenantID)
			c.Locals("actor_id", claims.Subject)
			return c.Next()
		}
		if os.Getenv("APP_ENV") != "development" && os.Getenv("APP_ENV") != "test" {
			return problem(c, 401, "unauthorized", "Authorization es obligatorio")
		}
		return c.Next()
	}
}

type bearerClaims struct {
	Issuer, Audience, Subject, TenantID string
	Scopes                              []string
	IssuedAt, NotBefore, Expires        int64
}

func hasScope(scopes []string, wanted string) bool {
	for _, v := range scopes {
		if v == wanted {
			return true
		}
	}
	return false
}
func verifyBearer(header string) (bearerClaims, error) {
	parts := strings.Split(strings.TrimSpace(strings.TrimPrefix(header, "Bearer ")), ".")
	if len(parts) != 3 {
		return bearerClaims{}, errors.New("malformed token")
	}
	decode := func(part string, out any) error {
		b, err := base64.RawURLEncoding.DecodeString(part)
		if err != nil {
			return err
		}
		return json.Unmarshal(b, out)
	}
	var h struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	var p struct {
		Issuer    string   `json:"iss"`
		Audience  string   `json:"aud"`
		Subject   string   `json:"sub"`
		TenantID  string   `json:"tenant_id"`
		Scopes    []string `json:"scopes"`
		IssuedAt  int64    `json:"iat"`
		NotBefore int64    `json:"nbf"`
		Expires   int64    `json:"exp"`
	}
	now := time.Now().Unix()
	if decode(parts[0], &h) != nil || decode(parts[1], &p) != nil || h.Alg != "RS256" || h.Kid == "" || p.Expires <= now || p.Subject == "" || uuid.Validate(p.Subject) != nil || uuid.Validate(p.TenantID) != nil {
		return bearerClaims{}, errors.New("invalid token")
	}
	if p.NotBefore != 0 && p.NotBefore > now+30 {
		return bearerClaims{}, errors.New("token not active")
	}
	if expected := os.Getenv("NEXO_JWT_ISSUER"); expected != "" && p.Issuer != expected {
		return bearerClaims{}, errors.New("issuer mismatch")
	}
	if expected := os.Getenv("NEXO_JWT_AUDIENCE"); expected != "" && p.Audience != expected {
		return bearerClaims{}, errors.New("audience mismatch")
	}
	url := os.Getenv("CORE_JWKS_URL")
	if url == "" {
		url = "http://localhost:8081/.well-known/jwks.json"
	}
	resp, err := (&http.Client{Timeout: 2 * time.Second}).Get(url)
	if err != nil || resp.StatusCode != 200 {
		if resp != nil {
			resp.Body.Close()
		}
		return bearerClaims{}, errors.New("jwks unavailable")
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var jwks struct {
		Keys []struct{ Kid, N, E string } `json:"keys"`
	}
	if json.Unmarshal(body, &jwks) != nil {
		return bearerClaims{}, errors.New("invalid jwks")
	}
	var pub *rsa.PublicKey
	for _, k := range jwks.Keys {
		if k.Kid == h.Kid {
			nb, e1 := base64.RawURLEncoding.DecodeString(k.N)
			eb, e2 := base64.RawURLEncoding.DecodeString(k.E)
			if e1 != nil || e2 != nil {
				return bearerClaims{}, errors.New("invalid jwk")
			}
			pub = &rsa.PublicKey{N: new(big.Int).SetBytes(nb), E: int(new(big.Int).SetBytes(eb).Int64())}
			break
		}
	}
	if pub == nil {
		return bearerClaims{}, errors.New("unknown kid")
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return bearerClaims{}, err
	}
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if rsa.VerifyPKCS1v15(pub, crypto.SHA256, sum[:], sig) != nil {
		return bearerClaims{}, errors.New("invalid signature")
	}
	return bearerClaims{Issuer: p.Issuer, Audience: p.Audience, Subject: p.Subject, TenantID: p.TenantID, Scopes: p.Scopes, IssuedAt: p.IssuedAt, NotBefore: p.NotBefore, Expires: p.Expires}, nil
}
func (s *Server) createReport(c fiber.Ctx) error {
	var b struct {
		EventID string `json:"event_id"`
		Scope   string `json:"scope"`
		Format  string `json:"format"`
	}
	if json.Unmarshal(c.Body(), &b) != nil || b.EventID == "" {
		return problem(c, 400, "invalid_body", "event_id es obligatorio")
	}
	if b.Format == "" {
		b.Format = "csv"
	}
	if b.Format != "csv" && b.Format != "json" {
		return problem(c, 400, "invalid_format", "format debe ser csv o json")
	}
	id := uuid.New()
	_, err := s.DB.Exec(c.Context(), `INSERT INTO operations.report_jobs(id,tenant_id,requested_by,event_id,scope,format,status) VALUES($1,$2::uuid,$3::uuid,$4,$5,$6,'pending')`, id, tenant(c), actor(c), b.EventID, b.Scope, b.Format)
	if err != nil {
		return problem(c, 503, "persistence_unavailable", err.Error())
	}
	return c.Status(202).JSON(fiber.Map{"id": id, "status": "pending", "progress": 0})
}
func (s *Server) getReport(c fiber.Ctx) error {
	var id string
	var status string
	var progress int
	var errCode *string
	err := s.DB.QueryRow(c.Context(), `SELECT id::text,status,progress,error_code FROM operations.report_jobs WHERE id=$1 AND tenant_id=$2::uuid`, c.Params("id"), tenant(c)).Scan(&id, &status, &progress, &errCode)
	if err != nil {
		return problem(c, 404, "report_not_found", "reporte no encontrado")
	}
	return c.JSON(fiber.Map{"id": id, "status": status, "progress": progress, "error_code": errCode})
}
func (s *Server) cancelReport(c fiber.Ctx) error {
	_, err := s.DB.Exec(c.Context(), `UPDATE operations.report_jobs SET status='cancelled',updated_at=now() WHERE id=$1 AND tenant_id=$2::uuid AND status IN ('pending','running')`, c.Params("id"), tenant(c))
	if err != nil {
		return problem(c, 503, "persistence_unavailable", err.Error())
	}
	return c.SendStatus(204)
}
func (s *Server) issueCertificate(c fiber.Ctx) error {
	var b struct {
		EventID        string `json:"event_id"`
		RegistrationID string `json:"registration_id"`
		DisplayName    string `json:"display_name"`
	}
	if json.Unmarshal(c.Body(), &b) != nil || b.EventID == "" || b.RegistrationID == "" || b.DisplayName == "" {
		return problem(c, 400, "invalid_body", "event_id, registration_id y display_name son obligatorios")
	}
	id := uuid.New()
	code := "NX-" + id.String()[:8]
	sum := sha256.Sum256([]byte("nexo:certificate-evidence:v1|" + tenant(c) + "|" + b.EventID + "|" + b.RegistrationID + "|" + code))
	evidence := base64.RawURLEncoding.EncodeToString(sum[:])
	_, err := s.DB.Exec(c.Context(), `INSERT INTO certificates.certificates(id,tenant_id,event_id,registration_id,verification_code,display_name,state,evidence_digest) VALUES($1,$2::uuid,$3,$4,$5,$6,'eligible',$7)`, id, tenant(c), b.EventID, b.RegistrationID, code, b.DisplayName, evidence)
	if err != nil {
		return problem(c, 503, "persistence_unavailable", err.Error())
	}
	return c.Status(201).JSON(fiber.Map{"id": id, "verification_code": code, "state": "eligible", "evidence_digest": evidence, "next_action": "request_hukid_delivery"})
}
func (s *Server) getCertificate(c fiber.Ctx) error {
	var id, code, state, evidence string
	err := s.DB.QueryRow(c.Context(), `SELECT id::text,verification_code,state,evidence_digest FROM certificates.certificates WHERE id=$1 AND tenant_id=$2::uuid`, c.Params("id"), tenant(c)).Scan(&id, &code, &state, &evidence)
	if err != nil {
		return problem(c, 404, "certificate_not_found", "certificado no encontrado")
	}
	return c.JSON(fiber.Map{"id": id, "verification_code": code, "state": state, "evidence_digest": evidence})
}
func (s *Server) verifyCertificate(c fiber.Ctx) error {
	var id, code, state, evidence string
	err := s.DB.QueryRow(c.Context(), `SELECT id::text,verification_code,state,evidence_digest FROM certificates.certificates WHERE verification_code=$1`, c.Params("code")).Scan(&id, &code, &state, &evidence)
	if err != nil {
		return problem(c, 404, "certificate_not_found", "certificado no encontrado")
	}
	// Public verification deliberately excludes the participant name and all
	// registration data. A holder can prove the credential through HukID.
	return c.JSON(fiber.Map{"id": id, "verification_code": code, "state": state, "evidence_digest": evidence})
}
func (s *Server) createDelivery(c fiber.Ctx) error {
	var in struct {
		Consent bool `json:"consent"`
	}
	if json.Unmarshal(c.Body(), &in) != nil || !in.Consent {
		return problem(c, 400, "consent_required", "se requiere consentimiento para enviar el nombre a HukID")
	}
	var cert, registrationID, displayName, eventID, code, evidence, state string
	err := s.DB.QueryRow(c.Context(), `SELECT id::text,registration_id,display_name,event_id,verification_code,evidence_digest,state FROM certificates.certificates WHERE id=$1 AND tenant_id=$2::uuid`, c.Params("id"), tenant(c)).Scan(&cert, &registrationID, &displayName, &eventID, &code, &evidence, &state)
	if err != nil {
		return problem(c, 404, "certificate_not_found", "certificado no encontrado")
	}
	key := c.Get("Idempotency-Key")
	if key == "" {
		return problem(c, 400, "idempotency_key_required", "Idempotency-Key es obligatorio")
	}
	var existingID, existingState string
	if err := s.DB.QueryRow(c.Context(), `SELECT id::text,state FROM integration.hukid_deliveries WHERE tenant_id=$1::uuid AND source_reference=$2`, tenant(c), cert).Scan(&existingID, &existingState); err == nil {
		return c.Status(200).JSON(fiber.Map{"id": existingID, "certificate_id": cert, "state": existingState, "source_reference": cert, "replayed": true})
	}
	if state != "eligible" && state != "failed" && state != "cancelled" {
		return problem(c, 409, "certificate_not_deliverable", "el certificado ya tiene una entrega HukID activa o finalizada")
	}
	config := uuid.Nil
	var issuer, definition string
	err = s.DB.QueryRow(c.Context(), `SELECT id,issuer_id,credential_definition FROM integration.hukid_configs WHERE tenant_id=$1::uuid AND status='active' LIMIT 1`, tenant(c)).Scan(&config, &issuer, &definition)
	if err != nil {
		return problem(c, 409, "hukid_not_configured", "no existe configuración HukID")
	}
	claims := map[string]any{
		"certificate": map[string]any{"id": cert, "verificationCode": code, "issuedAt": time.Now().UTC().Format(time.RFC3339), "verificationUrl": publicVerificationURL(code), "evidenceDigest": evidence},
		"recipient":   map[string]any{"registrationId": registrationID, "displayName": displayName},
		"event":       map[string]any{"id": eventID},
		"achievement": map[string]any{"attendanceRate": 1.0},
	}
	payload := map[string]any{"issuerId": issuer, "credentialDefinition": definition, "claims": claims, "delivery": map[string]any{"flow": "pre-authorized_code", "expiresInSeconds": 300, "txCodeRequired": true}, "source": map[string]any{"system": "nexo", "reference": cert, "evidenceDigest": evidence}}
	raw, err := json.Marshal(payload)
	if err != nil {
		return problem(c, 500, "claims_serialization_failed", "no se pudo preparar la solicitud HukID")
	}
	digest := sha256.Sum256(raw)
	ciphertext, nonce, keyVersion, err := secureclaims.Encrypt(raw)
	if err != nil {
		return problem(c, 503, "claims_protection_unavailable", "no se pudo proteger la solicitud HukID")
	}
	id := uuid.New()
	tx, err := s.DB.Begin(c.Context())
	if err != nil {
		return problem(c, 503, "persistence_unavailable", "no se pudo iniciar la entrega")
	}
	defer tx.Rollback(c.Context())
	_, err = tx.Exec(c.Context(), `INSERT INTO integration.hukid_deliveries(id,tenant_id,certificate_id,config_id,idempotency_key,source_reference,state,claims_ciphertext,claims_nonce,claims_key_version,claims_hash,consent_granted_at) VALUES($1,$2::uuid,$3,$4,$5,$6,'pending',$7,$8,$9,$10,now())`, id, tenant(c), cert, config, key, cert, ciphertext, nonce, keyVersion, base64.RawURLEncoding.EncodeToString(digest[:]))
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			var replayID, replayState, replayKey string
			if lookupErr := s.DB.QueryRow(c.Context(), `SELECT id::text,state,idempotency_key FROM integration.hukid_deliveries WHERE tenant_id=$1::uuid AND source_reference=$2`, tenant(c), cert).Scan(&replayID, &replayState, &replayKey); lookupErr == nil {
				if replayKey != key {
					return problem(c, 409, "delivery_exists", "el certificado ya tiene una entrega HukID")
				}
				return c.Status(200).JSON(fiber.Map{"id": replayID, "certificate_id": cert, "state": replayState, "source_reference": cert, "replayed": true})
			}
		}
		return problem(c, 503, "persistence_unavailable", err.Error())
	}
	if _, err = tx.Exec(c.Context(), `UPDATE certificates.certificates SET state='request_pending',claims_hash=$3,consent_granted_at=now() WHERE id=$1::uuid AND tenant_id=$2::uuid`, cert, tenant(c), base64.RawURLEncoding.EncodeToString(digest[:])); err != nil {
		return problem(c, 503, "persistence_unavailable", err.Error())
	}
	if err = tx.Commit(c.Context()); err != nil {
		return problem(c, 503, "persistence_unavailable", err.Error())
	}
	return c.Status(202).JSON(fiber.Map{"id": id, "certificate_id": cert, "state": "pending", "source_reference": cert})
}
func (s *Server) getDelivery(c fiber.Ctx) error {
	var id, cert, state string
	err := s.DB.QueryRow(c.Context(), `SELECT id::text,certificate_id::text,state FROM integration.hukid_deliveries WHERE certificate_id=$1 AND tenant_id=$2::uuid`, c.Params("id"), tenant(c)).Scan(&id, &cert, &state)
	if err != nil {
		return problem(c, 404, "delivery_not_found", "entrega no encontrada")
	}
	return c.JSON(fiber.Map{"id": id, "certificate_id": cert, "state": state})
}
func (s *Server) cancelDelivery(c fiber.Ctx) error {
	command, err := s.DB.Exec(c.Context(), `UPDATE integration.hukid_deliveries SET state='cancelled',updated_at=now() WHERE id=$1::uuid AND tenant_id=$2::uuid AND state IN ('pending','request_created','offer_redeemed')`, c.Params("id"), tenant(c))
	if err != nil {
		return problem(c, 503, "persistence_unavailable", "no se pudo cancelar la entrega")
	}
	if command.RowsAffected() == 0 {
		return problem(c, 409, "delivery_not_cancellable", "la entrega no puede cancelarse")
	}
	_, _ = s.DB.Exec(c.Context(), `UPDATE certificates.certificates SET state='cancelled' WHERE id=(SELECT certificate_id FROM integration.hukid_deliveries WHERE id=$1::uuid)`, c.Params("id"))
	return c.SendStatus(204)
}
func (s *Server) retryDelivery(c fiber.Ctx) error {
	command, err := s.DB.Exec(c.Context(), `UPDATE integration.hukid_deliveries SET state='pending',last_error_code=NULL,next_attempt_at=now(),updated_at=now() WHERE id=$1::uuid AND tenant_id=$2::uuid AND state IN ('failed','cancelled')`, c.Params("id"), tenant(c))
	if err != nil {
		return problem(c, 503, "persistence_unavailable", "no se pudo reintentar la entrega")
	}
	if command.RowsAffected() == 0 {
		return problem(c, 409, "delivery_not_retryable", "la entrega no puede reintentarse")
	}
	_, _ = s.DB.Exec(c.Context(), `UPDATE certificates.certificates SET state='request_pending' WHERE id=(SELECT certificate_id FROM integration.hukid_deliveries WHERE id=$1::uuid)`, c.Params("id"))
	return c.Status(202).JSON(fiber.Map{"id": c.Params("id"), "state": "pending"})
}
func publicVerificationURL(code string) string {
	base := strings.TrimRight(os.Getenv("NEXO_PUBLIC_BASE_URL"), "/")
	if base == "" {
		base = "http://localhost:8083"
	}
	return base + "/public/v1/certificates/" + code
}
func problem(c fiber.Ctx, status int, code, detail string) error {
	return c.Status(status).Type("application/problem+json").JSON(fiber.Map{"type": "https://errors.example.pe/" + code, "title": code, "status": status, "code": code, "detail": detail})
}
func _() { _ = os.Getenv }
