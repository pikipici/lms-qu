package audit

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

// Entry is the response shape per audit row, enriched dgn ActorName.
type Entry struct {
	ID            uuid.UUID      `json:"id"`
	ActorID       *uuid.UUID     `json:"actor_id,omitempty"`
	ActorName     *string        `json:"actor_name,omitempty"`
	ActorRole     *string        `json:"actor_role,omitempty"`
	Action        string         `json:"action"`
	TargetType    *string        `json:"target_type,omitempty"`
	TargetID      *uuid.UUID     `json:"target_id,omitempty"`
	TargetKelasID *uuid.UUID     `json:"target_kelas_id,omitempty"`
	Meta          datatypes.JSON `json:"meta,omitempty"`
	At            time.Time      `json:"at"`
}

// ListResponse is the response shape of GET /guru/kelas/:id/audit.
type ListResponse struct {
	Events []Entry `json:"events"`
	Total  int64   `json:"total"`
	Limit  int     `json:"limit"`
	Offset int     `json:"offset"`
}

// ActionsResponse describes the allowlisted action filter values for
// the FE dropdown. Returned by GET /guru/audit-actions (helper).
type ActionsResponse struct {
	Actions []string `json:"actions"`
}
