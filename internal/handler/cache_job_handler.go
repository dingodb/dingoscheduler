package handler

import (
	"bytes"
	"io"
	"net/http"

	"dingoscheduler/internal/model/dto"
	"dingoscheduler/internal/model/query"
	"dingoscheduler/internal/service"
	"dingoscheduler/pkg/consts"
	"dingoscheduler/pkg/util"

	"github.com/labstack/echo/v4"
	"github.com/young2j/gocopy"
	"go.uber.org/zap"
)

type CacheJobHandler struct {
	cacheJobService *service.CacheJobService
}

func NewCacheJobHandler(cacheJobService *service.CacheJobService) *CacheJobHandler {
	return &CacheJobHandler{
		cacheJobService: cacheJobService,
	}
}

func (handler *CacheJobHandler) CreateCacheJobHandler(c echo.Context) error {
	createCacheJobReq := new(query.CreateCacheJobReq)
	if err := c.Bind(createCacheJobReq); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "无效的 JSON 数据",
		})
	}
	if _, ok := consts.RepoTypesMapping[createCacheJobReq.Datatype]; !ok {
		zap.S().Errorf("MetaProxyCommon repoType:%s is not exist RepoTypesMapping", createCacheJobReq.Datatype)
		return util.ErrorPageNotFound(c)
	}
	org, repo := util.SplitOrgRepo(createCacheJobReq.OrgRepo)
	if org == "" || repo == "" {
		zap.S().Errorf("MetaProxyCommon org and repo is null")
		return util.ErrorRepoNotFound(c)
	}
	createCacheJobReq.Org = org
	createCacheJobReq.Repo = repo
	createCacheJobReq.Type = consts.CacheTypePreheat
	resp, err := handler.cacheJobService.CreateCacheJob(createCacheJobReq)
	if err != nil {
		return util.ResponseError(c, err)
	}
	response := c.Response()
	response.WriteHeader(resp.StatusCode)
	_, err = io.Copy(response, bytes.NewReader(resp.Body))
	if err != nil {
		return util.ResponseError(c, err)
	}
	return nil
}

func (handler *CacheJobHandler) ListCacheJobHandler(c echo.Context) error {
	aidcCode := c.QueryParam("aidcCode")
	instanceId, err := GetInstanceId(aidcCode)
	if err != nil {
		return util.ErrorRequestParam(c)
	}
	var (
		page, pageSize int
	)
	if page, err = extractPageParam(c, "page"); err != nil {
		return util.ErrorRequestParam(c)
	}
	if pageSize, err = extractPageParam(c, "pageSize"); err != nil {
		return util.ErrorRequestParam(c)
	}
	cacheJobs, total, err := handler.cacheJobService.ListCacheJob(instanceId, page, pageSize)
	if err != nil {
		return util.ResponseError(c, err)
	}
	cacheJobResps := make([]*dto.CacheJobResp, 0, len(cacheJobs))
	for _, job := range cacheJobs {
		cacheJobResp := &dto.CacheJobResp{}
		gocopy.Copy(cacheJobResp, job)
		cacheJobResp.CreatedAt = util.TimeToUnix(job.CreatedAt)
		cacheJobResps = append(cacheJobResps, cacheJobResp)
	}
	return util.NormalResponseData(c, util.PageData{Total: total, List: cacheJobResps})
}

func (handler *CacheJobHandler) StopCacheJobHandler(c echo.Context) error {
	jobStatusReq := new(query.JobStatusReq)
	if err := c.Bind(jobStatusReq); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "无效的 JSON 数据",
		})
	}
	err := handler.cacheJobService.StopCacheJob(jobStatusReq)
	if err != nil {
		return util.ResponseError(c, err)
	}
	return util.NormalResponseData(c, nil)
}

func (handler *CacheJobHandler) ResumeCacheJobHandler(c echo.Context) error {
	resumeCacheJobReq := new(query.ResumeCacheJobReq)
	if err := c.Bind(resumeCacheJobReq); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "无效的 JSON 数据",
		})
	}
	err := handler.cacheJobService.ResumeCacheJob(resumeCacheJobReq)
	if err != nil {
		return util.ResponseError(c, err)
	}
	return util.NormalResponseData(c, nil)
}

func (handler *CacheJobHandler) DeleteCacheJobHandler(c echo.Context) error {
	id := util.Atoi64(c.Param("id"))
	err := handler.cacheJobService.DeleteCacheJob(id)
	if err != nil {
		return util.ResponseError(c, err)
	}
	return util.NormalResponseData(c, nil)
}
