package service

import (
	"strings"
	"sync"

	"dingoscheduler/internal/dao"
	"dingoscheduler/internal/model/query"
	"dingoscheduler/pkg/config"

	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
)

var once sync.Once

type SysService struct {
	repositoryDao *dao.RepositoryDao
}

func NewSysService(repositoryDao *dao.RepositoryDao) *SysService {
	sysSvc := &SysService{}
	sysSvc.repositoryDao = repositoryDao
	once.Do(
		func() {
			if config.SysConfig.GetEnablePersistRepo() {
				go sysSvc.startPersistRepo()
			}
		})
	return sysSvc
}

func (s SysService) startPersistRepo() {
	c := cron.New(cron.WithSeconds())
	_, err := c.AddFunc(config.SysConfig.GetPersistRepoCron(), func() {
		ids := config.SysConfig.Scheduler.PersistRepo.InstanceIds
		instanceIds := strings.Split(ids, ",")
		if len(instanceIds) > 0 {
			err := s.repositoryDao.PersistRepo(&query.PersistRepoQuery{
				InstanceIds: instanceIds,
			})
			if err != nil {
				zap.S().Errorf("cron exec persistRepo err: %v", err)
				return
			}
		}
	})
	if err != nil {
		zap.S().Errorf("添加任务失败: %v", err)
		return
	}
	c.Start()
	defer c.Stop()
	select {}
}
