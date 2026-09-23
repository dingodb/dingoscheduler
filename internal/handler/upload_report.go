package handler

import (
	"dingoscheduler/pkg/inventory"
	"encoding/json"
	"github.com/labstack/echo/v4"
	"io"
	"net/http"
)

func (h *ManagerHandler) UploadReport(c echo.Context) error {
	var p inventory.Report
	d := json.NewDecoder(http.MaxBytesReader(c.Response(), c.Request().Body, 128<<20))
	d.DisallowUnknownFields()
	if err := d.Decode(&p); err != nil {
		return echo.NewHTTPError(400, err.Error())
	}
	if d.Decode(&struct{}{}) != io.EOF {
		return echo.NewHTTPError(400, "expected one report")
	}
	a, err := h.schedulerService.UploadReport(c.Request().Context(), &p)
	if err != nil {
		return c.JSON(409, map[string]string{"error": err.Error()})
	}
	return c.JSON(200, a)
}
func (h *ManagerHandler) UploadReportSession(c echo.Context) error {
	n, err := h.schedulerService.UploadReportSession(c.Request().Context(), c.Param("instanceId"))
	if err != nil {
		return c.JSON(409, map[string]string{"error": err.Error()})
	}
	return c.JSON(200, n)
}

func (h *ManagerHandler) UploadReconcileProgress(c echo.Context) error {
	var p struct {
		Epoch  string `json:"epoch"`
		Status string `json:"status"`
		Error  string `json:"error"`
	}
	d := json.NewDecoder(http.MaxBytesReader(c.Response(), c.Request().Body, 8192))
	if err := d.Decode(&p); err != nil {
		return echo.NewHTTPError(400, err.Error())
	}
	if err := h.schedulerService.UploadReconcileProgress(c.Request().Context(), c.Param("instanceId"), p.Epoch, p.Status, p.Error); err != nil {
		return echo.NewHTTPError(400, err.Error())
	}
	return c.JSON(200, map[string]bool{"ok": true})
}
func (h *ManagerHandler) UploadReconcile(c echo.Context) error {
	if c.Request().Method == http.MethodGet {
		n, err := h.schedulerService.UploadReportStatus(c.Request().Context(), c.Param("instanceId"))
		if err != nil {
			return c.JSON(500, map[string]string{"error": err.Error()})
		}
		return c.JSON(200, n)
	}
	n, err := h.schedulerService.ReconcileUploadInventory(c.Request().Context(), c.Param("instanceId"))
	if err != nil {
		return c.JSON(409, map[string]string{"error": err.Error()})
	}
	return c.JSON(202, n)
}
