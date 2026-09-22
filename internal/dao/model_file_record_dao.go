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
	pb "dingoscheduler/pkg/proto/manager"
	"dingoscheduler/pkg/repository"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

type ModelFileRecordDao struct {
	baseData *data.BaseData
}

func NewModelFileRecordDao(data *data.BaseData) *ModelFileRecordDao {
	return &ModelFileRecordDao{
		baseData: data,
	}
}

func (d *ModelFileRecordDao) BatchSave(records []model.ModelFileRecord) error {
	return d.baseData.BizDB.Transaction(func(tx *gorm.DB) error {
		for i := range records {
			if _, err := SaveRecordBySql(tx, &records[i]); err != nil {
				return err
			}
		}
		return nil
	})
}

func SaveRecordBySql(tx *gorm.DB, record *model.ModelFileRecord) (int64, error) {
	if _, err := repository.FromWire(record.Datatype, record.Org, record.Repo); err != nil {
		return 0, err
	}
	if err := tx.Omit("CreatedAt", "UpdatedAt").Create(record).Error; err != nil {
		return 0, err
	}
	return record.ID, nil
}

func (d *ModelFileRecordDao) FirstModelFileRecord(condition *query.ModelFileRecordQuery) (*model.ModelFileRecord, error) {
	if _, err := repository.FromWire(condition.Datatype, condition.Org, condition.Repo); err != nil {
		return nil, err
	}
	var records []*model.ModelFileRecord
	db := d.baseData.BizDB.Model(&model.ModelFileRecord{}).Select("id")
	db = db.Where("datatype = ? AND org = ? AND repo = ?", condition.Datatype, condition.Org, condition.Repo)
	if condition.Etag != "" {
		db = db.Where("etag = ?", condition.Etag)
	}
	if condition.FileName != "" {
		db = db.Where("name = ?", condition.FileName)
	}
	if err := db.Find(&records).Error; err != nil {
		return nil, err
	}

	if len(records) > 0 {
		return records[0], nil
	}
	return nil, nil
}

func (d *ModelFileRecordDao) SaveSchedulerRecord(req *pb.SchedulerFileRequest, process *model.ModelFileProcess) (int64, error) {
	var processId int64
	if err := d.baseData.BizDB.Transaction(func(tx *gorm.DB) error {
		record := &model.ModelFileRecord{
			Datatype: req.DataType,
			Org:      req.Org,
			Repo:     req.Repo,
			Name:     req.Name,
			Etag:     req.Etag,
			FileSize: req.FileSize,
		}
		lastId, err := SaveRecordBySql(tx, record)
		if err != nil {
			return err
		}
		process.RecordID = lastId
		processId, err = SaveProcessBySql(tx, process)
		if err != nil {
			return err
		}
		return nil
	}); err != nil {
		zap.S().Errorf("SaveSchedulerRecord err.%v", err)
		return 0, err
	}
	return processId, nil
}

// ExistEtags 查询指定Etag列表中已存在的Etag
func (d *ModelFileRecordDao) ExistEtags(etags []string) ([]string, error) {
	if len(etags) == 0 {
		return []string{}, nil
	}
	var existing []string
	// 假设使用GORM，通过IN查询已存在的Etag
	if err := d.baseData.BizDB.Model(&model.ModelFileRecord{}).
		Where("etag IN (?)", etags).
		Pluck("etag", &existing).Error; err != nil {
		return nil, err
	}
	return existing, nil
}

// GetIDsByEtags 根据Etag列表查询对应的ModelFileRecord记录的ID
func (d *ModelFileRecordDao) GetIDsByEtags(etags []string) ([]int64, error) {
	if len(etags) == 0 {
		return []int64{}, nil
	}

	var ids []int64
	// 假设使用GORM查询
	if err := d.baseData.BizDB.Model(&model.ModelFileRecord{}).
		Where("etag IN (?)", etags).
		Pluck("id", &ids).Error; err != nil {
		return nil, fmt.Errorf("查询Etag对应的ID失败: %w", err)
	}

	return ids, nil
}

// GetByIDs 根据ID列表查询完整的ModelFileRecord记录
func (d *ModelFileRecordDao) GetByIDs(ids []int64) ([]model.ModelFileRecord, error) {
	if len(ids) == 0 {
		return []model.ModelFileRecord{}, nil
	}

	var records []model.ModelFileRecord
	if err := d.baseData.BizDB.Model(&model.ModelFileRecord{}).Where("id IN (?)", ids).Find(&records).Error; err != nil {
		return nil, fmt.Errorf("根据ID查询ModelFileRecord失败: %w", err)
	}

	return records, nil
}

func (d *ModelFileRecordDao) FindDistinctOrgs() ([]string, error) {
	var orgs []string
	err := d.baseData.BizDB.Model(&model.ModelFileRecord{}).Distinct("org").Find(&orgs).Error
	return orgs, err
}

// GetIDsByEtagsOrFields 根据Etag列表查询，或者根据Datatype、Org、Repo、Name四者都匹配的条件查询对应的ID
func (d *ModelFileRecordDao) GetIDsByEtagsOrFields(etag, datatype, org, repo, name string) ([]int64, error) {
	// Content IDs are repository-scoped. An etag alone must never select another
	// namespace, nor may a filename OR broaden an exact content-ID deletion.
	if _, err := repository.FromWire(datatype, org, repo); err != nil {
		return nil, err
	}
	if etag == "" && name == "" {
		return nil, fmt.Errorf("etag or file path is required")
	}
	var ids []int64
	db := d.baseData.BizDB.Model(&model.ModelFileRecord{}).
		Where("datatype = ? AND org = ? AND repo = ?", datatype, org, repo)
	if etag != "" {
		db = db.Where("etag = ?", etag)
	}
	if name != "" {
		db = db.Where("name = ?", name)
	}
	if err := db.Pluck("id", &ids).Error; err != nil {
		return nil, err
	}
	return ids, nil
}

func (d *ModelFileRecordDao) BatchQueryByEtags(etags []string) ([]model.ModelFileRecord, error) {
	if len(etags) == 0 {
		return []model.ModelFileRecord{}, nil
	}

	var records []model.ModelFileRecord
	result := d.baseData.BizDB.Model(&model.ModelFileRecord{}).
		Where("etag IN (?)", etags).
		Find(&records)

	if result.Error != nil {
		return nil, fmt.Errorf("通过etag查询记录失败: %w", result.Error)
	}

	return records, nil
}

func (d *ModelFileRecordDao) BatchQueryByIDs(ids []int64) ([]model.ModelFileRecord, error) {
	if len(ids) == 0 {
		return []model.ModelFileRecord{}, nil
	}

	var records []model.ModelFileRecord
	result := d.baseData.BizDB.Model(&model.ModelFileRecord{}).
		Where("id IN (?)", ids).
		Find(&records)

	if result.Error != nil {
		return nil, fmt.Errorf("查询记录详情失败: %w", result.Error)
	}

	return records, nil
}

func (d *ModelFileRecordDao) ExistRecords(records []model.ModelFileRecord) ([]model.ModelFileRecord, error) {
	if len(records) == 0 {
		return []model.ModelFileRecord{}, nil
	}
	var existing []model.ModelFileRecord
	db := d.baseData.BizDB.Model(&model.ModelFileRecord{}).Where("1 = 0")

	for _, r := range records {
		db = db.Or("datatype = ? AND etag = ? AND name = ? AND org = ? AND repo = ?",
			r.Datatype, r.Etag, r.Name, r.Org, r.Repo)
	}

	if err := db.Find(&existing).Error; err != nil {
		return nil, err
	}
	return existing, nil
}
