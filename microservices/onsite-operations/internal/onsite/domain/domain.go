package domain

import (
	"errors"
	"strings"
	"time"
)

var (
	ErrAlreadyPresent         = errors.New("already_present")
	ErrIdempotencyKeyRequired = errors.New("idempotency_key_required")
	ErrIdempotencyKeyReused   = errors.New("idempotency_key_reused")
	ErrNotPresent             = errors.New("not_present")
	ErrInvalidState           = errors.New("invalid_state_transition")
	ErrDeviceRevoked          = errors.New("device_revoked")
	ErrAlreadyVoided          = errors.New("already_voided")
	ErrReasonRequired         = errors.New("reason_required")
)

// MinReasonLength es el largo mínimo de un motivo de anulación o revocación.
const MinReasonLength = 5

// ValidReason exige un motivo con contenido real (no "x" ni espacios).
func ValidReason(reason string) bool {
	return len([]rune(strings.TrimSpace(reason))) >= MinReasonLength
}

// TimelineEntry es un hecho de entrada o salida con su estado de anulación.
type TimelineEntry struct {
	FactID, Type string
	OccurredAt   time.Time
	Voided       bool
}

// EffectivePresence recorre la historia del ticket ignorando lo anulado: el último
// hecho vigente decide. Si es una entrada, devuelve su ID (la persona está presente).
// Ejemplo: entrada, salida y se anula la salida => vuelve a estar presente.
func EffectivePresence(entries []TimelineEntry) (string, time.Time) {
	var last *TimelineEntry
	for i := range entries {
		e := entries[i]
		if e.Voided || (e.Type != "check_in" && e.Type != "check_out") {
			continue
		}
		if last == nil || !e.OccurredAt.Before(last.OccurredAt) {
			last = &entries[i]
		}
	}
	if last == nil || last.Type != "check_in" {
		return "", time.Time{}
	}
	return last.FactID, last.OccurredAt
}

type Ticket struct {
	ID, EventID, Scope, Status, MaskedHolder, SearchCode string
	ExpiresAt                                            time.Time
}

type AttendanceFact struct {
	ID, TicketID, EventID, Scope, Type, ActorID, DeviceID, Reason, CompensatesFactID string
	OccurredAt                                                                       time.Time
	EffectiveStatus                                                                  string
}

type Device struct {
	ID, TenantID, Label, Status, VenueID, PublicKey, ReleaseVersion string
	LastSeenAt, RevokedAt                                           *time.Time
}
type Shift struct {
	ID, EventID, Scope, DeviceID, ActorID, Status string
	OpenedAt, ClosedAt                            *time.Time
}

func (t Ticket) Validate(event, scope string, now time.Time) string {
	if t.EventID != event {
		return "wrong_event"
	}
	if t.Scope != scope {
		return "wrong_scope"
	}
	if t.Status == "revoked" {
		return "ticket_revoked"
	}
	if t.Status == "expired" || (!t.ExpiresAt.IsZero() && !now.Before(t.ExpiresAt)) {
		return "ticket_expired"
	}
	if t.Status != "active" {
		return "ticket_invalid"
	}
	return ""
}

func ValidQuery(q string) bool { return len([]rune(strings.TrimSpace(q))) >= 4 }
