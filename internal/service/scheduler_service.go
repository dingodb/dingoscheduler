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
	"sync"
	"time"

	"dingoscheduler/internal/dao"
	"dingoscheduler/internal/data"
	"dingoscheduler/internal/model"
	"dingoscheduler/internal/model/dto"
	"dingoscheduler/internal/model/query"
	"dingoscheduler/pkg/config"
	"dingoscheduler/pkg/consts"
	myerr "dingoscheduler/pkg/error"
	pb "dingoscheduler/pkg/proto/manager"
	"dingoscheduler/pkg/util"

	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/emptypb"
)

var (
	heartGap = 5 * time.Minute
)

type SchedulerService struct {
	pb.UnimplementedManagerServer
	baseData            *data.BaseData
	dingospeedDao       *dao.DingospeedDao
	modelFileRecordDao  *dao.ModelFileRecordDao
	modelFileProcessDao *dao.ModelFileProcessDao
	repositoryDao       *dao.RepositoryDao
	scheudlerLock       sync.Mutex
}

func NewSchedulerService(
	baseData *data.BaseData,
	dingospeedDao *dao.DingospeedDao,
	modelFileRecordDao *dao.ModelFileRecordDao,
	modelFileProcessDao *dao.ModelFileProcessDao,
	repositoryDao *dao.RepositoryDao,
) *SchedulerService {
	return &SchedulerService{
		baseData:            baseData,
		dingospeedDao:       dingospeedDao,
		modelFileRecordDao:  modelFileRecordDao,
		modelFileProcessDao: modelFileProcessDao,
		repositoryDao:       repositoryDao,
	}
}

func (s *SchedulerService) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.RegisterResponse, error) {
	if req.InstanceId == "" || req.Host == "" || req.Port <= 0 {
		return nil, fmt.Errorf("invalid parameter")
	}
	dingospeed := &model.Dingospeed{
		InstanceID: req.InstanceId,
		Host:       req.Host,
		Port:       req.Port,
		Online:     req.Online,
		UpdatedAt:  time.Now(),
	}

	speed, err := s.dingospeedDao.GetEntity(req.InstanceId, req.Online)
	if err != nil {
		zap.S().Errorf("getEntity err.%v", err)
		return nil, err
	}
	if speed != nil {
		dingospeed.ID = speed.ID
		err = s.dingospeedDao.RegisterUpdate(dingospeed)
		if err != nil {
			return nil, err
		}
	} else {
		id, err := s.dingospeedDao.Save(dingospeed)
		if err != nil {
			return nil, err
		}
		dingospeed.ID = int32(id)
	}
	s.updateCache(req.InstanceId, req.Online)
	zap.S().Infof("register success.instanceId:%s, host:%s, port:%d, online:%v", req.InstanceId, req.Host, req.Port, req.Online)
	return &pb.RegisterResponse{
		Success: true,
		Id:      dingospeed.ID,
	}, nil
}

func (s *SchedulerService) Heartbeat(ctx context.Context, req *pb.HeartbeatRequest) (*emptypb.Empty, error) {
	if req.Id > 0 {
		err := s.dingospeedDao.HeartbeatUpdate(req.Id)
		if err != nil {
			return nil, err
		}
		s.updateCache(req.InstanceId, req.Online)
	} else {
		return nil, myerr.New(fmt.Sprintf("speed id is unlawful.id = %d", req.Id))
	}
	return nil, nil
}

func (s *SchedulerService) updateCache(instanceId string, online bool) {
	speedKey := util.GetSpeedKey(instanceId, online)
	if v, ok := s.baseData.Cache.Get(speedKey); ok {
		speed := v.(*model.Dingospeed)
		speed.UpdatedAt = time.Now()
		s.baseData.Cache.Set(speedKey, v, config.SysConfig.GetSpeedExpiration())
	} else {
		if _, err := s.dingospeedDao.GetEntity(instanceId, online); err != nil {
			zap.S().Errorf("GetEntity %s, %v err.%v", instanceId, online, err)
		}
	}
}

func (s *SchedulerService) getOptimumSpeed(instanceId string) *model.Dingospeed {
	speedOnlineKey := util.GetSpeedKey(instanceId, true)
	speedOfflineKey := util.GetSpeedKey(instanceId, false)
	if v, ok := s.baseData.Cache.Get(speedOnlineKey); ok {
		return v.(*model.Dingospeed)
	} else if v, ok = s.baseData.Cache.Get(speedOfflineKey); ok {
		return v.(*model.Dingospeed)
	} else {
		if speed, err := s.dingospeedDao.GetEntity(instanceId, true); err != nil {
			zap.S().Errorf("GetEntity %s err.%v", instanceId, err)
			return nil
		} else {
			return speed
		}
	}
}

func (s *SchedulerService) getApiLock(apiPath string) *sync.RWMutex {
	if val, ok := s.baseData.Cache.Get(apiPath); ok {
		s.baseData.Cache.Set(apiPath, val, config.SysConfig.GetCacheExpiration())
		return val.(*sync.RWMutex)
	}
	s.scheudlerLock.Lock()
	defer s.scheudlerLock.Unlock()
	if val, ok := s.baseData.Cache.Get(apiPath); ok {
		s.baseData.Cache.Set(apiPath, val, config.SysConfig.GetCacheExpiration())
		return val.(*sync.RWMutex)
	}
	newLock := &sync.RWMutex{}
	s.baseData.Cache.Set(apiPath, newLock, config.SysConfig.GetCacheExpiration())
	return newLock
}

func (s *SchedulerService) SchedulerFile(ctx context.Context, req *pb.SchedulerFileRequest) (*pb.SchedulerFileResponse, error) {
	schedulerFilePath := fmt.Sprintf("scheduler/%s/%s/%s/%s", req.DataType, req.Org, req.Repo, req.Etag)
	lock := s.getApiLock(schedulerFilePath)
	lock.Lock()
	defer lock.Unlock()
	record, err := s.modelFileRecordDao.GetModelFileRecord(&query.ModelFileRecordQuery{
		Datatype: req.DataType,
		Org:      req.Org,
		Repo:     req.Repo,
		FileName: req.Name,
		Etag:     req.Etag,
	})
	if err != nil {
		return nil, err
	}
	process := &model.ModelFileProcess{
		InstanceID: req.InstanceId,
	}
	if record != nil {
		var resp = &pb.SchedulerFileResponse{}
		processDtos, err := s.modelFileProcessDao.GetModelFileProcess(record.ID)
		if err != nil {
			return nil, err
		}
		if len(processDtos) > 0 {
			return s.schedulerFileForRecordAndProcess(processDtos, process, record.ID, req)
		} else {
			process.RecordID = record.ID
			process.OffsetNum = 0
			if processId, err := s.modelFileProcessDao.Save(process); err != nil {
				return nil, err
			} else {
				process.ID = processId
			}
			resp = &pb.SchedulerFileResponse{
				SchedulerType: consts.SchedulerNo,
				ProcessId:     process.ID,
			}
		}
		return resp, nil
	} else {
		if processId, err := s.modelFileRecordDao.SaveSchedulerRecord(req, process); err != nil {
			return nil, err
		} else {
			process.ID = processId
		}
		return &pb.SchedulerFileResponse{
			SchedulerType: consts.SchedulerNo,
			ProcessId:     process.ID,
		}, nil
	}
}

func (s *SchedulerService) schedulerFileForRecordAndProcess(processDtos []*dto.ModelFileProcessDto, process *model.ModelFileProcess, recordId int64, req *pb.SchedulerFileRequest) (resp *pb.SchedulerFileResponse, err error) {
	resp = &pb.SchedulerFileResponse{}
	processHistory := make(map[string]*dto.ModelFileProcessDto, 0)
	var masterProcess *dto.ModelFileProcessDto
	for _, item := range processDtos {
		tmp := item
		speed := s.getOptimumSpeed(item.InstanceID)
		if speed != nil {
			tmp.Host = speed.Host
			tmp.Port = speed.Port
			tmp.UpdatedAt = speed.UpdatedAt
		}
		// 标记要同步的process
		if masterProcess == nil && item.InstanceID != req.InstanceId &&
			item.OffsetNum > req.StartPos && time.Now().Sub(item.UpdatedAt) <= heartGap {
			masterProcess = tmp
		}
		if _, ok := processHistory[item.InstanceID]; !ok {
			processHistory[item.InstanceID] = tmp
		}
	}
	if masterProcess != nil {
		resp.SchedulerType = consts.SchedulerYes
		resp.MasterInstanceId = masterProcess.InstanceID
		resp.Host = masterProcess.Host
		resp.Port = masterProcess.Port
		resp.MaxOffset = masterProcess.OffsetNum
		process.MasterInstanceID = masterProcess.InstanceID
	} else {
		resp.SchedulerType = consts.SchedulerNo
		process.MasterInstanceID = ""
	}
	if processDto, ok := processHistory[req.InstanceId]; ok {
		// 存在下载进度，被重新调度要下载
		resp.ProcessId = processDto.ID
		process.ID = processDto.ID
		process.RecordID = processDto.RecordID
		if processDto.OffsetNum > req.StartPos {
			process.OffsetNum = req.StartPos
		} else {
			process.OffsetNum = processDto.OffsetNum
		}
		// 本地缓存被清空，数据库process将重新下载
		if err = s.modelFileProcessDao.ResetProcess(process); err != nil {
			return nil, err
		}
		return resp, nil
	} else {
		process.RecordID = recordId
		if processId, err := s.modelFileProcessDao.Save(process); err != nil {
			return nil, err
		} else {
			process.ID = processId
		}
		resp.ProcessId = process.ID
		return resp, nil
	}
}

func (s *SchedulerService) SyncFileProcess(ctx context.Context, req *pb.SchedulerFileRequest) (*emptypb.Empty, error) {
	record, err := s.modelFileRecordDao.GetModelFileRecord(&query.ModelFileRecordQuery{
		Datatype: req.DataType,
		Org:      req.Org,
		Repo:     req.Repo,
		Etag:     req.Etag,
	})
	if err != nil {
		return nil, err
	}
	process := &model.ModelFileProcess{
		InstanceID: req.InstanceId,
	}
	if record != nil {
		processDto, err := s.modelFileProcessDao.GetModelFileProcessByInstanceId(record.ID, req.InstanceId)
		if err != nil {
			return nil, err
		}
		if processDto != nil {

		} else {
			process.RecordID = record.ID
			process.OffsetNum = req.EndPos
			process.Status = 3 // download complete
			if _, err = s.modelFileProcessDao.Save(process); err != nil {
				return nil, err
			}
		}
		return nil, nil
	} else {
		if _, err = s.modelFileRecordDao.SaveSchedulerRecord(req, process); err != nil {
			return nil, err
		}
		return nil, nil
	}
}

func (s *SchedulerService) ReportFileProcess(ctx context.Context, req *pb.FileProcessRequest) (*emptypb.Empty, error) {
	if err := s.modelFileProcessDao.ReportFileProcess(req); err != nil {
		return nil, err
	}
	return nil, nil
}

func (s *SchedulerService) DeleteByEtagsAndFields(ctx context.Context, req *pb.DeleteByEtagsAndFieldsRequest) (*emptypb.Empty, error) {
	recordIds, err := s.modelFileRecordDao.GetIDsByEtagsOrFields(req.Etag, req.Datatype, req.Org, req.Repo, req.Name)
	if err != nil {
		return nil, fmt.Errorf("查询recordIds失败: %w", err)
	}

	if len(recordIds) > 0 && req.InstanceID != "" {
		_, err := s.modelFileProcessDao.DeleteByRecordIDAndInstanceID(recordIds, req.InstanceID)
		if err != nil {
			return nil, fmt.Errorf("删除ModelFileProcess记录失败: %w", err)
		}
	}

	if req.InstanceID != "" && req.Datatype != "" && req.Org != "" && req.Repo != "" {
		_, err := s.repositoryDao.DeleteByInstanceIdAndDatatypeAndOrgAndRepo(req.InstanceID, req.Datatype, req.Org, req.Repo)
		if err != nil {
			return nil, fmt.Errorf("删除Repository记录失败: %w", err)
		}
	}

	return &emptypb.Empty{}, nil
}
