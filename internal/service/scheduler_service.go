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
	"time"

	"dingoscheduler/internal/dao"
	"dingoscheduler/internal/model"
	"dingoscheduler/internal/model/dto"
	"dingoscheduler/internal/model/query"
	"dingoscheduler/pkg/consts"
	myerr "dingoscheduler/pkg/error"
	pb "dingoscheduler/pkg/proto/manager"

	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/emptypb"
)

var heartGap = 5 * time.Minute

type SchedulerService struct {
	pb.UnimplementedManagerServer
	dingospeedDao       *dao.DingospeedDao
	modelFileRecordDao  *dao.ModelFileRecordDao
	modelFileProcessDao *dao.ModelFileProcessDao
}

func NewSchedulerService(dingospeedDao *dao.DingospeedDao, modelFileRecordDao *dao.ModelFileRecordDao, modelFileProcessDao *dao.ModelFileProcessDao) *SchedulerService {
	return &SchedulerService{
		dingospeedDao:       dingospeedDao,
		modelFileRecordDao:  modelFileRecordDao,
		modelFileProcessDao: modelFileProcessDao,
	}
}

func (s *SchedulerService) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.RegisterResponse, error) {
	if req.InstanceId == "" || req.Host == "" || req.Port <= 0 {
		return nil, fmt.Errorf("invalid parameter")
	}
	dingospeed := model.Dingospeed{
		InstanceID: req.InstanceId,
		Host:       req.Host,
		Port:       req.Port,
		Online:     req.Online,
	}
	speed, err := s.dingospeedDao.GetEntity(req.InstanceId, req.Online)
	if err != nil {
		zap.S().Errorf("getEntity err.%v", err)
		return nil, err
	}
	if speed != nil {
		dingospeed.ID = speed.ID
		err = s.dingospeedDao.RegisterUpdate(&dingospeed)
		if err != nil {
			return nil, err
		}
	} else {
		err = s.dingospeedDao.Save(&dingospeed)
		if err != nil {
			return nil, err
		}
	}
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
	} else {
		return nil, myerr.New(fmt.Sprintf("speed id is unlawful.id = %d", req.Id))
	}
	return nil, nil
}

func (s *SchedulerService) SchedulerFile(ctx context.Context, req *pb.SchedulerFileRequest) (*pb.SchedulerFileResponse, error) {
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
		var resp = &pb.SchedulerFileResponse{}
		processDtos, err := s.modelFileProcessDao.GetModelFileProcess(record.ID)
		if err != nil {
			return nil, err
		}
		if len(processDtos) > 0 {
			processHistory := make(map[string]*dto.ModelFileProcessDto, 0)
			var masterProcess *dto.ModelFileProcessDto
			for _, item := range processDtos {
				// 标记要同步的process
				tmp := item
				if masterProcess == nil && item.InstanceID != req.InstanceId &&
					item.OffsetNum > req.StartPos && time.Now().Sub(item.UpdatedAt) <= heartGap {
					masterProcess = tmp
				}
				processHistory[item.InstanceID] = tmp
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
				process.RecordID = record.ID
				if err = s.modelFileProcessDao.Save(process); err != nil {
					return nil, err
				}
				resp.ProcessId = process.ID
				return resp, nil
			}
		} else {
			process.RecordID = record.ID
			process.OffsetNum = 0
			if err = s.modelFileProcessDao.Save(process); err != nil {
				return nil, err
			}
			resp = &pb.SchedulerFileResponse{
				SchedulerType: consts.SchedulerNo,
				ProcessId:     process.ID,
			}
		}
		return resp, nil
	} else {
		if err = s.modelFileRecordDao.SaveSchedulerRecord(req, process); err != nil {
			return nil, err
		}
		return &pb.SchedulerFileResponse{
			SchedulerType: consts.SchedulerNo,
			ProcessId:     process.ID,
		}, nil
	}
}

func (s *SchedulerService) ReportFileProcess(ctx context.Context, req *pb.FileProcessRequest) (*emptypb.Empty, error) {
	if err := s.modelFileProcessDao.ReportFileProcess(req); err != nil {
		return nil, err
	}
	return nil, nil
}
