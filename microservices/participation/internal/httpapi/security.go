package httpapi

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net"
	"net/http"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gofiber/fiber/v3"
)

const (
	deviceCookieName  = "participation_device"
	maxRateBuckets    = 50000
	maxUsedChallenges = 50000
)

type SecurityConfig struct {
	RateWindow           time.Duration
	IPLimit              int
	EventLimit           int
	DeviceLimit          int
	ChallengeTTL         time.Duration
	ChallengeDifficulty  uint8
	ChallengeSecret      []byte
	MetricsToken         string
	Allowlist            []netip.Prefix
	TrustedProxies       []netip.Prefix
	DeviceCookieSecure   bool
	DeviceCookieSameSite http.SameSite
}

type securityState struct {
	config SecurityConfig
	secret []byte

	rateMu      sync.Mutex
	rateBuckets map[string]rateBucket
	rateOps     uint64

	challengeMu    sync.Mutex
	usedChallenges map[string]time.Time

	metrics securityCounters
}

type rateBucket struct {
	window time.Time
	count  int
}

type securityCounters struct {
	publicRequests      atomic.Uint64
	rateLimited         atomic.Uint64
	challengesIssued    atomic.Uint64
	challengesAccepted  atomic.Uint64
	challengesRejected  atomic.Uint64
	challengeReplays    atomic.Uint64
	allowlistedRequests atomic.Uint64
}

type securityMetrics struct {
	PublicRequests      uint64 `json:"public_requests"`
	RateLimited         uint64 `json:"rate_limited"`
	ChallengesIssued    uint64 `json:"challenges_issued"`
	ChallengesAccepted  uint64 `json:"challenges_accepted"`
	ChallengesRejected  uint64 `json:"challenges_rejected"`
	ChallengeReplays    uint64 `json:"challenge_replays"`
	AllowlistedRequests uint64 `json:"allowlisted_requests"`
}

type challengeClaims struct {
	ID         string `json:"jti"`
	Site       string `json:"site"`
	Event      string `json:"event"`
	DeviceHash string `json:"device"`
	ExpiresAt  int64  `json:"exp"`
	Difficulty uint8  `json:"difficulty"`
}

type challengeResult uint8

const (
	challengeValid challengeResult = iota
	challengeMissing
	challengeInvalid
	challengeExpired
	challengeReplay
	challengeStateFull
)

func DefaultSecurityConfig() (SecurityConfig, error) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return SecurityConfig{}, fmt.Errorf("generate participation security secret: %w", err)
	}
	return SecurityConfig{
		RateWindow:           time.Minute,
		IPLimit:              120,
		EventLimit:           1200,
		DeviceLimit:          20,
		ChallengeTTL:         5 * time.Minute,
		ChallengeDifficulty:  14,
		ChallengeSecret:      secret,
		DeviceCookieSecure:   os.Getenv("APP_ENV") != "development" && os.Getenv("APP_ENV") != "test",
		DeviceCookieSameSite: http.SameSiteLaxMode,
	}, nil
}

func SecurityConfigFromEnv() (SecurityConfig, error) {
	config, err := DefaultSecurityConfig()
	if err != nil {
		return SecurityConfig{}, err
	}
	if config.RateWindow, err = envDuration("PARTICIPATION_RATE_LIMIT_WINDOW", config.RateWindow); err != nil {
		return SecurityConfig{}, err
	}
	if config.IPLimit, err = envPositiveInt("PARTICIPATION_RATE_LIMIT_IP", config.IPLimit); err != nil {
		return SecurityConfig{}, err
	}
	if config.EventLimit, err = envPositiveInt("PARTICIPATION_RATE_LIMIT_EVENT", config.EventLimit); err != nil {
		return SecurityConfig{}, err
	}
	if config.DeviceLimit, err = envPositiveInt("PARTICIPATION_RATE_LIMIT_DEVICE", config.DeviceLimit); err != nil {
		return SecurityConfig{}, err
	}
	if config.ChallengeTTL, err = envDuration("PARTICIPATION_CHALLENGE_TTL", config.ChallengeTTL); err != nil {
		return SecurityConfig{}, err
	}
	difficulty, err := envPositiveInt("PARTICIPATION_CHALLENGE_DIFFICULTY", int(config.ChallengeDifficulty))
	if err != nil {
		return SecurityConfig{}, err
	}
	if difficulty > 16 {
		return SecurityConfig{}, errors.New("PARTICIPATION_CHALLENGE_DIFFICULTY must be between 8 and 16")
	}
	config.ChallengeDifficulty = uint8(difficulty)
	if raw := strings.TrimSpace(os.Getenv("PARTICIPATION_CHALLENGE_SECRET")); raw != "" {
		decoded, decodeErr := base64.StdEncoding.DecodeString(raw)
		if decodeErr != nil || len(decoded) < 32 {
			return SecurityConfig{}, errors.New("PARTICIPATION_CHALLENGE_SECRET must be base64 for at least 32 random bytes")
		}
		config.ChallengeSecret = decoded
	}
	if config.Allowlist, err = parsePrefixes(os.Getenv("PARTICIPATION_SECURITY_ALLOWLIST_CIDRS")); err != nil {
		return SecurityConfig{}, fmt.Errorf("PARTICIPATION_SECURITY_ALLOWLIST_CIDRS: %w", err)
	}
	if config.TrustedProxies, err = parsePrefixes(os.Getenv("PARTICIPATION_TRUSTED_PROXY_CIDRS")); err != nil {
		return SecurityConfig{}, fmt.Errorf("PARTICIPATION_TRUSTED_PROXY_CIDRS: %w", err)
	}
	config.MetricsToken = strings.TrimSpace(os.Getenv("PARTICIPATION_SECURITY_METRICS_TOKEN"))
	if config.MetricsToken != "" && len(config.MetricsToken) < 32 {
		return SecurityConfig{}, errors.New("PARTICIPATION_SECURITY_METRICS_TOKEN must contain at least 32 characters")
	}
	if raw := strings.TrimSpace(os.Getenv("PARTICIPATION_DEVICE_COOKIE_SECURE")); raw != "" {
		config.DeviceCookieSecure, err = strconv.ParseBool(raw)
		if err != nil {
			return SecurityConfig{}, fmt.Errorf("PARTICIPATION_DEVICE_COOKIE_SECURE: %w", err)
		}
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("PARTICIPATION_DEVICE_COOKIE_SAMESITE"))) {
	case "", "lax":
		config.DeviceCookieSameSite = http.SameSiteLaxMode
	case "strict":
		config.DeviceCookieSameSite = http.SameSiteStrictMode
	case "none":
		config.DeviceCookieSameSite = http.SameSiteNoneMode
	default:
		return SecurityConfig{}, errors.New("PARTICIPATION_DEVICE_COOKIE_SAMESITE must be lax, strict, or none")
	}
	return config, validateSecurityConfig(config)
}

func NewSecurityState(config SecurityConfig) (*securityState, error) {
	if err := validateSecurityConfig(config); err != nil {
		return nil, err
	}
	secret := append([]byte(nil), config.ChallengeSecret...)
	if len(secret) == 0 {
		secret = make([]byte, 32)
		if _, err := rand.Read(secret); err != nil {
			return nil, fmt.Errorf("generate participation security secret: %w", err)
		}
	}
	config.ChallengeSecret = nil
	return &securityState{
		config:         config,
		secret:         secret,
		rateBuckets:    make(map[string]rateBucket),
		usedChallenges: make(map[string]time.Time),
	}, nil
}

func validateSecurityConfig(config SecurityConfig) error {
	if config.RateWindow < time.Second || config.RateWindow > time.Hour {
		return errors.New("PARTICIPATION_RATE_LIMIT_WINDOW must be between 1s and 1h")
	}
	if config.IPLimit <= 0 || config.EventLimit <= 0 || config.DeviceLimit <= 0 {
		return errors.New("participation rate limits must be positive")
	}
	if config.ChallengeTTL < time.Second || config.ChallengeTTL > 15*time.Minute {
		return errors.New("PARTICIPATION_CHALLENGE_TTL must be between 1s and 15m")
	}
	if config.ChallengeDifficulty < 8 || config.ChallengeDifficulty > 16 {
		return errors.New("PARTICIPATION_CHALLENGE_DIFFICULTY must be between 8 and 16")
	}
	if len(config.ChallengeSecret) != 0 && len(config.ChallengeSecret) < 32 {
		return errors.New("participation challenge secret must contain at least 32 bytes")
	}
	if config.MetricsToken != "" && len(config.MetricsToken) < 32 {
		return errors.New("participation metrics token must contain at least 32 characters")
	}
	if config.DeviceCookieSameSite != http.SameSiteLaxMode && config.DeviceCookieSameSite != http.SameSiteStrictMode && config.DeviceCookieSameSite != http.SameSiteNoneMode {
		return errors.New("participation device cookie SameSite must be lax, strict, or none")
	}
	if config.DeviceCookieSameSite == http.SameSiteNoneMode && !config.DeviceCookieSecure {
		return errors.New("SameSite=None requires a Secure participation device cookie")
	}
	for _, prefix := range append(append([]netip.Prefix(nil), config.Allowlist...), config.TrustedProxies...) {
		if !prefix.IsValid() {
			return errors.New("participation IP prefixes must be valid")
		}
	}
	return nil
}

func envPositiveInt(name string, fallback int) (int, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return value, nil
}

func envDuration(name string, fallback time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback, nil
	}
	value, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be a valid duration: %w", name, err)
	}
	return value, nil
}

func parsePrefixes(raw string) ([]netip.Prefix, error) {
	var prefixes []netip.Prefix
	for _, value := range strings.Split(raw, ",") {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			ip, ipErr := netip.ParseAddr(value)
			if ipErr != nil {
				return nil, fmt.Errorf("invalid IP address or CIDR %q", value)
			}
			if ip.Is4() {
				prefix = netip.PrefixFrom(ip, 32)
			} else {
				prefix = netip.PrefixFrom(ip, 128)
			}
		}
		prefixes = append(prefixes, prefix.Masked())
	}
	return prefixes, nil
}

func (s *securityState) publicIP(c fiber.Ctx) (netip.Addr, error) {
	remote := c.RequestCtx().RemoteAddr()
	if remote == nil {
		return netip.Addr{}, errors.New("request has no remote address")
	}
	host, _, err := net.SplitHostPort(remote.String())
	if err != nil {
		host = strings.Trim(remote.String(), "[]")
	}
	remoteIP, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("parse remote address: %w", err)
	}
	remoteIP = remoteIP.Unmap()
	return s.forwardedClientIP(remoteIP, c.Get("X-Forwarded-For")), nil
}

func (s *securityState) forwardedClientIP(remoteIP netip.Addr, forwarded string) netip.Addr {
	if !containsIP(s.config.TrustedProxies, remoteIP) {
		return remoteIP
	}
	forwarded = strings.TrimSpace(forwarded)
	if forwarded == "" {
		return remoteIP
	}
	parts := strings.Split(forwarded, ",")
	if len(parts) > 16 {
		return remoteIP
	}
	chain := make([]netip.Addr, len(parts))
	for i, part := range parts {
		ip, parseErr := netip.ParseAddr(strings.TrimSpace(part))
		if parseErr != nil {
			return remoteIP
		}
		chain[i] = ip.Unmap()
	}
	client := remoteIP
	for i := len(chain) - 1; i >= 0; i-- {
		if !containsIP(s.config.TrustedProxies, client) {
			break
		}
		client = chain[i]
	}
	return client
}

func containsIP(prefixes []netip.Prefix, ip netip.Addr) bool {
	for _, prefix := range prefixes {
		if prefix.Contains(ip) {
			return true
		}
	}
	return false
}

func (s *securityState) allowlisted(ip netip.Addr) bool {
	return containsIP(s.config.Allowlist, ip)
}

func (s *securityState) allowRequest(ip netip.Addr, site, event, deviceID string, now time.Time) (bool, time.Duration) {
	keys := []struct {
		key   string
		limit int
	}{
		{key: "ip:" + s.digest("ip:"+ip.String()), limit: s.config.IPLimit},
		{key: "event:" + s.digest("event:"+site+"\x00"+event), limit: s.config.EventLimit},
	}
	if deviceID != "" {
		keys = append(keys, struct {
			key   string
			limit int
		}{key: "device:" + s.digest("device:"+deviceID), limit: s.config.DeviceLimit})
	}

	windowStart := now.Truncate(s.config.RateWindow)
	s.rateMu.Lock()
	defer s.rateMu.Unlock()
	s.rateOps++
	if s.rateOps%256 == 0 {
		s.pruneRateBuckets(now)
	}
	var retry time.Duration
	missing := 0
	for _, item := range keys {
		bucket, exists := s.rateBuckets[item.key]
		if !exists {
			missing++
			continue
		}
		if !bucket.window.Equal(windowStart) {
			continue
		}
		if bucket.count >= item.limit {
			wait := windowStart.Add(s.config.RateWindow).Sub(now)
			if wait > retry {
				retry = wait
			}
		}
	}
	if retry > 0 {
		return false, retry
	}
	if len(s.rateBuckets)+missing > maxRateBuckets {
		s.pruneRateBuckets(now)
		if len(s.rateBuckets)+missing > maxRateBuckets {
			return false, s.config.RateWindow - now.Sub(windowStart)
		}
	}
	for _, item := range keys {
		bucket := s.rateBuckets[item.key]
		if !bucket.window.Equal(windowStart) {
			bucket = rateBucket{window: windowStart}
		}
		bucket.count++
		s.rateBuckets[item.key] = bucket
	}
	return true, 0
}

func (s *securityState) pruneRateBuckets(now time.Time) {
	for key, bucket := range s.rateBuckets {
		if !now.Before(bucket.window.Add(s.config.RateWindow)) {
			delete(s.rateBuckets, key)
		}
	}
}

func (s *securityState) issueChallenge(site, event, deviceID string, now time.Time) (string, time.Time, error) {
	jti, err := randomURLToken(24)
	if err != nil {
		return "", time.Time{}, err
	}
	expiresAt := now.Add(s.config.ChallengeTTL).Truncate(time.Second)
	claims := challengeClaims{
		ID:         jti,
		Site:       site,
		Event:      event,
		DeviceHash: s.digest("binding:" + deviceID),
		ExpiresAt:  expiresAt.Unix(),
		Difficulty: s.config.ChallengeDifficulty,
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", time.Time{}, err
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	return encoded + "." + s.sign("challenge:"+encoded), expiresAt, nil
}

func (s *securityState) verifyChallenge(token, proof, site, event, deviceID string, now time.Time) challengeResult {
	if token == "" || len(token) > 2048 || proof == "" {
		return challengeMissing
	}
	parts := strings.Split(token, ".")
	if len(parts) != 2 || parts[0] == "" || !hmac.Equal([]byte(parts[1]), []byte(s.sign("challenge:"+parts[0]))) {
		return challengeInvalid
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return challengeInvalid
	}
	var claims challengeClaims
	if err = json.Unmarshal(payload, &claims); err != nil || claims.ID == "" || claims.Difficulty < 8 || claims.Difficulty > 16 {
		return challengeInvalid
	}
	expiresAt := time.Unix(claims.ExpiresAt, 0)
	if !now.Before(expiresAt) {
		return challengeExpired
	}
	if claims.Site != site || claims.Event != event || !hmac.Equal([]byte(claims.DeviceHash), []byte(s.digest("binding:"+deviceID))) {
		return challengeInvalid
	}
	if proof == "" || len(proof) > 10 {
		return challengeInvalid
	}
	counter, err := strconv.ParseUint(proof, 10, 32)
	if err != nil || !hasLeadingZeroBits(sha256.Sum256([]byte(token+":"+strconv.FormatUint(counter, 10))), int(claims.Difficulty)) {
		return challengeInvalid
	}

	s.challengeMu.Lock()
	defer s.challengeMu.Unlock()
	for id, expiry := range s.usedChallenges {
		if !now.Before(expiry) {
			delete(s.usedChallenges, id)
		}
	}
	if _, exists := s.usedChallenges[claims.ID]; exists {
		s.metrics.challengeReplays.Add(1)
		return challengeReplay
	}
	if len(s.usedChallenges) >= maxUsedChallenges {
		return challengeStateFull
	}
	s.usedChallenges[claims.ID] = expiresAt
	s.metrics.challengesAccepted.Add(1)
	return challengeValid
}

func hasLeadingZeroBits(digest [sha256.Size]byte, bits int) bool {
	wholeBytes := bits / 8
	for i := 0; i < wholeBytes; i++ {
		if digest[i] != 0 {
			return false
		}
	}
	remaining := bits % 8
	return remaining == 0 || digest[wholeBytes]>>(8-remaining) == 0
}

func (s *securityState) ensureDeviceCookie(c fiber.Ctx, now time.Time) (string, error) {
	if deviceID, ok := s.readDeviceCookie(c.Get("Cookie")); ok {
		return deviceID, nil
	}
	deviceID, err := randomURLToken(32)
	if err != nil {
		return "", err
	}
	value := s.makeDeviceCookie(deviceID)
	cookie := (&http.Cookie{
		Name:     deviceCookieName,
		Value:    value,
		Path:     "/participation/public/v2",
		Expires:  now.Add(24 * time.Hour),
		MaxAge:   86400,
		HttpOnly: true,
		Secure:   s.config.DeviceCookieSecure,
		SameSite: s.config.DeviceCookieSameSite,
	}).String()
	c.Set("Set-Cookie", cookie)
	return deviceID, nil
}

func (s *securityState) readDeviceCookie(header string) (string, bool) {
	for _, part := range strings.Split(header, ";") {
		name, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok || name != deviceCookieName {
			continue
		}
		components := strings.Split(value, ".")
		if len(components) != 3 || components[0] != "v1" {
			return "", false
		}
		idBytes, err := base64.RawURLEncoding.DecodeString(components[1])
		if err != nil || len(idBytes) != 32 {
			return "", false
		}
		deviceID := components[1]
		expected := s.sign("device:" + deviceID)
		if !hmac.Equal([]byte(components[2]), []byte(expected)) {
			return "", false
		}
		return deviceID, true
	}
	return "", false
}

func (s *securityState) makeDeviceCookie(deviceID string) string {
	return "v1." + deviceID + "." + s.sign("device:"+deviceID)
}

func (s *securityState) sign(message string) string {
	mac := hmac.New(sha256.New, s.secret)
	_, _ = mac.Write([]byte(message))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (s *securityState) digest(message string) string {
	return s.sign(message)
}

func randomURLToken(size int) (string, error) {
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func (s *securityState) metricsSnapshot() securityMetrics {
	return securityMetrics{
		PublicRequests:      s.metrics.publicRequests.Load(),
		RateLimited:         s.metrics.rateLimited.Load(),
		ChallengesIssued:    s.metrics.challengesIssued.Load(),
		ChallengesAccepted:  s.metrics.challengesAccepted.Load(),
		ChallengesRejected:  s.metrics.challengesRejected.Load(),
		ChallengeReplays:    s.metrics.challengeReplays.Load(),
		AllowlistedRequests: s.metrics.allowlistedRequests.Load(),
	}
}

func retryAfterSeconds(wait time.Duration) int {
	seconds := int(math.Ceil(wait.Seconds()))
	if seconds < 1 {
		return 1
	}
	return seconds
}

func bearerMatches(header, token string) bool {
	if token == "" || !strings.HasPrefix(header, "Bearer ") {
		return false
	}
	provided := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
	return len(provided) == len(token) && subtleCompare([]byte(provided), []byte(token))
}

func subtleCompare(left, right []byte) bool {
	return hmac.Equal(left, right)
}
