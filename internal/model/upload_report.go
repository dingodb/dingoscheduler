package model

import "time"

// UploadReportNode fences all reports from older baselines. It survives empty inventories.
type UploadReportNode struct {
	InstanceID     string    `gorm:"primaryKey;size:191" json:"instanceId"`
	Epoch          string    `gorm:"size:64" json:"epoch"`
	PendingEpoch   string    `gorm:"size:64" json:"pendingEpoch,omitempty"`
	BaselineDigest string    `gorm:"size:64" json:"-"`
	Status         string    `gorm:"size:32" json:"status"`
	Error          string    `gorm:"type:text" json:"error,omitempty"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

// RepoHash includes node and the full repository identity; deletion keeps this row.
type UploadReportRepo struct {
	RepoHash   string `gorm:"primaryKey;size:64"`
	InstanceID string `gorm:"size:191;index"`
	Epoch      string `gorm:"size:64"`
	Sequence   uint64
	Digest     string `gorm:"size:64"`
}
