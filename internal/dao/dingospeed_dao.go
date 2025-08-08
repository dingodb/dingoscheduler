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
	"dingoscheduler/pkg/util"
)

type DingospeedDao struct {
	baseData *data.BaseData
}

func NewDingospeedDao(data *data.BaseData) *DingospeedDao {
	return &DingospeedDao{
		baseData: data,
	}
}

func (d *DingospeedDao) Save(speed *model.Dingospeed) error {
	if err := d.baseData.BizDB.Model(&model.Dingospeed{}).Save(speed).Error; err != nil {
		return err
	}
	return nil
}

func (d *DingospeedDao) RegisterUpdate(speed *model.Dingospeed) error {
	sql := fmt.Sprintf("UPDATE dingospeed SET host='%s', port=%d, updated_at = '%s' WHERE id = %d", speed.Host, speed.Port, util.GetCurrentTimeStr(), speed.ID)
	if err := d.baseData.BizDB.Exec(sql).Error; err != nil {
		return err
	}
	return nil
}

func (d *DingospeedDao) HeartbeatUpdate(id int32) error {
	sql := fmt.Sprintf("UPDATE dingospeed SET updated_at = '%s' WHERE id = %d", util.GetCurrentTimeStr(), id)
	if err := d.baseData.BizDB.Exec(sql).Error; err != nil {
		return err
	}
	return nil
}

func (d *DingospeedDao) GetEntityById(id int32) (*model.Dingospeed, error) {
	var speed model.Dingospeed
	if err := d.baseData.BizDB.Model(&model.Dingospeed{}).Where("id = ?", id).Find(&speed).Error; err != nil {
		return nil, err
	}
	return &speed, nil
}

func (d *DingospeedDao) GetEntity(instanceId string, online bool) (*model.Dingospeed, error) {
	var speeds []*model.Dingospeed
	// sql := fmt.Sprintf("select * from dingospeed where instance_id = '%s' and online = %v limit 1", instanceId, online)
	// if err := d.baseData.BizDB.Raw(sql).Scan(&speed).Error; err != nil { // [mysql] 2025/07/30 11:18:38 packets.go:68 [warn] unexpected sequence nr: expected 1, got 2
	// 	if errors.Is(err, gorm.ErrRecordNotFound) {
	// 		return nil, nil
	// 	}
	// 	return nil, err
	// }
	// 1个？=》 {"level":"ERROR","time":"2025-07-30 11:25:53","caller":"service/manager_service.go:62","msg":"getEntity err.Error 1105 (HY000): not a literal: ?1"}
	// 没有？=>{"level":"ERROR","time":"2025-07-30 11:30:21","caller":"service/manager_service.go:62","msg":"getEntity err.Error 1105 (HY000): not a literal: ?0"}
	// if err := d.baseData.BizDB.Table("dingospeed").Where(fmt.Sprintf("instance_id = '%s'", instanceId)).First(&speed2).Error; err != nil {
	// var speed model.Dingospeed
	// if err := d.baseData.BizDB.Table("dingospeed").Where("instance_id = ? and online = ?", instanceId, online).First(&speed).Error; err != nil {
	// 	if errors.Is(err, gorm.ErrRecordNotFound) {
	// 		return nil, nil
	// 	}
	// 	return nil, err
	// }
	if err := d.baseData.BizDB.Table("dingospeed").Where("instance_id = ? and online = ?", instanceId, online).Find(&speeds).Error; err != nil {
		return nil, err
	}
	if len(speeds) > 0 {
		return speeds[0], nil
	}
	return nil, nil
}
