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

package service

import (
	"fmt"

	"dingoscheduler/internal/dao"
	"dingoscheduler/internal/model/dto"
	"dingoscheduler/internal/model/query"
	"dingoscheduler/pkg/common"
	"dingoscheduler/pkg/consts"
	myerr "dingoscheduler/pkg/error"
	"dingoscheduler/pkg/util"

	"github.com/bytedance/sonic"
	"github.com/young2j/gocopy"
	"go.uber.org/zap"
)

type CacheJobService struct {
	dingospeedDao       *dao.DingospeedDao
	modelFileProcessDao *dao.ModelFileProcessDao
	cacheJobDao         *dao.CacheJobDao
	hfTokenDao          *dao.HfTokenDao
	lockDao             *dao.LockDao
}

func NewCacheJobService(dingospeedDao *dao.DingospeedDao, modelFileProcessDao *dao.ModelFileProcessDao,
	cacheJobDao *dao.CacheJobDao, hfTokenDao *dao.HfTokenDao, lockDao *dao.LockDao) *CacheJobService {
	return &CacheJobService{
		dingospeedDao:       dingospeedDao,
		cacheJobDao:         cacheJobDao,
		modelFileProcessDao: modelFileProcessDao,
		hfTokenDao:          hfTokenDao,
		lockDao:             lockDao,
	}
}

func (c *CacheJobService) ListCacheJob(instanceId, datatype string, page, pageSize int) ([]*dto.CacheJobResp, int64, error) {
	cacheJobs, size, err := c.cacheJobDao.ListCacheJob(&query.CacheJobQuery{
		Type:       consts.CacheTypePreheat,
		InstanceId: instanceId,
		Datatype:   datatype,
		Page:       page,
		PageSize:   pageSize,
	})
	if err != nil {
		return nil, 0, err
	}
	jobIds := make([]int64, 0)
	for _, job := range cacheJobs {
		if job.Status == consts.StatusCacheJobIng {
			jobIds = append(jobIds, job.ID)
		}
	}
	statusMap, err := c.getJobRealtimeStatus(jobIds, instanceId)
	if err != nil {
		return nil, 0, err
	}
	cacheJobResps := make([]*dto.CacheJobResp, 0, len(cacheJobs))
	for _, job := range cacheJobs {
		cacheJobResp := &dto.CacheJobResp{}
		gocopy.Copy(cacheJobResp, job)
		if status, ok := statusMap[job.ID]; ok {
			cacheJobResp.StockSpeed = status.StockSpeed
			cacheJobResp.StockProcess = status.StockProcess
		} else {
			cacheJobResp.StockSpeed = "-"
			cacheJobResp.StockProcess = job.Process
		}
		cacheJobResp.CreatedAt = util.TimeToUnix(job.CreatedAt)
		cacheJobResps = append(cacheJobResps, cacheJobResp)
	}
	return cacheJobResps, size, nil
}

func (c *CacheJobService) getJobRealtimeStatus(jobIds []int64, instanceId string) (map[int64]*query.RealtimeResp, error) {
	m := make(map[int64]*query.RealtimeResp, 0)
	if len(jobIds) > 0 {
		entity, err := c.dingospeedDao.GetEntity(instanceId, true)
		if err != nil {
			return nil, err
		}
		speedDomain := fmt.Sprintf("http://%s:%d", entity.Host, entity.Port)
		b, err := sonic.Marshal(query.RealtimeReq{
			CacheJobIds: jobIds,
		})
		if err != nil {
			return nil, err
		}
		resp, err := util.PostForDomain(speedDomain, "/api/cacheJob/realtime", "application/json", b, c.hfTokenDao.GetHeaders())
		if err != nil {
			return nil, err
		}
		realtimeData := make([]*query.RealtimeResp, 0)
		err = sonic.Unmarshal(resp.Body, &realtimeData)
		if err != nil {
			return nil, err
		}
		for _, item := range realtimeData {
			m[item.CacheJobId] = item
		}
	}
	return m, nil
}

func (c *CacheJobService) CreateCacheJob(createCacheJobReq *query.CreateCacheJobReq) (*common.Response, error) {
	zap.S().Debugf("Cache instanceId:%s, %s/%s", createCacheJobReq.InstanceId, createCacheJobReq.Org, createCacheJobReq.Repo)
	lock := c.lockDao.GetCacheJobReqLock(createCacheJobReq.OrgRepo)
	lock.Lock()
	defer lock.Unlock()
	cacheJob, err := c.cacheJobDao.GetCacheJob(&query.CacheJobQuery{InstanceId: createCacheJobReq.InstanceId, Type: createCacheJobReq.Type,
		Org: createCacheJobReq.Org, Repo: createCacheJobReq.Repo, Datatype: createCacheJobReq.Datatype})
	if err != nil {
		return nil, err
	}
	if cacheJob != nil {
		return nil, myerr.New("已存在该任务，不能再创建。")
	}
	entity, err := c.dingospeedDao.GetEntity(createCacheJobReq.InstanceId, true)
	if err != nil {
		return nil, err
	}
	if entity == nil {
		return nil, myerr.New("该区域dingspeed未注册。")
	}
	speedDomain := fmt.Sprintf("http://%s:%d", entity.Host, entity.Port)
	b, err := sonic.Marshal(createCacheJobReq)
	if err != nil {
		return nil, err
	}
	return util.PostForDomain(speedDomain, "/api/cacheJob/create", "application/json", b, c.hfTokenDao.GetHeaders())
}

func (c *CacheJobService) StopCacheJob(jobStatusReq *query.JobStatusReq) error {
	lock := c.lockDao.GetCacheJobReqLock(util.Itoa(jobStatusReq.Id))
	lock.Lock()
	defer lock.Unlock()
	cacheJob, err := c.cacheJobDao.GetCacheJob(&query.CacheJobQuery{Id: jobStatusReq.Id})
	if err != nil {
		return err
	}
	if cacheJob == nil {
		return myerr.New(fmt.Sprintf("任务不存在。"))
	}
	if cacheJob.Status != consts.StatusCacheJobIng {
		return myerr.New(fmt.Sprintf("job is not running, Can't be stopped.%d", cacheJob.Status))
	}
	entity, err := c.dingospeedDao.GetEntity(jobStatusReq.InstanceId, true)
	if err != nil {
		return err
	}
	if entity == nil {
		return myerr.New("该区域dingspeed未注册。")
	}
	err = c.cacheJobDao.UpdateCacheStatus(&query.UpdateJobStatusReq{Id: jobStatusReq.Id, Status: consts.StatusCacheJobStopping})
	if err != nil {
		return err
	}
	speedDomain := fmt.Sprintf("http://%s:%d", entity.Host, entity.Port)
	b, err := sonic.Marshal(jobStatusReq)
	if err != nil {
		return err
	}
	_, err = util.PostForDomain(speedDomain, "/api/cacheJob/stop", "application/json", b, c.hfTokenDao.GetHeaders())
	if err != nil {
		return err
	}
	return nil
}

func (c *CacheJobService) ResumeCacheJob(resumeCacheJobReq *query.ResumeCacheJobReq) error {
	lock := c.lockDao.GetCacheJobReqLock(util.Itoa(resumeCacheJobReq.Id))
	lock.Lock()
	defer lock.Unlock()
	cacheJob, err := c.cacheJobDao.GetCacheJob(&query.CacheJobQuery{Id: resumeCacheJobReq.Id})
	if err != nil {
		return err
	}
	if cacheJob == nil {
		return myerr.New(fmt.Sprintf("job is not exist.jobId:%d", resumeCacheJobReq.Id))
	}
	if cacheJob.Status != consts.StatusCacheJobBreak {
		return myerr.New("当前状态不可执行该操作。")
	}
	entity, err := c.dingospeedDao.GetEntity(resumeCacheJobReq.InstanceId, true)
	if err != nil {
		return err
	}
	if entity == nil {
		return myerr.New("该区域dingspeed未注册。")
	}
	speedDomain := fmt.Sprintf("http://%s:%d", entity.Host, entity.Port)
	resumeReq := &query.ResumeCacheJobReq{
		Id:          resumeCacheJobReq.Id,
		Type:        cacheJob.Type,
		InstanceId:  cacheJob.InstanceId,
		Datatype:    cacheJob.Datatype,
		Org:         cacheJob.Org,
		Repo:        cacheJob.Repo,
		UsedStorage: cacheJob.UsedStorage,
	}
	b, err := sonic.Marshal(resumeReq)
	if err != nil {
		return err
	}
	_, err = util.PostForDomain(speedDomain, "/api/cacheJob/resume", "application/json", b, c.hfTokenDao.GetHeaders())
	if err != nil {
		return err
	}
	return nil
}

func (c *CacheJobService) DeleteCacheJob(id int64) error {
	lock := c.lockDao.GetCacheJobReqLock(util.Itoa(id))
	lock.Lock()
	defer lock.Unlock()
	cacheJob, err := c.cacheJobDao.GetCacheJob(&query.CacheJobQuery{Id: id})
	if err != nil {
		return err
	}
	if cacheJob == nil {
		return myerr.New(fmt.Sprintf("记录不存在。"))
	}
	if cacheJob.Status == consts.StatusCacheJobIng || cacheJob.Status == consts.StatusCacheJobComplete {
		return myerr.New(fmt.Sprintf("当前缓存任务不能删除。"))
	}
	return c.cacheJobDao.Delete(id)
}
