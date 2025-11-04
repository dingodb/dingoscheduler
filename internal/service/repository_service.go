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
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"

	"dingoscheduler/internal/dao"
	"dingoscheduler/internal/data"
	"dingoscheduler/internal/model"
	"dingoscheduler/internal/model/dto"
	"dingoscheduler/internal/model/query"
	"dingoscheduler/pkg/common"
	"dingoscheduler/pkg/config"
	"dingoscheduler/pkg/consts"
	myerr "dingoscheduler/pkg/error"
	"dingoscheduler/pkg/util"

	"github.com/bytedance/sonic"
	"github.com/labstack/echo/v4"
	"github.com/young2j/gocopy"
)

type RepositoryService struct {
	baseData        *data.BaseData
	dingospeedDao   *dao.DingospeedDao
	repositoryDao   *dao.RepositoryDao
	organizationDao *dao.OrganizationDao
	client          *http.Client
	tagDao          *dao.TagDao
	persistSync     sync.Mutex
}

func NewRepositoryService(dingospeedDao *dao.DingospeedDao, modelFileProcessDao *dao.ModelFileProcessDao,
	repositoryDao *dao.RepositoryDao, baseData *data.BaseData, organizationDao *dao.OrganizationDao,
	tagDao *dao.TagDao) *RepositoryService {
	return &RepositoryService{
		baseData:        baseData,
		dingospeedDao:   dingospeedDao,
		repositoryDao:   repositoryDao,
		organizationDao: organizationDao,
		tagDao:          tagDao,
		client:          &http.Client{},
	}
}

func (s *RepositoryService) PersistRepo(repoQuery *query.PersistRepoReq) error {
	return s.repositoryDao.PersistRepo(repoQuery)
}

func (s *RepositoryService) RepositoryList(query *query.ModelQuery) ([]*dto.Repository, int64, error) {
	repositories, size, err := s.repositoryDao.ModelList(query)
	if err != nil {
		return nil, 0, err
	}
	repos := make([]*dto.Repository, 0)
	for _, item := range repositories {
		var repo dto.Repository
		gocopy.Copy(&repo, &item)
		if icon, err := s.organizationDao.GetOrganization(repo.Org); err != nil {
			return nil, 0, err
		} else {
			repo.Icon = fmt.Sprintf("%s%s", config.SysConfig.Oss.Path, icon)
		}
		repos = append(repos, &repo)
	}
	return repos, size, nil
}

func (s *RepositoryService) GetRepositoryById(id int64) (*dto.Repository, error) {
	repository, err := s.repositoryDao.Get(id)
	if err != nil {
		return nil, err
	}
	var repo dto.Repository
	gocopy.Copy(&repo, &repository)
	tags, err := s.tagDao.GetTagByRepoId(id)
	if err != nil {
		return nil, err
	}
	for _, tag := range tags {
		repo.Tags = append(repo.Tags, tag.Label)
	}
	if icon, err := s.organizationDao.GetOrganization(repository.Org); err != nil {
		return nil, err
	} else {
		repo.Icon = fmt.Sprintf("%s%s", config.SysConfig.Oss.Path, icon)
	}
	return &repo, nil
}

func (s *RepositoryService) RepositoryCardById(c echo.Context, instanceId string, id int64) (*common.Response, error) {
	cardKey := util.GetCardKey(instanceId, id)
	var commResp *common.Response
	if v, ok := s.baseData.Cache.Get(cardKey); ok {
		commResp = v.(*common.Response)
		s.baseData.Cache.Set(cardKey, commResp, config.SysConfig.GetCacheExpiration())
	} else {
		targetURL, repository, err := s.getRepository(instanceId, id)
		if err != nil {
			return nil, err
		}
		prefix := string(consts.RepoTypeModel)
		if repository.Datatype == string(consts.RepoTypeDataset) {
			prefix = string(consts.RepoTypeDataset)
		}
		forwardURL := fmt.Sprintf("%s/%s/%s/resolve/%s/README.md", targetURL.String(), prefix, repository.OrgRepo, repository.Sha)
		resp, err := s.requestForward(c, targetURL, forwardURL)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("读取响应体失败: %v", err)
		}
		headers := make(map[string]interface{}, 0)
		for key, values := range resp.Header {
			headers[key] = values
		}
		commResp = &common.Response{
			StatusCode: resp.StatusCode,
			Headers:    headers,
			Body:       body,
		}
		s.baseData.Cache.Set(cardKey, commResp, config.SysConfig.GetCacheExpiration())
	}
	return commResp, nil
}

func (s *RepositoryService) RepositoryFilesById(c echo.Context, instanceId string, id int64, filePath string) error {
	targetURL, repository, err := s.getRepository(instanceId, id)
	if err != nil {
		return err
	}
	prefix := string(consts.RepoTypeModel)
	if repository.Datatype == string(consts.RepoTypeDataset) {
		prefix = string(consts.RepoTypeDataset)
	}
	forwardURL := fmt.Sprintf("%s/api/%s/%s/files/%s/", targetURL.String(), prefix, repository.OrgRepo, repository.Sha)
	if filePath != "" {
		forwardURL += filePath
	}
	resp, err := s.requestForward(c, targetURL, forwardURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	for key, values := range resp.Header {
		for _, value := range values {
			c.Response().Header().Add(key, value)
		}
	}
	c.Response().WriteHeader(resp.StatusCode)
	if _, err := io.Copy(c.Response().Writer, resp.Body); err != nil {
		return fmt.Errorf("响应内容回传失败")
	}
	return nil
}

func (s *RepositoryService) getRepository(instanceId string, id int64) (*url.URL, *model.Repository, error) {
	entity, err := s.dingospeedDao.GetEntity(instanceId, true)
	if err != nil {
		return nil, nil, fmt.Errorf("GetEntity err")
	}
	if entity == nil {
		return nil, nil, fmt.Errorf("该区域dingspeed未注册。")
	}
	repository, err := s.repositoryDao.Get(id)
	if err != nil {
		return nil, nil, fmt.Errorf("repositoryDao get err")
	}
	speedDomain := fmt.Sprintf("http://%s:%d", entity.Host, entity.Port)
	targetURL, err := url.Parse(speedDomain)
	if err != nil {
		return nil, nil, fmt.Errorf("目标服务URL解析失败")
	}
	return targetURL, repository, nil
}

func (s *RepositoryService) requestForward(c echo.Context, targetURL *url.URL, forwardURL string) (*http.Response, error) {
	req, err := http.NewRequest(c.Request().Method, forwardURL, c.Request().Body)
	if err != nil {
		return nil, fmt.Errorf("创建转发请求失败")
	}
	for key, values := range c.Request().Header {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	req.Host = targetURL.Host
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("转发请求到目标服务失败")
	}
	return resp, nil
}

func (s *RepositoryService) MountRepository(repoReq *query.RepositoryReq) error {
	repository, err := s.repositoryDao.Get(repoReq.Id)
	if err != nil {
		return err
	}
	if repository == nil {
		return myerr.New(fmt.Sprintf("记录不存在。编号：%d", repoReq.Id))
	}
	entity, err := s.dingospeedDao.GetEntity(repository.InstanceId, true)
	if err != nil {
		return err
	}
	if entity == nil {
		return myerr.New("该区域dingspeed未注册。")
	}
	speedDomain := fmt.Sprintf("http://%s:%d", entity.Host, entity.Port)
	createCacheJobReq := &query.CreateCacheJobReq{
		RepositoryId: repository.ID,
		Type:         consts.CacheTypeMount,
		InstanceId:   repository.InstanceId,
		Org:          repository.Org, Repo: repository.Repo,
		Datatype: repository.Datatype,
	}
	b, err := sonic.Marshal(createCacheJobReq)
	if err != nil {
		return err
	}
	_, err = util.PostForDomain(speedDomain, "/api/cacheJob/create", "application/json", b, util.GetHeaders())
	if err != nil {
		return err
	}
	return nil
}
