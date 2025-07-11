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
	echo       *echo.Echo
	sysHandler *handler.SysHandler
	manHandler *handler.ManagerHandler
}

func NewHttpRouter(echo *echo.Echo, manHandler *handler.ManagerHandler, sysHandler *handler.SysHandler) *HttpRouter {
	r := &HttpRouter{
		echo:       echo,
		sysHandler: sysHandler,
		manHandler: manHandler,
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

}
