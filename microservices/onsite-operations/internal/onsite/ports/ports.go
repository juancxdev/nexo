package ports

import (
	"context"
	"nexo-onsite-operations/internal/onsite/domain"
	"time"
)

type Claims struct {
	ActorID, TenantID, Role, DeviceID, TraceID string
	Scopes                                     []string
}
type AttendancePage struct {
	Items      []domain.AttendanceFact
	NextCursor *string
}
type Event struct {
	ID                string    `json:"id"`
	Name              string    `json:"name"`
	Status            string    `json:"status"`
	StartsAt          time.Time `json:"starts_at"`
	EndsAt            time.Time `json:"ends_at"`
	Timezone          string    `json:"timezone"`
	PublishedRevision int       `json:"published_revision"`
}
type RosterEntry struct {
	RegistrationID string     `json:"registration_id"`
	TicketID       string     `json:"ticket_id"`
	DisplayName    string     `json:"display_name"`
	State          string     `json:"state"`
	CheckedInAt    *time.Time `json:"checked_in_at"`
	CheckedOutAt   *time.Time `json:"checked_out_at"`
}
type Clock interface{ Now() time.Time }
type TicketRepository interface {
	Search(context.Context, string, string, string, string) ([]domain.Ticket, error)
}
type AttendanceRepository interface {
	CheckIn(context.Context, domain.AttendanceFact, string, Claims, string) (domain.AttendanceFact, error)
	CheckOut(context.Context, domain.AttendanceFact, string, Claims, string) (domain.AttendanceFact, error)
	Void(context.Context, string, string, string, Claims, string) (domain.AttendanceFact, error)
	List(context.Context, string, string, string, string, int) (AttendancePage, error)
	Dashboard(context.Context, string, string) (map[string]any, error)
	Events(context.Context, string) ([]Event, error)
	Roster(context.Context, string, string, string) ([]RosterEntry, error)
}
type DeviceRepository interface {
	ListDevices(context.Context, string) ([]domain.Device, error)
	CreateDevice(context.Context, domain.Device) (domain.Device, error)
	UpdateDevice(context.Context, domain.Device) (domain.Device, error)
	RevokeDevice(context.Context, string, string, string) error
	CreateShift(context.Context, domain.Shift, Claims) (domain.Shift, error)
	CloseShift(context.Context, string, Claims) error
}
