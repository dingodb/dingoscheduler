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
	pb "dingoscheduler/pkg/proto/manager"
	"fmt"

	"go.uber.org/zap"

	"dingoscheduler/internal/data"
	"dingoscheduler/internal/model"
	"dingoscheduler/internal/model/query"

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

func (d *ModelFileRecordDao) Save(records *model.ModelFileRecord) error {
	if err := d.baseData.BizDB.Model(&model.ModelFileRecord{}).Save(records).Error; err != nil {
		return err
	}
	return nil
}

func (d *ModelFileRecordDao) BatchSave(records []model.ModelFileRecord) error {
	if err := d.baseData.BizDB.Model(&model.ModelFileRecord{}).CreateInBatches(&records, 5).Error; err != nil {
		return err
	}
	return nil
}

func (d *ModelFileRecordDao) GetModelFileRecord(condition *query.ModelFileRecordQuery) (*model.ModelFileRecord, error) {
	var records []*model.ModelFileRecord
	db := d.baseData.BizDB.Model(&model.ModelFileRecord{}).Select("id")
	if condition.Datatype != "" {
		db.Where("datatype = ?", condition.Datatype)
	}
	if condition.Org != "" {
		db.Where("org = ?", condition.Org)
	}
	if condition.Repo != "" {
		db.Where("repo = ?", condition.Repo)
	}
	if condition.Etag != "" {
		db.Where("etag = ?", condition.Etag)
	}
	if err := db.Find(&records).Error; err != nil {
		return nil, err
	}

	if len(records) > 0 {
		return records[0], nil
	}
	return nil, nil
}

func (d *ModelFileRecordDao) SaveSchedulerRecord(req *pb.SchedulerFileRequest, process *model.ModelFileProcess) error {
	if err := d.baseData.BizDB.Transaction(func(tx *gorm.DB) error {
		record := &model.ModelFileRecord{
			Datatype: req.DataType,
			Org:      req.Org,
			Repo:     req.Repo,
			Name:     req.Name,
			Etag:     req.Etag,
			FileSize: req.FileSize,
		}
		if err := tx.Create(record).Error; err != nil {
			return err
		}
		process.RecordID = record.ID
		process.OffsetNum = 0 // 初始
		if err := tx.Create(process).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		zap.S().Error("SaveSchedulerRecord err.%v", err)
		return err
	}
	return nil
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
