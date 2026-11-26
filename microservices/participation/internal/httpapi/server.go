package httpapi

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Server struct {
	DB            *pgxpool.Pool
	App           *fiber.App
	OperationsURL string
	InternalToken string
}

func New(db *pgxpool.Pool, operationsURL, internalToken string) *Server {
	s := &Server{DB: db, App: fiber.New(fiber.Config{AppName: "Nexo Participation"}), OperationsURL: strings.TrimRight(operationsURL, "/"), InternalToken: internalToken}
	s.routes()
	return s
}
func (s *Server) routes() {
	a := s.App
	a.Use(func(c fiber.Ctx) error {
		origin := c.Get("Origin")
		for _, candidate := range strings.Split(os.Getenv("CORS_ALLOWED_ORIGINS"), ",") {
			if strings.TrimSpace(candidate) == origin && origin != "" {
				c.Set("Access-Control-Allow-Origin", origin)
				c.Set("Vary", "Origin")
			}
		}
		c.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Idempotency-Key, X-Correlation-Id")
		c.Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
		if c.Method() == "OPTIONS" {
			return c.SendStatus(204)
		}
		return c.Next()
	})
	a.Get("/health/live", func(c fiber.Ctx) error { return c.JSON(fiber.Map{"status": "ok"}) })
	a.Get("/health/ready", func(c fiber.Ctx) error {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := s.DB.Ping(ctx); err != nil {
			return problem(c, 503, "database_unavailable", "base de datos no disponible")
		}
		return c.JSON(fiber.Map{"status": "ok"})
	})
	a.Get("/participation/public/v1/sites/:site/events/:event/form", s.form)
	a.Post("/participation/public/v1/sites/:site/events/:event/registrations", s.register)
	a.Get("/participation/self/v1/registrations/:id/ticket", s.ticket)
	a.Get("/participation/self/v1/registrations/:id/credential", s.credential)
}

type formRow struct {
	ID, Site, Event, EventID, Title, FormVersion, ConsentVersion, Binding, Consent string
	Schema                                                                         []byte
	Capacity, Confirmed, Remaining                                                 int
	Waitlist                                                                       bool
}

func (s *Server) form(c fiber.Ctx) error {
	var r formRow
	err := s.DB.QueryRow(c.Context(), `SELECT id::text,site_slug,event_slug,event_id,event_title,form_version_id::text,consent_version,binding_state,consent_text,schema,capacity,confirmed_count,remaining,waitlist_enabled FROM participation.forms WHERE site_slug=$1 AND event_slug=$2`, c.Params("site"), c.Params("event")).Scan(&r.ID, &r.Site, &r.Event, &r.EventID, &r.Title, &r.FormVersion, &r.ConsentVersion, &r.Binding, &r.Consent, &r.Schema, &r.Capacity, &r.Confirmed, &r.Remaining, &r.Waitlist)
	if err != nil {
		return problem(c, 404, "form_not_found", "formulario no encontrado")
	}
	var schema any
	_ = json.Unmarshal(r.Schema, &schema)
	state := "available"
	if r.Binding != "open" {
		state = "closed"
	} else if r.Remaining <= 0 {
		if r.Waitlist {
			state = "waitlist_only"
		} else {
			state = "closed"
		}
	} else if r.Remaining <= 10 {
		state = "limited"
	}
	return c.JSON(fiber.Map{"site_slug": r.Site, "event_slug": r.Event, "event_id": r.EventID, "event_title": r.Title, "form_version_id": r.FormVersion, "consent_version": r.ConsentVersion, "binding_state": r.Binding, "capacity_summary": fiber.Map{"remaining": r.Remaining, "public_state": state, "waitlist_enabled": r.Waitlist}, "consent_text": r.Consent, "schema": schema})
}

type registrationInput struct {
	FormVersionID   string `json:"form_version_id"`
	ConsentVersion  string `json:"consent_version"`
	ConsentAccepted bool   `json:"consent_accepted"`
	Attendee        struct {
		FullName    string `json:"full_name"`
		Email       string `json:"email"`
		Institution string `json:"institution"`
	} `json:"attendee"`
	Answers map[string]any `json:"answers"`
}

func (s *Server) register(c fiber.Ctx) error {
	key := strings.TrimSpace(c.Get("Idempotency-Key"))
	if key == "" {
		return problem(c, 400, "idempotency_key_required", "Idempotency-Key es obligatorio")
	}
	var in registrationInput
	if json.Unmarshal(c.Body(), &in) != nil || in.FormVersionID == "" || in.ConsentVersion == "" || !in.ConsentAccepted || in.Attendee.FullName == "" || in.Attendee.Email == "" {
		return problem(c, 400, "invalid_registration", "datos de registro incompletos")
	}
	emailNormalized := strings.ToLower(strings.TrimSpace(in.Attendee.Email))
	rawHash := sha256.Sum256(c.Body())
	hash := base64.RawURLEncoding.EncodeToString(rawHash[:])
	var existing string
	var existingHash string
	var existingState string
	var wait *int
	var ticket *string
	var encryptedToken []byte
	var token string
	var existingEventID string
	var existingHukidState string
	err := s.DB.QueryRow(c.Context(), `SELECT id::text,request_hash,state,waitlist_position,ticket_id,self_access_token_ciphertext,hukid_state FROM participation.registrations WHERE form_id=(SELECT id FROM participation.forms WHERE site_slug=$1 AND event_slug=$2) AND idempotency_key=$3`, c.Params("site"), c.Params("event"), key).Scan(&existing, &existingHash, &existingState, &wait, &ticket, &encryptedToken, &existingHukidState)
	if err == nil {
		_ = s.DB.QueryRow(c.Context(), `SELECT event_id FROM participation.forms WHERE site_slug=$1 AND event_slug=$2`, c.Params("site"), c.Params("event")).Scan(&existingEventID)
		if existingHash != hash {
			return problem(c, 409, "idempotency_key_reused", "el Idempotency-Key ya fue usado con otro payload")
		}
		token, _ = unprotectToken(encryptedToken)
		return receipt(c, existing, existingEventID, existingState, wait, ticket, token, existingHukidState)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return problem(c, 503, "persistence_unavailable", "no se pudo consultar el registro")
	}
	tx, err := s.DB.Begin(c.Context())
	if err != nil {
		return problem(c, 503, "persistence_unavailable", "no se pudo iniciar el registro")
	}
	defer tx.Rollback(c.Context())
	var formID, eventID, eventTitle, formVersion, consentVersion, tenantID string
	var remaining int
	var waitlist bool
	var binding string
	err = tx.QueryRow(c.Context(), `SELECT id::text,event_id,event_title,form_version_id::text,consent_version,remaining,waitlist_enabled,binding_state,tenant_id::text FROM participation.forms WHERE site_slug=$1 AND event_slug=$2 FOR UPDATE`, c.Params("site"), c.Params("event")).Scan(&formID, &eventID, &eventTitle, &formVersion, &consentVersion, &remaining, &waitlist, &binding, &tenantID)
	if err != nil {
		return problem(c, 404, "form_not_found", "formulario no encontrado")
	}
	if binding != "open" {
		return problem(c, 409, "registration_closed", "el registro no está abierto")
	}
	if in.FormVersionID != formVersion || in.ConsentVersion != consentVersion {
		return problem(c, 409, "form_version_mismatch", "el formulario fue actualizado")
	}
	var state string
	var pos *int
	if remaining > 0 {
		state = "confirmed"
		if _, err = tx.Exec(c.Context(), `UPDATE participation.forms SET remaining=remaining-1,confirmed_count=confirmed_count+1 WHERE id=$1::uuid`, formID); err != nil {
			return problem(c, 503, "persistence_unavailable", "no se pudo reservar capacidad")
		}
	} else if waitlist {
		state = "waitlisted"
		if err = tx.QueryRow(c.Context(), `SELECT COALESCE(MAX(waitlist_position),0)+1 FROM participation.registrations WHERE form_id=$1::uuid AND state='waitlisted'`, formID).Scan(&pos); err != nil {
			return problem(c, 503, "persistence_unavailable", "no se pudo asignar la lista de espera")
		}
	} else {
		return problem(c, 409, "capacity_exhausted", "el evento no tiene cupos disponibles")
	}
	id := uuid.New()
	token = uuid.NewString() + uuid.NewString()
	tokenHash := hashString(token)
	encryptedToken, err = protectToken(token)
	if err != nil {
		return problem(c, 503, "token_protection_unavailable", "no se pudo proteger el token de acceso")
	}
	var ticketID *string
	var ticketJWS *string
	if state == "confirmed" {
		v := uuid.NewString()
		ticketID = &v
		j := makeTicket(v, eventID, c.Params("site"), c.Params("event"))
		ticketJWS = &j
	}
	participantFingerprint := fingerprint(in.Attendee.FullName, in.Attendee.Institution, emailNormalized)
	_, err = tx.Exec(c.Context(), `INSERT INTO participation.registrations(id,form_id,state,full_name,email,email_normalized,participant_fingerprint,institution,answers,self_access_token_ciphertext,self_access_token_hash,ticket_id,ticket_jws,ticket_state,idempotency_key,request_hash,waitlist_position) VALUES($1,$2::uuid,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`, id, formID, state, in.Attendee.FullName, emailNormalized, emailNormalized, participantFingerprint, in.Attendee.Institution, in.Answers, encryptedToken, tokenHash, ticketID, ticketJWS, func() string {
		if state == "confirmed" {
			return "active"
		}
		return ""
	}(), key, hash, pos)
	if err != nil {
		if strings.Contains(err.Error(), "registrations_unique_email_per_form") || strings.Contains(err.Error(), "registrations_form_id_email_key") {
			return problem(c, 409, "already_registered", "el correo ya está registrado")
		}
		return problem(c, 503, "persistence_unavailable", "no se pudo guardar el registro")
	}
	if err = tx.Commit(c.Context()); err != nil {
		return problem(c, 503, "persistence_unavailable", "no se pudo confirmar el registro")
	}
	hukidState := "not_requested"
	if state == "confirmed" && s.OperationsURL != "" && s.InternalToken != "" {
		hukidState = s.requestHukID(c.Context(), tenantID, eventID, eventTitle, id.String(), *ticketID, in.Attendee.FullName, in.Answers)
		_, _ = s.DB.Exec(c.Context(), `UPDATE participation.registrations SET hukid_state=$2 WHERE id=$1::uuid`, id, hukidState)
	}
	return receipt(c, id.String(), eventID, state, pos, ticketID, token, hukidState)
}

func fingerprint(fullName, institution, email string) string {
	value := strings.Join([]string{strings.ToLower(strings.Join(strings.Fields(fullName), " ")), strings.ToLower(strings.Join(strings.Fields(institution), " ")), email}, "|")
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func (s *Server) requestHukID(ctx context.Context, tenantID, eventID, eventTitle, registrationID, ticketID, participantName string, answers map[string]any) string {
	role := "attendee"
	if value, ok := answers["role"].(string); ok && strings.TrimSpace(value) != "" {
		role = strings.TrimSpace(value)
	}
	body, _ := json.Marshal(map[string]any{
		"event_id": eventID, "event_title": eventTitle, "registration_id": registrationID,
		"ticket_id": ticketID, "participant_name": participantName, "participation_role": role,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.OperationsURL+"/internal/v1/participation-credentials", bytes.NewReader(body))
	if err != nil {
		return "request_failed"
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Service-Token", s.InternalToken)
	req.Header.Set("X-Tenant-Id", tenantID)
	req.Header.Set("Idempotency-Key", "participation:"+registrationID)
	resp, err := (&http.Client{Timeout: 3 * time.Second}).Do(req)
	if err != nil {
		return "pending"
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "request_failed"
	}
	return "pending"
}
func receipt(c fiber.Ctx, id, eventID, state string, pos *int, ticket *string, token, hukidState string) error {
	return c.Status(201).JSON(fiber.Map{"id": id, "event_id": eventID, "state": state, "waitlist_position": pos, "ticket_id": ticket, "self_access_token": token, "hukid_state": hukidState})
}
func (s *Server) ticket(c fiber.Ctx) error {
	auth := strings.TrimSpace(strings.TrimPrefix(c.Get("Authorization"), "Bearer "))
	if auth == "" {
		return problem(c, 401, "unauthorized", "token de acceso requerido")
	}
	var id, state, jws, expires string
	err := s.DB.QueryRow(c.Context(), `SELECT id::text,ticket_state,COALESCE(ticket_jws,''),(created_at+interval '24 hours')::text FROM participation.registrations WHERE id=$1 AND self_access_token_hash=$2`, c.Params("id"), hashString(auth)).Scan(&id, &state, &jws, &expires)
	if err != nil {
		return problem(c, 404, "ticket_not_found", "ticket no encontrado")
	}
	if state != "active" {
		return problem(c, 409, "ticket_unavailable", "ticket no disponible")
	}
	return c.JSON(fiber.Map{"id": id, "state": state, "ticket_jws": jws, "expires_at": expires})
}

func (s *Server) credential(c fiber.Ctx) error {
	auth := strings.TrimSpace(strings.TrimPrefix(c.Get("Authorization"), "Bearer "))
	if auth == "" {
		return problem(c, 401, "unauthorized", "token de acceso requerido")
	}
	var registrationID, tokenHash, tenantID string
	if err := s.DB.QueryRow(c.Context(), `SELECT r.id::text,r.self_access_token_hash,f.tenant_id::text FROM participation.registrations r JOIN participation.forms f ON f.id=r.form_id WHERE r.id=$1::uuid`, c.Params("id")).Scan(&registrationID, &tokenHash, &tenantID); err != nil || tokenHash != hashString(auth) {
		return problem(c, 404, "registration_not_found", "registro no encontrado")
	}
	if s.OperationsURL == "" || s.InternalToken == "" {
		return c.JSON(fiber.Map{"state": "not_configured"})
	}
	req, err := http.NewRequestWithContext(c.Context(), http.MethodGet, s.OperationsURL+"/internal/v1/participation-credentials/"+registrationID, nil)
	if err != nil {
		return problem(c, 503, "hukid_unavailable", "no se pudo consultar HukID")
	}
	req.Header.Set("X-Internal-Service-Token", s.InternalToken)
	req.Header.Set("X-Tenant-Id", tenantID)
	resp, err := (&http.Client{Timeout: 3 * time.Second}).Do(req)
	if err != nil {
		return problem(c, 503, "hukid_unavailable", "no se pudo consultar HukID")
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return c.JSON(fiber.Map{"state": "pending"})
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return problem(c, 503, "hukid_unavailable", "no se pudo consultar HukID")
	}
	var payload map[string]any
	if json.NewDecoder(resp.Body).Decode(&payload) != nil {
		return problem(c, 503, "hukid_invalid_response", "respuesta HukID inválida")
	}
	return c.JSON(payload)
}
func hashString(v string) string {
	h := sha256.Sum256([]byte(v))
	return base64.RawURLEncoding.EncodeToString(h[:])
}
func protectToken(token string) ([]byte, error) {
	key := sha256.Sum256([]byte(getenv("PARTICIPATION_TOKEN_KEY", "nexo-development-participation-token-key")))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, []byte(token), nil), nil
}
func unprotectToken(blob []byte) (string, error) {
	key := sha256.Sum256([]byte(getenv("PARTICIPATION_TOKEN_KEY", "nexo-development-participation-token-key")))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	n := gcm.NonceSize()
	if len(blob) < n {
		return "", errors.New("invalid token")
	}
	out, err := gcm.Open(nil, blob[:n], blob[n:], nil)
	return string(out), err
}
func getenv(k, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return fallback
}
func makeTicket(id, event, site, slug string) string {
	enc := base64.RawURLEncoding.EncodeToString
	kid, key := ticketSigningKey()
	header, _ := json.Marshal(map[string]string{"alg": "EdDSA", "kid": kid, "typ": "JWT"})
	payload, _ := json.Marshal(map[string]any{"jti": id, "aud": "onsite", "organization_id": site, "event_id": event, "event_slug": slug, "scope": "main", "exp": time.Now().Add(24 * time.Hour).Unix(), "release_version": "dev-1", "kid": kid})
	input := enc(header) + "." + enc(payload)
	return input + "." + enc(ed25519.Sign(key, []byte(input)))
}

func ticketSigningKey() (string, ed25519.PrivateKey) {
	kid := getenv("PARTICIPATION_TICKET_KID", "participation-dev-1")
	if raw := strings.TrimSpace(os.Getenv("PARTICIPATION_TICKET_SIGNING_SEED")); raw != "" {
		if seed, err := base64.StdEncoding.DecodeString(raw); err == nil && len(seed) == ed25519.SeedSize {
			return kid, ed25519.NewKeyFromSeed(seed)
		}
	}
	seed := sha256.Sum256([]byte("nexo-development-participation-ticket-key"))
	return kid, ed25519.NewKeyFromSeed(seed[:])
}
func problem(c fiber.Ctx, status int, code, detail string) error {
	return c.Status(status).Type("application/problem+json").JSON(fiber.Map{"type": "https://errors.example.pe/" + code, "title": code, "status": status, "code": code, "detail": detail})
}
