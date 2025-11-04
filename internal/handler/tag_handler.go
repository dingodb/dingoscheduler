package handler

import (
	"strings"

	"dingoscheduler/internal/model/query"
	"dingoscheduler/internal/service"
	myerr "dingoscheduler/pkg/error"
	"dingoscheduler/pkg/util"

	"github.com/labstack/echo/v4"
)

type TagHandler struct {
	tagService *service.TagService
}

func NewTagHandler(tagService *service.TagService) *TagHandler {
	return &TagHandler{
		tagService: tagService,
	}
}

func (handler *TagHandler) TagHandler(c echo.Context) error {
	tagTypesStr := c.QueryParam("type")
	tagSubTypeStr := c.QueryParam("subType")

	var tagTypes []string
	if tagTypesStr != "" {
		tagTypesStr = strings.TrimSpace(tagTypesStr)
		parts := strings.Split(tagTypesStr, ",")
		for _, part := range parts {
			trimmed := strings.TrimSpace(part)
			if trimmed != "" {
				tagTypes = append(tagTypes, trimmed)
			}
		}
	}

	var tagSubTypes []string
	if tagSubTypeStr != "" {
		tagSubTypeStr = strings.TrimSpace(tagSubTypeStr)
		parts := strings.Split(tagSubTypeStr, ",")
		for _, part := range parts {
			trimmed := strings.TrimSpace(part)
			if trimmed != "" {
				tagSubTypes = append(tagSubTypes, trimmed)
			}
		}
	}

	tags, err := handler.tagService.TagListByCondition(&query.TagQuery{
		Types:    tagTypes,
		SubTypes: tagSubTypes,
	})
	if err != nil {
		if e, ok := err.(myerr.Error); ok {
			return util.ErrorEntryUnknown(c, e.StatusCode(), e.Error())
		}
		return util.ErrorProxyError(c)
	}
	return util.ResponseData(c, tags)
}

func (handler *TagHandler) TaskTagHandler(c echo.Context) error {
	taskTags, err := handler.tagService.TaskTagList(&query.TagQuery{
		Types: []string{"pipeline_tag"},
	})
	if err != nil {
		if e, ok := err.(myerr.Error); ok {
			return util.ErrorEntryUnknown(c, e.StatusCode(), e.Error())
		}
		return util.ErrorProxyError(c)
	}
	return util.ResponseData(c, taskTags)
}

func (handler *TagHandler) MainTagHandler(c echo.Context) error {
	datasetStr := c.QueryParam("datatype")
	mainTags, err := handler.tagService.MainTagList(datasetStr)
	if err != nil {
		if e, ok := err.(myerr.Error); ok {
			return util.ErrorEntryUnknown(c, e.StatusCode(), e.Error())
		}
		return util.ErrorProxyError(c)
	}
	return util.ResponseData(c, mainTags)
}
