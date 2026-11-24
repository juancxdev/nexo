package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	api "nexo-event-operations/internal/httpapi"
	"nexo-event-operations/internal/worker"
)

const form = "0194eb00-0000-7000-8000-000000000101"

// Requiere las migraciones de Onsite, Participation y Event Operations y los seeds demo.
func TestCertificatesAndReports(t *testing.T) {
	url := os.Getenv("OPERATIONS_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("OPERATIONS_TEST_DATABASE_URL no configurada")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	srv := api.New(pool)
	attended, absent := uuid.NewString(), uuid.NewString()
	ticketA, ticketB := "TEST-OPS-"+uuid.NewString(), "TEST-OPS-"+uuid.NewString()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%v: %s", err, sql)
		}
	}
	for _, r := range []struct{ id, ticket, name string }{{attended, ticketA, "Ana Pérez Prueba"}, {absent, ticketB, "Bruno Ausente"}} {
		exec(`INSERT INTO participation.registrations(id,form_id,state,full_name,email,self_access_token_ciphertext,self_access_token_hash,ticket_id,ticket_state,idempotency_key,request_hash) VALUES($1,$2,'confirmed',$3,$4,'\x00',$5,$6,'active',$7,'h')`,
			r.id, form, r.name, r.id+"@example.test", "hash-"+r.id, r.ticket, uuid.NewString())
		exec(`INSERT INTO onsite.ticket_projections(ticket_id,event_id,scope_id,status,expires_at,masked_holder,search_code,confirmed_at) VALUES($1,'00000000-0000-0000-0000-000000000101','00000000-0000-0000-0000-000000000102','active',now()+interval '1 day','=HYPERLINK("x")',$1,now())`, r.ticket)
	}
	exec(`INSERT INTO onsite.attendance_facts(id,ticket_id,event_id,scope_id,type,actor_id,device_id,occurred_at) VALUES($1,$2,'00000000-0000-0000-0000-000000000101','00000000-0000-0000-0000-000000000102','check_in','00000000-0000-0000-0000-000000000004','00000000-0000-0000-0000-000000000103',now())`, uuid.NewString(), ticketA)
	defer func() {
		for _, q := range []string{
			`DELETE FROM integration.hukid_deliveries WHERE certificate_id IN (SELECT id FROM certificates.certificates WHERE registration_id IN ($1,$2))`,
			`UPDATE certificates.certificates SET replaced_by=NULL,replaces_id=NULL WHERE registration_id IN ($1,$2)`,
			`DELETE FROM certificates.certificates WHERE registration_id IN ($1,$2)`,
			`DELETE FROM participation.registrations WHERE id::text IN ($1,$2)`,
		} {
			_, _ = pool.Exec(ctx, q, attended, absent)
		}
		_, _ = pool.Exec(ctx, `DELETE FROM onsite.attendance_facts WHERE ticket_id IN ($1,$2)`, ticketA, ticketB)
		_, _ = pool.Exec(ctx, `DELETE FROM onsite.ticket_projections WHERE ticket_id IN ($1,$2)`, ticketA, ticketB)
	}()
	do := func(method, path, body string, headers map[string]string) (int, map[string]any, string) {
		t.Helper()
		req := httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
		req.Header.Set("Content-Type", "application/json")
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		res, err := srv.App.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		raw, _ := io.ReadAll(res.Body)
		out := map[string]any{}
		_ = json.Unmarshal(raw, &out)
		return res.StatusCode, out, string(raw)
	}

	// Elegibilidad real.
	if st, out, _ := do("POST", "/operations/v1/certificate-jobs", `{"event_id":"event-demo","registration_id":"`+absent+`"}`, nil); st != 422 || out["code"] != "not_eligible" {
		t.Fatalf("sin asistencia = %d %v", st, out)
	}
	st, cert, _ := do("POST", "/operations/v1/certificate-jobs", `{"event_id":"event-demo","registration_id":"`+attended+`","display_name":"Nombre Inventado"}`, nil)
	if st != 201 || cert["display_name"] != "Ana Pérez Prueba" {
		t.Fatalf("emisión = %d %v (el nombre debe venir de Participation)", st, cert)
	}
	if st, out, _ := do("POST", "/operations/v1/certificate-jobs", `{"event_id":"event-demo","registration_id":"`+attended+`"}`, nil); st != 409 || out["code"] != "already_issued" {
		t.Fatalf("duplicado = %d %v", st, out)
	}
	if st, _, raw := do("GET", "/operations/v1/eligible-registrations?event_id=event-demo", "", nil); st != 200 || !strings.Contains(raw, attended) || strings.Contains(raw, absent) {
		t.Fatalf("elegibles = %d %s", st, raw)
	}
	id := cert["id"].(string)
	if st, _, _ := do("POST", "/operations/v1/certificates/"+id+"/revocations", `{"reason":"x"}`, nil); st != 422 {
		t.Fatalf("motivo corto = %d", st)
	}
	st, re, _ := do("POST", "/operations/v1/certificates/"+id+"/reissues", `{"reason":"corrección de nombre"}`, nil)
	if st != 201 || re["replaces_id"] != id {
		t.Fatalf("reemisión = %d %v", st, re)
	}
	newID := re["id"].(string)
	if st, out, _ := do("POST", "/operations/v1/certificates/"+id+"/revocations", `{"reason":"ya reemplazado"}`, nil); st != 409 {
		t.Fatalf("revocar reemplazado = %d %v", st, out)
	}
	if st, _, _ := do("POST", "/operations/v1/certificates/"+newID+"/revocations", `{"reason":"asistencia mal registrada"}`, nil); st != 200 {
		t.Fatalf("revocar = %d", st)
	}
	st, _, page := do("GET", "/public/v1/certificates/"+re["verification_code"].(string), "", map[string]string{"Accept": "text/html"})
	if st != 200 || !strings.Contains(page, "Revocado") || strings.Contains(page, "Ana") {
		t.Fatalf("verificación pública = %d %s", st, page)
	}
	if st, _, page := do("GET", "/public/v1/certificates/"+cert["verification_code"].(string)+"?format=html", "", nil); st != 200 || !strings.Contains(page, "Reemplazado") {
		t.Fatalf("verificación del reemplazado = %d", st)
	}

	// Reportes reales.
	st, job, _ := do("POST", "/operations/v1/report-jobs", `{"event_id":"event-demo","scope":"main","format":"csv","report_type":"attendance"}`, nil)
	if st != 202 {
		t.Fatalf("crear reporte = %d", st)
	}
	jobID := job["id"].(string)
	if st, out, _ := do("GET", "/operations/v1/report-jobs/"+jobID+"/artifact", "", nil); st != 409 {
		t.Fatalf("descarga antes de generar = %d %v", st, out)
	}
	worker.New(pool).ProcessReportsOnce(ctx)
	st, got, _ := do("GET", "/operations/v1/report-jobs/"+jobID, "", nil)
	if st != 200 || got["status"] != "succeeded" || got["download_available"] != true {
		t.Fatalf("estado = %v", got)
	}
	st, _, csv := do("GET", "/operations/v1/report-jobs/"+jobID+"/artifact", "", nil)
	if st != 200 || !strings.HasPrefix(csv, "\xEF\xBB\xBFfact_id,ticket_id") || !strings.Contains(csv, ticketA) {
		t.Fatalf("CSV = %d %q", st, csv[:min(len(csv), 200)])
	}
	if strings.Contains(csv, ",=HYPERLINK") || !strings.Contains(csv, `'=HYPERLINK`) {
		t.Fatal("el CSV debe neutralizar fórmulas")
	}
	if st, _, _ := do("POST", "/operations/v1/report-jobs/"+jobID+"/purges", "", nil); st != 204 {
		t.Fatalf("purga = %d", st)
	}
	if st, _, _ := do("GET", "/operations/v1/report-jobs/"+jobID+"/artifact", "", nil); st != 410 {
		t.Fatalf("descarga purgada = %d", st)
	}
	if st, _, _ := do("POST", "/operations/v1/report-jobs/"+jobID+"/retries", "", nil); st != 202 {
		t.Fatalf("reintento = %d", st)
	}
	st, job2, _ := do("POST", "/operations/v1/report-jobs", `{"event_id":"event-demo","format":"json","report_type":"summary"}`, nil)
	worker.New(pool).ProcessReportsOnce(ctx)
	st, _, js := do("GET", "/operations/v1/report-jobs/"+job2["id"].(string)+"/artifact", "", nil)
	if st != 200 || !strings.Contains(js, "presentes_ahora") {
		t.Fatalf("resumen JSON = %d %s", st, js)
	}
	_, _ = pool.Exec(ctx, `DELETE FROM operations.report_jobs WHERE id::text IN ($1,$2)`, jobID, job2["id"])
}
