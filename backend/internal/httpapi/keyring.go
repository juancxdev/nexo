package httpapi

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// keyRing keeps only public metadata in its JSON index. Private keys live in
// files mounted from the runtime secret store (a Docker volume in development,
// a KMS/secret-manager projection in production).
type keyRing struct {
	mu     sync.RWMutex
	dir    string
	index  string
	keys   map[string]signingKey
	active string
}

type signingKey struct {
	Kid         string     `json:"kid"`
	State       string     `json:"state"`
	File        string     `json:"file"`
	ActivatedAt time.Time  `json:"activated_at"`
	RetireAfter *time.Time `json:"retire_after,omitempty"`
	N           string     `json:"n"`
	E           string     `json:"e"`
}

type keyRingIndex struct {
	Active string       `json:"active"`
	Keys   []signingKey `json:"keys"`
}

func newKeyRing(legacyPath string) (*keyRing, error) {
	dir := os.Getenv("NEXO_JWT_KEY_RING_DIR")
	if dir == "" {
		if legacyPath != "" {
			dir = filepath.Dir(legacyPath)
		} else {
			dir = filepath.Join(os.TempDir(), "nexo-jwt-keys")
		}
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	ring := &keyRing{dir: dir, index: filepath.Join(dir, "keyring.json"), keys: map[string]signingKey{}}
	if raw, err := os.ReadFile(ring.index); err == nil {
		var persisted keyRingIndex
		if err := json.Unmarshal(raw, &persisted); err != nil {
			return nil, fmt.Errorf("invalid key ring index: %w", err)
		}
		for _, item := range persisted.Keys {
			ring.keys[item.Kid] = item
		}
		ring.active = persisted.Active
		if ring.active == "" || ring.keys[ring.active].State != "active" {
			return nil, errors.New("key ring has no active signing key")
		}
		return ring, nil
	}

	path := legacyPath
	if path == "" {
		path = filepath.Join(dir, "nexo-dev-1.pem")
	}
	key, err := loadOrCreateSigningKey(path)
	if err != nil {
		return nil, err
	}
	entry := makeSigningKey("nexo-dev-1", "active", path, key, time.Now().UTC(), nil)
	ring.keys[entry.Kid] = entry
	ring.active = entry.Kid
	return ring, ring.persistLocked()
}

func makeSigningKey(kid, state, file string, key *rsa.PrivateKey, at time.Time, retireAfter *time.Time) signingKey {
	return signingKey{Kid: kid, State: state, File: file, ActivatedAt: at, RetireAfter: retireAfter, N: base64.RawURLEncoding.EncodeToString(key.PublicKey.N.Bytes()), E: base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.PublicKey.E)).Bytes())}
}

func (r *keyRing) persistLocked() error {
	items := make([]signingKey, 0, len(r.keys))
	for _, item := range r.keys {
		items = append(items, item)
	}
	raw, err := json.MarshalIndent(keyRingIndex{Active: r.active, Keys: items}, "", "  ")
	if err != nil {
		return err
	}
	tmp := r.index + ".tmp"
	if err := os.WriteFile(tmp, raw, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, r.index)
}

func (r *keyRing) activeKey() (signingKey, *rsa.PrivateKey, error) {
	r.mu.RLock()
	item, ok := r.keys[r.active]
	r.mu.RUnlock()
	if !ok || item.State != "active" {
		return signingKey{}, nil, errors.New("active signing key unavailable")
	}
	key, err := loadPrivateKey(item.File)
	return item, key, err
}

func loadPrivateKey(path string) (*rsa.PrivateKey, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, errors.New("invalid JWT private key PEM")
	}
	return x509.ParsePKCS1PrivateKey(block.Bytes)
}

func (r *keyRing) sign(payload []byte) (string, error) {
	item, key, err := r.activeKey()
	if err != nil {
		return "", err
	}
	header, _ := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT", "kid": item.Kid})
	unsigned := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	digest := sha256.Sum256([]byte(unsigned))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

func (r *keyRing) verify(kid, unsigned, encodedSignature string) bool {
	r.mu.RLock()
	item, ok := r.keys[kid]
	r.mu.RUnlock()
	if !ok || item.State == "revoked" || (item.RetireAfter != nil && time.Now().UTC().After(*item.RetireAfter)) {
		return false
	}
	n, e1 := base64.RawURLEncoding.DecodeString(item.N)
	e, e2 := base64.RawURLEncoding.DecodeString(item.E)
	sig, e3 := base64.RawURLEncoding.DecodeString(encodedSignature)
	if e1 != nil || e2 != nil || e3 != nil {
		return false
	}
	pub := &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}
	digest := sha256.Sum256([]byte(unsigned))
	return rsa.VerifyPKCS1v15(pub, crypto.SHA256, digest[:], sig) == nil
}

func (r *keyRing) jwks() []map[string]string {
	now := time.Now().UTC()
	r.mu.RLock()
	defer r.mu.RUnlock()
	keys := make([]map[string]string, 0, len(r.keys))
	for _, item := range r.keys {
		if item.State == "revoked" || (item.RetireAfter != nil && now.After(*item.RetireAfter)) {
			continue
		}
		keys = append(keys, map[string]string{"kty": "RSA", "use": "sig", "alg": "RS256", "kid": item.Kid, "n": item.N, "e": item.E})
	}
	return keys
}

func (r *keyRing) rotate(window time.Duration) (signingKey, error) {
	if window < 5*time.Minute {
		return signingKey{}, errors.New("rotation window must be at least five minutes")
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return signingKey{}, err
	}
	now := time.Now().UTC()
	kid := "nexo-" + now.Format("20060102T150405.000000000Z")
	path := filepath.Join(r.dir, kid+".pem")
	der := x509.MarshalPKCS1PrivateKey(key)
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: der}), 0600); err != nil {
		return signingKey{}, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	previous := r.keys[r.active]
	previous.State = "retiring"
	retire := now.Add(window)
	previous.RetireAfter = &retire
	r.keys[previous.Kid] = previous
	next := makeSigningKey(kid, "active", path, key, now, nil)
	r.keys[kid] = next
	r.active = kid
	if err := r.persistLocked(); err != nil {
		return signingKey{}, err
	}
	return next, nil
}
