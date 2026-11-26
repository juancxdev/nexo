package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"html/template"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const eligibilityRule = "attendance@1.0.0"

func (s *Server) certificateRoutes() {
	a := s.App
	a.Get("/operations/v1/certificates", s.private("certificates:issue"), s.listCertificates)
	a.Get("/operations/v1/eligible-registrations", s.private("certificates:issue"), s.eligibleRegistrations)
	a.Post("/operations/v1/certificates/:id/revocations", s.private("certificates:issue"), s.revokeCertificate)
	a.Post("/operations/v1/certificates/:id/reissues", s.private("certificates:issue"), s.reissueCertificate)
}

func validReason(r string) bool { return len([]rune(strings.TrimSpace(r))) >= 5 }

// createEligibleCertificate comprueba la regla y crea el certificado en estado eligible.
// Devuelve el cuerpo y el código HTTP (201 si se creó).
func (s *Server) createEligibleCertificate(ctx context.Context, tenantID, eventID, registrationID string, replaces *string) (fiber.Map, int, string, string) {
	var ticketID, fullName, partEvent string
	err := s.DB.QueryRow(ctx, `SELECT COALESCE(ticket_id,''),full_name,event_id FROM participation_contract.registrations_v1 WHERE registration_id::text=$1`, registrationID).Scan(&ticketID, &fullName, &partEvent)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, 422, "registration_not_found", "el registro no existe o no está confirmado"
	}
	if err != nil {
		return nil, 503, "participation_unavailable", "no se pudo leer el registro"
	}
	var sameEvent bool
	if err := s.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM onsite_contract.events_v1 WHERE tenant_id=$1::uuid AND event_id=$2 AND (event_id=$3 OR participation_event_id=$3))`, tenantID, eventID, partEvent).Scan(&sameEvent); err != nil {
		return nil, 503, "onsite_unavailable", "no se pudo leer el evento"
	}
	if !sameEvent {
		return nil, 422, "registration_not_in_event", "el registro pertenece a otro evento"
	}
	var firstFact string
	var attendedAt time.Time
	err = s.DB.QueryRow(ctx, `SELECT fact_id::text,occurred_at FROM onsite_contract.attendance_v1 WHERE tenant_id=$1::uuid AND event_id=$2 AND ticket_id=$3 AND type='check_in' AND NOT voided ORDER BY occurred_at LIMIT 1`, tenantID, eventID, ticketID).Scan(&firstFact, &attendedAt)
	if errors.Is(err, pgx.ErrNoRows) || ticketID == "" {
		return nil, 422, "not_eligible", "el participante no tiene una asistencia vigente registrada"
	}
	if err != nil {
		return nil, 503, "onsite_unavailable", "no se pudo leer la asistencia"
	}
	var endsAt time.Time
	if err := s.DB.QueryRow(ctx, `SELECT ends_at FROM onsite_contract.events_v1 WHERE tenant_id=$1::uuid AND event_id=$2 AND (event_id=$2 OR participation_event_id=$2) LIMIT 1`, tenantID, eventID).Scan(&endsAt); err != nil {
		return nil, 503, "onsite_unavailable", "no se pudo leer el cierre del evento"
	}
	if time.Now().UTC().Before(endsAt.UTC()) {
		return nil, 422, "event_not_finished", "el evento todavía no ha finalizado"
	}
	id := uuid.New()
	code := "NX-" + strings.ToUpper(id.String()[:8])
	sum := sha256.Sum256([]byte("nexo:certificate-evidence:v1|" + tenantID + "|" + eventID + "|" + registrationID + "|" + firstFact + "|" + eligibilityRule + "|" + code))
	evidence := base64.RawURLEncoding.EncodeToString(sum[:])
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return nil, 503, "persistence_unavailable", "no se pudo iniciar la emisión"
	}
	defer tx.Rollback(ctx)
	if replaces != nil {
		tag, err := tx.Exec(ctx, `UPDATE certificates.certificates SET state='replaced' WHERE id=$1::uuid AND tenant_id=$2::uuid AND state NOT IN ('revoked','replaced')`, *replaces, tenantID)
		if err != nil || tag.RowsAffected() == 0 {
			return nil, 409, "certificate_not_replaceable", "el certificado ya fue revocado o reemplazado"
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO certificates.certificates(id,tenant_id,event_id,registration_id,verification_code,display_name,state,evidence_digest,eligibility_rule_version,ticket_id,replaces_id) VALUES($1,$2::uuid,$3,$4,$5,$6,'eligible',$7,$8,$9,$10)`,
		id, tenantID, eventID, registrationID, code, fullName, evidence, eligibilityRule, ticketID, replaces)
	if err != nil {
		var pe *pgconn.PgError
		if errors.As(err, &pe) && pe.Code == "23505" {
			var existing string
			_ = s.DB.QueryRow(ctx, `SELECT id::text FROM certificates.certificates WHERE tenant_id=$1::uuid AND event_id=$2 AND registration_id=$3 AND state NOT IN ('revoked','replaced','cancelled','failed')`, tenantID, eventID, registrationID).Scan(&existing)
			return fiber.Map{"existing_id": existing}, 409, "already_issued", "el participante ya tiene un certificado vigente: " + existing
		}
		return nil, 503, "persistence_unavailable", "no se pudo guardar el certificado"
	}
	if replaces != nil {
		if _, err := tx.Exec(ctx, `UPDATE certificates.certificates SET replaced_by=$2 WHERE id=$1::uuid`, *replaces, id); err != nil {
			return nil, 503, "persistence_unavailable", "no se pudo enlazar la reemisión"
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, 503, "persistence_unavailable", "no se pudo confirmar la emisión"
	}
	return fiber.Map{"id": id, "verification_code": code, "state": "eligible", "display_name": fullName, "evidence_digest": evidence, "eligibility_rule_version": eligibilityRule, "attended_at": attendedAt, "replaces_id": replaces, "next_action": "request_hukid_delivery"}, 201, "", ""
}

type certificateView struct {
	ID             string     `json:"id"`
	EventID        string     `json:"event_id"`
	RegistrationID string     `json:"registration_id"`
	DisplayName    string     `json:"display_name"`
	Code           string     `json:"verification_code"`
	State          string     `json:"state"`
	IssuedAt       time.Time  `json:"issued_at"`
	RevokedAt      *time.Time `json:"revoked_at"`
	Reason         *string    `json:"revocation_reason"`
	ReplacesID     *string    `json:"replaces_id"`
	ReplacedBy     *string    `json:"replaced_by"`
}

func (s *Server) listCertificates(c fiber.Ctx) error {
	rows, err := s.DB.Query(c.Context(), `SELECT id::text,event_id,registration_id,display_name,verification_code,state,issued_at,revoked_at,revocation_reason,replaces_id::text,replaced_by::text
		FROM certificates.certificates WHERE tenant_id=$1::uuid AND ($2='' OR event_id=$2) ORDER BY issued_at DESC LIMIT 200`, tenant(c), c.Query("event_id"))
	if err != nil {
		return problem(c, 503, "persistence_unavailable", "no se pudieron leer los certificados")
	}
	defer rows.Close()
	items := []certificateView{}
	for rows.Next() {
		var v certificateView
		if err := rows.Scan(&v.ID, &v.EventID, &v.RegistrationID, &v.DisplayName, &v.Code, &v.State, &v.IssuedAt, &v.RevokedAt, &v.Reason, &v.ReplacesID, &v.ReplacedBy); err != nil {
			return problem(c, 503, "persistence_unavailable", "no se pudieron leer los certificados")
		}
		items = append(items, v)
	}
	return c.JSON(fiber.Map{"items": items})
}

// eligibleRegistrations lista a quienes cumplen la regla y si ya tienen certificado.
func (s *Server) eligibleRegistrations(c fiber.Ctx) error {
	event := c.Query("event_id")
	if event == "" {
		return problem(c, 400, "invalid_query", "event_id es obligatorio")
	}
	rows, err := s.DB.Query(c.Context(), `SELECT r.registration_id::text,r.full_name,r.ticket_id,MIN(a.occurred_at),
		(SELECT x.id::text FROM certificates.certificates x WHERE x.tenant_id=$1::uuid AND x.event_id=$2 AND x.registration_id=r.registration_id::text AND x.state NOT IN ('revoked','replaced','cancelled','failed') LIMIT 1)
		FROM participation_contract.registrations_v1 r
		JOIN onsite_contract.events_v1 e ON e.tenant_id=$1::uuid AND e.event_id=$2 AND e.ends_at <= now() AND (e.participation_event_id=r.event_id OR e.event_id=r.event_id)
		JOIN onsite_contract.attendance_v1 a ON a.tenant_id=$1::uuid AND a.event_id=$2 AND a.ticket_id=r.ticket_id AND a.type='check_in' AND NOT a.voided
		GROUP BY r.registration_id,r.full_name,r.ticket_id ORDER BY r.full_name`, tenant(c), event)
	if err != nil {
		return problem(c, 503, "source_unavailable", "no se pudo calcular la elegibilidad")
	}
	defer rows.Close()
	type item struct {
		RegistrationID string    `json:"registration_id"`
		DisplayName    string    `json:"display_name"`
		TicketID       string    `json:"ticket_id"`
		AttendedAt     time.Time `json:"attended_at"`
		CertificateID  *string   `json:"certificate_id"`
	}
	items := []item{}
	for rows.Next() {
		var v item
		if err := rows.Scan(&v.RegistrationID, &v.DisplayName, &v.TicketID, &v.AttendedAt, &v.CertificateID); err != nil {
			return problem(c, 503, "source_unavailable", "no se pudo calcular la elegibilidad")
		}
		items = append(items, v)
	}
	return c.JSON(fiber.Map{"rule": eligibilityRule, "items": items})
}

// revokeCertificate revoca con motivo y cancela las entregas HukID que no terminaron.
func (s *Server) revokeCertificate(c fiber.Ctx) error {
	var b struct {
		Reason string `json:"reason"`
	}
	if json.Unmarshal(c.Body(), &b) != nil || !validReason(b.Reason) {
		return problem(c, 422, "reason_required", "el motivo debe tener al menos 5 caracteres")
	}
	tx, err := s.DB.Begin(c.Context())
	if err != nil {
		return problem(c, 503, "persistence_unavailable", "no se pudo revocar")
	}
	defer tx.Rollback(c.Context())
	tag, err := tx.Exec(c.Context(), `UPDATE certificates.certificates SET state='revoked',revoked_at=now(),revocation_reason=$3 WHERE id=$1::uuid AND tenant_id=$2::uuid AND state NOT IN ('revoked','replaced')`, c.Params("id"), tenant(c), strings.TrimSpace(b.Reason))
	if err != nil {
		return problem(c, 503, "persistence_unavailable", "no se pudo revocar")
	}
	if tag.RowsAffected() == 0 {
		return problem(c, 409, "certificate_not_revocable", "el certificado no existe o ya fue revocado o reemplazado")
	}
	if _, err := tx.Exec(c.Context(), `UPDATE integration.hukid_deliveries SET state='cancelled',last_error_code='certificate_revoked',updated_at=now() WHERE certificate_id=$1::uuid AND tenant_id=$2::uuid AND state IN ('pending','request_created','offer_redeemed')`, c.Params("id"), tenant(c)); err != nil {
		return problem(c, 503, "persistence_unavailable", "no se pudo revocar")
	}
	if err := tx.Commit(c.Context()); err != nil {
		return problem(c, 503, "persistence_unavailable", "no se pudo revocar")
	}
	return c.JSON(fiber.Map{"id": c.Params("id"), "state": "revoked"})
}

// reissueCertificate reemplaza un certificado (p. ej. por un error de nombre corregido en
// Participation) volviendo a comprobar la elegibilidad. El anterior queda "replaced".
func (s *Server) reissueCertificate(c fiber.Ctx) error {
	var b struct {
		Reason string `json:"reason"`
	}
	if json.Unmarshal(c.Body(), &b) != nil || !validReason(b.Reason) {
		return problem(c, 422, "reason_required", "el motivo debe tener al menos 5 caracteres")
	}
	id := c.Params("id")
	var event, registration string
	if err := s.DB.QueryRow(c.Context(), `SELECT event_id,registration_id FROM certificates.certificates WHERE id=$1::uuid AND tenant_id=$2::uuid`, id, tenant(c)).Scan(&event, &registration); err != nil {
		return problem(c, 404, "certificate_not_found", "certificado no encontrado")
	}
	// El índice de "un certificado vigente" se respeta: primero se reemplaza, luego se crea (misma transacción).
	out, status, code, detail := s.createEligibleCertificate(c.Context(), tenant(c), event, registration, &id)
	if status != 201 {
		return problem(c, status, code, detail)
	}
	_, _ = s.DB.Exec(c.Context(), `UPDATE certificates.certificates SET revocation_reason=$2 WHERE id=$1::uuid`, id, "reemitido: "+strings.TrimSpace(b.Reason))
	return c.Status(201).JSON(out)
}

var verifyPage = template.Must(template.New("v").Parse(`<!DOCTYPE html>
<html lang="es"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="robots" content="noindex"><title>Verificación de certificado · Nexo</title>
<style>
body{margin:0;font-family:system-ui,-apple-system,"Segoe UI",sans-serif;background:#f4f5f7;color:#1c2330}
main{max-width:520px;margin:48px auto;padding:0 20px}
.card{background:#fff;border-radius:14px;padding:28px;box-shadow:0 1px 3px rgba(0,0,0,.08)}
.state{display:inline-block;padding:6px 12px;border-radius:999px;font-weight:600;font-size:14px}
.ok{background:#e6f4ea;color:#1e6b34}.bad{background:#fdecea;color:#a3261b}.warn{background:#fff4e0;color:#8a5300}
dl{display:grid;grid-template-columns:auto 1fr;gap:8px 16px;margin:20px 0 0}dt{color:#5b6475}dd{margin:0;font-weight:500;word-break:break-all}
p.note{color:#5b6475;font-size:14px;margin-top:20px;line-height:1.5}h1{font-size:20px;margin:0 0 16px}
</style></head><body><main><div class="card">
<h1>Verificación de certificado</h1>
{{if .Found}}<span class="state {{.Tone}}">{{.Label}}</span>
<dl><dt>Código</dt><dd>{{.Code}}</dd><dt>Evento</dt><dd>{{.Event}}</dd><dt>Emitido</dt><dd>{{.Issued}}</dd>{{if .Revoked}}<dt>Revocado</dt><dd>{{.Revoked}}</dd>{{end}}<dt>Huella de evidencia</dt><dd>{{.Evidence}}</dd></dl>
<p class="note">Por privacidad esta página no muestra el nombre del participante. Quien posee la credencial puede presentarla desde su billetera HukID.</p>
{{else}}<span class="state bad">No encontrado</span><p class="note">No existe un certificado con el código <strong>{{.Code}}</strong>. Revisa que esté escrito tal como aparece en el documento.</p>{{end}}
</div></main></body></html>`))

// verifyCertificateHTML atiende la verificación pública desde un navegador (OO-026).
func (s *Server) verifyCertificateHTML(c fiber.Ctx) error {
	code := c.Params("code")
	var state, evidence, event string
	var issued time.Time
	var revoked *time.Time
	err := s.DB.QueryRow(c.Context(), `SELECT c.state,c.evidence_digest,COALESCE(e.name,c.event_id),c.issued_at,c.revoked_at FROM certificates.certificates c
		LEFT JOIN onsite_contract.events_v1 e ON e.tenant_id=c.tenant_id AND e.event_id=c.event_id WHERE c.verification_code=$1`, code).Scan(&state, &evidence, &event, &issued, &revoked)
	data := map[string]any{"Found": err == nil, "Code": code}
	status := 200
	if err != nil {
		status = 404
	} else {
		label, tone := "Válido", "ok"
		switch state {
		case "revoked":
			label, tone = "Revocado", "bad"
		case "replaced":
			label, tone = "Reemplazado por un certificado más reciente", "warn"
		case "eligible", "request_pending", "offer_available", "offer_redeemed":
			label, tone = "Válido · credencial HukID en proceso", "ok"
		case "failed", "cancelled":
			label, tone = "Válido · entrega HukID pendiente", "warn"
		}
		data["Label"], data["Tone"], data["Event"], data["Evidence"] = label, tone, event, evidence
		data["Issued"] = issued.In(lima()).Format("02/01/2006 15:04")
		if revoked != nil {
			data["Revoked"] = revoked.In(lima()).Format("02/01/2006 15:04")
		}
	}
	c.Set("Content-Type", "text/html; charset=utf-8")
	c.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
	c.Set("X-Content-Type-Options", "nosniff")
	var sb strings.Builder
	if err := verifyPage.Execute(&sb, data); err != nil {
		return problem(c, 500, "render_failed", "no se pudo mostrar la verificación")
	}
	return c.Status(status).SendString(sb.String())
}

func lima() *time.Location {
	if loc, err := time.LoadLocation("America/Lima"); err == nil {
		return loc
	}
	return time.FixedZone("PET", -5*3600)
}
