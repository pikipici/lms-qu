package submission

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/pikip/lms/backend/internal/middleware"
)

func TestNewHandler(t *testing.T) {
	svc := &Service{}
	h := NewHandler(svc)
	if h == nil || h.svc != svc {
		t.Fatalf("NewHandler did not retain service")
	}
}

func TestMapServiceErr(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"not found", ErrNotFound, fiber.StatusNotFound, "not_found"},
		{"forbidden", ErrForbidden, fiber.StatusForbidden, "forbidden"},
		{"draft as not found", ErrTugasNotPublished, fiber.StatusNotFound, "not_found"},
		{"deadline", ErrDeadlinePassed, fiber.StatusForbidden, "deadline_passed"},
		{"already graded", ErrAlreadyGraded, fiber.StatusConflict, "already_graded"},
		{"attachment required", ErrAttachmentRequired, fiber.StatusBadRequest, "attachment_required"},
		{"attachment limit", ErrAttachmentLimit, fiber.StatusBadRequest, "attachment_limit_reached"},
		{"too large", ErrAttachmentTooLarge, fiber.StatusRequestEntityTooLarge, "payload_too_large"},
		{"unsupported mime", ErrUnsupportedMime, fiber.StatusUnsupportedMediaType, "unsupported_mime"},
		{"r2 required", ErrR2Required, fiber.StatusServiceUnavailable, "r2_unavailable"},
		{"upload failed", ErrAttachmentUploadFailed, fiber.StatusInternalServerError, "r2_put_failed"},
		{"invalid input", ErrInvalidInput, fiber.StatusBadRequest, "invalid_input"},
		{"version conflict", ErrVersionConflict, fiber.StatusConflict, "version_conflict"},
		{"internal", errors.New("boom"), fiber.StatusInternalServerError, "internal"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := fiber.New()
			app.Use(middleware.RequestID())
			app.Get("/", func(c *fiber.Ctx) error { return mapServiceErr(c, tt.err) })

			resp, err := app.Test(httptest.NewRequest("GET", "/", nil))
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != tt.status {
				t.Fatalf("status = %d, want %d", resp.StatusCode, tt.status)
			}
			var body map[string]any
			if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if body["code"] != tt.code || body["request_id"] == "" {
				t.Fatalf("error body = %+v", body)
			}
		})
	}
}
