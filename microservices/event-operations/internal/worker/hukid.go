package worker

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"nexo-event-operations/internal/reports"
	"nexo-event-operations/internal/secureclaims"
)

type Worker struct {
	DB     *pgxpool.Pool
	Client *http.Client
}

func New(db *pgxpool.Pool) *Worker {
	return &Worker{DB: db, Client: newHTTPClient()}
}

// newHTTPClient configura TLS de producción para HukID. La autenticación mTLS
// es opcional porque depende del entorno contratado; nunca se desactiva la
// verificación de certificados.
func newHTTPClient() *http.Client {
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	if caPath := strings.TrimSpace(os.Getenv("HUKID_CA_CERT_FILE")); caPath != "" {
		pem, err := os.ReadFile(filepath.Clean(caPath))
		if err == nil {
			pool, _ := x509.SystemCertPool()
			if pool == nil {
				pool = x509.NewCertPool()
			}
			if pool.AppendCertsFromPEM(pem) {
				tlsConfig.RootCAs = pool
			}
		}
	}
	if certPath, keyPath := os.Getenv("HUKID_CLIENT_CERT_FILE"), os.Getenv("HUKID_CLIENT_KEY_FILE"); certPath != "" && keyPath != "" {
		if cert, err := tls.LoadX509KeyPair(filepath.Clean(certPath), filepath.Clean(keyPath)); err == nil {
			tlsConfig.Certificates = []tls.Certificate{cert}
		}
	}
	return &http.Client{Timeout: 8 * time.Second, Transport: &http.Transport{TLSClientConfig: tlsConfig}}
}
func (w *Worker) Run(ctx context.Context) {
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			w.tick(ctx)
			w.tickReports(ctx)
		}
	}
}

// tickReports genera los reportes pendientes con datos reales (antes marcaba
// "succeeded" sin generar ningún archivo) y purga los vencidos.
func (w *Worker) tickReports(ctx context.Context) {
	_, _ = w.DB.Exec(ctx, `UPDATE operations.report_jobs SET status='expired',artifact=NULL,updated_at=now() WHERE status IN ('pending','running','succeeded','failed') AND expires_at<=now()`)
	rows, err := w.DB.Query(ctx, `WITH claimed AS (SELECT id FROM operations.report_jobs WHERE status='pending' AND expires_at>now() AND (lease_until IS NULL OR lease_until<now()) ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT 5)
		UPDATE operations.report_jobs j SET status='running',progress=25,attempts=attempts+1,lease_until=now()+interval '60 seconds',updated_at=now() FROM claimed WHERE j.id=claimed.id
		RETURNING j.id::text,j.tenant_id::text,j.event_id,j.scope,j.format,j.report_type`)
	if err != nil {
		return
	}
	type job struct{ id, tenant, event, scope, format, kind string }
	var jobs []job
	for rows.Next() {
		var j job
		if rows.Scan(&j.id, &j.tenant, &j.event, &j.scope, &j.format, &j.kind) == nil {
			jobs = append(jobs, j)
		}
	}
	rows.Close()
	for _, j := range jobs {
		w.runReport(ctx, j.id, j.tenant, j.event, j.scope, j.format, j.kind)
	}
}

func (w *Worker) runReport(ctx context.Context, id, tenant, event, scope, format, kind string) {
	table, err := reports.Build(ctx, w.DB, kind, tenant, event, scope)
	if err != nil {
		_, _ = w.DB.Exec(ctx, `UPDATE operations.report_jobs SET status='failed',error_code=$2,lease_until=NULL,updated_at=now() WHERE id=$1::uuid AND status='running'`, id, "source_unavailable")
		return
	}
	_, _ = w.DB.Exec(ctx, `UPDATE operations.report_jobs SET progress=75,updated_at=now() WHERE id=$1::uuid AND status='running'`, id)
	content, contentType, err := reports.Encode(table, format, kind, event, time.Now())
	if err != nil {
		_, _ = w.DB.Exec(ctx, `UPDATE operations.report_jobs SET status='failed',error_code='encoding_failed',lease_until=NULL,updated_at=now() WHERE id=$1::uuid AND status='running'`, id)
		return
	}
	sum := sha256.Sum256(content)
	// Si alguien canceló mientras se generaba, el WHERE status='running' no lo pisa.
	_, _ = w.DB.Exec(ctx, `UPDATE operations.report_jobs SET status='succeeded',progress=100,artifact=$2,content_type=$3,row_count=$4,artifact_key=$5,artifact_sha256=$6,error_code=NULL,lease_until=NULL,updated_at=now() WHERE id=$1::uuid AND status='running'`,
		id, content, contentType, len(table.Rows), fmt.Sprintf("reports/%s.%s", id, format), hex.EncodeToString(sum[:]))
}

func (w *Worker) tick(ctx context.Context) {
	rows, err := w.DB.Query(ctx, `WITH claimed AS (SELECT id FROM integration.hukid_deliveries WHERE state IN ('pending','request_created','offer_redeemed') AND next_attempt_at<=now() AND (lease_until IS NULL OR lease_until<now()) ORDER BY next_attempt_at FOR UPDATE SKIP LOCKED LIMIT 20) UPDATE integration.hukid_deliveries d SET lease_until=now()+interval '30 seconds' FROM claimed WHERE d.id=claimed.id RETURNING d.id::text`)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if rows.Scan(&id) != nil {
			continue
		}
		var key, state, requestID, base, oauth, certID, keyVersion string
		var ciphertext, nonce []byte
		if w.DB.QueryRow(ctx, `SELECT d.idempotency_key,d.state,COALESCE(d.hukid_request_id,''),c.base_url,c.oauth_url,COALESCE(d.certificate_id::text,''),d.claims_ciphertext,d.claims_nonce,d.claims_key_version FROM integration.hukid_deliveries d JOIN integration.hukid_configs c ON c.id=d.config_id WHERE d.id=$1::uuid`, id).Scan(&key, &state, &requestID, &base, &oauth, &certID, &ciphertext, &nonce, &keyVersion) == nil {
			if v := strings.TrimRight(os.Getenv("HUKID_BASE_URL"), "/"); v != "" {
				base = v
			}
			if v := strings.TrimRight(os.Getenv("HUKID_OAUTH_URL"), "/"); v != "" {
				oauth = v
			}
			_ = w.process(ctx, id, key, state, requestID, base, oauth, certID, ciphertext, nonce, keyVersion)
		}
	}
}
func (w *Worker) process(ctx context.Context, id, key, state, requestID, base, oauth, certID string, ciphertext, nonce []byte, keyVersion string) error {
	token, err := w.token(ctx, oauth)
	if err != nil {
		return w.retry(ctx, id, "token_unavailable")
	}
	if state == "pending" {
		body, err := secureclaims.Decrypt(ciphertext, nonce, keyVersion)
		if err != nil {
			return w.fail(ctx, id, "claims_decryption_failed")
		}
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, base+"/issuance-requests", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", key)
		resp, e := w.Client.Do(req)
		if e != nil {
			return w.retry(ctx, id, "network_error")
		}
		defer resp.Body.Close()
		responseBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if resp.StatusCode == http.StatusUnprocessableEntity || resp.StatusCode == http.StatusConflict {
			code := fmt.Sprintf("hukid_%d", resp.StatusCode)
			var remote struct {
				Code string `json:"code"`
			}
			if json.Unmarshal(responseBody, &remote) == nil && remote.Code != "" {
				code += ":" + remote.Code
			}
			return w.fail(ctx, id, code)
		}
		if resp.StatusCode == http.StatusTooManyRequests {
			return w.retryAfter(ctx, id, "hukid_429", resp.Header.Get("Retry-After"))
		}
		if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
			return w.retry(ctx, id, fmt.Sprintf("hukid_%d", resp.StatusCode))
		}
		var out struct {
			ID    string `json:"id"`
			Offer string `json:"credentialOfferUri"`
		}
		if json.Unmarshal(responseBody, &out) != nil {
			return w.retry(ctx, id, "invalid_response")
		}
		if out.ID == "" || out.Offer == "" {
			return w.fail(ctx, id, "invalid_response")
		}
		_, err = w.DB.Exec(ctx, `UPDATE integration.hukid_deliveries SET state='request_created',hukid_request_id=$2,credential_offer_uri=$3,offer_expires_at=now()+interval '5 minutes',attempts=attempts+1,next_attempt_at=now()+interval '5 seconds',lease_until=NULL,updated_at=now() WHERE id=$1::uuid`, id, out.ID, out.Offer)
		if err == nil {
			_, _ = w.DB.Exec(ctx, `UPDATE certificates.certificates SET state='offer_available' WHERE id=$1::uuid`, certID)
		}
		return err
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, base+"/issuance-requests/"+requestID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, e := w.Client.Do(req)
	if e != nil {
		return w.retry(ctx, id, "network_error")
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return w.retry(ctx, id, fmt.Sprintf("hukid_%d", resp.StatusCode))
	}
	var out struct {
		OfferState    string `json:"offerState"`
		IssuanceState string `json:"issuanceState"`
		CredentialID  string `json:"credentialId"`
	}
	if json.NewDecoder(resp.Body).Decode(&out) != nil {
		return w.retry(ctx, id, "invalid_response")
	}
	next := state
	if out.IssuanceState == "issued" {
		next = "credential_issued"
	} else if out.IssuanceState == "failed" || out.OfferState == "expired" {
		next = "failed"
	} else if out.IssuanceState == "cancelled" || out.OfferState == "cancelled" {
		next = "cancelled"
	} else if out.OfferState == "redeemed" {
		next = "offer_redeemed"
	}
	if next != state {
		_, err = w.DB.Exec(ctx, `UPDATE integration.hukid_deliveries SET state=$2,credential_id=NULLIF($3,''),next_attempt_at=now()+interval '5 seconds',lease_until=NULL,updated_at=now() WHERE id=$1::uuid`, id, next, out.CredentialID)
		if err == nil {
			certificateState := map[string]string{"credential_issued": "credential_issued", "offer_redeemed": "offer_redeemed", "failed": "failed", "cancelled": "cancelled"}[next]
			if certificateState != "" {
				_, _ = w.DB.Exec(ctx, `UPDATE certificates.certificates SET state=$2 WHERE id=$1::uuid`, certID, certificateState)
			}
		}
	} else {
		_, err = w.DB.Exec(ctx, `UPDATE integration.hukid_deliveries SET attempts=attempts+1,next_attempt_at=now()+interval '5 seconds',lease_until=NULL,updated_at=now() WHERE id=$1::uuid`, id)
	}
	return err
}
func (w *Worker) token(ctx context.Context, oauthURL string) (string, error) {
	values := url.Values{"grant_type": {"client_credentials"}, "scope": {"issuance:create issuance:read"}}
	if clientID, clientSecret := os.Getenv("HUKID_CLIENT_ID"), os.Getenv("HUKID_CLIENT_SECRET"); clientID != "" && clientSecret != "" {
		// HukID Integration API usa client authentication en el formulario OAuth.
		// No usar Basic aquí: además de coincidir con el contrato canónico,
		// evita que los adaptadores local, sandbox y productivo diverjan.
		values.Set("client_id", clientID)
		values.Set("client_secret", clientSecret)
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, oauthURL, bytes.NewBufferString(values.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := w.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("hukid_%d", resp.StatusCode)
	}
	var out struct {
		AccessToken string `json:"access_token"`
	}
	if err = json.NewDecoder(resp.Body).Decode(&out); err != nil || out.AccessToken == "" {
		return "", fmt.Errorf("invalid token")
	}
	return out.AccessToken, nil
}
func (w *Worker) retry(ctx context.Context, id, code string) error {
	_, err := w.DB.Exec(ctx, `UPDATE integration.hukid_deliveries SET attempts=attempts+1,last_error_code=$2,next_attempt_at=now()+make_interval(secs=>LEAST(300,power(2,LEAST(attempts,8)))),lease_until=NULL,updated_at=now(),state=CASE WHEN attempts+1>=8 THEN 'failed' ELSE state END WHERE id=$1::uuid`, id, code)
	return err
}
func (w *Worker) retryAfter(ctx context.Context, id, code, retryAfter string) error {
	seconds, err := strconv.Atoi(retryAfter)
	if err != nil || seconds < 1 || seconds > 300 {
		return w.retry(ctx, id, code)
	}
	_, err = w.DB.Exec(ctx, `UPDATE integration.hukid_deliveries SET attempts=attempts+1,last_error_code=$2,next_attempt_at=now()+make_interval(secs=>$3),lease_until=NULL,updated_at=now() WHERE id=$1::uuid`, id, code, seconds)
	return err
}
func (w *Worker) fail(ctx context.Context, id, code string) error {
	_, err := w.DB.Exec(ctx, `UPDATE integration.hukid_deliveries SET state='failed',attempts=attempts+1,last_error_code=$2,lease_until=NULL,updated_at=now() WHERE id=$1::uuid`, id, code)
	return err
}

// ProcessReportsOnce ejecuta una pasada del generador de reportes (pruebas y operación manual).
func (w *Worker) ProcessReportsOnce(ctx context.Context) { w.tickReports(ctx) }
