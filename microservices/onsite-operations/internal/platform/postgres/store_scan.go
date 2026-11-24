package postgres

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"nexo-onsite-operations/internal/onsite/domain"
	"nexo-onsite-operations/internal/onsite/ports"
)

var (
	// ErrRevocationsStale impide aceptar un provisional si la lista de revocaciones está vieja (OO-017).
	ErrRevocationsStale = errors.New("revocations_stale")
	// ErrNotProvisionalCandidate indica que el QR no es de un ticket por resolver.
	ErrNotProvisionalCandidate = errors.New("not_provisional_candidate")
)

// ScanResult es la respuesta de verificar un QR. Sólo trae el nombre enmascarado.
type ScanResult struct {
	Accepted    bool           `json:"accepted"`
	Reason      string         `json:"reason,omitempty"`
	CanResolve  bool           `json:"can_resolve"`
	Provisional bool           `json:"provisional"`
	Ticket      *ScannedTicket `json:"ticket,omitempty"`
}

type ScannedTicket struct {
	ID           string    `json:"ticket_id"`
	EventID      string    `json:"event_id"`
	Scope        string    `json:"scope"`
	MaskedHolder string    `json:"masked_holder"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// SessionEntry es una entrada registrada a una sesión.
type SessionEntry struct {
	ID           string    `json:"id"`
	TicketID     string    `json:"ticket_id"`
	Session      string    `json:"session"`
	MaskedHolder string    `json:"masked_holder"`
	OccurredAt   time.Time `json:"occurred_at"`
	Entries      int       `json:"entries"`
	Capacity     int       `json:"capacity"`
}

// RevocationMaxAge es la antigüedad máxima aceptada de la lista de revocaciones.
func RevocationMaxAge() time.Duration {
	if d, err := time.ParseDuration(os.Getenv("ONSITE_REVOCATION_MAX_AGE")); err == nil && d > 0 {
		return d
	}
	return 15 * time.Minute
}

func (s *Store) ticketKeys(ctx context.Context, tenant string) (map[string]domain.TicketKey, error) {
	rows, err := s.Pool.Query(ctx, `SELECT kid,status,public_key FROM onsite.ticket_keys WHERE tenant_id=$1::uuid`, tenant)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	keys := map[string]domain.TicketKey{}
	for rows.Next() {
		var k domain.TicketKey
		var pub []byte
		if err := rows.Scan(&k.Kid, &k.Status, &pub); err != nil {
			return nil, err
		}
		k.PublicKey = ed25519.PublicKey(pub)
		keys[k.Kid] = k
	}
	return keys, rows.Err()
}

// RevocationFresh indica si la lista de revocaciones se sincronizó hace poco.
func (s *Store) RevocationFresh(ctx context.Context, tenant string) (bool, *time.Time, error) {
	var at time.Time
	err := s.Pool.QueryRow(ctx, `SELECT synced_at FROM onsite.revocation_state WHERE tenant_id=$1::uuid`, tenant).Scan(&at)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil, nil
	}
	if err != nil {
		return false, nil, err
	}
	return time.Since(at) <= RevocationMaxAge(), &at, nil
}

func (s *Store) recordRejection(ctx context.Context, c ports.Claims, event, scope, reason string) {
	// Va en su propia operación: el rechazo queda aunque falle lo demás.
	_, _ = s.Pool.Exec(ctx, `INSERT INTO onsite.scan_rejections(id,tenant_id,event_id,scope_code,reason,actor_id,device_id)
		VALUES($1,$2::uuid,(SELECT id FROM onsite.events WHERE tenant_id=$2::uuid AND source_event_id=$3),$4,$5,$6::uuid,NULLIF($7,'')::uuid)`,
		uuid.NewString(), c.TenantID, event, scope, reason, c.ActorID, c.DeviceID)
}

// tokenEventMatches acepta el ID del evento de Onsite o el de Participation.
func (s *Store) tokenEventMatches(ctx context.Context, tenant, event, tokenEvent string) (bool, error) {
	var ok bool
	err := s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM onsite.events WHERE tenant_id=$1::uuid AND source_event_id=$2 AND (source_event_id=$3 OR participation_event_id=$3))`, tenant, event, tokenEvent).Scan(&ok)
	return ok, err
}

// verifyToken aplica todas las reglas del QR y devuelve el ticket aceptado o el motivo.
func (s *Store) verifyToken(ctx context.Context, raw, event, scope string, session bool, c ports.Claims) (domain.TicketClaims, *ScannedTicket, bool, string, error) {
	keys, err := s.ticketKeys(ctx, c.TenantID)
	if err != nil {
		return domain.TicketClaims{}, nil, false, "", err
	}
	claims, reason := domain.VerifyTicketToken(raw, keys, time.Now())
	if reason != "" {
		return claims, nil, false, reason, nil
	}
	if ok, err := s.tokenEventMatches(ctx, c.TenantID, event, claims.Event); err != nil {
		return claims, nil, false, "", err
	} else if !ok {
		return claims, nil, false, domain.RejectWrongEvent, nil
	}
	if reason := claims.CheckScope(scope, session); reason != "" {
		return claims, nil, false, reason, nil
	}
	var revoked bool
	if err := s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM onsite.ticket_revocations WHERE tenant_id=$1::uuid AND jti=$2)`, c.TenantID, claims.JTI).Scan(&revoked); err != nil {
		return claims, nil, false, "", err
	}
	if revoked {
		return claims, nil, false, domain.RejectRevoked, nil
	}
	var t domain.Ticket
	var provisional bool
	err = s.Pool.QueryRow(ctx, `SELECT t.ticket_id,e.source_event_id,sc.code,t.status,t.expires_at,t.masked_holder,t.provisional
		FROM onsite.ticket_projections t JOIN onsite.events e ON e.id=t.event_id JOIN onsite.scopes sc ON sc.id=t.scope_id
		WHERE t.ticket_id=$1 AND e.tenant_id=$2::uuid`, claims.JTI, c.TenantID).Scan(&t.ID, &t.EventID, &t.Scope, &t.Status, &t.ExpiresAt, &t.MaskedHolder, &provisional)
	if errors.Is(err, pgx.ErrNoRows) {
		return claims, nil, false, domain.RejectNotMaterialized, nil
	}
	if err != nil {
		return claims, nil, false, "", err
	}
	target := scope
	if session {
		target = t.Scope
	}
	if reason := t.Validate(event, target, time.Now().UTC()); reason != "" {
		return claims, nil, false, reason, nil
	}
	return claims, &ScannedTicket{ID: t.ID, EventID: t.EventID, Scope: t.Scope, MaskedHolder: t.MaskedHolder, ExpiresAt: t.ExpiresAt}, provisional, "", nil
}

// VerifyScan valida un QR en la puerta (OO-010). Todo rechazo queda registrado (OO-016).
func (s *Store) VerifyScan(ctx context.Context, raw, event, scope string, c ports.Claims) (ScanResult, error) {
	_, ticket, provisional, reason, err := s.verifyToken(ctx, raw, event, scope, false, c)
	if err != nil {
		return ScanResult{}, err
	}
	if reason != "" {
		s.recordRejection(ctx, c, event, scope, reason)
		result := ScanResult{Reason: reason}
		if reason == domain.RejectNotMaterialized && (c.Role == "supervisor" || c.Role == "manager") {
			fresh, _, err := s.RevocationFresh(ctx, c.TenantID)
			if err != nil {
				return ScanResult{}, err
			}
			result.CanResolve = fresh
		}
		return result, nil
	}
	return ScanResult{Accepted: true, Ticket: ticket, Provisional: provisional}, nil
}

// replay devuelve la respuesta guardada si la clave de idempotencia ya se usó.
func replay(ctx context.Context, tx pgx.Tx, key, actor, hash string, out any) (bool, error) {
	var oldHash string
	var body []byte
	err := tx.QueryRow(ctx, `SELECT request_hash,response_body FROM onsite.idempotency_records WHERE key=$1 AND actor_id=$2 FOR UPDATE`, key, actor).Scan(&oldHash, &body)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if oldHash != hash {
		return false, domain.ErrIdempotencyKeyReused
	}
	return true, json.Unmarshal(body, out)
}

func remember(ctx context.Context, tx pgx.Tx, key, actor, hash string, out any) error {
	body, _ := json.Marshal(out)
	_, err := tx.Exec(ctx, `INSERT INTO onsite.idempotency_records(key,actor_id,request_hash,status_code,response_body) VALUES($1,$2,$3,201,$4)`, key, actor, hash, body)
	return err
}

func auditTx(ctx context.Context, tx pgx.Tx, c ports.Claims, action, kind, id string, meta map[string]any) error {
	raw, _ := json.Marshal(meta)
	_, err := tx.Exec(ctx, `INSERT INTO onsite.audit_log(id,tenant_id,actor_id,action,resource_type,resource_id,trace_id,metadata) VALUES($1,$2,$3,$4,$5,$6,$7,$8::jsonb)`,
		uuid.NewString(), c.TenantID, c.ActorID, action, kind, id, c.TraceID, string(raw))
	return err
}

func isUniqueViolation(err error) bool {
	var pe *pgconn.PgError
	return errors.As(err, &pe) && pe.Code == "23505"
}

// ResolveProvisional acepta un ticket recién emitido que aún no llegó a la proyección (OO-017).
// Sólo supervisión, sólo con la lista de revocaciones fresca, y queda auditado.
func (s *Store) ResolveProvisional(ctx context.Context, raw, event, scope, hash, key string, c ports.Claims) (ScannedTicket, error) {
	var saved ScannedTicket
	claims, _, _, reason, err := s.verifyToken(ctx, raw, event, scope, false, c)
	if err != nil {
		return saved, err
	}
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return saved, err
	}
	defer tx.Rollback(ctx)
	if ok, err := replay(ctx, tx, key, c.ActorID, hash, &saved); err != nil || ok {
		return saved, err
	}
	if reason != domain.RejectNotMaterialized {
		if reason == "" {
			return saved, ErrNotProvisionalCandidate
		}
		return saved, errors.New(reason)
	}
	if fresh, _, err := s.RevocationFresh(ctx, c.TenantID); err != nil {
		return saved, err
	} else if !fresh {
		return saved, ErrRevocationsStale
	}
	var eventUUID, scopeUUID string
	if err := tx.QueryRow(ctx, `SELECT e.id::text,sc.id::text FROM onsite.events e JOIN onsite.scopes sc ON sc.event_id=e.id WHERE e.tenant_id=$1::uuid AND e.source_event_id=$2 AND sc.code=$3`, c.TenantID, event, scope).Scan(&eventUUID, &scopeUUID); err != nil {
		return saved, errors.New(domain.RejectWrongScope)
	}
	expires := time.Unix(claims.Expires, 0).UTC()
	if _, err := tx.Exec(ctx, `INSERT INTO onsite.ticket_projections(ticket_id,event_id,scope_id,status,expires_at,masked_holder,search_code,confirmed_at,provisional)
		VALUES($1,$2::uuid,$3::uuid,'active',$4,'Ticket provisional',$1,now(),true)`, claims.JTI, eventUUID, scopeUUID, expires); err != nil {
		if isUniqueViolation(err) {
			return saved, ErrNotProvisionalCandidate
		}
		return saved, err
	}
	for _, code := range claims.Sessions {
		if _, err := tx.Exec(ctx, `INSERT INTO onsite.ticket_sessions(ticket_id,scope_id) SELECT $1,id FROM onsite.scopes WHERE event_id=$2::uuid AND code=$3 AND kind='session' ON CONFLICT DO NOTHING`, claims.JTI, eventUUID, code); err != nil {
			return saved, err
		}
	}
	if err := auditTx(ctx, tx, c, "ticket.provisional_verification", "ticket", claims.JTI, map[string]any{"event": event, "scope": scope, "kid": claims.Kid}); err != nil {
		return saved, err
	}
	saved = ScannedTicket{ID: claims.JTI, EventID: event, Scope: scope, MaskedHolder: "Ticket provisional", ExpiresAt: expires}
	if err := remember(ctx, tx, key, c.ActorID, hash, saved); err != nil {
		return saved, err
	}
	return saved, tx.Commit(ctx)
}

// ScanStats cuenta los rechazos del evento por motivo en las últimas 24 horas.
func (s *Store) ScanStats(ctx context.Context, event, tenant string) (map[string]int, error) {
	rows, err := s.Pool.Query(ctx, `SELECT r.reason,COUNT(*) FROM onsite.scan_rejections r JOIN onsite.events e ON e.id=r.event_id
		WHERE e.tenant_id=$1::uuid AND e.source_event_id=$2 AND r.occurred_at > now()-interval '24 hours' GROUP BY r.reason`, tenant, event)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var reason string
		var n int
		if err := rows.Scan(&reason, &n); err != nil {
			return nil, err
		}
		out[reason] = n
	}
	return out, rows.Err()
}

// ListSessions lista las sesiones del evento con su ocupación.
func (s *Store) ListSessions(ctx context.Context, event, tenant string) ([]domain.Session, error) {
	rows, err := s.Pool.Query(ctx, `SELECT sc.code,sc.name,r.access,COALESCE(r.capacity,0),r.opens_at,r.closes_at,
		(SELECT COUNT(*) FROM onsite.session_entries x WHERE x.scope_id=sc.id)
		FROM onsite.scopes sc JOIN onsite.events e ON e.id=sc.event_id JOIN onsite.session_rules r ON r.scope_id=sc.id
		WHERE e.tenant_id=$1::uuid AND e.source_event_id=$2 AND sc.kind='session' ORDER BY sc.name`, tenant, event)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Session{}
	for rows.Next() {
		var x domain.Session
		if err := rows.Scan(&x.Code, &x.Name, &x.Access, &x.Capacity, &x.OpensAt, &x.ClosesAt, &x.Entries); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// EnterSession registra el ingreso a una sesión con un QR firmado o un código de ticket.
// No crea ni modifica hechos de asistencia del evento.
func (s *Store) EnterSession(ctx context.Context, event, session, ticketID, raw, hash, key string, c ports.Claims) (SessionEntry, error) {
	var saved SessionEntry
	var claims domain.TicketClaims
	if raw != "" {
		var ticket *ScannedTicket
		var reason string
		var err error
		claims, ticket, _, reason, err = s.verifyToken(ctx, raw, event, session, true, c)
		if err != nil {
			return saved, err
		}
		if reason != "" {
			s.recordRejection(ctx, c, event, session, reason)
			return saved, errors.New(reason)
		}
		ticketID = ticket.ID
	}
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return saved, err
	}
	defer tx.Rollback(ctx)
	if ok, err := replay(ctx, tx, key, c.ActorID, hash, &saved); err != nil || ok {
		return saved, err
	}
	var sessionUUID string
	var rule domain.Session
	err = tx.QueryRow(ctx, `SELECT sc.id::text,sc.name,r.access,COALESCE(r.capacity,0),r.opens_at,r.closes_at FROM onsite.scopes sc JOIN onsite.events e ON e.id=sc.event_id JOIN onsite.session_rules r ON r.scope_id=sc.id
		WHERE e.tenant_id=$1::uuid AND e.source_event_id=$2 AND sc.code=$3 AND sc.kind='session'`, c.TenantID, event, session).Scan(&sessionUUID, &rule.Name, &rule.Access, &rule.Capacity, &rule.OpensAt, &rule.ClosesAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return saved, errors.New(domain.RejectSessionNotFound)
	}
	if err != nil {
		return saved, err
	}
	var t domain.Ticket
	err = tx.QueryRow(ctx, `SELECT t.ticket_id,e.source_event_id,sc.code,t.status,t.expires_at,t.masked_holder FROM onsite.ticket_projections t JOIN onsite.events e ON e.id=t.event_id JOIN onsite.scopes sc ON sc.id=t.scope_id
		WHERE t.ticket_id=$1 AND e.tenant_id=$2::uuid`, ticketID, c.TenantID).Scan(&t.ID, &t.EventID, &t.Scope, &t.Status, &t.ExpiresAt, &t.MaskedHolder)
	if errors.Is(err, pgx.ErrNoRows) {
		s.recordRejection(ctx, c, event, session, "ticket_invalid")
		return saved, errors.New("ticket_invalid")
	}
	if err != nil {
		return saved, err
	}
	if reason := t.Validate(event, t.Scope, time.Now().UTC()); reason != "" {
		s.recordRejection(ctx, c, event, session, reason)
		return saved, errors.New(reason)
	}
	// El aforo se controla serializando las entradas de la sesión.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('onsite:session:' || $1))`, sessionUUID); err != nil {
		return saved, err
	}
	var entitled, already bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM onsite.ticket_sessions WHERE ticket_id=$1 AND scope_id=$2::uuid),
		EXISTS(SELECT 1 FROM onsite.session_entries WHERE ticket_id=$1 AND scope_id=$2::uuid),
		(SELECT COUNT(*) FROM onsite.session_entries WHERE scope_id=$2::uuid)`, t.ID, sessionUUID).Scan(&entitled, &already, &rule.Entries); err != nil {
		return saved, err
	}
	if already {
		return saved, errors.New(domain.RejectAlreadyInSession)
	}
	if reason := domain.SessionDecision(rule, entitled || claims.AllowsSession(session), time.Now()); reason != "" {
		s.recordRejection(ctx, c, event, session, reason)
		return saved, errors.New(reason)
	}
	saved = SessionEntry{ID: uuid.NewString(), TicketID: t.ID, Session: session, MaskedHolder: t.MaskedHolder, OccurredAt: time.Now().UTC(), Entries: rule.Entries + 1, Capacity: rule.Capacity}
	if _, err := tx.Exec(ctx, `INSERT INTO onsite.session_entries(id,ticket_id,scope_id,actor_id,device_id,occurred_at) VALUES($1,$2,$3::uuid,$4::uuid,NULLIF($5,'')::uuid,$6)`, saved.ID, t.ID, sessionUUID, c.ActorID, c.DeviceID, saved.OccurredAt); err != nil {
		if isUniqueViolation(err) {
			return SessionEntry{}, errors.New(domain.RejectAlreadyInSession)
		}
		return SessionEntry{}, err
	}
	if err := auditTx(ctx, tx, c, "session.entry", "session_entry", saved.ID, map[string]any{"ticket_id": t.ID, "session": session}); err != nil {
		return SessionEntry{}, err
	}
	if err := remember(ctx, tx, key, c.ActorID, hash, saved); err != nil {
		return SessionEntry{}, err
	}
	return saved, tx.Commit(ctx)
}

// ShiftView es un turno con su dispositivo, para la pantalla de supervisión.
type ShiftView struct {
	ID          string     `json:"id"`
	EventID     string     `json:"event_id"`
	Scope       string     `json:"scope"`
	DeviceID    string     `json:"device_id"`
	DeviceLabel string     `json:"device_label"`
	ActorID     string     `json:"actor_id"`
	Status      string     `json:"status"`
	OpenedAt    time.Time  `json:"opened_at"`
	ClosedAt    *time.Time `json:"closed_at"`
}

// ListShifts lista los turnos recientes del evento (abiertos primero).
func (s *Store) ListShifts(ctx context.Context, event, tenant string) ([]ShiftView, error) {
	rows, err := s.Pool.Query(ctx, `SELECT sh.id::text,e.source_event_id,sc.code,sh.device_id::text,d.label,sh.actor_id::text,sh.status,sh.opened_at,sh.closed_at
		FROM onsite.shifts sh JOIN onsite.events e ON e.id=sh.event_id JOIN onsite.scopes sc ON sc.id=sh.scope_id JOIN onsite.devices d ON d.id=sh.device_id
		WHERE sh.tenant_id=$1::uuid AND e.source_event_id=$2 ORDER BY (sh.status='open') DESC, sh.opened_at DESC LIMIT 100`, tenant, event)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ShiftView{}
	for rows.Next() {
		var v ShiftView
		if err := rows.Scan(&v.ID, &v.EventID, &v.Scope, &v.DeviceID, &v.DeviceLabel, &v.ActorID, &v.Status, &v.OpenedAt, &v.ClosedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
