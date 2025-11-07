//  Copyright (c) 2025 dingodb.com, Inc. All Rights Reserved
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http:www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.

package router

import (
	"dingoscheduler/internal/handler"
	"dingoscheduler/pkg/config"

	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type HttpRouter struct {
	echo              *echo.Echo
	sysHandler        *handler.SysHandler
	managerHandler    *handler.ManagerHandler
	repositoryHandler *handler.RepositoryHandler
	tagHandler        *handler.TagHandler
	cacheJobHandler   *handler.CacheJobHandler
}

func NewHttpRouter(echo *echo.Echo, managerHandler *handler.ManagerHandler, sysHandler *handler.SysHandler,
	repositoryHandler *handler.RepositoryHandler, tagHandler *handler.TagHandler, cacheJobHandler *handler.CacheJobHandler) *HttpRouter {
	r := &HttpRouter{
		echo:              echo,
		sysHandler:        sysHandler,
		managerHandler:    managerHandler,
		repositoryHandler: repositoryHandler,
		tagHandler:        tagHandler,
		cacheJobHandler:   cacheJobHandler,
	}
	r.initRouter()
	return r
}

func (r *HttpRouter) GetHandler() *echo.Echo {
	return r.echo
}

func (r *HttpRouter) initRouter() {
	// 系统信息
	r.echo.GET("/info", r.sysHandler.Info)
	if config.SysConfig.EnableMetric() {
		r.echo.GET("/metrics", echo.WrapHandler(promhttp.Handler()))
	}
	r.echo.POST("/api/persistRepo", r.managerHandler.PersistRepoHandler) // 持久化仓库
	r.echo.GET("/api/refreshToken", r.managerHandler.RefreshToken)       // 持久化仓库
	r.repositoryRouter()                                                 // repository接口
	r.cacheJobRouter()                                                   // 模型缓存
}

func (r *HttpRouter) repositoryRouter() {
	r.echo.GET("/api/v1/repositories", r.repositoryHandler.RepositoriesHandler)                                // 仓库列表
	r.echo.GET("/api/v1/repository/:id", r.repositoryHandler.RepositoryInfoHandler)                            // 单个仓库信息描述
	r.echo.GET("/api/v1/repository/card/:aidcCode/:id", r.repositoryHandler.RepositoryCardHandler)             // 仓库介绍
	r.echo.GET("/api/v1/repository/files/:aidcCode/:id/", r.repositoryHandler.RepositoryFilesHandler)          // 仓库文件目录
	r.echo.GET("/api/v1/repository/files/:aidcCode/:id/:filePath", r.repositoryHandler.RepositoryFilesHandler) // 仓库文件目录
	r.echo.POST("/api/v1/repositories/mount", r.repositoryHandler.MountRepositoryHandler)                      // 挂载缓存

	r.echo.GET("/api/v1/tags", r.tagHandler.TagHandler)
	r.echo.GET("/api/v1/task_tags", r.tagHandler.TaskTagHandler)
	r.echo.GET("/api/v1/main_tags", r.tagHandler.MainTagHandler)
}

func (r *HttpRouter) cacheJobRouter() {
	r.echo.GET("/api/v1/cacheJob/list", r.cacheJobHandler.ListCacheJobHandler)
	r.echo.POST("/api/v1/cacheJob/create", r.cacheJobHandler.CreateCacheJobHandler)
	r.echo.POST("/api/v1/cacheJob/stop", r.cacheJobHandler.StopCacheJobHandler)
	r.echo.POST("/api/v1/cacheJob/resume", r.cacheJobHandler.ResumeCacheJobHandler)
	r.echo.DELETE("/api/v1/cacheJob/:id", r.cacheJobHandler.DeleteCacheJobHandler)
}
