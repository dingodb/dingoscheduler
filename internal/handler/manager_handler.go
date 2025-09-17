package handler

import (
	"net/http"

	"dingoscheduler/internal/model/query"
	"dingoscheduler/internal/service"
	"dingoscheduler/pkg/consts"
	myerr "dingoscheduler/pkg/error"
	"dingoscheduler/pkg/util"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

type ManagerHandler struct {
	schedulerService  *service.SchedulerService
	preheatJobService *service.PreheatJobService
	repositoryService *service.RepositoryService
}

func NewManagerHandler(schedulerService *service.SchedulerService, preheatJobService *service.PreheatJobService, repositoryService *service.RepositoryService) *ManagerHandler {
	return &ManagerHandler{
		schedulerService:  schedulerService,
		preheatJobService: preheatJobService,
		repositoryService: repositoryService,
	}
}

func (handler *ManagerHandler) PreheatHandler(c echo.Context) error {
	job := new(query.PreheatJobQuery)
	if err := c.Bind(job); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "无效的 JSON 数据",
		})
	}
	if _, ok := consts.RepoTypesMapping[job.Datatype]; !ok {
		zap.S().Errorf("MetaProxyCommon repoType:%s is not exist RepoTypesMapping", job.Datatype)
		return util.ErrorPageNotFound(c)
	}
	if job.Org == "" && job.Repo == "" {
		zap.S().Errorf("MetaProxyCommon org and repo is null")
		return util.ErrorRepoNotFound(c)
	}
	err := handler.preheatJobService.Preheat(c, job)
	if err != nil {
		if e, ok := err.(myerr.Error); ok {
			return util.ErrorEntryUnknown(c, e.StatusCode(), e.Error())
		}
		return util.ErrorProxyError(c)
	}
	return util.ResponseData(c, nil)
}

func (handler *ManagerHandler) PersistRepoHandler(c echo.Context) error {
	job := new(query.PersistRepoQuery)
	if err := c.Bind(job); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "无效的 JSON 数据",
		})
	}
	err := handler.repositoryService.PersistRepo(c, job)
	if err != nil {
		if e, ok := err.(myerr.Error); ok {
			return util.ErrorEntryUnknown(c, e.StatusCode(), e.Error())
		}
		return util.ResponseError(c)
	}
	return util.ResponseData(c, map[string]string{"data": "操作成功"})
}
