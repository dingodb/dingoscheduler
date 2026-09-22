package handler

import (
	"context"
	"dingoscheduler/internal/dao"
	"dingoscheduler/internal/model"
	_ "embed"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

//go:embed node_health.html
var nodeHealthPage string

// The page fetches node data from the health API.
func NodeHealthPage(c echo.Context) error {
	c.Response().Header().Set("Cache-Control", "no-store")
	return c.HTML(http.StatusOK, nodeHealthPage)
}

func (h *ManagerHandler) NodeHealth(c echo.Context) error {
	after, err := strconv.ParseInt(defaultQuery(c, "after", "0"), 10, 32)
	if err != nil || after < 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid after")
	}
	limit, err := strconv.Atoi(defaultQuery(c, "limit", "100"))
	if err != nil || limit < 1 || limit > 200 {
		return echo.NewHTTPError(http.StatusBadRequest, "limit must be 1..200")
	}
	items, err := h.schedulerService.NodeHealth(c.Request().Context(), int32(after), limit)
	if err != nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "node status unavailable")
	}
	var next int32
	if len(items) == limit {
		next = items[len(items)-1].ID
	}
	// Endpoints are optional for old clients; request explicitly for discovery.
	var output any = items
	if c.QueryParam("endpoints") == "true" {
		ids := make([]int32, 0, len(items))
		for _, item := range items {
			ids = append(ids, item.ID)
		}
		endpoints, err := h.schedulerService.NodeEndpoints(c.Request().Context(), ids)
		if err != nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "node endpoints unavailable")
		}
		type discovered struct {
			dao.NodeHealthView
			model.NodeEndpoint
		}
		rows := make([]discovered, 0, len(items))
		for _, item := range items {
			rows = append(rows, discovered{item, endpoints[item.ID]})
		}
		output = rows
	}
	c.Response().Header().Set("Cache-Control", "no-store")
	return c.JSON(http.StatusOK, struct {
		Items     any   `json:"items"`
		NextAfter int32 `json:"nextAfter"`
	}{output, next})
}

func defaultQuery(c echo.Context, key, fallback string) string {
	if value := c.QueryParam(key); value != "" {
		return value
	}
	return fallback
}

func (h *ManagerHandler) NodeRepositories(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 32)
	if err != nil || id < 1 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid node id")
	}
	after, err := strconv.ParseInt(defaultQuery(c, "after", "0"), 10, 64)
	if err != nil || after < 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid after")
	}
	limit, err := strconv.Atoi(defaultQuery(c, "limit", "20"))
	if err != nil || limit < 1 || limit > 100 {
		return echo.NewHTTPError(http.StatusBadRequest, "limit must be 1..100")
	}
	ctx, cancel := context.WithTimeout(c.Request().Context(), 5*time.Second)
	defer cancel()
	page, err := h.schedulerService.NodeRepositories(ctx, int32(id), after, limit)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return echo.NewHTTPError(http.StatusNotFound, "node not found")
	}
	if err != nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "repository records unavailable")
	}
	c.Response().Header().Set("Cache-Control", "no-store")
	return c.JSON(http.StatusOK, page)
}
