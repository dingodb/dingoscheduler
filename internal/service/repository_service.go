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

	"dingoscheduler/internal/dao"
	"dingoscheduler/internal/data"
	"dingoscheduler/internal/model"
	"dingoscheduler/internal/model/dto"
	"dingoscheduler/internal/model/query"
	"dingoscheduler/pkg/common"
	"dingoscheduler/pkg/config"
	myerr "dingoscheduler/pkg/error"
	"dingoscheduler/pkg/util"

	"github.com/bytedance/sonic"
	"github.com/labstack/echo/v4"
	"github.com/young2j/gocopy"
	"go.uber.org/zap"
)

type RepositoryService struct {
	baseData            *data.BaseData
	dingospeedDao       *dao.DingospeedDao
	modelFileProcessDao *dao.ModelFileProcessDao
	repositoryDao       *dao.RepositoryDao
	organizationDao     *dao.OrganizationDao
	organizationService *OrganizationService
	tagDao              *dao.TagDao
	client              *http.Client
}

func NewRepositoryService(dingospeedDao *dao.DingospeedDao, modelFileProcessDao *dao.ModelFileProcessDao,
	repositoryDao *dao.RepositoryDao, tagDao *dao.TagDao, baseData *data.BaseData, organizationDao *dao.OrganizationDao, organizationService *OrganizationService) *RepositoryService {
	return &RepositoryService{
		dingospeedDao:       dingospeedDao,
		repositoryDao:       repositoryDao,
		modelFileProcessDao: modelFileProcessDao,
		tagDao:              tagDao,
		baseData:            baseData,
		organizationDao:     organizationDao,
		organizationService: organizationService,
		client:              &http.Client{},
	}
}

func (s *RepositoryService) PersistRepo(c echo.Context, repoQuery *query.PersistRepoQuery) error {
	zap.S().Debugf("PersistRepo instanceId:%s", repoQuery.InstanceIds)
	var (
		pipelineMap map[string]string
		err         error
	)
	pipelineMap, err = s.cachePipelineTags()
	if err != nil {
		return err
	}
	for _, instanceId := range repoQuery.InstanceIds {
		// 存在下载记录和进度，但【模型】在仓库不存在，没有数据集。
		freeRepositories, err := s.repositoryDao.GetFreeModelRepository(instanceId)
		if err != nil {
			return err
		}
		if len(freeRepositories) == 0 {
			return myerr.New("没有要持久化的仓库。")
		}
		for _, repository := range freeRepositories {
			entity, err := s.dingospeedDao.GetEntity(instanceId, true)
			if err != nil {
				return err
			}
			if entity == nil {
				return myerr.New("该区域dingspeed未注册。")
			}
			speedDomain := fmt.Sprintf("http://%s:%d", entity.Host, entity.Port)
			orgRepo := util.GetOrgRepo(repository.Org, repository.Repo)
			resp, err := s.dingospeedDao.RemoteRequestMeta(speedDomain, repository.Datatype, orgRepo, "main", repoQuery.Token)
			if err != nil {
				return err
			}
			if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusTemporaryRedirect {
				return myerr.NewAppendCode(resp.StatusCode, "RemoteRequestMeta err")
			}
			var metaData dto.CommitHfSha
			if err = sonic.Unmarshal(resp.Body, &metaData); err != nil {
				zap.S().Errorf("unmarshal error.%v", err)
				return err
			}
			// 根据当前版本的元数据与下载进度、进度比较，只将完整的模型做保存。
			isComplete, err := s.verifyRepoComplete(&metaData, instanceId, repository.Datatype, repository.Org, repository.Repo)
			if err != nil {
				return err
			}
			if !isComplete {
				continue
			}
			// 保存组织图片
			err = s.organizationService.PersistOrgLogo(repository.Org)
			if err != nil {
				return err
			}
			repo := &model.Repository{
				InstanceId:    instanceId,
				Datatype:      repository.Datatype,
				Org:           repository.Org,
				Repo:          repository.Repo,
				OrgRepo:       orgRepo,
				LikeNum:       metaData.Likes,
				DownloadNum:   metaData.Downloads,
				PipelineTagId: metaData.PipelineTag,
				PipelineTag:   pipelineMap[metaData.PipelineTag],
				LastModified:  metaData.LastModified,
				UsedStorage:   metaData.UsedStorage,
				Sha:           metaData.Sha,
			}
			tags := make([]*model.RepositoryTag, 0)
			for _, tag := range metaData.Tags {
				tags = append(tags, &model.RepositoryTag{
					TagId: tag,
				})
			}
			err = s.repositoryDao.RepoAndTagSave(repo, tags)
			if err != nil {
				zap.S().Errorf("repository save err.%v", err)
				return err
			}
		}
	}
	return nil
}

func (s *RepositoryService) cachePipelineTags() (map[string]string, error) {
	pipelineTags, err := s.tagDao.TagListByCondition(&query.TagQuery{
		Types: []string{"pipeline_tag"},
	})
	if err != nil {
		return nil, err
	}
	pipelineMap := make(map[string]string, 0)
	for _, item := range pipelineTags {
		pipelineMap[item.ID] = item.Label
	}
	return pipelineMap, nil
}

func (s *RepositoryService) verifyRepoComplete(metaData *dto.CommitHfSha, instanceId, datatype, org, repo string) (bool, error) {
	size, err := s.repositoryDao.VerifyRepoComplete(instanceId, datatype, org, repo)
	if err != nil {
		return false, err
	}
	fileCount := len(metaData.Siblings)
	if size >= int64(fileCount) {
		return true, nil
	}
	return false, nil
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
		forwardURL := fmt.Sprintf("%s/models/%s/resolve/%s/README.md", targetURL.String(), repository.OrgRepo, repository.Sha)
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
	forwardURL := fmt.Sprintf("%s/api/models/%s/files/%s/", targetURL.String(), repository.OrgRepo, repository.Sha)
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
