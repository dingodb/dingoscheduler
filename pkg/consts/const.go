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

package consts

var RepoTypesMapping = map[string]RepoType{
	"models":   RepoTypeModel,
	"spaces":   RepoTypeSpace,
	"datasets": RepoTypeDataset,
}

// repo类型
type RepoType string

const (
	RepoTypeModel   RepoType = RepoType("models")
	RepoTypeSpace            = RepoType("spaces")
	RepoTypeDataset          = RepoType("datasets")
)

const (
	// 支持的数据库
	DB_MYSQL = "mysql"
)

const (
	SchedulerNo  = 1
	SchedulerYes = 2
)

const PromSource = "source"
const PromOrgRepo = "orgRepo"

const (
	Huggingface        = "huggingface"
	Hfmirror           = "hf-mirror"
	RequestSourceInner = "inner"
)

const (
	StatusDownloading   = 1
	StatusDownloadBreak = 2
	StatusDownloaded    = 3
)

const (
	CacheTypePreheat = 1
	CacheTypeMount   = 2

	StatusCacheJobDefault  = 0
	StatusCacheJobIng      = 1
	StatusCacheJobBreak    = 2
	StatusCacheJobComplete = 3
	StatusCacheJobStopping = 4
)

const (
	RequestTypeHead = "head"
	RequestTypeGet  = "get"
)

const OverseasHfNetLoc = "huggingface.co"
