package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"

	"go.uber.org/zap"

	"dingoscheduler/internal/router"
	"dingoscheduler/pkg/config"

	"github.com/labstack/echo/v4"
)

type HTTPServer struct {
	*http.Server
	lis     net.Listener
	network string
	address string
	http    *router.HttpRouter
}

func NewHTTPServer(config *config.Config, httpRouter *router.HttpRouter) *HTTPServer {
	s := &HTTPServer{
		network: "tcp",
		address: fmt.Sprintf("%s:%d", config.Server.Host, config.Server.Port),
		http:    httpRouter,
	}
	s.Server = &http.Server{
		Handler:        s.http.GetHandler(),
		ReadTimeout:    0,
		WriteTimeout:   0, // 对用户侧的响应设置永不超时
		MaxHeaderBytes: 1 << 20,
	}
	return s
}

func (s *HTTPServer) Start(ctx context.Context) error {
	lis, err := net.Listen(s.network, s.address)
	if err != nil {
		return err
	}
	s.lis = lis
	s.BaseContext = func(net.Listener) context.Context {
		return ctx
	}
	zap.S().Infof("[HTTP] server listening on: %s", s.lis.Addr().String())
	if err := s.Serve(s.lis); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (s *HTTPServer) Stop(ctx context.Context) error {
	zap.S().Infof("[HTTP] server shutdown.")
	return s.Shutdown(ctx)
}

func NewEngine() *echo.Echo {
	r := echo.New()
	return r
}
