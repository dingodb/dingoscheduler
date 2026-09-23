// Copyright (c) 2025 dingodb.com, Inc. All Rights Reserved
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http:www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package data

import (
	"dingoscheduler/internal/authority"
	"errors"
	"fmt"

	"dingoscheduler/internal/model"
	"dingoscheduler/pkg/config"
	"dingoscheduler/pkg/consts"
	myorm "dingoscheduler/pkg/gorm"

	"github.com/google/wire"
	"github.com/patrickmn/go-cache"
	"gorm.io/gorm"
)

var BaseDataProvider = wire.NewSet(NewBaseData)

type BaseData struct {
	BizDB *gorm.DB
	Cache *cache.Cache
}

func initDB(dbConfig *config.DBConfig) (*gorm.DB, error) {
	var dbClient *gorm.DB
	var err error
	switch dbConfig.Type {
	case consts.DB_MYSQL:
		dbClient, err = myorm.NewMysqlClient(dbConfig)
	default:
		err = errors.New(fmt.Sprintf("unknown db type: %s", dbConfig.Type))
	}

	return dbClient, err
}

func NewBaseData(conf *config.Config) (*BaseData, func(), error) {
	bizClient, err := initDB(&conf.BizDBConfig)
	if err != nil {
		return nil, nil, err
	}
	gCache := cache.New(config.SysConfig.GetDefaultExpiration(), config.SysConfig.GetCleanupInterval())
	cleanup := func() {
		bizDb, _ := bizClient.DB()
		_ = bizDb.Close()
	}

	var debug = conf.Server.Mode != "release"
	if debug {
		bizClient = bizClient.Debug()
	}
	// First-phase uploaded inventory is an additive schema. AutoMigrate only
	// creates/extends these dedicated tables and never migrates legacy remote
	// repository or download records.
	if err = bizClient.AutoMigrate(&model.NodeEndpoint{}, &model.UploadInventoryState{}, &model.UploadInventoryFile{}, &model.UploadInventoryHolding{}, &model.UploadReportNode{}, &model.UploadReportRepo{}, &authority.Definition{}, &authority.Receipt{}); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("migrate uploaded inventory schema: %w", err)
	}
	return &BaseData{
		BizDB: bizClient,
		Cache: gCache,
	}, cleanup, nil
}
