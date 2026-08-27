package trial

import "fmt"

// ComponentType classifies a physical component bound to an install position.
type ComponentType string

const (
	ComponentHousing           ComponentType = "housing"
	ComponentEndCap            ComponentType = "end_cap"
	ComponentSealRing          ComponentType = "seal_ring"
	ComponentFastener          ComponentType = "fastener"
	ComponentPressureSensor    ComponentType = "pressure_sensor"
	ComponentTemperatureSensor ComponentType = "temperature_sensor"
	ComponentPump              ComponentType = "pump"
	ComponentValve             ComponentType = "valve"
)

// Binding maps a component serial number to a single install position for a
// specific trial round. A serial number and a position may each hold at most
// one effective binding at any instant.
type Binding struct {
	TrialID  string        `json:"trial_id"`
	Round    int           `json:"round"`
	Serial   string        `json:"serial"`
	Type     ComponentType `json:"type"`
	Position string        `json:"position"`
}

// Lease grants a trial round exclusive use of a resource until a logical
// expiry instant. Submitting evidence re-validates the lease owner, round, and
// validity window.
type Lease struct {
	TrialID    string `json:"trial_id"`
	Round      int    `json:"round"`
	ResourceID string `json:"resource_id"`
	Holder     string `json:"holder"`
	Token      string `json:"token"`
	ExpiresAt  int64  `json:"expires_at"`
	Active     bool   `json:"active"`
}

// Expired reports whether the lease is no longer valid at the given logical
// instant. A lease is valid strictly before its expiry; at the expiry instant
// itself it is already expired.
func (l Lease) Expired(nowMs int64) bool { return nowMs >= l.ExpiresAt }

// String renders a stable lease identity for error reporting.
func (l Lease) String() string {
	return fmt.Sprintf("%s/%s/%s", l.TrialID, l.ResourceID, l.Token)
}
