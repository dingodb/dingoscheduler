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

func (d *ModelFileProcessDao) Save(process *model.ModelFileProcess) error {
	if err := d.baseData.BizDB.Model(&model.ModelFileProcess{}).Save(process).Error; err != nil {
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
	process := &model.ModelFileProcess{
		ID:        req.ProcessId,
		Status:    req.Status,
		OffsetNum: req.EndPos,
	}
	var sql string
	if process.Status == consts.StatusDownloadBreak {
		sql = fmt.Sprintf("UPDATE model_file_process SET status = %d, updated_at = '%s' WHERE id = %d", process.Status, util.GetCurrentTimeStr(), process.ID)
	} else {
		sql = fmt.Sprintf("UPDATE model_file_process SET offset_num = %d, status = %d, updated_at = '%s' WHERE id = %d and offset_num >= %d",
			process.OffsetNum, process.Status, util.GetCurrentTimeStr(), process.ID, req.StaPos)
	}
	if err := d.baseData.BizDB.Exec(sql).Error; err != nil {
		return err
	}
	return nil
}

func (d *ModelFileProcessDao) GetModelFileProcess(recordId int64) ([]*dto.ModelFileProcessDto, error) {
	var processes []*dto.ModelFileProcessDto
	if err := d.baseData.BizDB.Table("model_file_process t1").Select("t1.id, t1.record_id, t1.instance_id, t1.offset_num, t2.host, t2.port, t2.updated_at").
		Joins("left join dingospeed t2 on t1.instance_id = t2.instance_id and t2.online = true").
		Where("record_id=?", recordId).Order("t1.offset_num desc").Find(&processes).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return processes, nil
}

func (d *ModelFileProcessDao) BatchSave(records []model.ModelFileProcess) error {
	if err := d.baseData.BizDB.Model(&model.ModelFileProcess{}).CreateInBatches(&records, 5).Error; err != nil {
		return err
	}
	return nil
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

func (d *ModelFileProcessDao) GetModelFileProcessByCondition(datatype, org, repo, name, etag, area string) ([]*model.ModelFileProcess, error) {
	var processes []*model.ModelFileProcess
	if err := d.baseData.BizDB.Table("model_file_process t1").Select("t1.id").
		Joins("inner join model_file_record t2 on t1.record_id  = t2.id ").
		Where("t2.datatype = ? and t2.org= ? and t2.repo = ? and t2.name = ? and t2.etag = ? and t1.instance_id = ? and t1.status = 3",
			datatype, org, repo, name, etag, area).Find(&processes).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return processes, nil
}
