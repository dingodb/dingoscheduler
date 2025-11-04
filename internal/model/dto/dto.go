package dto

type Repository struct {
	ID           int64    `gorm:"column:id;primaryKey;autoIncrement:true" json:"id"`
	Org          string   `gorm:"column:org;not null" json:"org"`
	OrgRepo      string   `gorm:"column:org_repo;not null" json:"orgRepo"`
	LikeNum      int      `gorm:"column:like_num;not null" json:"likeNum"`
	DownloadNum  int      `gorm:"column:download_num;not null" json:"downloadNum"`
	PipelineTag  string   `gorm:"column:pipeline_tag;not null" json:"pipelineTag"`
	LastModified string   `gorm:"column:last_modified;not null" json:"lastModified"`
	Sha          string   `gorm:"column:sha;not null" json:"sha"`
	Icon         string   `gorm:"column:icon;not null" json:"icon"`
	Tags         []string `gorm:"-" json:"tags,omitempty"`
}

type CacheJobResp struct {
	ID          int64  `gorm:"column:id;primaryKey;autoIncrement:true" json:"id"`
	Type        int32  `gorm:"column:type;not null" json:"type"`
	InstanceId  string `gorm:"column:instance_id;not null" json:"instanceId"`
	Datatype    string `gorm:"column:datatype;not null" json:"datatype"`
	Org         string `gorm:"column:org;not null" json:"org"`
	Repo        string `gorm:"column:repo;not null" json:"repo"`
	UsedStorage int64  `gorm:"column:used_storage;not null" json:"usedStorage"`
	Commit      string `gorm:"column:commit;not null" json:"commit"`
	Status      int32  `gorm:"column:status;not null" json:"status"`
	ErrorMsg    string `gorm:"column:error_msg;not null" json:"errorMsg"`
	CreatedAt   int64  `gorm:"column:created_at;not null;default:CURRENT_TIMESTAMP" json:"createdAt"`
}
