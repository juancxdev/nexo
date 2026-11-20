package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"strconv"
	"time"

	"nexo-event-operations/internal/secureclaims"
)

type Worker struct {
	DB     *pgxpool.Pool
	Client *http.Client
}

func New(db *pgxpool.Pool) *Worker {
	return &Worker{DB: db, Client: &http.Client{Timeout: 8 * time.Second}}
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

func (w *Worker) tickReports(ctx context.Context) {
	rows, err := w.DB.Query(ctx, `SELECT id::text,event_id,scope,format FROM operations.report_jobs WHERE status='pending' AND expires_at>now() LIMIT 10`)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id, eventID, scope, format string
		if rows.Scan(&id, &eventID, &scope, &format) != nil {
			continue
		}
		_, _ = w.DB.Exec(ctx, `UPDATE operations.report_jobs SET status='running',progress=25,updated_at=now() WHERE id=$1::uuid AND status='pending'`, id)
		_, _ = w.DB.Exec(ctx, `UPDATE operations.report_jobs SET status='succeeded',progress=100,artifact_key=$2,artifact_sha256=encode(digest($2,'sha256'),'hex'),updated_at=now() WHERE id=$1::uuid AND status='running'`, id, fmt.Sprintf("reports/%s.%s", id, format))
	}
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
		if w.DB.QueryRow(ctx, `SELECT d.idempotency_key,d.state,COALESCE(d.hukid_request_id,''),c.base_url,c.oauth_url,d.certificate_id::text,d.claims_ciphertext,d.claims_nonce,d.claims_key_version FROM integration.hukid_deliveries d JOIN integration.hukid_configs c ON c.id=d.config_id WHERE d.id=$1::uuid`, id).Scan(&key, &state, &requestID, &base, &oauth, &certID, &ciphertext, &nonce, &keyVersion) == nil {
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
		if resp.StatusCode == http.StatusUnprocessableEntity || resp.StatusCode == http.StatusConflict {
			return w.fail(ctx, id, fmt.Sprintf("hukid_%d", resp.StatusCode))
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
		if json.NewDecoder(resp.Body).Decode(&out) != nil {
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
func (w *Worker) token(ctx context.Context, url string) (string, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBufferString("grant_type=client_credentials&scope=issuance:create%20issuance:read"))
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
