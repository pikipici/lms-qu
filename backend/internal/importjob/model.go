// Package importjob holds the bulk-import job domain (CSV-driven user creation).
//
// The directory is named importjob (not import) because import is a Go
// reserved keyword and cannot be used as a package name.
package importjob

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

// Status is the ImportJob lifecycle marker (locked decision #54).
type Status string

const (
	// StatusPreview means CSV parsed, preview rows ready, awaiting admin confirm.
	StatusPreview Status = "preview"
	// StatusProcessing means admin confirmed; user inserts in flight.
	StatusProcessing Status = "processing"
	// StatusCompleted means all users inserted; credentials.csv ready.
	StatusCompleted Status = "completed"
	// StatusExpired means preview window elapsed without confirmation.
	StatusExpired Status = "expired"
	// StatusCancelled means admin actively cancelled before confirm (Task 2.D.3).
	StatusCancelled Status = "cancelled"
	// StatusFailed means processing aborted with errors_json populated.
	StatusFailed Status = "failed"
)

// ImportJob tracks a CSV bulk-import lifecycle (#54).
//
// PreviewRowsJSON / ErrorsJSON are JSONB blobs. CredentialsCSV holds the R2
// object key for the generated credentials.csv (Task 2.D.5); ObjectKey holds
// the R2 object key for the uploaded raw CSV (Task 2.D.2).
type ImportJob struct {
	ID              uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	AdminID         *uuid.UUID     `gorm:"type:uuid" json:"admin_id,omitempty"`
	Filename        string         `gorm:"not null" json:"filename"`
	ObjectKey       *string        `gorm:"column:object_key" json:"object_key,omitempty"`
	Status          Status         `gorm:"not null" json:"status"`
	TotalRows       int            `gorm:"not null;default:0" json:"total_rows"`
	ValidCount      int            `gorm:"not null;default:0" json:"valid_count"`
	InvalidCount    int            `gorm:"not null;default:0" json:"invalid_count"`
	SuccessCount    int            `gorm:"not null;default:0" json:"success_count"`
	FailCount       int            `gorm:"not null;default:0" json:"fail_count"`
	PreviewRowsJSON datatypes.JSON `gorm:"type:jsonb;column:preview_rows_json" json:"preview_rows,omitempty"`
	ErrorsJSON      datatypes.JSON `gorm:"type:jsonb;column:errors_json" json:"errors,omitempty"`
	CredentialsCSV  *string        `gorm:"column:credentials_csv" json:"credentials_csv,omitempty"`
	ExpiresAt       time.Time      `gorm:"not null" json:"expires_at"`
	CreatedAt       time.Time      `json:"created_at"`
	ConfirmedAt     *time.Time     `json:"confirmed_at,omitempty"`
	CompletedAt     *time.Time     `json:"completed_at,omitempty"`
}

// TableName binds the struct to the import_jobs table.
func (ImportJob) TableName() string {
	return "import_jobs"
}
