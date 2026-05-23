package feed

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/pikip/lms/backend/internal/auth"
	"github.com/pikip/lms/backend/internal/middleware"
)

type fakeFeedService struct {
	res  *ListResponse
	err  error
	seen struct {
		guruID uuid.UUID
		role   string
		cursor string
		limit  int
	}
}

func (f *fakeFeedService) List(ctx context.Context, guruID uuid.UUID, callerRole string, cursor string, limit int) (*ListResponse, error) {
	f.seen.guruID = guruID
	f.seen.role = callerRole
	f.seen.cursor = cursor
	f.seen.limit = limit
	return f.res, f.err
}

func newFeedTestApp(svc serviceAPI, userID uuid.UUID, role string) *fiber.App {
	app := fiber.New()
	app.Use(middleware.RequestID())
	app.Use(func(c *fiber.Ctx) error {
		if userID != uuid.Nil {
			c.Locals(middleware.LocalsUserID, userID)
		}
		if role != "" {
			c.Locals(middleware.LocalsUserRole, role)
		}
		return c.Next()
	})
	app.Get("/feed", (&Handler{svc: svc}).List)
	return app
}

func TestHandlerListSuccess(t *testing.T) {
	userID := uuid.New()
	eventID := uuid.New().String()
	svc := &fakeFeedService{res: &ListResponse{Events: []Event{{ID: eventID, Kind: EventSiswaJoin, At: time.Unix(1, 0).UTC()}}, NextCursor: "next"}}
	app := newFeedTestApp(svc, userID, string(auth.Guru))

	req := httptest.NewRequest("GET", "/feed?cursor=abc&limit=7", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusOK)
	}
	if svc.seen.guruID != userID || svc.seen.role != string(auth.Guru) || svc.seen.cursor != "abc" || svc.seen.limit != 7 {
		t.Fatalf("service args = %+v", svc.seen)
	}

	var body ListResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Events) != 1 || body.Events[0].ID != eventID || body.NextCursor != "next" {
		t.Fatalf("body = %+v", body)
	}
}

func TestHandlerListRejectsInvalidLimit(t *testing.T) {
	app := newFeedTestApp(&fakeFeedService{}, uuid.New(), string(auth.Guru))

	resp, err := app.Test(httptest.NewRequest("GET", "/feed?limit=0", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusBadRequest)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["code"] != "invalid_limit" || body["request_id"] == "" {
		t.Fatalf("error body = %+v", body)
	}
}

func TestHandlerListMapsServiceErrors(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"forbidden", ErrForbidden, fiber.StatusForbidden, "forbidden"},
		{"invalid cursor", ErrInvalidCursor, fiber.StatusBadRequest, "invalid_cursor"},
		{"internal", errors.New("db down"), fiber.StatusInternalServerError, "internal"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := newFeedTestApp(&fakeFeedService{err: tt.err}, uuid.New(), string(auth.Guru))
			resp, err := app.Test(httptest.NewRequest("GET", "/feed", nil))
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

func TestHandlerListRequiresUserContext(t *testing.T) {
	app := newFeedTestApp(&fakeFeedService{}, uuid.Nil, string(auth.Guru))

	resp, err := app.Test(httptest.NewRequest("GET", "/feed", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusInternalServerError)
	}
}
