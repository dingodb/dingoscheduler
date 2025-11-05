package handler

import (
	"dingoscheduler/internal/model/query"
	"dingoscheduler/internal/service"
	"dingoscheduler/pkg/util"

	"github.com/labstack/echo/v4"
)

type ManagerHandler struct {
	schedulerService  *service.SchedulerService
	repositoryService *service.RepositoryService
}

func NewManagerHandler(schedulerService *service.SchedulerService, repositoryService *service.RepositoryService) *ManagerHandler {
	return &ManagerHandler{
		schedulerService:  schedulerService,
		repositoryService: repositoryService,
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
