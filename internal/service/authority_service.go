package service

import (
	"context"
	"dingoscheduler/internal/authority"
)

func (s *SchedulerService) OfficialRevision(ctx context.Context, r authority.Request) (any, error) {
	return authority.New(s.baseData.BizDB).Do(ctx, r)
}
