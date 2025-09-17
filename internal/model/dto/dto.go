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
