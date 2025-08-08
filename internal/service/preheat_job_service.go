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

package service

import (
	"context"
	"fmt"
	"net/http"

	"dingoscheduler/internal/dao"
	"dingoscheduler/internal/model"
	"dingoscheduler/internal/model/dto"
	"dingoscheduler/internal/model/query"
	"dingoscheduler/internal/service/task"
	"dingoscheduler/pkg/common"
	"dingoscheduler/pkg/consts"
	myerr "dingoscheduler/pkg/error"
	"dingoscheduler/pkg/util"

	"github.com/bytedance/sonic"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

type PreheatJobService struct {
	dingospeedDao       *dao.DingospeedDao
	modelFileProcessDao *dao.ModelFileProcessDao
	preheatJobDao       *dao.PreheatJobDao
	Pool                *common.Pool
}

func NewPreheatJobService(dingospeedDao *dao.DingospeedDao, modelFileProcessDao *dao.ModelFileProcessDao,
	preheatJobDao *dao.PreheatJobDao) *PreheatJobService {
	return &PreheatJobService{
		dingospeedDao:       dingospeedDao,
		preheatJobDao:       preheatJobDao,
		modelFileProcessDao: modelFileProcessDao,
		Pool:                common.NewPool(30),
	}
}

func (s *PreheatJobService) Preheat(c echo.Context, job *query.PreheatJobQuery) error {
	zap.S().Debugf("Preheat:%s, %s/%s/%s/%s", job.Area, job.Datatype, job.Org, job.Repo)
	preheatJob, err := s.preheatJobDao.GetPreheatJob(job)
	if err != nil {
		return err
	}
	if preheatJob != nil {
		return myerr.New("已存在该任务，不能再创建。")
	}
	entity, err := s.dingospeedDao.GetEntity(job.Area, true)
	if err != nil {
		return err
	}
	if entity == nil {
		return myerr.New("该区域dingspeed未注册。")
	}
	speedDomain := fmt.Sprintf("http://%s:%d", entity.Host, entity.Port)
	orgRepo := util.GetOrgRepo(job.Org, job.Repo)
	resp, err := s.preheatJobDao.RemoteRequestMeta(speedDomain, job.Datatype, orgRepo, "main", job.Token)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusTemporaryRedirect {
		return myerr.NewAppendCode(resp.StatusCode, "RemoteRequestMeta err")
	}
	var sha dto.CommitHfSha
	if err = sonic.Unmarshal(resp.Body, &sha); err != nil {
		zap.S().Errorf("unmarshal error.%v", err)
		return err
	}
	jobModel := model.PreheatJob{
		Area:      job.Area,
		Datatype:  job.Datatype,
		Org:       job.Org,
		Repo:      job.Repo,
		Token:     job.Token,
		Revision:  sha.Sha,
		FileTotal: len(sha.Siblings),
		MetaInfo:  string(resp.Body),
		Status:    consts.StatusPreheatCommit,
	}
	if err = s.Pool.Submit(context.Background(), &PreheatTask{
		preheatJobDao:       s.preheatJobDao,
		modelFileProcessDao: s.modelFileProcessDao,
		Job:                 &jobModel,
		Domain:              speedDomain,
	}); err != nil {
		return err
	}
	err = s.preheatJobDao.Save(&jobModel)
	if err != nil {
		return err
	}
	return nil
}

type PreheatTask struct {
	preheatJobDao       *dao.PreheatJobDao
	modelFileProcessDao *dao.ModelFileProcessDao
	Job                 *model.PreheatJob
	Domain              string
}

func (p *PreheatTask) DoTask() {
	var sha dto.CommitHfSha
	if err := sonic.Unmarshal([]byte(p.Job.MetaInfo), &sha); err != nil {
		zap.S().Errorf("unmarshal error.%v", err)
		return
	}
	batchNum := 8
	downloadPool := common.NewPool(batchNum)
	for _, fileName := range sha.Siblings {
		infos, err := p.preheatJobDao.RemoteRequestPathsInfo(p.Domain, p.Job.Datatype, p.Job.Org, p.Job.Repo, p.Job.Revision,
			p.Job.Token, []string{fileName.Rfilename})
		if err != nil {
			zap.S().Errorf("RemoteRequestPathsInfo err,%v", err)
			continue
		}
		if len(infos) != 1 {
			zap.S().Errorf("RemoteRequestPathsInfo err")
			continue
		}
		pathInfo := infos[0]
		var etag string
		if pathInfo.Lfs.Oid != "" {
			etag = pathInfo.Lfs.Oid
		} else {
			etag = pathInfo.Oid
		}
		// 判断文件是否下载
		processes, err := p.modelFileProcessDao.GetModelFileProcessByCondition(p.Job.Datatype, p.Job.Org, p.Job.Repo, fileName.Rfilename, etag, p.Job.Area)
		if err != nil {
			return
		}
		// 已下载完成
		if len(processes) != 0 {
			continue
		}
		if err := downloadPool.Submit(context.Background(), &task.DownloadTask{
			Job:      p.Job,
			Domain:   p.Domain,
			FileName: fileName.Rfilename,
		}); err != nil {
			zap.S().Errorf("submit error.%v", err)
			return
		}
	}
}
