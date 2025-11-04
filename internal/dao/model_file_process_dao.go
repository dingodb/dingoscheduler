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
	"errors"
	"fmt"

	"dingoscheduler/internal/data"
	"dingoscheduler/internal/model"
	"dingoscheduler/internal/model/dto"
	"dingoscheduler/pkg/consts"
	pb "dingoscheduler/pkg/proto/manager"
	"dingoscheduler/pkg/util"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

type ModelFileProcessDao struct {
	baseData *data.BaseData
}

func NewModelFileProcessDao(data *data.BaseData) *ModelFileProcessDao {
	return &ModelFileProcessDao{
		baseData: data,
	}
}

func (d *ModelFileProcessDao) Save(process *model.ModelFileProcess) (int64, error) {
	return SaveProcessBySql(d.baseData.BizDB, process)
}

func SaveProcessBySql(tx *gorm.DB, process *model.ModelFileProcess) (int64, error) {
	recordSql := fmt.Sprintf("INSERT INTO model_file_process(record_id, instance_id, offset_num, status, master_instance_id) VALUES (%d, '%s',%d,%d,'%s')", process.RecordID, process.InstanceID, process.OffsetNum, process.Status, process.MasterInstanceID)
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

func (d *ModelFileProcessDao) BatchSave(processes []model.ModelFileProcess) error {
	tx := d.baseData.BizDB.Begin()
	if tx.Error != nil {
		zap.S().Error("开启事务失败: %v", tx.Error)
		return tx.Error
	}

	db, err := tx.DB()
	if err != nil {
		tx.Rollback()
		zap.S().Error("从事务获取 DB 实例失败: %v", err)
		return err
	}

	for _, process := range processes {
		sql := fmt.Sprintf(
			"INSERT INTO model_file_process(record_id, instance_id, offset_num, status, master_instance_id) VALUES(%d,'%s',%d,%d,'%s')",
			process.RecordID,
			process.InstanceID,
			process.OffsetNum,
			process.Status,
			process.MasterInstanceID,
		)

		result, err := db.Exec(sql)
		if err != nil {
			tx.Rollback()
			zap.S().Error("批量插入失败: %v, SQL: %s", err, sql)
			return err
		}

		_, _ = result.LastInsertId()
	}

	if err := tx.Commit().Error; err != nil {
		tx.Rollback()
		zap.S().Fatalf("事务提交失败: %v", err)
		return err
	}

	return nil
}

func (d *ModelFileProcessDao) ResetProcess(process *model.ModelFileProcess) error {
	sql := fmt.Sprintf("UPDATE model_file_process SET offset_num = %d, status = %d, updated_at = '%s' WHERE id = %d",
		process.OffsetNum, process.Status, util.GetCurrentTimeStr(), process.ID)
	if err := d.baseData.BizDB.Exec(sql).Error; err != nil {
		return err
	}
	return nil
}

func (d *ModelFileProcessDao) ReportFileProcess(req *pb.FileProcessRequest) error {
	var sql string
	if req.Status == consts.StatusDownloadBreak {
		sql = fmt.Sprintf("UPDATE model_file_process SET status = %d, updated_at = '%s' WHERE id = %d", req.Status, util.GetCurrentTimeStr(), req.ProcessId)
	} else {
		sql = fmt.Sprintf("UPDATE model_file_process SET offset_num = %d, status = %d, updated_at = '%s' WHERE id = %d and offset_num <= %d",
			req.EndPos, req.Status, util.GetCurrentTimeStr(), req.ProcessId, req.StaPos)
	}
	if err := d.baseData.BizDB.Exec(sql).Error; err != nil {
		return err
	}
	return nil
}

func (d *ModelFileProcessDao) GetModelFileProcess(recordId int64) ([]*dto.ModelFileProcessDto, error) {
	var processes []*dto.ModelFileProcessDto
	if err := d.baseData.BizDB.Table("model_file_process t1").Select("t1.id, t1.record_id, t1.instance_id, t1.offset_num").
		Where("t1.record_id=?", recordId).Order("t1.offset_num desc").Find(&processes).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return processes, nil
}

func (d *ModelFileProcessDao) GetModelFileProcessByInstanceId(recordId int64, instanceId string) (*dto.ModelFileProcessDto, error) {
	var processes []*dto.ModelFileProcessDto
	if err := d.baseData.BizDB.Table("model_file_process t1").Select("t1.id, t1.record_id, t1.instance_id, t1.offset_num").
		Where("t1.record_id=? and t1.instance_id = ?", recordId, instanceId).Find(&processes).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if len(processes) > 0 {
		return processes[0], nil
	}
	return nil, nil
}

// ExistRecordIDs 查询指定InstanceID下，哪些RecordID已存在对应的ModelFileProcess记录
func (d *ModelFileProcessDao) ExistRecordIDs(instanceID string, recordIDs []int64) ([]int64, error) {
	if len(recordIDs) == 0 {
		return []int64{}, nil
	}

	var existingRecordIDs []int64
	if err := d.baseData.BizDB.Model(&model.ModelFileProcess{}).
		Where("instance_id = ? AND record_id IN (?)", instanceID, recordIDs).
		Pluck("record_id", &existingRecordIDs).Error; err != nil {
		return nil, fmt.Errorf("查询已存在的ModelFileProcess记录失败: %w", err)
	}

	return existingRecordIDs, nil
}

func (d *ModelFileProcessDao) GetModelFileProcessByCondition(datatype, org, repo, name, etag, instanceId string) ([]*model.ModelFileProcess, error) {
	var processes []*model.ModelFileProcess
	if err := d.baseData.BizDB.Table("model_file_process t1").Select("t1.id").
		Joins("inner join model_file_record t2 on t1.record_id  = t2.id ").
		Where("t2.datatype = ? and t2.org= ? and t2.repo = ? and t2.name = ? and t2.etag = ? and t1.instance_id = ? and t1.status = 3",
			datatype, org, repo, name, etag, instanceId).Find(&processes).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return processes, nil
}

func (d *ModelFileProcessDao) DeleteByRecordIDAndInstanceID(recordID []int64, instanceID string) (int64, error) {
	result := d.baseData.BizDB.Model(&model.ModelFileProcess{}).
		Where("record_id in (?) AND instance_id = ?", recordID, instanceID).
		Delete(&model.ModelFileProcess{})

	if result.Error != nil {
		return 0, result.Error
	}

	return result.RowsAffected, nil
}
