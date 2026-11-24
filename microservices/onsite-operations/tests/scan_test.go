package tests

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"fmt"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"nexo-onsite-operations/internal/onsite/domain"
	api "nexo-onsite-operations/internal/platform/http"
	"nexo-onsite-operations/internal/platform/postgres"
)

// Requiere seeds/demo-day.sql y seeds/demo-day-qr.sql (clave de desarrollo y sesiones).
func TestSignedQRAndSessions(t *testing.T) {
	url := os.Getenv("ONSITE_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("ONSITE_TEST_DATABASE_URL no configurada")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	router := api.NewRouter(postgres.New(pool))
	seed := sha256.Sum256([]byte(domain.DevTicketSeedPhrase))
	key := ed25519.NewKeyFromSeed(seed[:])
	sign := func(jti string, edit func(*domain.TicketClaims)) string {
		c := domain.TicketClaims{JTI: jti, Audience: "onsite", Organization: "incuba-unas", Event: "demo-day-2026", Scope: "main", Expires: time.Now().Add(time.Hour).Unix(), Kid: "participation-dev-1"}
		if edit != nil {
			edit(&c)
		}
		return domain.SignTicketToken("participation-dev-1", key, c)
	}
	projected := "TEST-QR-" + uuid.NewString()
	fresh := "TEST-QR-NEW-" + uuid.NewString()
	if _, err := pool.Exec(ctx, `INSERT INTO onsite.ticket_projections(ticket_id,event_id,scope_id,status,expires_at,masked_holder,search_code,confirmed_at) VALUES($1,'00000000-0000-0000-0000-000000000101','00000000-0000-0000-0000-000000000102','active',now()+interval '1 day','Q*** Test',$1,now())`, projected); err != nil {
		t.Fatal(err)
	}
	_, _ = pool.Exec(ctx, `UPDATE onsite.revocation_state SET synced_at=now()`)
	defer func() {
		for _, id := range []string{projected, fresh} {
			_, _ = pool.Exec(ctx, `DELETE FROM onsite.session_entries WHERE ticket_id=$1`, id)
			_, _ = pool.Exec(ctx, `DELETE FROM onsite.ticket_sessions WHERE ticket_id=$1`, id)
			_, _ = pool.Exec(ctx, `DELETE FROM onsite.ticket_projections WHERE ticket_id=$1`, id)
		}
	}()
	scan := func(token string, headers map[string]string) map[string]any {
		st, out := call(t, router, "/onsite/v1/scans", []byte(fmt.Sprintf(`{"token":%q,"event_id":"event-demo","scope":"main"}`, token)), headers)
		if st != 200 {
			t.Fatalf("scan status=%d %v", st, out)
		}
		return out
	}
	staff := map[string]string{"X-Mock-Role": "staff"}
	sup := map[string]string{"X-Mock-Role": "supervisor"}

	if out := scan(sign(projected, nil), staff); out["accepted"] != true {
		t.Fatalf("QR válido rechazado: %v", out)
	}
	fake := "eyJhbGciOiJIUzI1NiJ9.eyJqdGkiOiJ4In0.development-signature"
	for name, tc := range map[string]struct{ token, want string }{
		"HS256 de desarrollo": {fake, "qr_unsupported_alg"},
		"otro evento":         {sign(projected, func(c *domain.TicketClaims) { c.Event = "otro" }), "wrong_event"},
		"otra zona":           {sign(projected, func(c *domain.TicketClaims) { c.Scope = "cyber" }), "wrong_scope"},
		"revocado":            {sign("DEMO-2026-REVOCADO", nil), "ticket_revoked"},
		"vencido":             {sign(projected, func(c *domain.TicketClaims) { c.Expires = time.Now().Add(-time.Hour).Unix() }), "ticket_expired"},
	} {
		if out := scan(tc.token, staff); out["accepted"] != false || out["reason"] != tc.want {
			t.Errorf("%s: %v (se esperaba %s)", name, out, tc.want)
		}
	}

	// Ticket recién emitido que aún no llegó: staff no puede resolverlo, supervisión sí.
	freshToken := sign(fresh, func(c *domain.TicketClaims) { c.Sessions = []string{"charla-vip"} })
	if out := scan(freshToken, staff); out["reason"] != "ticket_not_materialized" || out["can_resolve"] != false {
		t.Fatalf("staff: %v", out)
	}
	if out := scan(freshToken, sup); out["can_resolve"] != true {
		t.Fatalf("supervisor debe poder resolver: %v", out)
	}
	body := []byte(fmt.Sprintf(`{"token":%q,"event_id":"event-demo","scope":"main"}`, freshToken))
	if st, _ := call(t, router, "/onsite/v1/scans/provisional", body, staff); st != 403 {
		t.Fatalf("staff resolviendo provisional = %d", st)
	}
	if st, out := call(t, router, "/onsite/v1/scans/provisional", body, sup); st != 201 {
		t.Fatalf("provisional = %d %v", st, out)
	}
	if out := scan(freshToken, staff); out["accepted"] != true || out["provisional"] != true {
		t.Fatalf("tras resolver debe aceptarse como provisional: %v", out)
	}

	// Sesiones: la restringida sólo con permiso (aquí viene en el QR); no se entra dos veces.
	enter := func(session, payload string) (int, map[string]any) {
		return call(t, router, "/onsite/v1/sessions/"+session+"/entries", []byte(payload), staff)
	}
	if st, out := enter("charla-vip", fmt.Sprintf(`{"event_id":"event-demo","ticket_id":%q}`, projected)); st != 422 || out["code"] != "session_not_allowed" {
		t.Fatalf("restringida sin permiso = %d %v", st, out)
	}
	if st, out := enter("charla-vip", fmt.Sprintf(`{"event_id":"event-demo","token":%q}`, freshToken)); st != 201 {
		t.Fatalf("restringida con permiso = %d %v", st, out)
	}
	if st, out := enter("charla-vip", fmt.Sprintf(`{"event_id":"event-demo","token":%q}`, freshToken)); st != 409 || out["code"] != "already_in_session" {
		t.Fatalf("segunda entrada = %d %v", st, out)
	}
	if st, out := enter("taller-ia", fmt.Sprintf(`{"event_id":"event-demo","ticket_id":%q}`, projected)); st != 201 {
		t.Fatalf("taller abierto = %d %v", st, out)
	}
	// La entrada a sesión no toca la asistencia principal.
	var facts int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM onsite.attendance_facts WHERE ticket_id IN ($1,$2)`, projected, fresh).Scan(&facts)
	if facts != 0 {
		t.Fatalf("la sesión creó %d hechos de asistencia", facts)
	}

	req := httptest.NewRequest("GET", "/onsite/v1/scan-stats?event_id=event-demo", nil)
	req.Header.Set("X-Mock-Role", "supervisor")
	res, err := router.App.Test(req)
	if err != nil || res.StatusCode != 200 {
		t.Fatalf("scan-stats = %v %v", res.StatusCode, err)
	}
}
