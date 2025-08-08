package dto

import "time"

type ModelFileProcessDto struct {
	ID         int64     `gorm:"column:id;primaryKey;autoIncrement:true" json:"id"`
	RecordID   int64     `gorm:"column:record_id;not null" json:"record_id"`
	InstanceID string    `gorm:"column:instance_id;not null" json:"instance_id"`
	OffsetNum  int64     `gorm:"column:offset_num;not null" json:"offset_num"`
	Host       string    `gorm:"column:host;not null" json:"host"`
	Port       int32     `gorm:"column:port;not null" json:"port"`
	UpdatedAt  time.Time `gorm:"column:updated_at" json:"updated_at"`
}
