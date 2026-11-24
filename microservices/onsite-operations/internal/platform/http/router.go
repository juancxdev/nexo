package http

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"nexo-onsite-operations/internal/hukid"
	"nexo-onsite-operations/internal/onsite/application"
	"nexo-onsite-operations/internal/onsite/domain"
	"nexo-onsite-operations/internal/onsite/ports"
	"nexo-onsite-operations/internal/platform/postgres"
)

type Router struct {
	App      *fiber.App
	Service  application.Service
	DB       *postgres.Store
	Verifier *hukid.Client
}

func NewRouter(db *postgres.Store) *Router {
	r := &Router{App: fiber.New(fiber.Config{AppName: "Nexo Onsite Operations", BodyLimit: 1 << 20}), DB: db, Verifier: hukid.NewFromEnv(os.Getenv), Service: application.Service{Tickets: db, Attendance: db, Devices: db}}
	r.routes()
	return r
}
func (r *Router) trace(c fiber.Ctx) string {
	id := c.Get("X-Request-Id")
	if _, err := uuid.Parse(id); err != nil {
		id = uuid.NewString()
	}
	c.Set("X-Request-Id", id)
	c.Locals("trace_id", id)
	return id
}
func traceID(c fiber.Ctx) string {
	if id, ok := c.Locals("trace_id").(string); ok && id != "" {
		return id
	}
	return c.Get("X-Request-Id")
}
func problem(c fiber.Ctx, status int, code, detail string) error {
	id := traceID(c)
	return c.Status(status).Type("application/problem+json").JSON(fiber.Map{"type": "https://errors.example.pe/" + code, "title": code, "status": status, "code": code, "detail": detail, "trace_id": id})
}
func (r *Router) auth(scope string) fiber.Handler {
	return func(c fiber.Ctx) error {
		env := os.Getenv("APP_ENV")
		if env == "" {
			env = "development"
		}
		role := c.Get("X-Mock-Role")
		actor := c.Get("X-Mock-Actor")
		tenant := c.Get("X-Mock-Tenant")
		device := c.Get("X-Mock-Device")
		authz := strings.TrimSpace(c.Get("Authorization"))
		testMode := env == "test" && strings.EqualFold(os.Getenv("NEXO_TEST_MODE"), "true")
		if !testMode && (role != "" || actor != "" || tenant != "" || device != "") {
			return problem(c, 401, "unauthorized", "mock claims are disabled")
		}
		if authz != "" {
			verified, err := verifyBearer(authz)
			if err != nil {
				return problem(c, 401, "unauthorized", "token inválido o vencido")
			}
			actor, tenant, role, device = verified.ActorID, verified.TenantID, verified.Role, verified.DeviceID
			if role == "owner" || role == "admin" {
				role = "manager"
			}
			if !hasScope(verified.Scopes, scope) {
				return problem(c, 403, "scope_forbidden", "el token no tiene el scope requerido")
			}
			c.Locals("claims", verified)
			return c.Next()
		}
		if !testMode && authz == "" {
			return problem(c, 401, "unauthorized", "Authorization es obligatorio")
		}
		if role == "" {
			role = "staff"
		}
		if actor == "" {
			actor = "00000000-0000-0000-0000-000000000004"
		}
		if tenant == "" {
			tenant = "00000000-0000-0000-0000-000000000001"
		}
		if device == "" {
			device = "00000000-0000-0000-0000-000000000103"
		}
		if scope == "onsite:read" && role != "staff" && role != "supervisor" && role != "manager" {
			return problem(c, 403, "assignment_forbidden", "rol sin permiso")
		}
		if scope == "onsite:checkin" && role != "staff" && role != "supervisor" && role != "manager" {
			return problem(c, 403, "assignment_forbidden", "rol sin permiso")
		}
		if scope == "onsite:void" && role != "supervisor" && role != "manager" {
			return problem(c, 403, "scope_forbidden", "se requiere permiso de supervisor")
		}
		c.Locals("claims", ports.Claims{ActorID: actor, TenantID: tenant, Role: role, DeviceID: device, TraceID: traceID(c)})
		return c.Next()
	}
}
func hasScope(scopes []string, wanted string) bool {
	for _, s := range scopes {
		if s == wanted {
			return true
		}
	}
	return false
}
func verifyBearer(header string) (ports.Claims, error) {
	raw := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return ports.Claims{}, errors.New("malformed token")
	}
	var h struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	var p struct {
		Issuer    string   `json:"iss"`
		Audience  string   `json:"aud"`
		Subject   string   `json:"sub"`
		SessionID string   `json:"sid"`
		TenantID  string   `json:"tenant_id"`
		Roles     []string `json:"roles"`
		Scopes    []string `json:"scopes"`
		Expires   int64    `json:"exp"`
		IssuedAt  int64    `json:"iat"`
		NotBefore int64    `json:"nbf"`
	}
	decodePart := func(v string, out any) error {
		b, e := base64.RawURLEncoding.DecodeString(v)
		if e != nil {
			return e
		}
		return json.Unmarshal(b, out)
	}
	if decodePart(parts[0], &h) != nil || decodePart(parts[1], &p) != nil || h.Alg != "RS256" {
		return ports.Claims{}, errors.New("invalid token")
	}
	if p.Expires <= time.Now().Unix() {
		return ports.Claims{}, errors.New("expired token")
	}
	if p.NotBefore != 0 && p.NotBefore > time.Now().Unix()+30 {
		return ports.Claims{}, errors.New("token not active")
	}
	expectedIssuer := os.Getenv("NEXO_JWT_ISSUER")
	if expectedIssuer != "" && p.Issuer != expectedIssuer {
		return ports.Claims{}, errors.New("issuer mismatch")
	}
	expectedAudience := os.Getenv("NEXO_JWT_AUDIENCE")
	if expectedAudience != "" && p.Audience != expectedAudience {
		return ports.Claims{}, errors.New("audience mismatch")
	}
	url := os.Getenv("CORE_JWKS_URL")
	if url == "" {
		url = "http://localhost:8081/.well-known/jwks.json"
	}
	resp, err := http.Get(url)
	if err != nil {
		return ports.Claims{}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var jwks struct {
		Keys []struct{ Kid, N, E string } `json:"keys"`
	}
	if json.Unmarshal(body, &jwks) != nil {
		return ports.Claims{}, errors.New("invalid jwks")
	}
	var pub *rsa.PublicKey
	for _, k := range jwks.Keys {
		if k.Kid == h.Kid {
			nb, e1 := base64.RawURLEncoding.DecodeString(k.N)
			eb, e2 := base64.RawURLEncoding.DecodeString(k.E)
			if e1 != nil || e2 != nil {
				return ports.Claims{}, errors.New("invalid jwk")
			}
			n := new(big.Int).SetBytes(nb)
			ev := new(big.Int).SetBytes(eb).Int64()
			pub = &rsa.PublicKey{N: n, E: int(ev)}
			break
		}
	}
	if pub == nil {
		return ports.Claims{}, errors.New("unknown kid")
	}
	sig, _ := base64.RawURLEncoding.DecodeString(parts[2])
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if rsa.VerifyPKCS1v15(pub, crypto.SHA256, sum[:], sig) != nil {
		return ports.Claims{}, errors.New("invalid signature")
	}
	role := ""
	if len(p.Roles) > 0 {
		role = p.Roles[0]
	}
	return ports.Claims{ActorID: p.Subject, TenantID: p.TenantID, Role: role, Scopes: p.Scopes, DeviceID: "00000000-0000-0000-0000-000000000103"}, nil
}
func claims(c fiber.Ctx) ports.Claims { v, _ := c.Locals("claims").(ports.Claims); return v }
func (r *Router) ensureAssignment(c fiber.Ctx, event, scope string) error {
	ok, err := r.DB.ValidAssignment(c.Context(), claims(c), event, scope)
	if err != nil {
		return problem(c, 503, "persistence_unavailable", "no se pudo validar la asignación")
	}
	if !ok {
		return problem(c, 403, "assignment_forbidden", "el operador no tiene una asignación vigente")
	}
	return nil
}
func (r *Router) routes() {
	a := r.App
	a.Use(func(c fiber.Ctx) error {
		if origin := allowedOrigin(c.Get("Origin")); origin != "" {
			c.Set("Access-Control-Allow-Origin", origin)
			c.Set("Vary", "Origin")
		}
		c.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Idempotency-Key, X-Correlation-Id, X-Request-Id")
		c.Set("Access-Control-Allow-Methods", "GET,POST,PATCH,OPTIONS")
		if c.Method() == "OPTIONS" {
			return c.SendStatus(204)
		}
		r.trace(c)
		return c.Next()
	})
	a.Get("/health/live", func(c fiber.Ctx) error { return c.JSON(fiber.Map{"status": "ok"}) })
	a.Get("/health/ready", func(c fiber.Ctx) error {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := r.DB.Ready(ctx); err != nil {
			return problem(c, 503, "database_unavailable", "base de datos no disponible")
		}
		return c.JSON(fiber.Map{"status": "ok", "database": "ready"})
	})
	a.Get("/onsite/v1/readiness", r.auth("onsite:checkin"), r.readiness)
	a.Get("/onsite/v1/devices", r.auth("onsite:read"), r.listDevices)
	a.Post("/onsite/v1/devices", r.auth("onsite:void"), r.createDevice)
	a.Patch("/onsite/v1/devices/:id", r.auth("onsite:void"), r.updateDevice)
	a.Post("/onsite/v1/devices/:id/revocations", r.auth("onsite:void"), r.revokeDevice)
	a.Post("/onsite/v1/shifts", r.auth("onsite:checkin"), r.openShift)
	a.Post("/onsite/v1/shifts/:id/closures", r.auth("onsite:checkin"), r.closeShift)
	a.Get("/onsite/v1/tickets/search", r.auth("onsite:checkin"), r.search)
	a.Post("/onsite/v1/attendance/check-ins", r.auth("onsite:checkin"), r.checkIn)
	a.Post("/onsite/v1/attendance/check-outs", r.auth("onsite:checkin"), r.checkOut)
	a.Post("/onsite/v1/attendance/facts/:id/voids", r.auth("onsite:void"), r.voidFact)
	a.Get("/onsite/v1/attendance", r.auth("onsite:read"), r.list)
	a.Get("/onsite/v1/dashboard", r.auth("onsite:read"), r.dashboard)
	a.Post("/onsite/v1/verification-sessions", r.auth("onsite:checkin"), r.startVerification)
	a.Get("/onsite/v1/verification-sessions/:id", r.auth("onsite:checkin"), r.getVerification)
}

func (r *Router) startVerification(c fiber.Ctx) error {
	var body struct {
		TTLSeconds int    `json:"ttl_seconds"`
		EventID    string `json:"event_id"`
		Scope      string `json:"scope"`
		TicketID   string `json:"ticket_id"`
	}
	if len(c.Body()) > 0 && json.Unmarshal(c.Body(), &body) != nil {
		return problem(c, 400, "invalid_body", "ttl_seconds debe ser un entero")
	}
	if body.EventID == "" || body.Scope == "" || body.TicketID == "" {
		return problem(c, 400, "invalid_body", "event_id, scope y ticket_id son obligatorios")
	}
	if body.TTLSeconds != 0 && (body.TTLSeconds < 60 || body.TTLSeconds > 900) {
		return problem(c, 400, "invalid_ttl", "ttl_seconds debe estar entre 60 y 900")
	}
	if err := r.ensureAssignment(c, body.EventID, body.Scope); err != nil {
		return err
	}
	key := strings.TrimSpace(c.Get("Idempotency-Key"))
	if _, err := uuid.Parse(key); err != nil {
		return problem(c, 400, "idempotency_key_required", "Idempotency-Key debe ser UUID")
	}
	var existingID, existingStatus, existingEvent, existingScope, existingTicket string
	if err := r.DB.Pool.QueryRow(c.Context(), `SELECT session_id,status,event_id,scope,ticket_id FROM onsite.hukid_verification_sessions WHERE tenant_id=$1::uuid AND idempotency_key=$2`, claims(c).TenantID, key).Scan(&existingID, &existingStatus, &existingEvent, &existingScope, &existingTicket); err == nil {
		if existingEvent != body.EventID || existingScope != body.Scope || existingTicket != body.TicketID {
			return problem(c, 409, "idempotency_key_reused", "la clave fue usada con otro contexto")
		}
		return c.Status(200).JSON(fiber.Map{"session_id": existingID, "status": existingStatus, "replayed": true})
	}
	result, err := r.Verifier.StartVerification(c.Context(), claims(c).TenantID, body.TTLSeconds)
	if errors.Is(err, hukid.ErrNotConfigured) {
		return problem(c, 503, "hukid_not_configured", "el nodo HukID no está configurado")
	}
	if err != nil {
		return hukidProblem(c, err, "no se pudo iniciar la verificación HukID")
	}
	if _, err = r.DB.Pool.Exec(c.Context(), `INSERT INTO onsite.hukid_verification_sessions(session_id,tenant_id,event_id,scope,ticket_id,idempotency_key,expires_at) VALUES($1,$2::uuid,$3,$4,$5,$6,$7)`, result.SessionID, claims(c).TenantID, body.EventID, body.Scope, body.TicketID, key, result.ExpiresAt); err != nil {
		return problem(c, 503, "persistence_unavailable", "no se pudo guardar la sesión HukID")
	}
	return c.Status(201).JSON(result)
}

func (r *Router) getVerification(c fiber.Ctx) error {
	var localTenant string
	if err := r.DB.Pool.QueryRow(c.Context(), `SELECT tenant_id::text FROM onsite.hukid_verification_sessions WHERE session_id=$1 AND tenant_id=$2::uuid`, c.Params("id"), claims(c).TenantID).Scan(&localTenant); err != nil {
		return problem(c, 404, "verification_session_not_found", "sesión no encontrada")
	}
	result, err := r.Verifier.GetVerification(c.Context(), claims(c).TenantID, c.Params("id"))
	if errors.Is(err, hukid.ErrNotConfigured) {
		return problem(c, 503, "hukid_not_configured", "el nodo HukID no está configurado")
	}
	if err != nil {
		return hukidProblem(c, err, "no se pudo consultar la verificación HukID")
	}
	var eventID, scope, ticketID, oldStatus, idem string
	if err := r.DB.Pool.QueryRow(c.Context(), `SELECT event_id,scope,ticket_id,status,idempotency_key FROM onsite.hukid_verification_sessions WHERE session_id=$1 AND tenant_id=$2::uuid`, c.Params("id"), claims(c).TenantID).Scan(&eventID, &scope, &ticketID, &oldStatus, &idem); err != nil {
		return problem(c, 404, "verification_session_not_found", "sesión no encontrada")
	}
	if result.Status == "completed" && result.Result == "pass" && oldStatus != "completed" {
		hash := sha256.Sum256([]byte(c.Params("id") + "|" + ticketID + "|" + eventID + "|" + scope))
		fact, checkErr := r.Service.CheckIn(c.Context(), domain.AttendanceFact{TicketID: ticketID, EventID: eventID, Scope: scope}, hex.EncodeToString(hash[:]), claims(c), idem)
		if checkErr != nil && !errors.Is(checkErr, domain.ErrAlreadyPresent) {
			return problem(c, 422, "verification_checkin_failed", "la verificación fue válida pero no se pudo registrar el check-in")
		}
		_, _ = r.DB.Pool.Exec(c.Context(), `UPDATE onsite.hukid_verification_sessions SET status='completed',result='pass',completed_at=COALESCE(completed_at,now()) WHERE session_id=$1`, c.Params("id"))
		return c.JSON(fiber.Map{"session": result, "check_in": fiber.Map{"id": fact.ID, "ticket_id": ticketID, "event_id": eventID, "scope": scope, "status": "present"}, "replayed": errors.Is(checkErr, domain.ErrAlreadyPresent)})
	}
	if result.Status == "completed" || result.Status == "expired" {
		_, _ = r.DB.Pool.Exec(c.Context(), `UPDATE onsite.hukid_verification_sessions SET status=$2,result=$3,completed_at=CASE WHEN $2='completed' THEN now() ELSE completed_at END WHERE session_id=$1`, c.Params("id"), result.Status, result.Result)
	}
	return c.JSON(result)
}

func hukidProblem(c fiber.Ctx, err error, fallback string) error {
	var httpErr *hukid.HTTPError
	if errors.As(err, &httpErr) {
		code := map[int]string{401: "hukid_unauthorized", 403: "hukid_forbidden", 404: "verification_not_found", 422: "hukid_invalid_request", 429: "hukid_rate_limited", 503: "hukid_unavailable"}[httpErr.Status]
		if code == "" {
			code = "hukid_upstream_error"
		}
		return problem(c, httpErr.Status, code, fallback)
	}
	return problem(c, 503, "hukid_unavailable", fallback)
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

func (r *Router) listDevices(c fiber.Ctx) error {
	items, err := r.Service.Devices.ListDevices(c.Context(), claims(c).TenantID)
	if err != nil {
		return problem(c, 503, "persistence_unavailable", "no se pudieron consultar dispositivos")
	}
	return c.JSON(fiber.Map{"items": items})
}
func (r *Router) createDevice(c fiber.Ctx) error {
	var b struct {
		Label          string `json:"label"`
		VenueID        string `json:"venue_id"`
		PublicKey      string `json:"public_key"`
		ReleaseVersion string `json:"release_version"`
	}
	if json.Unmarshal(c.Body(), &b) != nil || strings.TrimSpace(b.Label) == "" {
		return problem(c, 400, "invalid_body", "label es obligatorio")
	}
	d, err := r.Service.Devices.CreateDevice(c.Context(), domain.Device{TenantID: claims(c).TenantID, Label: strings.TrimSpace(b.Label), VenueID: b.VenueID, PublicKey: b.PublicKey, ReleaseVersion: b.ReleaseVersion})
	if err != nil {
		return problem(c, 409, "device_create_failed", "no se pudo registrar el dispositivo")
	}
	return c.Status(201).JSON(d)
}
func (r *Router) updateDevice(c fiber.Ctx) error {
	var b struct {
		Label          string `json:"label"`
		VenueID        string `json:"venue_id"`
		PublicKey      string `json:"public_key"`
		ReleaseVersion string `json:"release_version"`
	}
	if json.Unmarshal(c.Body(), &b) != nil {
		return problem(c, 400, "invalid_body", "JSON inválido")
	}
	d, err := r.Service.Devices.UpdateDevice(c.Context(), domain.Device{ID: c.Params("id"), TenantID: claims(c).TenantID, Label: b.Label, VenueID: b.VenueID, PublicKey: b.PublicKey, ReleaseVersion: b.ReleaseVersion})
	if err != nil {
		return problem(c, 404, "device_not_found", "dispositivo no encontrado")
	}
	return c.JSON(d)
}
func (r *Router) revokeDevice(c fiber.Ctx) error {
	var b struct {
		Reason string `json:"reason"`
	}
	_ = json.Unmarshal(c.Body(), &b)
	if strings.TrimSpace(b.Reason) == "" {
		return problem(c, 400, "reason_required", "reason es obligatorio")
	}
	if err := r.Service.Devices.RevokeDevice(c.Context(), c.Params("id"), claims(c).TenantID, b.Reason); err != nil {
		return problem(c, 404, "device_not_found", "dispositivo no encontrado")
	}
	return c.SendStatus(204)
}
func (r *Router) openShift(c fiber.Ctx) error {
	var b struct {
		EventID string `json:"event_id"`
		Scope   string `json:"scope"`
	}
	if json.Unmarshal(c.Body(), &b) != nil || b.EventID == "" || b.Scope == "" {
		return problem(c, 400, "invalid_body", "event_id y scope son obligatorios")
	}
	if err := r.ensureAssignment(c, b.EventID, b.Scope); err != nil {
		return err
	}
	sh, err := r.Service.Devices.CreateShift(c.Context(), domain.Shift{EventID: b.EventID, Scope: b.Scope}, claims(c))
	if err != nil {
		return problem(c, 409, "shift_conflict", "no se pudo abrir el turno")
	}
	return c.Status(201).JSON(sh)
}
func (r *Router) closeShift(c fiber.Ctx) error {
	if err := r.Service.Devices.CloseShift(c.Context(), c.Params("id"), claims(c)); err != nil {
		return problem(c, 409, "shift_not_open", "el turno no está abierto")
	}
	return c.SendStatus(204)
}
func (r *Router) readiness(c fiber.Ctx) error {
	if c.Query("event_id") == "" || c.Query("scope") == "" {
		return problem(c, 422, "invalid_scope", "event_id y scope son obligatorios")
	}
	if err := r.ensureAssignment(c, c.Query("event_id"), c.Query("scope")); err != nil {
		return err
	}
	return c.JSON(fiber.Map{"ready": true, "event_id": c.Query("event_id"), "scope": c.Query("scope"), "device_id": claims(c).DeviceID})
}
func (r *Router) search(c fiber.Ctx) error {
	q := strings.TrimSpace(c.Query("q"))
	if !domain.ValidQuery(q) {
		return problem(c, 400, "invalid_query", "q requiere al menos 4 caracteres")
	}
	if c.Query("event_id") == "" || c.Query("scope") == "" {
		return problem(c, 400, "invalid_scope", "event_id y scope son obligatorios")
	}
	if err := r.ensureAssignment(c, c.Query("event_id"), c.Query("scope")); err != nil {
		return err
	}
	items, err := r.Service.Search(c.Context(), q, c.Query("event_id"), c.Query("scope"), claims(c).TenantID)
	if err != nil {
		return problem(c, 503, "persistence_unavailable", "no se pudo buscar")
	}
	out := make([]fiber.Map, 0, len(items))
	for _, t := range items {
		out = append(out, fiber.Map{"ticket_id": t.ID, "masked_holder": t.MaskedHolder, "status": t.Status})
	}
	return c.JSON(out)
}
func (r *Router) checkIn(c fiber.Ctx) error {
	var b struct {
		TicketID              string `json:"ticket_id"`
		EventID               string `json:"event_id"`
		Scope                 string `json:"scope"`
		VerificationSessionID string `json:"verification_session_id"`
	}
	if err := json.Unmarshal(c.Body(), &b); err != nil {
		return problem(c, 400, "invalid_body", "JSON inválido")
	}
	if b.TicketID == "" || b.EventID == "" || b.Scope == "" {
		return problem(c, 400, "invalid_body", "ticket_id, event_id y scope son obligatorios")
	}
	if err := r.ensureAssignment(c, b.EventID, b.Scope); err != nil {
		return err
	}
	if os.Getenv("APP_ENV") == "production" {
		if b.VerificationSessionID == "" {
			return problem(c, 403, "verification_required", "se requiere una presentación HukID válida")
		}
		var sessionEvent, sessionScope, sessionTicket, sessionStatus, sessionResult string
		err := r.DB.Pool.QueryRow(c.Context(), `SELECT event_id,scope,ticket_id,status,COALESCE(result,'') FROM onsite.hukid_verification_sessions WHERE session_id=$1 AND tenant_id=$2::uuid`, b.VerificationSessionID, claims(c).TenantID).Scan(&sessionEvent, &sessionScope, &sessionTicket, &sessionStatus, &sessionResult)
		if err != nil || sessionEvent != b.EventID || sessionScope != b.Scope || sessionTicket != b.TicketID || sessionStatus != "completed" || sessionResult != "pass" {
			return problem(c, 403, "verification_required", "la presentación HukID no corresponde al ticket")
		}
	}
	key := strings.TrimSpace(c.Get("Idempotency-Key"))
	if _, err := uuid.Parse(key); err != nil {
		return problem(c, 400, "idempotency_key_required", "Idempotency-Key debe ser UUID")
	}
	hash := sha256.Sum256(c.Body())
	fact, err := r.Service.CheckIn(c.Context(), domain.AttendanceFact{TicketID: b.TicketID, EventID: b.EventID, Scope: b.Scope}, hex.EncodeToString(hash[:]), claims(c), key)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrAlreadyPresent):
			return problem(c, 409, "already_present", "el ticket ya tiene presencia efectiva")
		case errors.Is(err, domain.ErrIdempotencyKeyReused):
			return problem(c, 409, "idempotency_key_reused", "la clave fue usada con otro body")
		case errors.Is(err, domain.ErrIdempotencyKeyRequired):
			return problem(c, 400, "idempotency_key_required", "Idempotency-Key es obligatorio")
		default:
			return problem(c, 422, err.Error(), err.Error())
		}
	}
	return c.Status(201).JSON(fiber.Map{"id": fact.ID, "ticket_id": fact.TicketID, "event_id": fact.EventID, "scope": fact.Scope, "type": fact.Type, "occurred_at": fact.OccurredAt, "effective_status": fact.EffectiveStatus})
}
func (r *Router) checkOut(c fiber.Ctx) error {
	var b struct {
		TicketID string `json:"ticket_id"`
		EventID  string `json:"event_id"`
		Scope    string `json:"scope"`
	}
	if json.Unmarshal(c.Body(), &b) != nil || b.TicketID == "" || b.EventID == "" || b.Scope == "" {
		return problem(c, 400, "invalid_body", "ticket_id, event_id y scope son obligatorios")
	}
	if err := r.ensureAssignment(c, b.EventID, b.Scope); err != nil {
		return err
	}
	key := strings.TrimSpace(c.Get("Idempotency-Key"))
	if _, err := uuid.Parse(key); err != nil {
		return problem(c, 400, "idempotency_key_required", "Idempotency-Key debe ser UUID")
	}
	hash := sha256.Sum256(c.Body())
	fact, err := r.Service.CheckOut(c.Context(), domain.AttendanceFact{TicketID: b.TicketID, EventID: b.EventID, Scope: b.Scope}, hex.EncodeToString(hash[:]), claims(c), key)
	if err != nil {
		if errors.Is(err, domain.ErrNotPresent) {
			return problem(c, 409, "not_present", "el ticket no tiene presencia efectiva")
		}
		if errors.Is(err, domain.ErrIdempotencyKeyReused) {
			return problem(c, 409, "idempotency_key_reused", "la clave fue usada con otro body")
		}
		return problem(c, 422, err.Error(), err.Error())
	}
	return c.Status(201).JSON(fact)
}
func (r *Router) voidFact(c fiber.Ctx) error {
	var b struct {
		Reason string `json:"reason"`
	}
	if json.Unmarshal(c.Body(), &b) != nil || strings.TrimSpace(b.Reason) == "" {
		return problem(c, 400, "reason_required", "reason es obligatorio")
	}
	key := strings.TrimSpace(c.Get("Idempotency-Key"))
	if _, err := uuid.Parse(key); err != nil {
		return problem(c, 400, "idempotency_key_required", "Idempotency-Key debe ser UUID")
	}
	payload := struct{ FactID, Reason string }{c.Params("id"), b.Reason}
	hash := sha256.Sum256(c.Body())
	fact, err := r.Service.Void(c.Context(), payload.FactID, payload.Reason, hex.EncodeToString(hash[:]), claims(c), key)
	if err != nil {
		if errors.Is(err, domain.ErrIdempotencyKeyReused) {
			return problem(c, 409, "idempotency_key_reused", "la clave fue usada con otro body")
		}
		return problem(c, 409, err.Error(), err.Error())
	}
	return c.Status(201).JSON(fact)
}
func (r *Router) list(c fiber.Ctx) error {
	if c.Query("event_id") == "" || c.Query("scope") == "" {
		return problem(c, 400, "invalid_scope", "event_id y scope son obligatorios")
	}
	if err := r.ensureAssignment(c, c.Query("event_id"), c.Query("scope")); err != nil {
		return err
	}
	limit := 50
	if raw := c.Query("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 100 {
			return problem(c, 400, "invalid_limit", "limit debe estar entre 1 y 100")
		}
		limit = parsed
	}
	cursor := strings.TrimSpace(c.Query("cursor"))
	if cursor != "" {
		if _, err := time.Parse(time.RFC3339Nano, cursor); err != nil {
			return problem(c, 400, "invalid_cursor", "cursor debe ser una fecha RFC3339")
		}
	}
	page, err := r.Service.List(c.Context(), c.Query("event_id"), c.Query("scope"), claims(c).TenantID, cursor, limit)
	if err != nil {
		return problem(c, 503, "persistence_unavailable", "no se pudo consultar historial")
	}
	items := make([]fiber.Map, 0, len(page.Items))
	for _, fact := range page.Items {
		items = append(items, fiber.Map{
			"id":                  fact.ID,
			"ticket_id":           fact.TicketID,
			"event_id":            fact.EventID,
			"scope":               fact.Scope,
			"type":                fact.Type,
			"actor_id":            fact.ActorID,
			"device_id":           fact.DeviceID,
			"occurred_at":         fact.OccurredAt,
			"reason":              fact.Reason,
			"compensates_fact_id": fact.CompensatesFactID,
			"effective_status":    fact.EffectiveStatus,
		})
	}
	return c.JSON(fiber.Map{"items": items, "next_cursor": page.NextCursor})
}
func (r *Router) dashboard(c fiber.Ctx) error {
	event := c.Query("event_id")
	if event == "" {
		return problem(c, 400, "invalid_event", "event_id es obligatorio")
	}
	if err := r.ensureAssignment(c, event, "main"); err != nil {
		return err
	}
	data, err := r.Service.Dashboard(c.Context(), event, claims(c).TenantID)
	if err != nil {
		return problem(c, 503, "persistence_unavailable", "no se pudo consultar dashboard")
	}
	return c.JSON(data)
}

var _ = fmt.Sprintf
