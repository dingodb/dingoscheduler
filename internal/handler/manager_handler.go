package handler

import (
	"dingoscheduler/internal/model/query"
	"dingoscheduler/internal/service"
	"dingoscheduler/pkg/util"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

type ManagerHandler struct {
	schedulerService  *service.SchedulerService
	repositoryService *service.RepositoryService
	hfTokenService    *service.HfTokenService
	managerService    *service.ManagerService
}

func NewManagerHandler(schedulerService *service.SchedulerService, repositoryService *service.RepositoryService,
	hfTokenService *service.HfTokenService, managerService *service.ManagerService) *ManagerHandler {
	return &ManagerHandler{
		schedulerService:  schedulerService,
		repositoryService: repositoryService,
		hfTokenService:    hfTokenService,
		managerService:    managerService,
	}
}

func (handler *ManagerHandler) PersistRepoHandler(c echo.Context) error {
	job := new(query.PersistRepoReq)
	if err := c.Bind(job); err != nil {
		return util.ErrorRequestParamCN(c)
	}
	err := handler.repositoryService.PersistRepo(job)
	if err != nil {
		return util.ResponseError(c, err)
	}
	return util.NormalResponseData(c, nil)
}

func (handler *ManagerHandler) RefreshToken(c echo.Context) error {
	return util.NormalResponseData(c, handler.hfTokenService.RefreshToken())
}

func (handler *ManagerHandler) ExecWaitTaskHandler(c echo.Context) error {
	waitTaskReq := new(query.WaitTaskReq)
	if err := c.Bind(waitTaskReq); err != nil {
		return util.ErrorRequestParamCN(c)
	}
	if waitTaskReq.Limit == 0 {
		waitTaskReq.Limit = 30
	}
	err := handler.managerService.ExecWaitTask(waitTaskReq)
	if err != nil {
		zap.S().Errorf("GetRepositoryById err.%v", err)
		return util.ResponseError(c)
	}
	return util.NormalResponseData(c, nil)
}
