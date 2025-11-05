package handler

import (
	"dingoscheduler/internal/model/query"
	"dingoscheduler/internal/service"
	myerr "dingoscheduler/pkg/error"
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
		if e, ok := err.(myerr.Error); ok {
			return util.ErrorEntryUnknown(c, e.StatusCode(), e.Error())
		}
		return util.ResponseError(c)
	}
	return util.NormalResponseData(c, nil)
}
