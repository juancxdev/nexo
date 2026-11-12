package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"time"
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
	rows, err := w.DB.Query(ctx, `SELECT d.id::text,d.idempotency_key,d.state,d.hukid_request_id,c.issuer_id,c.credential_definition,c.base_url,c.oauth_url,cert.id::text,cert.display_name,cert.event_id,cert.artifact_sha256 FROM integration.hukid_deliveries d JOIN integration.hukid_configs c ON c.id=d.config_id JOIN certificates.certificates cert ON cert.id=d.certificate_id WHERE d.state IN ('pending','request_created','offer_redeemed') AND d.next_attempt_at<=now() LIMIT 20`)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id, key, state, requestID, issuer, definition, base, oauth, certID, name, eventID, digest string
		if rows.Scan(&id, &key, &state, &requestID, &issuer, &definition, &base, &oauth, &certID, &name, &eventID, &digest) == nil {
			_ = w.process(ctx, id, key, state, requestID, issuer, definition, base, oauth, certID, name, eventID, digest)
		}
	}
}
func (w *Worker) process(ctx context.Context, id, key, state, requestID, issuer, definition, base, oauth, certID, name, eventID, digest string) error {
	token, err := w.token(ctx, oauth)
	if err != nil {
		return w.retry(ctx, id, "token_unavailable")
	}
	if state == "pending" {
		payload := map[string]any{"issuerId": issuer, "credentialDefinition": definition, "claims": map[string]any{"certificate": map[string]any{"id": certID, "issuedAt": time.Now().UTC().Format(time.RFC3339), "artifactSha256": digest}, "recipient": map[string]any{"registrationId": certID, "displayName": name}, "event": map[string]any{"id": eventID}}, "delivery": map[string]any{"flow": "pre-authorized_code", "expiresInSeconds": 300, "txCodeRequired": true}, "source": map[string]any{"system": "nexo", "reference": certID, "evidenceDigest": digest}}
		body, _ := json.Marshal(payload)
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, base+"/issuance-requests", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", key)
		resp, e := w.Client.Do(req)
		if e != nil {
			return w.retry(ctx, id, "network_error")
		}
		defer resp.Body.Close()
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
		_, err = w.DB.Exec(ctx, `UPDATE integration.hukid_deliveries SET state='request_created',hukid_request_id=$2,credential_offer_uri=$3,attempts=attempts+1,next_attempt_at=now()+interval '5 seconds',updated_at=now() WHERE id=$1::uuid`, id, out.ID, out.Offer)
		return err
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, base+"/issuance-requests/"+requestID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, e := w.Client.Do(req)
	if e != nil {
		return w.retry(ctx, id, "network_error")
	}
	defer resp.Body.Close()
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
	} else if out.IssuanceState == "cancelled" || out.OfferState == "cancelled" {
		next = "cancelled"
	} else if out.OfferState == "redeemed" {
		next = "offer_redeemed"
	}
	if next != state {
		_, err = w.DB.Exec(ctx, `UPDATE integration.hukid_deliveries SET state=$2,credential_id=NULLIF($3,''),next_attempt_at=now()+interval '5 seconds',updated_at=now() WHERE id=$1::uuid`, id, next, out.CredentialID)
	} else {
		_, err = w.DB.Exec(ctx, `UPDATE integration.hukid_deliveries SET attempts=attempts+1,next_attempt_at=now()+interval '5 seconds',updated_at=now() WHERE id=$1::uuid`, id)
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
	var out struct {
		AccessToken string `json:"access_token"`
	}
	if err = json.NewDecoder(resp.Body).Decode(&out); err != nil || out.AccessToken == "" {
		return "", fmt.Errorf("invalid token")
	}
	return out.AccessToken, nil
}
func (w *Worker) retry(ctx context.Context, id, code string) error {
	_, err := w.DB.Exec(ctx, `UPDATE integration.hukid_deliveries SET attempts=attempts+1,last_error_code=$2,next_attempt_at=now()+make_interval(secs=>LEAST(300,power(2,LEAST(attempts,8)))),updated_at=now(),state=CASE WHEN attempts+1>=8 THEN 'failed' ELSE state END WHERE id=$1::uuid`, id, code)
	return err
}
