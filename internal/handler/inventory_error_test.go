package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"dingoscheduler/pkg/inventory"
	driver "github.com/go-sql-driver/mysql"
	"github.com/labstack/echo/v4"
)

func TestInventoryErrorsDistinguishRejectionTemporaryAndUnknown(t *testing.T) {
	for _, tc := range []struct {
		name      string
		err       error
		status    int
		retryable any
	}{
		{"invalid", inventory.Reject("invalid_inventory_report", "invalid file"), 400, false},
		{"identity", inventory.Reject("inventory_identity_conflict", "size conflict"), 409, false},
		{"duplicate", fmt.Errorf("wrapped: %w", &driver.MySQLError{Number: 1062}), 409, false},
		{"deadlock", fmt.Errorf("wrapped: %w", &driver.MySQLError{Number: 1213}), 503, true},
		{"lock_timeout", &driver.MySQLError{Number: 1205}, 503, true},
		{"timeout", context.DeadlineExceeded, 503, true},
		{"unknown", fmt.Errorf("storage failure"), 500, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c := echo.New().NewContext(httptest.NewRequest("POST", "/", nil), rec)
			if err := writeInventoryError(c, tc.err); err != nil {
				t.Fatal(err)
			}
			var body map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if rec.Code != tc.status || body["retryable"] != tc.retryable || body["error"] == nil || body["code"] == nil {
				t.Fatalf("%d %s", rec.Code, rec.Body.String())
			}
		})
	}
}
