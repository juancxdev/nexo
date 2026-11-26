package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func openIntegrationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("PARTICIPATION_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("PARTICIPATION_TEST_DATABASE_URL not set; skipping PostgreSQL integration test")
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	if raw := os.Getenv("PARTICIPATION_DB_MAX_CONNS"); raw != "" {
		if n, convErr := strconv.ParseInt(raw, 10, 32); convErr == nil && n > 0 {
			cfg.MaxConns = int32(n)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	if err = pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("ping postgres: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

type seededForm struct {
	FormID         string
	SiteSlug       string
	EventSlug      string
	EventID        string
	FormVersionID  string
	ConsentVersion string
}

func seedTestForm(t *testing.T, pool *pgxpool.Pool, capacity int, waitlist bool) seededForm {
	t.Helper()
	f := seededForm{
		FormID:         uuid.NewString(),
		SiteSlug:       "test-site-" + uuid.NewString()[:8],
		EventSlug:      "test-event-" + uuid.NewString()[:8],
		EventID:        "evt-" + uuid.NewString()[:8],
		FormVersionID:  uuid.NewString(),
		ConsentVersion: "consent-v1",
	}
	_, err := pool.Exec(context.Background(), `
		INSERT INTO participation.forms (
			id, site_slug, event_slug, event_id, event_title,
			form_version_id, consent_version, binding_state, consent_text,
			schema, capacity, confirmed_count, remaining, waitlist_enabled
		) VALUES ($1::uuid, $2, $3, $4, 'PA-041 Test Event', $5::uuid, $6, 'open', 'Acepto', '{}'::jsonb, $7, 0, $7, $8)
	`, f.FormID, f.SiteSlug, f.EventSlug, f.EventID, f.FormVersionID, f.ConsentVersion, capacity, waitlist)
	if err != nil {
		t.Fatalf("seed form: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM participation.registrations WHERE form_id=$1::uuid`, f.FormID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM participation.forms WHERE id=$1::uuid`, f.FormID)
	})
	return f
}

type regReceipt struct {
	ID               string  `json:"id"`
	EventID          string  `json:"event_id"`
	State            string  `json:"state"`
	WaitlistPosition *int    `json:"waitlist_position"`
	TicketID         *string `json:"ticket_id"`
	SelfAccessToken  string  `json:"self_access_token"`
}

func doRegisterV1(app *fiber.App, f seededForm, email, idempKey string) (int, regReceipt, error) {
	body, _ := json.Marshal(map[string]any{
		"form_version_id":  f.FormVersionID,
		"consent_version":  f.ConsentVersion,
		"consent_accepted": true,
		"attendee": map[string]string{
			"full_name":   "Participant " + email,
			"email":       email,
			"institution": "UNAS",
		},
		"answers": map[string]any{"role": "student"},
	})
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/participation/public/v1/sites/%s/events/%s/registrations", f.SiteSlug, f.EventSlug), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", idempKey)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 15 * time.Second})
	if err != nil {
		return 0, regReceipt{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out regReceipt
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out, nil
}

func doCancel(app *fiber.App, regID, token string) (int, map[string]any, error) {
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/participation/self/v1/registrations/%s/cancel", regID), nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 15 * time.Second})
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out, nil
}

func TestIntegrationCancelAndWaitlistPromotion(t *testing.T) {
	pool := openIntegrationPool(t)
	srv := New(pool, "", "")
	f := seedTestForm(t, pool, 1, true)

	codeA, recA, err := doRegisterV1(srv.App, f, "alice@example.edu", "idem-a-"+uuid.NewString())
	if err != nil || codeA != http.StatusCreated || recA.State != "confirmed" || recA.TicketID == nil {
		t.Fatalf("register A failed: code=%d err=%v rec=%+v", codeA, err, recA)
	}

	codeB, recB, err := doRegisterV1(srv.App, f, "bob@example.edu", "idem-b-"+uuid.NewString())
	if err != nil || codeB != http.StatusCreated || recB.State != "waitlisted" || recB.WaitlistPosition == nil || *recB.WaitlistPosition != 1 {
		t.Fatalf("register B failed: code=%d err=%v rec=%+v", codeB, err, recB)
	}

	// Sin token -> 401
	if code, _, _ := doCancel(srv.App, recA.ID, ""); code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d", code)
	}
	// Token ajeno -> 404 (no permite cancelar registros ajenos)
	if code, _, _ := doCancel(srv.App, recA.ID, recB.SelfAccessToken); code != http.StatusNotFound {
		t.Fatalf("expected 404 with wrong token, got %d", code)
	}

	// Cancelación válida de A -> revoca ticket A y promueve a B
	code, body, err := doCancel(srv.App, recA.ID, recA.SelfAccessToken)
	if err != nil || code != http.StatusOK || body["state"] != "cancelled" {
		t.Fatalf("cancel A failed: code=%d err=%v body=%v", code, err, body)
	}

	// Idempotencia de cancelación
	codeAgain, bodyAgain, err := doCancel(srv.App, recA.ID, recA.SelfAccessToken)
	if err != nil || codeAgain != http.StatusOK || bodyAgain["state"] != "cancelled" {
		t.Fatalf("idempotent cancel A failed: code=%d err=%v body=%v", codeAgain, err, bodyAgain)
	}

	// Ticket de A ya no está disponible (409)
	tReqA := httptest.NewRequest(http.MethodGet, "/participation/self/v1/registrations/"+recA.ID+"/ticket", nil)
	tReqA.Header.Set("Authorization", "Bearer "+recA.SelfAccessToken)
	tRespA, err := srv.App.Test(tReqA, fiber.TestConfig{Timeout: 5 * time.Second})
	if err != nil || tRespA.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 for revoked ticket A, got %d err=%v", tRespA.StatusCode, err)
	}

	// Ticket de B ahora está activo y es distinto al de A
	tReqB := httptest.NewRequest(http.MethodGet, "/participation/self/v1/registrations/"+recB.ID+"/ticket", nil)
	tReqB.Header.Set("Authorization", "Bearer "+recB.SelfAccessToken)
	tRespB, err := srv.App.Test(tReqB, fiber.TestConfig{Timeout: 5 * time.Second})
	if err != nil || tRespB.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for promoted ticket B, got %d err=%v", tRespB.StatusCode, err)
	}

	// Verificar vista de contrato (B confirmado, A ausente)
	var contractCount int
	if err := pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM participation_contract.registrations_v1 WHERE event_id=$1`, f.EventID).Scan(&contractCount); err != nil || contractCount != 1 {
		t.Fatalf("expected 1 row in contract view, got %d err=%v", contractCount, err)
	}

	// Cancelar a B sin más waitlist -> libera cupo (remaining=1, confirmed_count=0)
	if codeB2, _, err := doCancel(srv.App, recB.ID, recB.SelfAccessToken); err != nil || codeB2 != http.StatusOK {
		t.Fatalf("cancel B failed: code=%d err=%v", codeB2, err)
	}
	var remaining, confirmed int
	if err := pool.QueryRow(context.Background(), `SELECT remaining, confirmed_count FROM participation.forms WHERE id=$1::uuid`, f.FormID).Scan(&remaining, &confirmed); err != nil {
		t.Fatalf("query form: %v", err)
	}
	if remaining != 1 || confirmed != 0 {
		t.Fatalf("expected remaining=1 confirmed=0, got remaining=%d confirmed=%d", remaining, confirmed)
	}
}

func TestIntegrationConcurrentRegistrationNoOverbookingAndUniqueTickets(t *testing.T) {
	pool := openIntegrationPool(t)
	srv := New(pool, "", "")
	const capacity = 25
	const totalRequests = capacity * 3
	f := seedTestForm(t, pool, capacity, true)

	var wg sync.WaitGroup
	var errCount atomic.Int64
	for i := 0; i < totalRequests; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			code, _, err := doRegisterV1(srv.App, f, fmt.Sprintf("user-%d@example.edu", idx), fmt.Sprintf("idem-%d-%s", idx, uuid.NewString()))
			if err != nil || code != http.StatusCreated {
				errCount.Add(1)
			}
		}(i)
	}
	wg.Wait()

	if errCount.Load() != 0 {
		t.Fatalf("expected 0 registration errors, got %d", errCount.Load())
	}

	var remaining, confirmed, dbConfirmed, dbWaitlisted, distinctTickets int
	if err := pool.QueryRow(context.Background(), `SELECT remaining, confirmed_count FROM participation.forms WHERE id=$1::uuid`, f.FormID).Scan(&remaining, &confirmed); err != nil {
		t.Fatalf("query form: %v", err)
	}
	if err := pool.QueryRow(context.Background(), `
		SELECT
			COUNT(*) FILTER (WHERE state='confirmed'),
			COUNT(*) FILTER (WHERE state='waitlisted'),
			COUNT(DISTINCT ticket_id) FILTER (WHERE ticket_id IS NOT NULL)
		FROM participation.registrations WHERE form_id=$1::uuid
	`, f.FormID).Scan(&dbConfirmed, &dbWaitlisted, &distinctTickets); err != nil {
		t.Fatalf("query registrations: %v", err)
	}
	if remaining != 0 || confirmed != capacity || dbConfirmed != capacity || dbWaitlisted != totalRequests-capacity || distinctTickets != capacity {
		t.Fatalf("invariant violation: remaining=%d confirmed=%d dbConfirmed=%d dbWaitlisted=%d distinctTickets=%d", remaining, confirmed, dbConfirmed, dbWaitlisted, distinctTickets)
	}
}

func TestIntegrationConcurrentIdempotency(t *testing.T) {
	pool := openIntegrationPool(t)
	srv := New(pool, "", "")
	f := seedTestForm(t, pool, 10, true)

	const workers = 20
	sharedKey := "shared-idem-" + uuid.NewString()
	ids := make([]string, workers)
	var wg sync.WaitGroup
	var failures atomic.Int64
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			code, rec, err := doRegisterV1(srv.App, f, "same@example.edu", sharedKey)
			if err != nil || code != http.StatusCreated {
				failures.Add(1)
				return
			}
			ids[idx] = rec.ID
		}(i)
	}
	wg.Wait()

	if failures.Load() != 0 {
		t.Fatalf("expected 0 failures in concurrent idempotency, got %d", failures.Load())
	}
	for i := 1; i < workers; i++ {
		if ids[i] != ids[0] || ids[0] == "" {
			t.Fatalf("idempotency mismatch: ids[0]=%q ids[%d]=%q", ids[0], i, ids[i])
		}
	}
	var count int
	_ = pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM participation.registrations WHERE form_id=$1::uuid`, f.FormID).Scan(&count)
	if count != 1 {
		t.Fatalf("expected exactly 1 row, got %d", count)
	}
}

func TestIntegrationConcurrentCancellationAndRegistration(t *testing.T) {
	pool := openIntegrationPool(t)
	srv := New(pool, "", "")
	const capacity = 20
	f := seedTestForm(t, pool, capacity, true)

	confirmed := make([]regReceipt, 0, capacity)
	for i := 0; i < capacity; i++ {
		code, rec, err := doRegisterV1(srv.App, f, fmt.Sprintf("init-c-%d@example.edu", i), "init-c-"+uuid.NewString())
		if err != nil || code != http.StatusCreated || rec.State != "confirmed" {
			t.Fatalf("seed confirmed %d failed: %d %v", i, code, err)
		}
		confirmed = append(confirmed, rec)
	}
	for i := 0; i < capacity; i++ {
		code, rec, err := doRegisterV1(srv.App, f, fmt.Sprintf("init-w-%d@example.edu", i), "init-w-"+uuid.NewString())
		if err != nil || code != http.StatusCreated || rec.State != "waitlisted" {
			t.Fatalf("seed waitlisted %d failed: %d %v", i, code, err)
		}
	}

	var wg sync.WaitGroup
	var errCount atomic.Int64
	for i := 0; i < capacity; i++ {
		wg.Add(2)
		go func(rec regReceipt) {
			defer wg.Done()
			if code, _, err := doCancel(srv.App, rec.ID, rec.SelfAccessToken); err != nil || code != http.StatusOK {
				errCount.Add(1)
			}
		}(confirmed[i])
		go func(idx int) {
			defer wg.Done()
			if code, _, err := doRegisterV1(srv.App, f, fmt.Sprintf("new-%d@example.edu", idx), "new-"+uuid.NewString()); err != nil || code != http.StatusCreated {
				errCount.Add(1)
			}
		}(i)
	}
	wg.Wait()

	if errCount.Load() != 0 {
		t.Fatalf("expected 0 errors during concurrent cancel+register, got %d", errCount.Load())
	}

	var remaining, formConfirmed, dbConfirmed, distinctActiveTickets, badCancelledTickets int
	_ = pool.QueryRow(context.Background(), `SELECT remaining, confirmed_count FROM participation.forms WHERE id=$1::uuid`, f.FormID).Scan(&remaining, &formConfirmed)
	_ = pool.QueryRow(context.Background(), `
		SELECT
			COUNT(*) FILTER (WHERE state='confirmed'),
			COUNT(DISTINCT ticket_id) FILTER (WHERE state='confirmed' AND ticket_state='active'),
			COUNT(*) FILTER (WHERE state='cancelled' AND ticket_state='active')
		FROM participation.registrations WHERE form_id=$1::uuid
	`, f.FormID).Scan(&dbConfirmed, &distinctActiveTickets, &badCancelledTickets)

	if remaining < 0 || formConfirmed != dbConfirmed || formConfirmed+remaining != capacity || distinctActiveTickets != dbConfirmed || badCancelledTickets != 0 {
		t.Fatalf("invariant violation: remaining=%d formConfirmed=%d dbConfirmed=%d distinctActiveTickets=%d badCancelledTickets=%d",
			remaining, formConfirmed, dbConfirmed, distinctActiveTickets, badCancelledTickets)
	}
}

// TestLoad3xPeakRegistrationWaitlistCancel ejecuta un perfil reproducible de 3x el pico base supuesto
// (pico base = 100 operaciones, carga 3x = 300 operaciones mixtas: registro, waitlist y cancelación)
// con 60 goroutines concurrentes contra PostgreSQL aislado y reporta p50, p95, p99, throughput y pgxpool.Stat().
func TestLoad3xPeakRegistrationWaitlistCancel(t *testing.T) {
	pool := openIntegrationPool(t)
	srv := New(pool, "", "")

	const (
		capacity      = 60
		baselineOps   = 100 // Supuesto documentado: pico base de ensayo = 100 operaciones
		loadMultiplier = 3
		totalOps      = baselineOps * loadMultiplier // 300 operaciones
		concurrency   = 60
	)
	f := seedTestForm(t, pool, capacity, true)

	// Pre-sembrar 30 registros cancelables para mezclar cancelaciones reales bajo carga
	cancelTargets := make([]regReceipt, 0, 30)
	for i := 0; i < 30; i++ {
		code, rec, err := doRegisterV1(srv.App, f, fmt.Sprintf("pre-%d@example.edu", i), "pre-"+uuid.NewString())
		if err != nil || code != http.StatusCreated {
			t.Fatalf("pre-seed %d failed: code=%d err=%v", i, code, err)
		}
		cancelTargets = append(cancelTargets, rec)
	}

	type opResult struct {
		latency time.Duration
		ok      bool
	}
	results := make([]opResult, totalOps)
	jobs := make(chan int, totalOps)
	for i := 0; i < totalOps; i++ {
		jobs <- i
	}
	close(jobs)

	startAll := time.Now()
	var wg sync.WaitGroup
	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				opStart := time.Now()
				var ok bool
				if idx%10 == 0 && idx/10 < len(cancelTargets) {
					rec := cancelTargets[idx/10]
					code, _, err := doCancel(srv.App, rec.ID, rec.SelfAccessToken)
					ok = err == nil && code == http.StatusOK
				} else {
					code, _, err := doRegisterV1(srv.App, f, fmt.Sprintf("load-%d@example.edu", idx), fmt.Sprintf("load-idem-%d-%s", idx, uuid.NewString()))
					ok = err == nil && code == http.StatusCreated
				}
				results[idx] = opResult{latency: time.Since(opStart), ok: ok}
			}
		}()
	}
	wg.Wait()
	totalElapsed := time.Since(startAll)

	latencies := make([]time.Duration, 0, totalOps)
	var failures int
	for _, r := range results {
		if !r.ok {
			failures++
		}
		latencies = append(latencies, r.latency)
	}
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })

	percentile := func(p float64) time.Duration {
		if len(latencies) == 0 {
			return 0
		}
		idx := int(float64(len(latencies)-1) * p)
		return latencies[idx]
	}

	stat := pool.Stat()
	throughput := float64(totalOps) / totalElapsed.Seconds()

	var remaining, formConfirmed, dbConfirmed, distinctActiveTickets int
	_ = pool.QueryRow(context.Background(), `SELECT remaining, confirmed_count FROM participation.forms WHERE id=$1::uuid`, f.FormID).Scan(&remaining, &formConfirmed)
	_ = pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FILTER (WHERE state='confirmed'), COUNT(DISTINCT ticket_id) FILTER (WHERE state='confirmed' AND ticket_state='active')
		FROM participation.registrations WHERE form_id=$1::uuid
	`, f.FormID).Scan(&dbConfirmed, &distinctActiveTickets)

	t.Logf("PA-041 Load 3x Metrics: ops=%d concurrency=%d elapsed=%s throughput=%.2f req/s failures=%d p50=%s p95=%s p99=%s max=%s",
		totalOps, concurrency, totalElapsed.Round(time.Millisecond), throughput, failures,
		percentile(0.50).Round(time.Microsecond),
		percentile(0.95).Round(time.Microsecond),
		percentile(0.99).Round(time.Microsecond),
		latencies[len(latencies)-1].Round(time.Microsecond),
	)
	t.Logf("PA-041 pgxpool.Stat: max_conns=%d total_conns=%d acquire_count=%d empty_acquire_count=%d acquire_duration=%s",
		stat.MaxConns(), stat.TotalConns(), stat.AcquireCount(), stat.EmptyAcquireCount(), stat.AcquireDuration().Round(time.Millisecond),
	)
	t.Logf("PA-041 DB Invariants: capacity=%d remaining=%d form_confirmed=%d db_confirmed=%d distinct_active_tickets=%d",
		capacity, remaining, formConfirmed, dbConfirmed, distinctActiveTickets,
	)

	if failures != 0 {
		t.Fatalf("expected 0 failed operations under 3x load, got %d", failures)
	}
	if remaining < 0 || formConfirmed != dbConfirmed || formConfirmed > capacity || distinctActiveTickets != dbConfirmed {
		t.Fatalf("DB invariant violation under 3x load: remaining=%d formConfirmed=%d dbConfirmed=%d distinctActiveTickets=%d",
			remaining, formConfirmed, dbConfirmed, distinctActiveTickets)
	}
}
