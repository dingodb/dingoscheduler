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
	"sync"

	"dingoscheduler/internal/data"
	"dingoscheduler/internal/model"
	"dingoscheduler/pkg/common"
	"dingoscheduler/pkg/config"
	"dingoscheduler/pkg/util"
)

type DingospeedDao struct {
	baseData *data.BaseData
	mu       sync.Mutex
}

func NewDingospeedDao(data *data.BaseData) *DingospeedDao {
	return &DingospeedDao{
		baseData: data,
	}
}

func (d *DingospeedDao) Save(speed *model.Dingospeed) (int64, error) {
	insertSql := fmt.Sprintf("INSERT INTO dingospeed(instance_id, host, port, online) VALUES('%s','%s',%d,%v)", speed.InstanceID, speed.Host, speed.Port, speed.Online)
	db, err := d.baseData.BizDB.DB()
	if err != nil {
		return 0, err
	}
	result, err := db.Exec(insertSql)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
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
	speedKey := util.GetSpeedKey(instanceId, online)
	if v, ok := d.baseData.Cache.Get(speedKey); ok {
		d.baseData.Cache.Set(speedKey, v, config.SysConfig.GetSpeedExpiration())
		return v.(*model.Dingospeed), nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if v, ok := d.baseData.Cache.Get(speedKey); ok {
		d.baseData.Cache.Set(speedKey, v, config.SysConfig.GetSpeedExpiration())
		return v.(*model.Dingospeed), nil
	}
	speeds := make([]model.Dingospeed, 0)
	if err := d.baseData.BizDB.Table("dingospeed").Where("instance_id = ? and online = ?", instanceId, online).Find(&speeds).Error; err != nil {
		return nil, err
	}
	if len(speeds) > 0 {
		speed := &speeds[0]
		d.baseData.Cache.Set(speedKey, speed, config.SysConfig.GetSpeedExpiration())
		return speed, nil
	}
	return nil, nil
}

func (d *DingospeedDao) RemoteRequestMeta(domain, repoType, orgRepo, commit, authorization string) (*common.Response, error) {
	var reqUri string
	if commit == "" {
		reqUri = fmt.Sprintf("/api/%s/%s", repoType, orgRepo)
	} else {
		reqUri = fmt.Sprintf("/api/%s/%s/revision/%s", repoType, orgRepo, commit)
	}
	headers := map[string]string{}
	if authorization != "" {
		headers["authorization"] = fmt.Sprintf("Bearer %s", authorization)
	}
	return util.RetryRequest(func() (*common.Response, error) {
		return util.GetForDomain(domain, reqUri, headers)
	})
}
