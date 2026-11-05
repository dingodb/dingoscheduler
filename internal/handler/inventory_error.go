package handler

import (
	"context"
	"errors"
	"net"
	"net/http"

	"dingoscheduler/pkg/inventory"
	driver "github.com/go-sql-driver/mysql"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

func writeInventoryError(c echo.Context, err error) error {
	status, code := http.StatusInternalServerError, "inventory_internal_error"
	body := map[string]any{"error": err.Error()}
	var rejection *inventory.Rejection
	var dbErr *driver.MySQLError
	var netErr net.Error
	switch {
	case errors.As(err, &rejection):
		status, code = http.StatusConflict, rejection.Code
		if code == "invalid_inventory_report" {
			status = http.StatusBadRequest
		}
		body["retryable"] = false
	case errors.Is(err, gorm.ErrDuplicatedKey) || (errors.As(err, &dbErr) && dbErr.Number == 1062):
		status, code = http.StatusConflict, "inventory_storage_conflict"
		body["retryable"] = false
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled), errors.As(err, &netErr):
		status, code = http.StatusServiceUnavailable, "inventory_temporarily_unavailable"
		body["retryable"] = true
	case dbErr != nil && (dbErr.Number == 1205 || dbErr.Number == 1213):
		status, code = http.StatusServiceUnavailable, "inventory_database_busy"
		body["retryable"] = true
	}
	// Unclassified server errors omit retryable; clients apply a finite budget.
	body["code"] = code
	return c.JSON(status, body)
}
