package service

import (
	"path/filepath"
	"time"

	"dingoscheduler/internal/dao"
	"dingoscheduler/internal/model"
	"dingoscheduler/pkg/config"
	"dingoscheduler/pkg/util"

	"go.uber.org/zap"
)

type OrganizationService struct {
	organizationDao *dao.OrganizationDao
}

func NewOrganizationService(organizationDao *dao.OrganizationDao) *OrganizationService {
	return &OrganizationService{
		organizationDao: organizationDao,
	}
}

func (o *OrganizationService) PersistOrgLogo(org string) error {
	icon, err := o.organizationDao.GetOrganization(org)
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
	if err = o.organizationDao.SaveOrgBySql(orgEntity); err != nil {
		zap.S().Errorf("处理repo [%s] 失败：插入organization表错误，%v，跳过", org, err)
		return err
	}
	return nil
}
