package handler

import (
	"dingoscheduler/internal/model"
	"dingoscheduler/internal/service"
	"dingoscheduler/pkg/app"
	"dingoscheduler/pkg/util"

	"github.com/labstack/echo/v4"
)

type SysHandler struct {
	sysService *service.SysService
}

func NewSysHandler(sysService *service.SysService) *SysHandler {
	return &SysHandler{
		sysService: sysService,
	}
}

func (s *SysHandler) Info(c echo.Context) error {
	info := &model.SystemInfo{}
	if appInfo, ok := app.FromContext(c.Request().Context()); ok {
		info.Id = appInfo.ID()
		info.Name = appInfo.Name()
		info.Version = appInfo.Version()
		info.StartTime = appInfo.StartTime()
	}
	return util.NormalResponseData(c, info)
}
