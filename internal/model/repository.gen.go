package model

import (
	"time"
)

const TableNameRepository = "repository"

// PreheatJob mapped from table <preheat_job>
type Repository struct {
	ID            int64     `gorm:"column:id;primaryKey;autoIncrement:true" json:"id"`
	InstanceId    string    `gorm:"column:instance_id;not null" json:"instance_id"`
	Datatype      string    `gorm:"column:datatype;not null" json:"datatype"`
	Org           string    `gorm:"column:org;not null" json:"org"`
	Repo          string    `gorm:"column:repo;not null" json:"repo"`
	OrgRepo       string    `gorm:"column:org_repo;not null" json:"org_repo"`
	LikeNum       int       `gorm:"column:like_num;not null" json:"like_num"`
	DownloadNum   int       `gorm:"column:download_num;not null" json:"download_num"`
	PipelineTagId string    `gorm:"column:pipeline_tag_id;not null" json:"pipeline_tag_id"`
	PipelineTag   string    `gorm:"column:pipeline_tag;not null" json:"pipeline_tag"`
	LastModified  string    `gorm:"column:last_modified;not null" json:"last_modified"`
	UsedStorage   int64     `gorm:"column:used_storage;" json:"used_storage"`
	Sha           string    `gorm:"column:sha;not null" json:"sha"`
	Status        int32     `gorm:"column:status;not null" json:"status"`
	ErrorMsg      string    `gorm:"column:error_msg;not null" json:"error_msg"`
	CreatedAt     time.Time `gorm:"column:created_at;not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt     time.Time `gorm:"column:updated_at;not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
}

// TableName PreheatJob's table name
func (*Repository) TableName() string {
	return TableNameRepository
}
