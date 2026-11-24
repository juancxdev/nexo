package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	api "nexo-onsite-operations/internal/platform/http"
	"nexo-onsite-operations/internal/platform/postgres"
)

// Regresiones de los arreglos aportados sobre la base de Juan.
func TestAportesOnsite(t *testing.T) {
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
	ticket := "TEST-APORTES-" + uuid.NewString()
	if _, err = pool.Exec(ctx, `INSERT INTO onsite.ticket_projections(ticket_id,event_id,scope_id,status,expires_at,masked_holder,search_code,confirmed_at) VALUES($1,'00000000-0000-0000-0000-000000000101','00000000-0000-0000-0000-000000000102','active',now()+interval '1 day','T*** Aportes',$1,now())`, ticket); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = pool.Exec(ctx, `DELETE FROM onsite.effective_attendance WHERE ticket_id=$1`, ticket)
		_, _ = pool.Exec(ctx, `DELETE FROM onsite.attendance_facts WHERE ticket_id=$1 AND type='void'`, ticket)
		_, _ = pool.Exec(ctx, `DELETE FROM onsite.attendance_facts WHERE ticket_id=$1`, ticket)
		_, _ = pool.Exec(ctx, `DELETE FROM onsite.ticket_projections WHERE ticket_id=$1`, ticket)
	}()
	body := []byte(fmt.Sprintf(`{"ticket_id":%q,"event_id":"event-demo","scope":"main"}`, ticket))
	present := func() bool {
		var n int
		_ = pool.QueryRow(ctx, `SELECT count(*) FROM onsite.effective_attendance WHERE ticket_id=$1`, ticket).Scan(&n)
		return n == 1
	}
	facts := func() int {
		var n int
		_ = pool.QueryRow(ctx, `SELECT count(*) FROM onsite.attendance_facts WHERE ticket_id=$1`, ticket).Scan(&n)
		return n
	}

	// 1) Sin asignación vigente: 403 y el registro NO se crea (antes se creaba igual).
	st, _ := call(t, router, "/onsite/v1/attendance/check-ins", body, map[string]string{"X-Mock-Role": "staff", "X-Mock-Actor": uuid.NewString()})
	if st != 403 || facts() != 0 {
		t.Fatalf("sin asignación: status=%d hechos=%d; se esperaba 403 y 0", st, facts())
	}

	staff := map[string]string{"X-Mock-Role": "staff"}
	if st, _ := call(t, router, "/onsite/v1/attendance/check-ins", body, staff); st != 201 {
		t.Fatalf("check-in = %d", st)
	}
	st, out := call(t, router, "/onsite/v1/attendance/check-outs", body, staff)
	if st != 201 || present() {
		t.Fatalf("check-out = %d presente=%v", st, present())
	}
	checkOutID, _ := out["ID"].(string)
	voidURL := "/onsite/v1/attendance/facts/" + checkOutID + "/voids"
	sup := map[string]string{"X-Mock-Role": "supervisor"}

	// 2) Motivo demasiado corto: 422.
	if st, _ := call(t, router, voidURL, []byte(`{"reason":"x"}`), sup); st != 422 {
		t.Fatalf("motivo corto = %d; se esperaba 422", st)
	}
	// 3) Anular la salida devuelve la presencia.
	if st, b := call(t, router, voidURL, []byte(`{"reason":"salida marcada por error"}`), sup); st != 201 {
		t.Fatalf("anular salida = %d %v", st, b)
	}
	if !present() {
		t.Fatal("al anular la salida la persona debe volver a estar presente")
	}
	// 4) No se puede anular dos veces.
	if st, b := call(t, router, voidURL, []byte(`{"reason":"segundo intento de anular"}`), sup); st != 409 || b["code"] != "already_voided" {
		t.Fatalf("doble anulación = %d %v; se esperaba 409 already_voided", st, b)
	}
}

func call(t *testing.T, router *api.Router, path string, body []byte, headers map[string]string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest("POST", path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.NewString())
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := router.App.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	out := map[string]any{}
	_ = json.Unmarshal(raw, &out)
	return res.StatusCode, out
}

// Revocar un dispositivo guarda el motivo y cierra sus turnos abiertos.
func TestRevokeDeviceClosesShifts(t *testing.T) {
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
	var tenant string
	if err := pool.QueryRow(ctx, `SELECT tenant_id::text FROM onsite.events WHERE id='00000000-0000-0000-0000-000000000101'`).Scan(&tenant); err != nil {
		t.Fatal(err)
	}
	device, shift := uuid.NewString(), uuid.NewString()
	if _, err := pool.Exec(ctx, `INSERT INTO onsite.devices(id,tenant_id,label,status) VALUES($1,$2::uuid,'Prueba revocación','active')`, device, tenant); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = pool.Exec(ctx, `DELETE FROM onsite.shifts WHERE device_id=$1`, device)
		_, _ = pool.Exec(ctx, `DELETE FROM onsite.devices WHERE id=$1`, device)
	}()
	if _, err := pool.Exec(ctx, `INSERT INTO onsite.shifts(id,tenant_id,event_id,scope_id,device_id,actor_id,status) VALUES($1,$2::uuid,'00000000-0000-0000-0000-000000000101','00000000-0000-0000-0000-000000000102',$3,'00000000-0000-0000-0000-000000000004','open')`, shift, tenant, device); err != nil {
		t.Fatal(err)
	}
	if err := postgres.New(pool).RevokeDevice(ctx, device, tenant, "equipo extraviado"); err != nil {
		t.Fatal(err)
	}
	var reason, status string
	_ = pool.QueryRow(ctx, `SELECT coalesce(revoked_reason,'') FROM onsite.devices WHERE id=$1`, device).Scan(&reason)
	_ = pool.QueryRow(ctx, `SELECT status FROM onsite.shifts WHERE id=$1`, shift).Scan(&status)
	if reason != "equipo extraviado" || status != "closed" {
		t.Fatalf("motivo=%q turno=%q", reason, status)
	}
}
