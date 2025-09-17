package dto

type Tag struct {
	ID      string `gorm:"column:id;primaryKey;" json:"id"`
	Label   string `gorm:"column:label;not null" json:"label"`
	Type    string `gorm:"column:type;not null" json:"type"`
	SubType string `gorm:"column:sub_type;not null" json:"sub_type"`
}

type GroupedTagDTO struct {
	SubType string `json:"sub_type"` // 分组的子类型
	Tags    []*Tag `json:"tags"`     // 该子类型下的所有标签
}

type GroupedByTypeDTO struct {
	Type     string `json:"type"` // 分组的类型
	Tags     []*Tag `json:"tags"` // 该类型下的所有标签
	TotalNum int    `json:"totalNum"`
}
