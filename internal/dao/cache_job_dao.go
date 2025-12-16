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

package dao

import (
	"fmt"
	"strings"

	"dingoscheduler/internal/data"
	"dingoscheduler/internal/model"
	"dingoscheduler/internal/model/query"
	"dingoscheduler/pkg/consts"
	"dingoscheduler/pkg/util"

	"github.com/bytedance/sonic"
	"go.uber.org/zap"
)

type CacheJobDao struct {
	baseData      *data.BaseData
	repositoryDao *RepositoryDao
}

func NewCacheJobDao(data *data.BaseData, repositoryDao *RepositoryDao) *CacheJobDao {
	return &CacheJobDao{
		baseData:      data,
		repositoryDao: repositoryDao,
	}
}

func (c *CacheJobDao) Save(preheatJob *model.CacheJob) error {
	if err := c.baseData.BizDB.Model(&model.CacheJob{}).Save(preheatJob).Error; err != nil {
		return err
	}
	return nil
}

func (c *CacheJobDao) GetCacheJob(condition *query.CacheJobQuery) (*model.CacheJob, error) {
	var preheatJobs []*model.CacheJob
	db := c.baseData.BizDB.Model(&model.CacheJob{})
	if condition.Id != 0 {
		db.Where("id = ?", condition.Id)
	}
	if condition.Type != 0 {
		db.Where("type = ?", condition.Type)
	}
	if condition.InstanceId != "" {
		db.Where("instance_id = ?", condition.InstanceId)
	}
	if condition.Datatype != "" {
		db.Where("datatype = ?", condition.Datatype)
	}
	if condition.Org != "" {
		db.Where("org = ?", condition.Org)
	}
	if condition.Repo != "" {
		db.Where("repo = ?", condition.Repo)
	}
	if err := db.Find(&preheatJobs).Error; err != nil {
		return nil, err
	}
	if len(preheatJobs) > 0 {
		return preheatJobs[0], nil
	}
	return nil, nil
}

func (c *CacheJobDao) UpdateCacheStatus(statusReq *query.UpdateJobStatusReq) error {
	var (
		newMsgStr string
	)
	if statusReq.ErrorMsg != "" {
		msg := make(map[string]string, 0)
		msg["msg"] = statusReq.ErrorMsg
		msgStr, err := sonic.Marshal(msg)
		if err != nil {
			return err
		}
		newMsgStr = strings.ReplaceAll(string(msgStr), "'", "''")
	}
	var sql string
	if statusReq.Process > 0 {
		sql = fmt.Sprintf("UPDATE cache_job SET  status = %d, error_msg = '%s', updated_at = '%s', process = %f WHERE id = %d",
			statusReq.Status, newMsgStr, util.GetCurrentTimeStr(), statusReq.Process, statusReq.Id)
	} else {
		sql = fmt.Sprintf("UPDATE cache_job SET  status = %d, error_msg = '%s', updated_at = '%s' WHERE id = %d",
			statusReq.Status, newMsgStr, util.GetCurrentTimeStr(), statusReq.Id)
	}
	if err := c.baseData.BizDB.Exec(sql).Error; err != nil {
		return err
	}
	return nil
}

func (c *CacheJobDao) UpdateStatusAndRepo(jobStatusReq *query.UpdateJobStatusReq) error {
	err := c.UpdateCacheStatus(jobStatusReq)
	if err != nil {
		return err
	}
	if jobStatusReq.Status == consts.RunningStatusJobComplete {
		err = c.repositoryDao.PersistRepo(&query.PersistRepoReq{InstanceIds: []string{jobStatusReq.InstanceId},
			Org: jobStatusReq.Org, Repo: jobStatusReq.Repo, OffVerify: true})
		if err != nil {
			return err
		}
	}
	return nil
}
func (c *CacheJobDao) Delete(id int64) error {
	if err := c.baseData.BizDB.Where("id = ?", id).Delete(&model.CacheJob{}).Error; err != nil {
		return err
	}
	return nil
}

func (c *CacheJobDao) ListCacheJob(condition *query.CacheJobQuery) ([]*model.CacheJob, int64, error) {
	var cacheJobs []*model.CacheJob
	db := c.baseData.BizDB.Model(&model.CacheJob{})
	if condition.Id != 0 {
		db.Where("id = ?", condition.Id)
	}
	if condition.Type != 0 {
		db.Where("type = ?", condition.Type)
	}
	if condition.InstanceId != "" {
		db.Where("instance_id = ?", condition.InstanceId)
	}
	if condition.Datatype != "" {
		db.Where("datatype = ?", condition.Datatype)
	}
	if condition.Org != "" {
		db.Where("org = ?", condition.Org)
	}
	if condition.Repo != "" {
		db.Where("repo = ?", condition.Repo)
	}
	var count int64
	if err := db.Count(&count).Error; err != nil {
		zap.S().Error("统计数量失败", err)
		return nil, 0, err
	}
	offset, pageSize := paginate(condition.Page, condition.PageSize)
	db.Order(fmt.Sprintf("created_at desc offset %d limit %d", offset, pageSize))
	if err := db.Find(&cacheJobs).Error; err != nil {
		return nil, 0, err
	}
	return cacheJobs, count, nil
}

func (c *CacheJobDao) GetUnCacheJob(instanceId string, ids []int, runningStatus []int32, limit int) ([]*model.CacheJob, error) {
	cacheJobs := make([]*model.CacheJob, 0)
	db := c.baseData.BizDB.Table("cache_job t1")
	if instanceId != "" {
		db.Where("t1.instance_id = ?", instanceId)
	}
	if len(ids) > 0 {
		db.Where("t1.id in (?)", ids)
	}
	if len(runningStatus) > 0 {
		db.Where("t1.status in (?)", runningStatus)
	}
	if limit > 0 {
		db.Limit(limit)
	}
	err := db.Find(&cacheJobs).Error // 中断或等待中的
	return cacheJobs, err
}
