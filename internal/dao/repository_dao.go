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

	"go.uber.org/zap"
	"gorm.io/gorm"
)

type RepositoryDao struct {
	baseData         *data.BaseData
	repositoryTagDao *RepositoryTagDao
}

func NewRepositoryDao(data *data.BaseData, repositoryTagDao *RepositoryTagDao) *RepositoryDao {
	return &RepositoryDao{
		baseData:         data,
		repositoryTagDao: repositoryTagDao,
	}
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
	if err := r.baseData.BizDB.Model(&model.Repository{}).Select("id, org, org_repo,like_num, download_num, pipeline_tag,last_modified,sha ").Where("id = ?", id).Find(&repository).Error; err != nil {
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

func (r *RepositoryDao) GetFreeModelRepository(instanceId string) ([]*model.Repository, error) {
	var repositories []*model.Repository
	err := r.baseData.BizDB.Table("model_file_record t1").Select("distinct t1.datatype, t1.org, t1.repo ").
		Where("t1.datatype = 'models' and t1.id in (SELECT x.record_id FROM dingo.model_file_process x where x.instance_id = ?) and t1.repo not in (select repo from repository where instance_id = ?)", instanceId, instanceId).Find(&repositories).Error
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
	db := r.baseData.BizDB.Table("repository t1").Select("t1.id, t1.org, t1.org_repo, t1.like_num, t1.download_num, t1.pipeline_tag, t1.last_modified ")
	if query.InstanceId != "" {
		db.Where("t1.instance_id = ?", query.InstanceId)
	}
	if query.Name != "" {
		db.Where(fmt.Sprintf("t1.org_repo like '%s'", "%"+query.Name+"%"))
	}
	if query.PipelineTag != "" {
		db.Where("t1.pipeline_tag_id = ?", query.PipelineTag)
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
		zap.S().Error("统计collect数量失败", err)
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

func (d *RepositoryDao) DeleteByInstanceIdAndDatatypeAndOrgAndRepo(instanceId string, datatype string, org string, repo string) (int64, error) {
	result := d.baseData.BizDB.Model(&model.Repository{}).
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
