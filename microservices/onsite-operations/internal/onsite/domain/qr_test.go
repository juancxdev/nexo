package domain

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestVerifyTicketToken(t *testing.T) {
	now := time.Date(2026, 11, 24, 9, 0, 0, 0, time.UTC)
	pub, priv, _ := ed25519.GenerateKey(nil)
	_, otherPriv, _ := ed25519.GenerateKey(nil)
	keys := map[string]TicketKey{"k1": {Kid: "k1", Status: "active", PublicKey: pub}}
	claims := func() TicketClaims {
		return TicketClaims{JTI: "t-1", Audience: "onsite", Organization: "incuba-unas", Event: "demo-day-2026", Scope: "main", Expires: now.Add(time.Hour).Unix(), Kid: "k1"}
	}
	with := func(edit func(*TicketClaims)) string { c := claims(); edit(&c); return SignTicketToken("k1", priv, c) }
	good := SignTicketToken("k1", priv, claims())
	parts := strings.Split(good, ".")
	forged := func() string {
		c := claims()
		c.JTI = "t-otro"
		b, _ := json.Marshal(c)
		return parts[0] + "." + base64.RawURLEncoding.EncodeToString(b) + "." + parts[2]
	}()
	hs256 := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","kid":"participation-dev-1"}`)) + "." + parts[1] + ".development-signature"
	cases := []struct {
		name, token string
		keys        map[string]TicketKey
		want        string
	}{
		{"válido", good, keys, ""},
		{"clave retirada aún verifica", good, map[string]TicketKey{"k1": {Kid: "k1", Status: "retired", PublicKey: pub}}, ""},
		{"clave comprometida", good, map[string]TicketKey{"k1": {Kid: "k1", Status: "compromised", PublicKey: pub}}, RejectUnknownKey},
		{"firma de otra clave", SignTicketToken("k1", otherPriv, claims()), keys, RejectBadSignature},
		{"contenido alterado", forged, keys, RejectBadSignature},
		{"HS256 de desarrollo", hs256, keys, RejectUnsupportedAlg},
		{"kid desconocido", SignTicketToken("k9", priv, claims()), keys, RejectUnknownKey},
		{"audiencia ajena", with(func(c *TicketClaims) { c.Audience = "otro" }), keys, RejectWrongAudience},
		{"vencido", with(func(c *TicketClaims) { c.Expires = now.Add(-time.Minute).Unix() }), keys, RejectExpired},
		{"basura", "hola", keys, RejectMalformed},
	}
	for _, tc := range cases {
		if _, got := VerifyTicketToken(tc.token, tc.keys, now); got != tc.want {
			t.Errorf("%s: got %q want %q", tc.name, got, tc.want)
		}
	}
	if c, _ := VerifyTicketToken(good, keys, now); c.CheckScope("cyber", false) != RejectWrongScope || c.CheckScope("cyber", true) != "" {
		t.Error("la zona sólo se exige fuera de sesiones")
	}
}

func TestSessionDecision(t *testing.T) {
	now := time.Now()
	later := now.Add(time.Hour)
	if SessionDecision(Session{Access: SessionRestricted}, false, now) != RejectSessionNotAllowed {
		t.Error("restringida sin permiso")
	}
	if SessionDecision(Session{Access: SessionOpen, Capacity: 2, Entries: 2}, true, now) != RejectSessionFull {
		t.Error("aforo lleno")
	}
	if SessionDecision(Session{Access: SessionOpen, OpensAt: &later}, true, now) != RejectSessionClosed {
		t.Error("aún no abre")
	}
	if SessionDecision(Session{Access: SessionOpen, Capacity: 2, Entries: 1}, false, now) != "" {
		t.Error("abierta con cupo")
	}
}

func TestEffectivePresence(t *testing.T) {
	t0 := time.Now()
	in := TimelineEntry{FactID: "in", Type: "check_in", OccurredAt: t0}
	out := TimelineEntry{FactID: "out", Type: "check_out", OccurredAt: t0.Add(time.Minute)}
	if id, _ := EffectivePresence([]TimelineEntry{in, out}); id != "" {
		t.Error("tras la salida no está presente")
	}
	out.Voided = true
	if id, _ := EffectivePresence([]TimelineEntry{in, out}); id != "in" {
		t.Error("anular la salida devuelve la presencia")
	}
	in.Voided = true
	if id, _ := EffectivePresence([]TimelineEntry{in, out}); id != "" {
		t.Error("todo anulado: ausente")
	}
}
