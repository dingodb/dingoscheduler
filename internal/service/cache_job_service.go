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
	"dingoscheduler/internal/model"
	"dingoscheduler/internal/model/query"
	"dingoscheduler/pkg/common"
	"dingoscheduler/pkg/consts"
	myerr "dingoscheduler/pkg/error"
	"dingoscheduler/pkg/util"

	"github.com/bytedance/sonic"
	"go.uber.org/zap"
)

type CacheJobService struct {
	dingospeedDao       *dao.DingospeedDao
	modelFileProcessDao *dao.ModelFileProcessDao
	cacheJobDao         *dao.CacheJobDao
}

func NewCacheJobService(dingospeedDao *dao.DingospeedDao, modelFileProcessDao *dao.ModelFileProcessDao,
	cacheJobDao *dao.CacheJobDao) *CacheJobService {
	return &CacheJobService{
		dingospeedDao:       dingospeedDao,
		cacheJobDao:         cacheJobDao,
		modelFileProcessDao: modelFileProcessDao,
	}
}

func (p *CacheJobService) ListCacheJob(instanceId string, page, pageSize int) ([]*model.CacheJob, int64, error) {
	cacheJobs, size, err := p.cacheJobDao.ListCacheJob(&query.CacheJobQuery{
		Type:       consts.CacheTypePreheat,
		InstanceId: instanceId,
		Page:       page,
		PageSize:   pageSize,
	})
	if err != nil {
		return nil, 0, err
	}
	return cacheJobs, size, nil
}

func (p *CacheJobService) CreateCacheJob(createCacheJobReq *query.CreateCacheJobReq) (*common.Response, error) {
	zap.S().Debugf("Cache:%s, %s/%s", createCacheJobReq.InstanceId, createCacheJobReq.Org, createCacheJobReq.Repo)
	cacheJob, err := p.cacheJobDao.GetCacheJob(&query.CacheJobQuery{InstanceId: createCacheJobReq.InstanceId, Type: createCacheJobReq.Type,
		Org: createCacheJobReq.Org, Repo: createCacheJobReq.Repo, Datatype: createCacheJobReq.Datatype})
	if err != nil {
		return nil, err
	}
	if cacheJob != nil {
		return nil, myerr.New("已存在该任务，不能再创建。")
	}
	entity, err := p.dingospeedDao.GetEntity(createCacheJobReq.InstanceId, true)
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
	return util.PostForDomain(speedDomain, "/api/cacheJob/create", "application/json", b, util.GetHeaders())
}

func (p *CacheJobService) StopCacheJob(jobStatusReq *query.JobStatusReq) error {
	cacheJob, err := p.cacheJobDao.GetCacheJob(&query.CacheJobQuery{Id: jobStatusReq.Id})
	if err != nil {
		return err
	}
	if cacheJob == nil {
		return myerr.New(fmt.Sprintf("任务不存在，编号:%d", jobStatusReq.Id))
	}
	if cacheJob.Status != consts.StatusCacheJobIng {
		return myerr.New(fmt.Sprintf("job is not running, Can't be stopped.%d", cacheJob.Status))
	}
	entity, err := p.dingospeedDao.GetEntity(jobStatusReq.InstanceId, true)
	if err != nil {
		return err
	}
	if entity == nil {
		return myerr.New("该区域dingspeed未注册。")
	}
	speedDomain := fmt.Sprintf("http://%s:%d", entity.Host, entity.Port)
	b, err := sonic.Marshal(jobStatusReq)
	if err != nil {
		return err
	}
	_, err = util.PostForDomain(speedDomain, "/api/cacheJob/stop", "application/json", b, util.GetHeaders())
	if err != nil {
		return err
	}
	return nil
}

func (p *CacheJobService) ResumeCacheJob(resumeCacheJobReq *query.ResumeCacheJobReq) error {
	cacheJob, err := p.cacheJobDao.GetCacheJob(&query.CacheJobQuery{Id: resumeCacheJobReq.Id})
	if err != nil {
		return err
	}
	if cacheJob == nil {
		return myerr.New(fmt.Sprintf("job is not exist.jobId:%d", resumeCacheJobReq.Id))
	}
	if cacheJob.Status != consts.StatusCacheJobBreak {
		return myerr.New("当前状态不可执行该操作。")
	}
	entity, err := p.dingospeedDao.GetEntity(resumeCacheJobReq.InstanceId, true)
	if err != nil {
		return err
	}
	if entity == nil {
		return myerr.New("该区域dingspeed未注册。")
	}
	speedDomain := fmt.Sprintf("http://%s:%d", entity.Host, entity.Port)
	resumeReq := &query.ResumeCacheJobReq{
		Id:         resumeCacheJobReq.Id,
		Type:       cacheJob.Type,
		InstanceId: cacheJob.InstanceId,
		Datatype:   cacheJob.Datatype,
		Org:        cacheJob.Org,
		Repo:       cacheJob.Repo,
	}
	b, err := sonic.Marshal(resumeReq)
	if err != nil {
		return err
	}
	_, err = util.PostForDomain(speedDomain, "/api/cacheJob/resume", "application/json", b, util.GetHeaders())
	if err != nil {
		return err
	}
	return nil
}

func (p *CacheJobService) DeleteCacheJob(id int64) error {
	cacheJob, err := p.cacheJobDao.GetCacheJob(&query.CacheJobQuery{Id: id})
	if err != nil {
		return err
	}
	if cacheJob == nil {
		return myerr.New(fmt.Sprintf("记录不存在。编号：%d", id))
	}
	if cacheJob.Status == consts.StatusCacheJobIng || cacheJob.Status == consts.StatusCacheJobComplete {
		return myerr.New(fmt.Sprintf("当前缓存任务不能删除。编号：%d", id))
	}
	return p.cacheJobDao.Delete(id)
}
