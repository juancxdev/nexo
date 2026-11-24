package http

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"nexo-onsite-operations/internal/onsite/domain"
	"nexo-onsite-operations/internal/platform/postgres"
)

// Rutas del QR firmado (OO-010/016/017) y de sesiones (OO-015).
func (r *Router) registerScanRoutes(a *fiber.App) {
	a.Post("/onsite/v1/scans", r.auth("onsite:checkin"), r.scan)
	a.Post("/onsite/v1/scans/provisional", r.auth("onsite:void"), r.resolveProvisional)
	a.Get("/onsite/v1/scan-stats", r.auth("onsite:read"), r.scanStats)
	a.Get("/onsite/v1/sessions", r.auth("onsite:read"), r.listSessions)
	a.Post("/onsite/v1/sessions/:code/entries", r.auth("onsite:checkin"), r.enterSession)
}

type scanBody struct {
	Token   string `json:"token"`
	EventID string `json:"event_id"`
	Scope   string `json:"scope"`
}

func bodyHash(c fiber.Ctx) string {
	h := sha256.Sum256(c.Body())
	return hex.EncodeToString(h[:])
}

func idemKey(c fiber.Ctx) (string, bool) {
	key := strings.TrimSpace(c.Get("Idempotency-Key"))
	_, err := uuid.Parse(key)
	return key, err == nil
}

// scan verifica el QR. Un rechazo responde 200 con accepted=false y el motivo:
// es un resultado esperado de la puerta, no un error del sistema.
func (r *Router) scan(c fiber.Ctx) error {
	var b scanBody
	if json.Unmarshal(c.Body(), &b) != nil || b.Token == "" || b.EventID == "" || b.Scope == "" {
		return problem(c, 400, "invalid_body", "token, event_id y scope son obligatorios")
	}
	if err := r.ensureAssignment(c, b.EventID, b.Scope); err != nil {
		return err
	}
	res, err := r.DB.VerifyScan(c.Context(), b.Token, b.EventID, b.Scope, claims(c))
	if err != nil {
		return problem(c, 503, "persistence_unavailable", "no se pudo verificar el QR")
	}
	return c.JSON(res)
}

func (r *Router) resolveProvisional(c fiber.Ctx) error {
	var b scanBody
	if json.Unmarshal(c.Body(), &b) != nil || b.Token == "" || b.EventID == "" || b.Scope == "" {
		return problem(c, 400, "invalid_body", "token, event_id y scope son obligatorios")
	}
	cl := claims(c)
	if cl.Role != "supervisor" && cl.Role != "manager" {
		return problem(c, 403, "role_forbidden", "sólo supervisión puede aceptar un ticket provisional")
	}
	key, ok := idemKey(c)
	if !ok {
		return problem(c, 400, "idempotency_key_required", "Idempotency-Key debe ser UUID")
	}
	t, err := r.DB.ResolveProvisional(c.Context(), b.Token, b.EventID, b.Scope, bodyHash(c), key, cl)
	switch {
	case err == nil:
		return c.Status(201).JSON(t)
	case errors.Is(err, postgres.ErrRevocationsStale):
		return problem(c, 409, "revocations_stale", "la lista de revocaciones no está al día")
	case errors.Is(err, postgres.ErrNotProvisionalCandidate):
		return problem(c, 409, "not_provisional_candidate", "el ticket ya está en la proyección")
	case errors.Is(err, domain.ErrIdempotencyKeyReused):
		return problem(c, 409, "idempotency_key_reused", "la clave fue usada con otro body")
	case strings.HasPrefix(err.Error(), "qr_") || strings.HasPrefix(err.Error(), "ticket_") || strings.HasPrefix(err.Error(), "wrong_"):
		return problem(c, 422, err.Error(), "el QR no es válido para este punto de control")
	}
	return problem(c, 503, "persistence_unavailable", "no se pudo registrar el ticket provisional")
}

func (r *Router) scanStats(c fiber.Ctx) error {
	event := c.Query("event_id")
	if event == "" {
		return problem(c, 400, "invalid_query", "event_id es obligatorio")
	}
	stats, err := r.DB.ScanStats(c.Context(), event, claims(c).TenantID)
	if err != nil {
		return problem(c, 503, "persistence_unavailable", "no se pudieron leer los rechazos")
	}
	total := 0
	for _, n := range stats {
		total += n
	}
	return c.JSON(fiber.Map{"event_id": event, "window": "24h", "total": total, "by_reason": stats})
}

func (r *Router) listSessions(c fiber.Ctx) error {
	event := c.Query("event_id")
	if event == "" {
		return problem(c, 400, "invalid_query", "event_id es obligatorio")
	}
	items, err := r.DB.ListSessions(c.Context(), event, claims(c).TenantID)
	if err != nil {
		return problem(c, 503, "persistence_unavailable", "no se pudieron leer las sesiones")
	}
	return c.JSON(fiber.Map{"items": items})
}

func (r *Router) enterSession(c fiber.Ctx) error {
	var b struct {
		EventID  string `json:"event_id"`
		TicketID string `json:"ticket_id"`
		Token    string `json:"token"`
	}
	if json.Unmarshal(c.Body(), &b) != nil || b.EventID == "" || (b.TicketID == "" && b.Token == "") {
		return problem(c, 400, "invalid_body", "event_id y ticket_id o token son obligatorios")
	}
	session := c.Params("code")
	if err := r.ensureAssignment(c, b.EventID, session); err != nil {
		return err
	}
	key, ok := idemKey(c)
	if !ok {
		return problem(c, 400, "idempotency_key_required", "Idempotency-Key debe ser UUID")
	}
	cl := claims(c)
	entry, err := r.DB.EnterSession(c.Context(), b.EventID, session, strings.TrimSpace(b.TicketID), strings.TrimSpace(b.Token), bodyHash(c), key, cl)
	if err == nil {
		return c.Status(201).JSON(entry)
	}
	code := err.Error()
	switch {
	case errors.Is(err, domain.ErrIdempotencyKeyReused):
		return problem(c, 409, "idempotency_key_reused", "la clave fue usada con otro body")
	case code == domain.RejectSessionNotFound:
		return problem(c, 404, code, "la sesión no existe")
	case code == domain.RejectAlreadyInSession || code == domain.RejectSessionFull:
		return problem(c, 409, code, "no se puede registrar la entrada")
	case strings.HasPrefix(code, "session_") || strings.HasPrefix(code, "qr_") || strings.HasPrefix(code, "ticket_") || strings.HasPrefix(code, "wrong_"):
		return problem(c, 422, code, "el ticket no puede entrar a esta sesión")
	}
	return problem(c, 503, "persistence_unavailable", "no se pudo registrar la entrada")
}
