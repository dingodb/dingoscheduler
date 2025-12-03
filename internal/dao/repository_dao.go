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
	"net/http"
	"strings"
	"sync"

	"dingoscheduler/internal/data"
	"dingoscheduler/internal/model"
	"dingoscheduler/internal/model/dto"
	"dingoscheduler/internal/model/query"
	myerr "dingoscheduler/pkg/error"
	"dingoscheduler/pkg/util"

	"github.com/bytedance/sonic"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type RepositoryDao struct {
	baseData         *data.BaseData
	repositoryTagDao *RepositoryTagDao
	dingospeedDao    *DingospeedDao
	organizationDao  *OrganizationDao
	tagDao           *TagDao
	hfTokenDao       *HfTokenDao
	persistSync      sync.Mutex
}

func NewRepositoryDao(data *data.BaseData, repositoryTagDao *RepositoryTagDao, tagDao *TagDao,
	dingospeedDao *DingospeedDao, organizationDao *OrganizationDao, hfTokenDao *HfTokenDao) *RepositoryDao {
	return &RepositoryDao{
		baseData:         data,
		tagDao:           tagDao,
		repositoryTagDao: repositoryTagDao,
		dingospeedDao:    dingospeedDao,
		organizationDao:  organizationDao,
		hfTokenDao:       hfTokenDao,
	}
}

func (r *RepositoryDao) PersistRepo(persistRepoReq *query.PersistRepoReq) error {
	zap.S().Debugf("PersistRepo start instanceId:%s， org:%s, repo:%s", persistRepoReq.InstanceIds, persistRepoReq.Org, persistRepoReq.Repo)
	var (
		pipelineMap map[string]string
		err         error
	)
	r.persistSync.Lock()
	defer r.persistSync.Unlock()
	pipelineMap, err = r.cachePipelineTags()
	if err != nil {
		return err
	}
	for _, instanceId := range persistRepoReq.InstanceIds {
		if instanceId == "" {
			continue
		}
		// 存在下载记录和进度，但【模型】在仓库不存在，没有数据集。
		freeRepositories, err := r.GetFreeRepository(instanceId, persistRepoReq.Org, persistRepoReq.Repo)
		if err != nil {
			return err
		}
		if len(freeRepositories) == 0 {
			zap.S().Warnf("instanceId:%s 没有要持久化的仓库。", instanceId)
			continue
		}
		speed, err := r.dingospeedDao.GetEntity(instanceId, true)
		if err != nil {
			return err
		}
		if speed == nil {
			return myerr.New("该区域dingospeed未注册。")
		}
		speedDomain := fmt.Sprintf("http://%s:%d", speed.Host, speed.Port)
		for _, repository := range freeRepositories {
			if err = r.singleRepositoryPersist(repository, instanceId, speedDomain, pipelineMap, persistRepoReq.OffVerify); err != nil {
				zap.S().Errorf("singleRepositoryPersist err.%v", err)
				continue
			}
		}
	}
	zap.S().Debugf("PersistRepo end instanceId:%s， org:%s, repo:%s", persistRepoReq.InstanceIds, persistRepoReq.Org, persistRepoReq.Repo)
	return nil
}

func (r *RepositoryDao) singleRepositoryPersist(repository *model.Repository, instanceId, speedDomain string, pipelineMap map[string]string, offVerify bool) error {
	orgRepo := util.GetOrgRepo(repository.Org, repository.Repo)
	metaResp, err := r.dingospeedDao.RemoteRequestMeta(speedDomain, repository.Datatype, orgRepo, "main", r.hfTokenDao.GetHeaders())
	if err != nil {
		return err
	}
	if metaResp.StatusCode != http.StatusOK && metaResp.StatusCode != http.StatusTemporaryRedirect {
		return myerr.NewAppendCode(metaResp.StatusCode, fmt.Sprintf("RemoteRequestMeta err,%s", orgRepo))
	}
	var metaData dto.CommitHfSha
	if err = sonic.Unmarshal(metaResp.Body, &metaData); err != nil {
		zap.S().Errorf("unmarshal error.orgRepo:%s, %v", orgRepo, err)
		return err
	}
	if !offVerify {
		// 根据当前版本的元数据与下载进度、进度比较，只将完整的模型做保存。
		isComplete, err := r.verifyRepoComplete(&metaData, instanceId, repository.Datatype, repository.Org, repository.Repo)
		if err != nil {
			return err
		}
		if !isComplete {
			zap.S().Infof("repo file unComplete.%s", orgRepo)
			return nil
		}
	}
	// 保存组织图片
	err = r.organizationDao.PersistOrgLogo(repository.Org)
	if err != nil {
		zap.S().Errorf("PersistOrgLogo err.org:%s, %v", repository.Org, err)
	}
	repo := &model.Repository{
		InstanceId:    instanceId,
		Datatype:      repository.Datatype,
		Org:           repository.Org,
		Repo:          repository.Repo,
		OrgRepo:       orgRepo,
		LikeNum:       metaData.Likes,
		DownloadNum:   metaData.Downloads,
		PipelineTagId: metaData.PipelineTag,
		PipelineTag:   pipelineMap[metaData.PipelineTag],
		LastModified:  metaData.LastModified,
		UsedStorage:   metaData.UsedStorage,
		Sha:           metaData.Sha,
	}
	tags := make([]*model.RepositoryTag, 0)
	for _, tag := range metaData.Tags {
		tags = append(tags, &model.RepositoryTag{
			TagId: tag,
		})
	}
	err = r.RepoAndTagSave(repo, tags)
	if err != nil {
		zap.S().Errorf("repository save err.orgRepo:%s,%v", orgRepo, err)
		return err
	}
	return nil
}

func (r *RepositoryDao) cachePipelineTags() (map[string]string, error) {
	pipelineTags, err := r.tagDao.TagListByCondition(&query.TagQuery{
		Types: []string{"pipeline_tag"},
	})
	if err != nil {
		return nil, err
	}
	pipelineMap := make(map[string]string, 0)
	for _, item := range pipelineTags {
		pipelineMap[item.ID] = item.Label
	}
	return pipelineMap, nil
}

func (r *RepositoryDao) verifyRepoComplete(metaData *dto.CommitHfSha, instanceId, datatype, org, repo string) (bool, error) {
	size, err := r.VerifyRepoComplete(instanceId, datatype, org, repo)
	if err != nil {
		return false, err
	}
	fileCount := len(metaData.Siblings)
	if size >= int64(fileCount) {
		return true, nil
	}
	return false, nil
}

func (r *RepositoryDao) SaveBySql(tx *gorm.DB, repo *model.Repository) (int64, error) {
	recordSql := fmt.Sprintf("INSERT INTO repository (instance_id, datatype, org, repo, org_repo, like_num, download_num, pipeline_tag_id, pipeline_tag, last_modified, used_storage, sha)"+
		" VALUES( '%s', '%s', '%s', '%s', '%s', %d, %d, '%s', '%s', '%s', %d, '%s')",
		repo.InstanceId, repo.Datatype, repo.Org, repo.Repo, repo.OrgRepo, repo.LikeNum, repo.DownloadNum, repo.PipelineTagId, repo.PipelineTag, repo.LastModified, repo.UsedStorage, repo.Sha)
	db, err := tx.DB()
	if err != nil {
		return 0, err
	}
	result, err := db.Exec(recordSql)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (r *RepositoryDao) Get(id int64) (*model.Repository, error) {
	var repository []*model.Repository
	if err := r.baseData.BizDB.Model(&model.Repository{}).Select("id, instance_id, datatype, org, repo, org_repo,like_num, download_num, pipeline_tag,last_modified,sha ").Where("id = ?", id).Find(&repository).Error; err != nil {
		return nil, err
	}
	if len(repository) > 0 {
		return repository[0], nil
	}
	return nil, fmt.Errorf("No record found")
}

func (r *RepositoryDao) RepoAndTagSave(repository *model.Repository, tags []*model.RepositoryTag) error {
	if err := r.baseData.BizDB.Transaction(func(tx *gorm.DB) error {
		lastId, err := r.SaveBySql(tx, repository)
		if err != nil {
			return err
		}
		for i := range tags {
			tags[i].RepoId = lastId
		}
		if err = r.repositoryTagDao.BatchSave(tx, tags); err != nil {
			return err
		}
		return nil
	}); err != nil {
		zap.S().Error("RepoAndTagSave err.%v", err)
		return err
	}
	return nil
}

func (r *RepositoryDao) GetFreeRepository(instanceId, org, repo string) ([]*model.Repository, error) {
	var repositories []*model.Repository
	tx := r.baseData.BizDB.Table("model_file_record t1").Select("distinct t1.datatype, t1.org, t1.repo ")
	if org != "" && repo != "" {
		tx.Where(fmt.Sprintf(" t1.org = '%s' and t1.repo= '%s'", org, repo))
	}
	err := tx.Where("t1.id in (SELECT x.record_id FROM dingo.model_file_process x where x.instance_id = ?) "+
		"and t1.repo not in (select repo from repository where instance_id = ?)", instanceId, instanceId).Find(&repositories).Error
	return repositories, err
}

func (r *RepositoryDao) VerifyRepoComplete(instanceId, datatype, org, repo string) (int64, error) {
	var recordCount int64
	err := r.baseData.BizDB.Table("model_file_record t1").Select("t1.id").InnerJoins(", model_file_process t2").
		Where("t1.datatype = ? and t1.org=? and t1.repo= ? and t1.id = t2.record_id and t2.instance_id = ? and t1.file_size = t2.offset_num", datatype, org, repo, instanceId).Count(&recordCount).Error
	return recordCount, err
}

func (r *RepositoryDao) ModelList(query *query.ModelQuery) ([]*model.Repository, int64, error) {
	repositories := make([]*model.Repository, 0)
	db := r.baseData.BizDB.Table("repository t1").Select("t1.id, t1.org, t1.org_repo, t1.like_num, t1.download_num, t1.sha, t1.pipeline_tag, t1.last_modified, t1.used_storage, t1.status")
	if query.InstanceId != "" {
		db.Where("t1.instance_id = ?", query.InstanceId)
	}
	if query.Name != "" {
		db.Where(fmt.Sprintf("t1.org_repo like '%s'", "%"+query.Name+"%"))
	}
	if query.PipelineTag != "" {
		db.Where("t1.pipeline_tag_id = ?", query.PipelineTag)
	}
	if query.Datatype != "" {
		db.Where("t1.datatype = ?", query.Datatype)
	}

	if query.Status != "" {
		db.Where("t1.status = ?", util.Atoi(query.Status))
	}

	tags := make([]string, 0)
	if query.Library != "" {
		tags = append(tags, strings.Split(query.Library, ",")...)
	}
	if query.Apps != "" {
		tags = append(tags, strings.Split(query.Apps, ",")...)
	}
	if query.InferenceProvider != "" {
		tags = append(tags, strings.Split(query.InferenceProvider, ",")...)
	}
	if query.Language != "" {
		tags = append(tags, strings.Split(query.Language, ",")...)
	}
	if query.License != "" {
		tags = append(tags, strings.Split(query.License, ",")...)
	}
	if query.Other != "" {
		tags = append(tags, strings.Split(query.Other, ",")...)
	}
	if len(tags) > 0 {
		db.Where(" t1.id in (select repo_id from repository_tag where tag_id in (?))", tags)
	}
	var count int64
	if err := db.Count(&count).Error; err != nil {
		zap.S().Error("统计数量失败", err)
		return nil, 0, err
	}
	offset, pageSize := paginate(query.Page, query.PageSize)
	if query.Sort != "" && query.Order != "" {
		db.Order(fmt.Sprintf("%s %s offset %d limit %d", query.Sort, query.Order, offset, pageSize))
	} else {
		db.Order(fmt.Sprintf("offset %d limit %d", offset, pageSize))
	}
	err := db.Find(&repositories).Error
	return repositories, count, err
}

func paginate(page, pageSize int) (int, int) {
	if page <= 0 {
		page = 1
	}
	switch {
	case pageSize > 100:
		pageSize = 100
	case pageSize <= 0:
		pageSize = 10
	}
	offset := (page - 1) * pageSize
	return offset, pageSize
}

func (r *RepositoryDao) DeleteByInstanceIdAndDatatypeAndOrgAndRepo(instanceId string, datatype string, org string, repo string) (int64, error) {
	result := r.baseData.BizDB.Model(&model.Repository{}).
		Where("instance_id = ?", instanceId).
		Where("datatype = ?", datatype).
		Where("org = ?", org).
		Where("repo = ?", repo).
		Delete(&model.Repository{})

	if result.Error != nil {
		return 0, result.Error
	}

	return result.RowsAffected, nil
}

func (r *RepositoryDao) UpdateRepositoryMountStatus(statusReq *query.UpdateMountStatusReq) error {
	var (
		msgStr    []byte
		err       error
		newMsgStr string
	)
	if statusReq.ErrorMsg != "" {
		msg := make(map[string]string, 0)
		msg["msg"] = statusReq.ErrorMsg
		msgStr, err = sonic.Marshal(msg)
		if err != nil {
			return err
		}
		newMsgStr = strings.ReplaceAll(string(msgStr), "'", "''")
	}
	sql := fmt.Sprintf("UPDATE repository SET  status = %d, error_msg = '%s', updated_at = '%s' WHERE id = %d",
		statusReq.Status, newMsgStr, util.GetCurrentTimeStr(), statusReq.Id)
	if err = r.baseData.BizDB.Exec(sql).Error; err != nil {
		return err
	}
	return nil
}
