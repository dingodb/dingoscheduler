package main

import (
	"flag"
	"os"
	"path/filepath" // 新增：用于提取文件名
	"time"

	"dingoscheduler/internal/dao"
	"dingoscheduler/internal/data"
	"dingoscheduler/internal/model"
	"dingoscheduler/pkg/config"
	"dingoscheduler/pkg/util"

	"go.uber.org/zap"
)

var (
	configPath string
)

func init() {
	flag.StringVar(&configPath, "config", "config/config.yaml", "配置文件路径")
}

func main() {
	flag.Parse()
	conf, err := config.Scan(configPath)
	if err != nil {
		zap.S().Fatalf("读取配置文件失败：%v", err)
	}
	zap.S().Infof("成功读取配置文件，路径：%s", configPath)

	imageSavePath := conf.Avatar.Path
	if imageSavePath == "" {
		zap.S().Fatalf("请在配置文件中指定头像本地暂存路径（Avatar.Path）")
	}
	if err := os.MkdirAll(imageSavePath, 0755); err != nil {
		zap.S().Fatalf("创建本地暂存目录失败：%v", err)
	}

	ossBucketName := conf.Oss.BucketName
	if ossBucketName == "" {
		zap.S().Fatalf("请在配置文件中指定OSS桶名（OSS.BucketName）")
	}

	zap.S().Infof("初始化完成：本地暂存路径=%s，OSS桶名=%s", imageSavePath, ossBucketName)
	baseData, cleanup, err := data.NewBaseData(conf)
	if err != nil {
		zap.S().Fatalf("初始化数据库连接失败：%v", err)
	}
	defer func() {
		cleanup()
		zap.S().Infof("程序退出，已释放数据库连接")
	}()
	zap.S().Infof("数据库连接初始化成功")

	modelFileRecordDao := dao.NewModelFileRecordDao(baseData)
	organizationDao := dao.NewOrganizationDao(baseData)
	zap.S().Infof("DAO层初始化完成（ModelFileRecordDao、OrganizationDao）")
	orgs, err := modelFileRecordDao.FindDistinctOrgs()
	if err != nil {
		zap.S().Fatalf("从model_file_record表查询去重repo失败：%v", err)
	}
	zap.S().Infof("共查询到 %d 个去重repo记录", len(orgs))
	if len(orgs) == 0 {
		zap.S().Warnf("未查询到任何repo记录，无需处理")
		return
	}

	existingNames, err := organizationDao.FindAllNames()
	if err != nil {
		zap.S().Fatalf("从organization表获取已存在名称失败：%v", err)
	}
	existingOrgMap := make(map[string]bool, len(existingNames))
	for _, name := range existingNames {
		existingOrgMap[name] = true
	}
	zap.S().Infof("从organization表查询到 %d 个已存在组织", len(existingOrgMap))

	successCount := 0
	ossOption := &util.ImageUploadOption{
		Region:  conf.Oss.Region,
		Timeout: 15 * time.Second,
	}
	for idx, org := range orgs {
		if org == "" {
			zap.S().Warnf("处理第 %d/%d 个repo：空repo，跳过", idx+1, len(orgs))
			continue
		}

		if existingOrgMap[org] {
			zap.S().Infof("处理第 %d/%d 个repo：%s 已存在于organization表，跳过", idx+1, len(orgs), org)
			continue
		}

		zap.S().Infof("开始处理第 %d/%d 个repo：%s", idx+1, len(orgs), org)
		avatarURL, err := util.FetchAvatarURL(org)
		if err != nil {
			zap.S().Errorf("处理repo [%s] 失败：获取头像URL错误，%v，跳过", org, err)
			continue
		}

		zap.S().Infof("org [%s] 成功获取头像URL：%s", org, avatarURL)
		fullOssObjectKey, err := util.DownloadAvatar(
			avatarURL,
			imageSavePath,
			org,
			ossBucketName,
			ossOption,
		)
		if err != nil {
			zap.S().Errorf("处理repo [%s] 失败：头像上传OSS错误，%v，跳过", org, err)
			continue
		}

		onlyFileName := filepath.Base(fullOssObjectKey)
		zap.S().Infof("org [%s] 头像上传OSS成功：完整路径=%s，提取文件名=%s",
			org, fullOssObjectKey, onlyFileName)

		org := &model.Organization{
			Name: org,
			Icon: onlyFileName,
		}

		if err := organizationDao.SaveOrgBySql(org); err != nil {
			zap.S().Errorf("处理repo [%s] 失败：插入organization表错误，%v，跳过", org, err)
			continue
		}
		successCount++
		zap.S().Infof("org [%s] 处理成功：已插入organization表（name=%s, 存入数据库的头像名=%s）",
			org, org, onlyFileName)
	}

	zap.S().Infof("所有repo处理完成！总数量：%d，成功插入：%d，跳过（空/已存在）：%d",
		len(orgs), successCount, len(orgs)-successCount)
}
