package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"dingoscheduler/internal/authority"
	"github.com/labstack/echo/v4"
)

// This internal API is intended for a trusted service network.
// Spinfield checks end-user authorization and repository scope on every call.
func (h *ManagerHandler) OfficialRevision(c echo.Context) error {
	var r authority.Request
	dec := json.NewDecoder(http.MaxBytesReader(c.Response(), c.Request().Body, 8<<20))
	if dec.Decode(&r) != nil || dec.Decode(&struct{}{}) != io.EOF {
		return c.JSON(400, map[string]string{"message": "invalid request"})
	}
	if r.Actor == "" {
		return c.JSON(403, map[string]string{"message": "authenticated actor required"})
	}
	result, err := h.schedulerService.OfficialRevision(c.Request().Context(), r)
	if err != nil {
		status := 500
		var e *authority.Failure
		if errors.As(err, &e) {
			status = e.Status
		}
		return c.JSON(status, map[string]string{"message": err.Error()})
	}
	return c.JSON(200, result)
}
