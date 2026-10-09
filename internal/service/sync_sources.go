package service

import (
	"context"
	"dingoscheduler/internal/dao"
)

func (s *SchedulerService) UploadedSyncSources(ctx context.Context, q dao.SyncSourceQuery) ([]dao.SyncSource, error) {
	return s.repositoryDao.UploadedSyncSources(ctx, q)
}
