package handler

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"

	"dingoscheduler/internal/authority"
	"github.com/labstack/echo/v4"
)

// Only the authenticated Spinfield business gateway may call this internal API.
// End-user authorization and repository scope are checked there on every call.
func (h *ManagerHandler) OfficialRevision(c echo.Context) error {
	key := os.Getenv("DINGO_AUTHORITY_KEY")
	if key == "" || subtle.ConstantTimeCompare([]byte(c.Request().Header.Get("Authorization")), []byte("Bearer "+key)) != 1 {
		return c.JSON(http.StatusForbidden, map[string]string{"message": "official gateway authentication required"})
	}
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
