package model

import "time"

// UploadInventoryState is the accepted complete-inventory watermark for one
// Speed. It is deliberately separate from dingospeed/repository: heartbeat
// availability and uploaded-file truth have different lifecycles.
type UploadInventoryState struct {
	InstanceID        string     `gorm:"column:instance_id;primaryKey;size:191" json:"instanceId"`
	Epoch             string     `gorm:"column:epoch;size:64;not null" json:"epoch"`
	EpochStartedAt    time.Time  `gorm:"column:epoch_started_at;not null" json:"epochStartedAt"`
	LastSequence      uint64     `gorm:"column:last_sequence;not null" json:"lastSequence"`
	InventoryComplete bool       `gorm:"column:inventory_complete;not null" json:"inventoryComplete"`
	LastAttemptAt     time.Time  `gorm:"column:last_attempt_at;not null" json:"lastAttemptAt"`
	LastConfirmedAt   *time.Time `gorm:"column:last_confirmed_at" json:"lastConfirmedAt,omitempty"`
	ErrorMessage      string     `gorm:"column:error_message;type:text" json:"errorMessage,omitempty"`
}

func (*UploadInventoryState) TableName() string { return "upload_inventory_state" }

// UploadInventoryFile is an uploaded-domain identity. IdentityHash avoids a
// MySQL oversized composite index while the individual identity columns remain
// queryable and human-readable.
type UploadInventoryFile struct {
	ID           int64     `gorm:"column:id;primaryKey;autoIncrement" json:"id,string"`
	IdentityHash string    `gorm:"column:identity_hash;size:64;not null;uniqueIndex" json:"-"`
	Namespace    string    `gorm:"column:namespace;size:255;not null;index:idx_upload_repo" json:"namespace"`
	RepoType     string    `gorm:"column:repo_type;size:32;not null;index:idx_upload_repo" json:"repoType"`
	Repo         string    `gorm:"column:repo;size:1024;not null" json:"repo"`
	Path         string    `gorm:"column:path;size:1000;not null" json:"path"`
	SHA256       string    `gorm:"column:sha256;size:64;not null;index" json:"sha256"`
	Size         int64     `gorm:"column:size;not null" json:"size"`
	CreatedAt    time.Time `gorm:"column:created_at;not null;autoCreateTime" json:"createdAt"`
	UpdatedAt    time.Time `gorm:"column:updated_at;not null;autoUpdateTime" json:"updatedAt"`
}

func (*UploadInventoryFile) TableName() string { return "upload_inventory_file" }

type UploadInventoryHolding struct {
	ID          int64     `gorm:"column:id;primaryKey;autoIncrement" json:"id,string"`
	FileID      int64     `gorm:"column:file_id;not null;uniqueIndex:uk_upload_holding;index" json:"fileId,string"`
	InstanceID  string    `gorm:"column:instance_id;size:191;not null;uniqueIndex:uk_upload_holding;index" json:"instanceId"`
	Sequence    uint64    `gorm:"column:sequence;not null" json:"sequence"`
	ConfirmedAt time.Time `gorm:"column:confirmed_at;not null" json:"confirmedAt"`
}

func (*UploadInventoryHolding) TableName() string { return "upload_inventory_holding" }
