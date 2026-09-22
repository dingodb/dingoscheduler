package service

import (
	"context"
	"dingoscheduler/internal/dao"
	pb "dingoscheduler/pkg/proto/manager"
)

func (s *SchedulerService) IngestRepository(ctx context.Context, req *pb.IngestRepositoryRequest) (*pb.IngestRepositoryResponse, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.repositoryDao.IngestPublished(ctx, req)
}

func (s *SchedulerService) UploadedHoldings(ctx context.Context, instanceID, namespace, repoType, repo, path, sha string) ([]dao.UploadedHoldingView, error) {
	return s.repositoryDao.ListUploadedHoldings(ctx, instanceID, namespace, repoType, repo, path, sha)
}

func (s *SchedulerService) UploadedNodeInventoryState(ctx context.Context, instanceID string) (*dao.UploadedNodeInventoryStateView, error) {
	return s.repositoryDao.GetUploadedNodeInventoryState(ctx, instanceID)
}

func (s *SchedulerService) UploadedRepositories(ctx context.Context) ([]dao.UploadedRepositoryView, error) {
	return s.repositoryDao.ListUploadedRepositories(ctx)
}
