package domain

import "time"

type User struct {
	ID           string
	Email        string
	Name         string
	PasswordHash string
	Active       bool
	CreatedAt    time.Time
}

type Session struct {
	ID           string
	UserID       string
	AccessHash   []byte
	RefreshHash  []byte
	ExpiresAt    time.Time
	RefreshUntil time.Time
	RevokedAt    *time.Time
}

type Claims struct {
	Issuer    string   `json:"iss"`
	Audience  string   `json:"aud"`
	Subject   string   `json:"sub"`
	SessionID string   `json:"sid"`
	TenantID  string   `json:"tenant_id,omitempty"`
	TenantIDs []string `json:"tenant_ids,omitempty"`
	Roles     []string `json:"roles,omitempty"`
	Scopes    []string `json:"scopes,omitempty"`
	IssuedAt  int64    `json:"iat"`
	Expires   int64    `json:"exp"`
	JWTID     string   `json:"jti"`
}
