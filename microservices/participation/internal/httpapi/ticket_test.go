package httpapi

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
)

// El QR emitido debe ser un JWS EdDSA verificable con la clave publicada.
func TestMakeTicketIsSignedWithEd25519(t *testing.T) {
	token := makeTicket("t-1", "demo-day-2026", "incuba-unas", "congreso")
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("token mal formado: %s", token)
	}
	var header map[string]string
	raw, _ := base64.RawURLEncoding.DecodeString(parts[0])
	_ = json.Unmarshal(raw, &header)
	if header["alg"] != "EdDSA" || header["kid"] != "participation-dev-1" {
		t.Fatalf("header = %v", header)
	}
	_, key := ticketSigningKey()
	sig, _ := base64.RawURLEncoding.DecodeString(parts[2])
	if !ed25519.Verify(key.Public().(ed25519.PublicKey), []byte(parts[0]+"."+parts[1]), sig) {
		t.Fatal("la firma no verifica")
	}
	var claims map[string]any
	raw, _ = base64.RawURLEncoding.DecodeString(parts[1])
	_ = json.Unmarshal(raw, &claims)
	if claims["aud"] != "onsite" || claims["scope"] != "main" || claims["jti"] != "t-1" || claims["event_id"] != "demo-day-2026" {
		t.Fatalf("claims = %v", claims)
	}
	for _, pii := range []string{"email", "full_name", "name"} {
		if _, ok := claims[pii]; ok {
			t.Fatalf("el QR no debe llevar %s", pii)
		}
	}
	// La clave pública de desarrollo es la sembrada en Onsite.
	if got := hex.EncodeToString(key.Public().(ed25519.PublicKey)); got != "a73b5e172f8d462d7df1a15651e65d7af01f011bdcc6e558a8dee82d70fa6e0a" {
		t.Fatalf("la clave pública de desarrollo no coincide con la de Onsite: %s", got)
	}
}
