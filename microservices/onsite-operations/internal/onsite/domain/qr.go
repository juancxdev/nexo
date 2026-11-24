package domain

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"
)

// Códigos de rechazo de un QR (OO-016). Son estables: el frontend los traduce y
// las métricas los agrupan. Ninguno revela datos personales.
const (
	RejectMalformed       = "qr_malformed"
	RejectUnsupportedAlg  = "qr_unsupported_alg"
	RejectUnknownKey      = "qr_unknown_key"
	RejectBadSignature    = "qr_bad_signature"
	RejectWrongAudience   = "qr_wrong_audience"
	RejectExpired         = "ticket_expired"
	RejectWrongEvent      = "wrong_event"
	RejectWrongScope      = "wrong_scope"
	RejectRevoked         = "ticket_revoked"
	RejectNotMaterialized = "ticket_not_materialized"
)

// TicketAudience es la audiencia que Participation pone a los QR ("aud":"onsite").
const TicketAudience = "onsite"

// ClockSkew tolera pequeñas diferencias de hora entre emisor y dispositivo.
const ClockSkew = 30 * time.Second

// TicketKey es una clave pública publicada por Participation.
// Una clave "retired" sigue verificando tickets ya emitidos; "compromised" no.
type TicketKey struct {
	Kid, Status string
	PublicKey   ed25519.PublicKey
}

// TicketClaims sigue el formato que ya emite Participation (makeTicket): jti es el
// ID del ticket. El QR no lleva nombre, correo ni documento.
type TicketClaims struct {
	JTI            string   `json:"jti"`
	Audience       string   `json:"aud"`
	Organization   string   `json:"organization_id"`
	Event          string   `json:"event_id"`
	Scope          string   `json:"scope"`
	Sessions       []string `json:"sessions,omitempty"`
	Expires        int64    `json:"exp"`
	ReleaseVersion string   `json:"release_version,omitempty"`
	Kid            string   `json:"kid,omitempty"`
}

type tokenHeader struct {
	Alg string `json:"alg"`
	Kid string `json:"kid"`
	Typ string `json:"typ"`
}

// LooksLikeTicketToken distingue un QR firmado (JWS compacto) de un código manual.
func LooksLikeTicketToken(raw string) bool {
	raw = strings.TrimSpace(raw)
	return strings.Count(raw, ".") == 2 && strings.HasPrefix(raw, "eyJ")
}

// VerifyTicketToken valida estructura, algoritmo, clave, firma, audiencia y vigencia.
// Las claves vienen del tenant del operador, así un QR de otra organización no verifica.
// Las reglas de evento, zona y base de datos se aplican después.
func VerifyTicketToken(raw string, keys map[string]TicketKey, now time.Time) (TicketClaims, string) {
	raw = strings.TrimSpace(raw)
	parts := strings.Split(raw, ".")
	if len(parts) != 3 || len(raw) > 4096 {
		return TicketClaims{}, RejectMalformed
	}
	var header tokenHeader
	if !decodeSegment(parts[0], &header) {
		return TicketClaims{}, RejectMalformed
	}
	// Lista cerrada: nunca "none" ni algoritmos simétricos (el HS256 de desarrollo se rechaza).
	if header.Alg != "EdDSA" {
		return TicketClaims{}, RejectUnsupportedAlg
	}
	key, ok := keys[header.Kid]
	if !ok || key.Status == "compromised" || len(key.PublicKey) != ed25519.PublicKeySize {
		return TicketClaims{}, RejectUnknownKey
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !ed25519.Verify(key.PublicKey, []byte(parts[0]+"."+parts[1]), signature) {
		return TicketClaims{}, RejectBadSignature
	}
	var claims TicketClaims
	if !decodeSegment(parts[1], &claims) || claims.JTI == "" || claims.Expires == 0 {
		return TicketClaims{}, RejectMalformed
	}
	if claims.Audience != TicketAudience {
		return claims, RejectWrongAudience
	}
	if !now.Before(time.Unix(claims.Expires, 0).Add(ClockSkew)) {
		return claims, RejectExpired
	}
	return claims, ""
}

// CheckScope compara la zona del QR con la del punto de control. Para sesiones no se
// exige la zona principal: las sesiones tienen sus propias reglas.
func (c TicketClaims) CheckScope(scope string, session bool) string {
	if !session && c.Scope != "" && c.Scope != scope {
		return RejectWrongScope
	}
	return ""
}

// AllowsSession indica si el propio QR declara acceso a una sesión.
func (c TicketClaims) AllowsSession(code string) bool {
	for _, s := range c.Sessions {
		if s == code {
			return true
		}
	}
	return false
}

// SignTicketToken firma un QR con el mismo formato que Participation. Se usa en
// pruebas y para generar QR de demostración.
func SignTicketToken(kid string, key ed25519.PrivateKey, claims TicketClaims) string {
	header, _ := json.Marshal(tokenHeader{Alg: "EdDSA", Kid: kid, Typ: "JWT"})
	body, _ := json.Marshal(claims)
	input := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(body)
	return input + "." + base64.RawURLEncoding.EncodeToString(ed25519.Sign(key, []byte(input)))
}

func decodeSegment(segment string, out any) bool {
	raw, err := base64.RawURLEncoding.DecodeString(segment)
	if err != nil {
		return false
	}
	return json.Unmarshal(raw, out) == nil
}

// Session describe una sesión del evento (OO-015). Capacity 0 = sin aforo local.
type Session struct {
	Code     string     `json:"code"`
	Name     string     `json:"name"`
	Access   string     `json:"access"`
	Capacity int        `json:"capacity"`
	Entries  int        `json:"entries"`
	OpensAt  *time.Time `json:"opens_at,omitempty"`
	ClosesAt *time.Time `json:"closes_at,omitempty"`
}

const (
	SessionOpen       = "open"
	SessionRestricted = "restricted"
)

const (
	RejectSessionNotAllowed = "session_not_allowed"
	RejectSessionFull       = "session_full"
	RejectSessionClosed     = "session_closed"
	RejectSessionNotFound   = "session_not_found"
	RejectAlreadyInSession  = "already_in_session"
)

// SessionDecision aplica las reglas de una sesión: horario, permiso y aforo.
func SessionDecision(s Session, entitled bool, now time.Time) string {
	if (s.OpensAt != nil && now.Before(*s.OpensAt)) || (s.ClosesAt != nil && !now.Before(*s.ClosesAt)) {
		return RejectSessionClosed
	}
	if s.Access == SessionRestricted && !entitled {
		return RejectSessionNotAllowed
	}
	if s.Capacity > 0 && s.Entries >= s.Capacity {
		return RejectSessionFull
	}
	return ""
}

// DevTicketSeedPhrase origina la semilla de la clave de desarrollo compartida con Participation
// (sha256 de "nexo-development-participation-ticket-key"). Sólo para local y pruebas.
const DevTicketSeedPhrase = "nexo-development-participation-ticket-key"
