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
	"strings"
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
	repokey "dingoscheduler/pkg/repository"
	"dingoscheduler/pkg/util"

	"github.com/bytedance/sonic"
	"github.com/labstack/echo/v4"
	"github.com/young2j/gocopy"
	"go.uber.org/zap"
)

type RepositoryService struct {
	baseData        *data.BaseData
	dingospeedDao   *dao.DingospeedDao
	repositoryDao   *dao.RepositoryDao
	organizationDao *dao.OrganizationDao
	tagDao          *dao.TagDao
	hfTokenDao      *dao.HfTokenDao
	client          *http.Client
	persistSync     sync.Mutex
}

func NewRepositoryService(dingospeedDao *dao.DingospeedDao,
	repositoryDao *dao.RepositoryDao, baseData *data.BaseData, organizationDao *dao.OrganizationDao,
	tagDao *dao.TagDao, hfTokenDao *dao.HfTokenDao) *RepositoryService {
	return &RepositoryService{
		baseData:        baseData,
		dingospeedDao:   dingospeedDao,
		repositoryDao:   repositoryDao,
		organizationDao: organizationDao,
		tagDao:          tagDao,
		hfTokenDao:      hfTokenDao,
		client:          &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }},
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
		if icon, err := s.organizationDao.GetOrganization(upstreamLogoOrg(repo.Org, repo.Repo)); err != nil {
			return nil, 0, err
		} else {
			if icon != "" {
				repo.Icon = fmt.Sprintf("%s%s", config.SysConfig.Oss.Path, icon)
			}
		}
		setRepositoryIdentity(&repo)
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
	if icon, err := s.organizationDao.GetOrganization(upstreamLogoOrg(repository.Org, repository.Repo)); err != nil {
		return nil, err
	} else {
		if icon != "" {
			repo.Icon = fmt.Sprintf("%s%s", config.SysConfig.Oss.Path, icon)
		}
	}
	setRepositoryIdentity(&repo)
	return &repo, nil
}

func (s *RepositoryService) RepositoryCardById(c echo.Context, instanceId string, id int64) (*common.Response, error) {
	targetURL, repository, err := s.getRepository(instanceId, id)
	if err != nil {
		return nil, err
	}
	uri, err := storageAPIKey(repository.Datatype, repository.Org, repository.Repo).OperationURI("file", repository.Sha, "README.md")
	if err != nil {
		return nil, err
	}
	resp, err := s.requestForward(c, targetURL, targetURL.String()+uri)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	headers := make(map[string]interface{})
	for key, values := range resp.Header {
		headers[key] = values
	}
	// Always pass through DingoSpeed authorization, even for a cached README.
	return &common.Response{StatusCode: resp.StatusCode, Headers: headers, Body: body}, nil
}

func (s *RepositoryService) RepositoryFilesById(c echo.Context, instanceId string, id int64, filePath string) error {
	targetURL, repository, err := s.getRepository(instanceId, id)
	if err != nil {
		return err
	}
	uri, err := storageAPIKey(repository.Datatype, repository.Org, repository.Repo).OperationURI("files", repository.Sha, filePath)
	if err != nil {
		return err
	}
	forwardURL := targetURL.String() + uri
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
	if repository.Status == consts.RunningStatusJobIng || repository.Status == consts.RunningStatusJobComplete {
		return myerr.New("当前状态不可执行该操作。")
	}
	entity, err := s.dingospeedDao.GetEntity(repository.InstanceId, false) // 挂载到公共目录，通过离线模式处理
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
		Org:          repository.Org,
		Repo:         repository.Repo,
		Datatype:     repository.Datatype,
	}
	b, err := sonic.Marshal(createCacheJobReq)
	if err != nil {
		return err
	}
	authHeaders := make(map[string]string)
	if repoReq.Token != "" {
		authHeaders["Authorization"] = fmt.Sprintf("Bearer %s", repoReq.Token)
	} else {
		authHeaders = s.hfTokenDao.ProviderHeaders(storageAPIKey(repository.Datatype, repository.Org, repository.Repo))
	}
	var status int32 = consts.RunningStatusJobIng
	_, err = util.PostForDomain(speedDomain, "/api/cacheJob/create", "application/json", b, authHeaders)
	if err != nil {
		status = consts.RunningStatusJobStop
	}
	if err = s.repositoryDao.UpdateRepositoryMountStatus(&query.UpdateMountStatusReq{
		Id:     repository.ID,
		Status: status,
	}); err != nil {
		zap.S().Errorf("UpdateRepositoryMountStatus err.%v", err)
		return myerr.New("更新状态错误。")
	}
	return nil
}

func upstreamLogoOrg(org, repo string) string {
	if strings.Contains(org, "/") {
		return ""
	}
	return org
}
func storageAPIKey(repoType, org, repo string) repokey.Key {
	k, _ := repokey.FromWire(repoType, org, repo)
	return k
}

func setRepositoryIdentity(r *dto.Repository) {
	k, e := repokey.FromWire(r.Datatype, r.Org, r.Repo)
	if e != nil {
		return
	}
	r.Namespace = k.Namespace
	r.FullRepo, r.RepositoryID = k.Repo, k.ID()
	if strings.HasPrefix(r.Org, "dingo-local/") {
		r.Org = k.Namespace
		r.Repo = k.Repo
		r.OrgRepo = k.ID()
	}
}
