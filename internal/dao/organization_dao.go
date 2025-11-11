package dao

import (
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"dingoscheduler/internal/data"
	"dingoscheduler/internal/model"
	"dingoscheduler/pkg/config"
	"dingoscheduler/pkg/util"

	"go.uber.org/zap"
)

var mu sync.Mutex

type OrganizationDao struct {
	baseData *data.BaseData
}

func NewOrganizationDao(data *data.BaseData) *OrganizationDao {
	return &OrganizationDao{
		baseData: data,
	}
}

// organizationDao的相关方法示例
func (o *OrganizationDao) ExistsByField(field, value string) (bool, error) {
	var count int64
	err := o.baseData.BizDB.Table("organization").Where(field+" = ?", value).Count(&count).Error
	return count > 0, err
}

func (o *OrganizationDao) Insert(org *model.Organization) error {
	return o.baseData.BizDB.Table("organization").Create(org).Error
}

func (o *OrganizationDao) UpdateByField(field, value string, org *model.Organization) error {
	return o.baseData.BizDB.Table("organization").Where(field+" = ?", value).Updates(org).Error
}

func (o *OrganizationDao) GetOrganization(orgName string) (string, error) {
	orgKey := util.GetOrgNameKey(orgName)
	if v, ok := o.baseData.Cache.Get(orgKey); ok {
		o.baseData.Cache.Set(orgKey, v, config.SysConfig.GetDefaultExpiration())
		return v.(string), nil
	}
	mu.Lock()
	defer mu.Unlock()
	if v, ok := o.baseData.Cache.Get(orgKey); ok {
		o.baseData.Cache.Set(orgKey, v, config.SysConfig.GetDefaultExpiration())
		return v.(string), nil
	}
	orgs := make([]*model.Organization, 0)
	if err := o.baseData.BizDB.Table("organization").Find(&orgs).Error; err != nil {
		return "", err
	}
	for _, org := range orgs {
		o.baseData.Cache.Set(util.GetOrgNameKey(org.Name), org.Icon, config.SysConfig.GetDefaultExpiration())
	}
	if v, ok := o.baseData.Cache.Get(orgKey); ok {
		return v.(string), nil
	}
	return "", nil
}

func (o *OrganizationDao) FindAllNames() ([]string, error) {
	var names []string
	err := o.baseData.BizDB.Table("organization").Pluck("name", &names).Error
	if err != nil {
		zap.S().Errorf("查询organization表name字段失败：%v", err)
		return nil, err
	}
	return names, nil
}

func (o *OrganizationDao) SaveOrgBySql(org *model.Organization) error {
	orgSql := fmt.Sprintf("INSERT INTO organization(name, icon) VALUES ('%s','%s')", org.Name, org.Icon)
	if err := o.baseData.BizDB.Exec(orgSql).Error; err != nil {
		return err
	}
	orgKey := util.GetOrgNameKey(org.Name)
	o.baseData.Cache.Delete(orgKey)
	return nil
}

func (o *OrganizationDao) PersistOrgLogo(org string) error {
	icon, err := o.GetOrganization(org)
	if err != nil {
		return err
	}
	if icon != "" {
		return nil
	}
	avatarURL, err := util.FetchAvatarURL(org)
	if err != nil {
		zap.S().Errorf("处理repo [%s] 失败：获取头像URL错误，%v，跳过", org, err)
		return err
	}
	ossOption := &util.ImageUploadOption{
		Region:  config.SysConfig.Oss.Region,
		Timeout: 15 * time.Second,
	}
	fullOssObjectKey, err := util.DownloadAvatar(
		avatarURL,
		config.SysConfig.Avatar.Path,
		org,
		config.SysConfig.Oss.BucketName,
		ossOption,
	)
	if err != nil {
		zap.S().Errorf("处理repo [%s] 失败：头像上传OSS错误，%v，跳过", org, err)
		return err
	}
	onlyFileName := filepath.Base(fullOssObjectKey)
	orgEntity := &model.Organization{
		Name: org,
		Icon: onlyFileName,
	}
	if err = o.SaveOrgBySql(orgEntity); err != nil {
		zap.S().Errorf("处理repo [%s] 失败：插入organization表错误，%v，跳过", org, err)
		return err
	}
	return nil
}
