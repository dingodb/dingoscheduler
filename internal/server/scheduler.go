package server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net"
	"os"

	"dingoscheduler/internal/service"
	"dingoscheduler/pkg/config"
	"dingoscheduler/pkg/proto/manager"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type SchedulerServer struct {
	grpcServer     *grpc.Server
	managerService *service.SchedulerService
}

func NewSchedulerServer(managerService *service.SchedulerService) *SchedulerServer {
	return &SchedulerServer{
		managerService: managerService,
	}
}

func (s *SchedulerServer) Start(ctx context.Context) error {
	zap.S().Infof("[GRPC] server start...")
	opts := []grpc.ServerOption{}
	ssl := config.SysConfig.Server.Ssl
	if ssl.EnableCA {
		ct := credential(ssl.CrtFile, ssl.KeyFile, ssl.CaFile)
		if ct == nil {
			zap.S().Errorf("RunServer GET_CREDENTIAL_ERROR")
			os.Exit(-1)
		}
		opts = append(opts, grpc.Creds(ct))
	}

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", config.SysConfig.Scheduler.Port))
	if err != nil {
		return err
	}
	grpcServer := grpc.NewServer(opts...)
	manager.RegisterManagerServer(grpcServer, s.managerService)
	s.grpcServer = grpcServer
	if err := grpcServer.Serve(lis); err != nil {
		zap.S().Errorf("grpc server start fail: %v", err)
		return err
	}
	return nil
}

func credential(crtFile, keyFile, caFile string) credentials.TransportCredentials {
	cert, err := tls.LoadX509KeyPair(crtFile, keyFile)
	if err != nil {
		zap.S().Errorf("Credential LOAD_X509_ERROR:%s crtFile:%s keyFile:%s", err.Error(), crtFile, keyFile)
		return nil
	}

	caBytes, err := ioutil.ReadFile(caFile)
	if err != nil {
		zap.S().Errorf("Credential READ_CAFILE_ERROR:%s caFile:%s", err.Error(), caFile)
		return nil
	}

	certPool := x509.NewCertPool()
	if ok := certPool.AppendCertsFromPEM(caBytes); !ok {
		zap.S().Errorf("Credential APPEND_CERT_ERROR: %v", err)
		return nil
	}

	return credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    certPool,
	})
}

func (s *SchedulerServer) Stop(ctx context.Context) error {
	zap.S().Infof("[GRPC] server shutdown.")
	if s.grpcServer != nil {
		s.grpcServer.Stop()
	}
	return nil
}
