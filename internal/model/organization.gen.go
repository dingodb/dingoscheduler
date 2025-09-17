package model

const TableNameOrganization = "organization"

type Organization struct {
	ID   int64  `gorm:"column:id;primaryKey;autoIncrement:true" json:"id"`
	Name string `gorm:"column:name;not null" json:"name"`
	Icon string `gorm:"column:icon;not null" json:"icon"`
}

// TableName Organization's table name
func (*Organization) TableName() string {
	return TableNameOrganization
}
