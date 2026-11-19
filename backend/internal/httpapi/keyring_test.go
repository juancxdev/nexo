package httpapi

import (
	"encoding/base64"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestKeyRingKeepsPreviousKeyDuringTransition(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("NEXO_JWT_KEY_RING_DIR", dir)
	ring, err := newKeyRing(filepath.Join(dir, "legacy.pem"))
	if err != nil {
		t.Fatal(err)
	}
	first, err := ring.sign([]byte(`{"sub":"first"}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = ring.rotate(5 * time.Minute); err != nil {
		t.Fatal(err)
	}
	second, err := ring.sign([]byte(`{"sub":"second"}`))
	if err != nil {
		t.Fatal(err)
	}
	firstParts := strings.Split(first, ".")
	secondParts := strings.Split(second, ".")
	if len(firstParts) != 3 || len(secondParts) != 3 {
		t.Fatal("expected signed JWT values")
	}
	if !ring.verify(jwtKID(t, firstParts[0]), firstParts[0]+"."+firstParts[1], firstParts[2]) {
		t.Fatal("previous key must verify tokens during the transition window")
	}
	if !ring.verify(jwtKID(t, secondParts[0]), secondParts[0]+"."+secondParts[1], secondParts[2]) {
		t.Fatal("active key must verify new tokens")
	}
	if len(ring.jwks()) != 2 {
		t.Fatal("JWKS must publish active and retiring keys")
	}
}

func jwtKID(t *testing.T, header string) string {
	t.Helper()
	raw, err := base64.RawURLEncoding.DecodeString(header)
	if err != nil {
		t.Fatal(err)
	}
	var value struct {
		KID string `json:"kid"`
	}
	if err = json.Unmarshal(raw, &value); err != nil || value.KID == "" {
		t.Fatal("signed JWT does not contain a kid")
	}
	return value.KID
}
