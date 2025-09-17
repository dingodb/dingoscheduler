package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"

	"dingoscheduler/internal/dao"
	"dingoscheduler/internal/data"
	"dingoscheduler/internal/model"
	"dingoscheduler/pkg/config"

	"go.uber.org/zap"
)

// Tag 通用标签结构体（增加subType字段）
type Tag struct {
	Type    string `json:"type"`
	Label   string `json:"label"`
	ID      string `json:"id"`
	SubType string `json:"subType"`
}

type AllTagsResponse map[string][]Tag

func readBody(body io.ReadCloser) string {
	defer body.Close()
	content, err := io.ReadAll(body)
	if err != nil {
		return "读取响应体失败：" + err.Error()
	}
	return string(content)
}

func syncAllTags(modelTagDao *dao.TagDao) error {
	apiURL := "https://hf-mirror.com/api/models-tags-by-type"

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return fmt.Errorf("创建HTTP请求失败：%w", err)
	}
	req.Header.Set("User-Agent", "Go-Tag-Sync/1.0")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("发送HTTP请求失败：%w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyContent := readBody(resp.Body)
		return fmt.Errorf("API返回非成功状态码：%d，响应内容：%s", resp.StatusCode, bodyContent)
	}

	var allTags AllTagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&allTags); err != nil {
		return fmt.Errorf("解析JSON响应失败：%w", err)
	}

	if len(allTags) == 0 {
		zap.S().Warn("从API获取到的标签为空，无需写入数据库")
		return nil
	}
	zap.S().Infof("成功从API获取到%d种类型的标签", len(allTags))

	var allDbTags []*model.Tag
	for tagType, tags := range allTags {
		zap.S().Infof("开始处理类型为[%s]的标签，共%d个", tagType, len(tags))
		for _, apiTag := range tags {
			if apiTag.Type == "" {
				apiTag.Type = tagType
				zap.S().Debugf("标签[id=%s]类型为空，使用外层类型[%s]", apiTag.ID, tagType)
			}

			dbTag := &model.Tag{
				ID:      apiTag.ID,
				Label:   apiTag.Label,
				Type:    apiTag.Type,
				SubType: apiTag.SubType,
			}

			allDbTags = append(allDbTags, dbTag)
		}
	}

	if len(allDbTags) > 0 {
		zap.S().Infof("准备批量插入%d个标签", len(allDbTags))
		if err := modelTagDao.CreateBatch(allDbTags); err != nil {
			return fmt.Errorf("批量插入标签失败：%w", err)
		}
		zap.S().Infof("批量插入标签成功，共插入%d个标签", len(allDbTags))
	} else {
		zap.S().Warn("没有需要插入的标签数据")
	}

	return nil
}

var (
	configPath string
)

func init() {
	flag.StringVar(&configPath, "config", "/Users/shijie/yonyou/dingoscheduler/config/config.yaml", "配置文件路径")
}

func main() {
	logger, err := zap.NewDevelopment()
	if err != nil {
		fmt.Printf("初始化zap日志失败: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()
	zap.ReplaceGlobals(logger)

	flag.Parse()

	if len(os.Args) > 1 {
		configPath = os.Args[1]
	}

	conf, err := config.Scan(configPath)
	if err != nil {
		zap.S().Fatalf("读取配置文件失败：%v，路径：%s", err, configPath)
	}
	zap.S().Infof("成功读取配置文件，路径：%s", configPath)

	baseData, cleanup, err := data.NewBaseData(conf)
	if err != nil {
		zap.S().Fatalf("初始化数据库连接失败：%v", err)
	}

	defer func() {
		cleanup()
		zap.S().Infof("程序退出，已释放数据库连接")
	}()
	zap.S().Infof("数据库连接初始化成功")

	modelTagDao := dao.NewTagDao(baseData)
	zap.S().Infof("TagDao初始化完成，开始同步所有类型标签")

	if err := syncAllTags(modelTagDao); err != nil {
		zap.S().Fatalf("标签同步任务失败：%v", err)
	}
	zap.S().Infof("所有类型标签同步任务全部完成！")
}
