package model

// NodeEndpoint extends registration without changing legacy inventory tables.
type NodeEndpoint struct {
	NodeID        int32  `gorm:"primaryKey" json:"-"`
	ManagementURL string `gorm:"size:2048" json:"managementUrl"`
	DownloadURL   string `gorm:"size:2048" json:"downloadUrl"`
}
