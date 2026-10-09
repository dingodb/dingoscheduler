package handler

import (
	"strconv"

	"dingoscheduler/internal/dao"
	"dingoscheduler/pkg/repository"
	"github.com/labstack/echo/v4"
)

// Sync file locations use the existing Scheduler API boundary. Only registered
// endpoints are returned; request input cannot supply a source URL.
func (h *ManagerHandler) UploadedSyncSources(c echo.Context) error {
	for _, values := range c.QueryParams() {
		if len(values) != 1 {
			return c.JSON(400, map[string]string{"message": "duplicate query parameter"})
		}
	}
	size, err := strconv.ParseInt(c.QueryParam("size"), 10, 64)
	if err != nil {
		return c.JSON(400, map[string]string{"message": "size is required"})
	}
	q := dao.SyncSourceQuery{Key: repository.Key{Namespace: c.QueryParam("namespace"), RepoType: c.QueryParam("repoType"), Repo: c.QueryParam("repo")}, Path: c.QueryParam("path"), SHA256: c.QueryParam("sha256"), Size: size, ExcludeInstanceID: c.QueryParam("excludeInstanceId")}
	if err := q.Validate(); err != nil {
		return c.JSON(400, map[string]string{"message": err.Error()})
	}
	items, err := h.schedulerService.UploadedSyncSources(c.Request().Context(), q)
	if err != nil {
		return c.JSON(503, map[string]string{"message": "file source lookup unavailable"})
	}
	c.Response().Header().Set("Cache-Control", "no-store")
	return c.JSON(200, struct {
		Items []dao.SyncSource `json:"items"`
	}{items})
}
