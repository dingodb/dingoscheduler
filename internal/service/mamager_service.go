package service

import (
	"fmt"

	"dingoscheduler/internal/dao"
	"dingoscheduler/internal/model/query"
	"dingoscheduler/pkg/consts"
)

type ManagerService struct {
	repositoryDao     *dao.RepositoryDao
	repositoryService *RepositoryService
	cacheJobDao       *dao.CacheJobDao
	cacheJobService   *CacheJobService
}

func NewManagerService(repositoryDao *dao.RepositoryDao, repositoryService *RepositoryService, cacheJobDao *dao.CacheJobDao,
	cacheJobService *CacheJobService) *ManagerService {
	return &ManagerService{
		repositoryService: repositoryService,
		repositoryDao:     repositoryDao,
		cacheJobDao:       cacheJobDao,
		cacheJobService:   cacheJobService,
	}
}

func (s *ManagerService) ExecWaitTask(waitTaskReq *query.WaitTaskReq) error {
	execStatus := []int32{consts.RunningStatusJobBreak, consts.RunningStatusJobWait}
	if waitTaskReq.Type == consts.CacheTypePreheat {
		unCacheJobs, err := s.cacheJobDao.GetUnCacheJob(waitTaskReq.InstanceId, waitTaskReq.Ids, execStatus, waitTaskReq.Limit)
		if err != nil {
			return err
		}
		for _, i := range unCacheJobs {
			err = s.cacheJobService.ResumeCacheJob(&query.ResumeCacheJobReq{
				Id:         i.ID,
				InstanceId: waitTaskReq.InstanceId,
			})
			if err != nil {
				return err
			}
		}
	} else if waitTaskReq.Type == consts.CacheTypeMount {
		repositories, err := s.repositoryDao.GetUnmountRepository(waitTaskReq.InstanceId, waitTaskReq.Ids, execStatus, waitTaskReq.Limit)
		if err != nil {
			return err
		}
		for _, i := range repositories {
			err = s.repositoryService.MountRepository(&query.RepositoryReq{
				Id: i.ID,
			})
			if err != nil {
				return err
			}
		}
	} else {
		return fmt.Errorf("type is invalid")
	}
	return nil
}
