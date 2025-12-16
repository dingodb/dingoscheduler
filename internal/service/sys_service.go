package service

import (
	"strings"
	"sync"

	"dingoscheduler/internal/dao"
	"dingoscheduler/internal/model/query"
	"dingoscheduler/pkg/config"
	"dingoscheduler/pkg/consts"

	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
)

var once sync.Once

type SysService struct {
	repositoryDao *dao.RepositoryDao
	cacheJobDao   *dao.CacheJobDao
}

func NewSysService(repositoryDao *dao.RepositoryDao, cacheJobDao *dao.CacheJobDao) *SysService {
	sysSvc := &SysService{}
	sysSvc.repositoryDao = repositoryDao
	sysSvc.cacheJobDao = cacheJobDao
	once.Do(
		func() {
			if config.SysConfig.GetEnablePersistRepo() {
				go sysSvc.startPersistRepo()
			}
		})
	if err := sysSvc.repairJobRunStatus(); err != nil {
		panic(err)
	}
	return sysSvc
}

// 同步到repository的仓库数据有如下几种情况：
// 1.用户通过hf-cli命令下载完成，需定时校验入库；若出现离线未上传，在speed端定时处理本地日志，上报record，process数据，再由本定时器入库repository，需校验。
// 2.用户在alayanew上下载缓存任务，执行完成后改仓库入库，无需校验；
// 3.通过接口对外暴露同步，允许用户手动触发，需校验；
func (s SysService) startPersistRepo() {
	c := cron.New(cron.WithSeconds())
	_, err := c.AddFunc(config.SysConfig.GetPersistRepoCron(), func() {
		instanceIds := config.SysConfig.Scheduler.PersistRepo.InstanceIds
		if instanceIds != "" {
			instanceIdSlice := strings.Split(instanceIds, ",")
			if len(instanceIdSlice) > 0 {
				err := s.repositoryDao.PersistRepo(&query.PersistRepoReq{
					InstanceIds: instanceIdSlice, OffVerify: false,
				})
				if err != nil {
					zap.S().Errorf("cron exec persistRepo err: %v", err)
				}
			}
		}
	})
	if err != nil {
		zap.S().Errorf("添加PersistRepo任务失败: %v", err)
		return
	}
	c.Start()
	defer c.Stop()
	select {}
}

func (s SysService) repairJobRunStatus() error {
	if unCacheJobs, err := s.cacheJobDao.GetUnCacheJob("", []int{}, []int32{consts.RunningStatusJobStopping}, 0); err != nil {
		zap.S().Errorf("GetUnmountRepository err.%v", err)
	} else {
		for _, i := range unCacheJobs {
			err = s.cacheJobDao.UpdateStatusAndRepo(&query.UpdateJobStatusReq{
				Id:     i.ID,
				Status: consts.RunningStatusJobStop,
			})
			if err != nil {
				return err
			}
		}
	}
	if repositories, err := s.repositoryDao.GetUnmountRepository("", []int{}, []int32{consts.RunningStatusJobStopping}, 0); err != nil {
		zap.S().Errorf("GetUnmountRepository err.%v", err)
	} else {
		for _, i := range repositories {
			err = s.repositoryDao.UpdateRepositoryMountStatus(&query.UpdateMountStatusReq{
				Id:     i.ID,
				Status: consts.RunningStatusJobStop,
			})
			if err != nil {
				return err
			}
		}
	}
	return nil
}
