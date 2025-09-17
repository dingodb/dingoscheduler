package model

const TableNameRepositoryTag = "repository_tag"

// PreheatJob mapped from table <preheat_job>
type RepositoryTag struct {
	ID     int64  `gorm:"column:id;primaryKey;autoIncrement:true" json:"id"`
	RepoId int64  `gorm:"column:repo_id;not null" json:"repo_id"`
	TagId  string `gorm:"column:tag_id;not null" json:"tag_id"`
}

// TableName PreheatJob's table name
func (*RepositoryTag) TableName() string {
	return TableNameRepositoryTag
}
