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
	"dingoscheduler/pkg/util"

	"github.com/bytedance/sonic"
)

type PreheatJobDao struct {
	baseData *data.BaseData
}

func NewPreheatJobDao(data *data.BaseData) *PreheatJobDao {
	return &PreheatJobDao{
		baseData: data,
	}
}

func (d *PreheatJobDao) Save(preheatJob *model.PreheatJob) error {
	if err := d.baseData.BizDB.Model(&model.PreheatJob{}).Save(preheatJob).Error; err != nil {
		return err
	}
	return nil
}

func (d *PreheatJobDao) GetPreheatJob(condition *query.PreheatJobQuery) (*model.PreheatJob, error) {
	var preheatJobs []*model.PreheatJob
	db := d.baseData.BizDB.Model(&model.PreheatJob{}).Select("id")
	if condition.Area != "" {
		db.Where("area = ?", condition.Area)
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

func (d *PreheatJobDao) RemoteRequestMeta(domain, repoType, orgRepo, commit, authorization string) (*common.Response, error) {
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

func (d *PreheatJobDao) RemoteRequestPathsInfo(domain, dataType, org, repo, revision, token string, fileNames []string) ([]common.PathsInfo, error) {
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
