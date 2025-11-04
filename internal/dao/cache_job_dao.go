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

	"dingoscheduler/internal/data"
	"dingoscheduler/internal/model"
	"dingoscheduler/internal/model/query"
	"dingoscheduler/pkg/common"
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

func (d *CacheJobDao) Save(preheatJob *model.CacheJob) error {
	if err := d.baseData.BizDB.Model(&model.CacheJob{}).Save(preheatJob).Error; err != nil {
		return err
	}
	return nil
}

func (d *CacheJobDao) GetCacheJob(condition *query.CacheJobQuery) (*model.CacheJob, error) {
	var preheatJobs []*model.CacheJob
	db := d.baseData.BizDB.Model(&model.CacheJob{})
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

func (d *CacheJobDao) RemoteRequestPathsInfo(domain, dataType, org, repo, revision, token string, fileNames []string) ([]common.PathsInfo, error) {
	var reqUri = "/api/getPathInfo"
	headers := map[string]string{}
	if token != "" {
		headers["authorization"] = fmt.Sprintf("Bearer %s", token)
	}
	query := query.PathInfoQuery{
		Datatype:  dataType,
		Org:       org,
		Repo:      repo,
		Revision:  revision,
		Token:     token,
		FileNames: fileNames,
	}
	b, err := sonic.Marshal(query)
	if err != nil {
		return nil, err
	}
	response, err := util.RetryRequest(func() (*common.Response, error) {
		return util.PostForDomain(domain, reqUri, "application/json", b, headers)
	})
	if err != nil {
		return nil, err
	}
	ret := make([]common.PathsInfo, 0)
	err = sonic.Unmarshal(response.Body, &ret)
	if err != nil {
		return nil, err
	}
	return ret, nil
}

func (d *CacheJobDao) UpdateStatus(statusReq *query.UpdateJobStatusReq) error {
	var (
		msgStr []byte
		err    error
	)
	if statusReq.ErrorMsg != "" {
		msg := make(map[string]string, 0)
		msg["msg"] = statusReq.ErrorMsg
		msgStr, err = sonic.Marshal(msg)
		if err != nil {
			return err
		}
	}
	sql := fmt.Sprintf("UPDATE cache_job SET  status = %d, error_msg = '%s', updated_at = '%s' WHERE id = %d",
		statusReq.Status, string(msgStr), util.GetCurrentTimeStr(), statusReq.Id)
	if err := d.baseData.BizDB.Exec(sql).Error; err != nil {
		return err
	}
	return nil
}

func (d *CacheJobDao) UpdateStatusAndRepo(jobStatusReq *query.UpdateJobStatusReq) error {
	err := d.UpdateStatus(jobStatusReq)
	if err != nil {
		return err
	}
	if jobStatusReq.Status == consts.StatusCacheJobComplete {
		err = d.repositoryDao.PersistRepo(&query.PersistRepoReq{InstanceIds: []string{jobStatusReq.InstanceId},
			Org: jobStatusReq.Org, Repo: jobStatusReq.Repo, OffVerify: true})
		if err != nil {
			return err
		}
	}
	return nil
}
func (d *CacheJobDao) Delete(id int64) error {
	if err := d.baseData.BizDB.Where("id = ?", id).Delete(&model.CacheJob{}).Error; err != nil {
		return err
	}
	return nil
}

func (d *CacheJobDao) UpdateMountCachePid(mountCachePidReq *query.UpdateMountCachePidReq) error {
	sql := fmt.Sprintf("UPDATE mount_cache_job SET shell_pid = %d, updated_at = '%s' WHERE id = %d",
		mountCachePidReq.Pid, util.GetCurrentTimeStr(), mountCachePidReq.Id)
	if err := d.baseData.BizDB.Exec(sql).Error; err != nil {
		return err
	}
	return nil
}

func (d *CacheJobDao) ListCacheJob(condition *query.CacheJobQuery) ([]*model.CacheJob, int64, error) {
	var cacheJobs []*model.CacheJob
	db := d.baseData.BizDB.Model(&model.CacheJob{})
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
