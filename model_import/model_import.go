package main

import (
	"encoding/csv"
	"encoding/hex"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"dingoscheduler/internal/dao"
	"dingoscheduler/internal/data"
	"dingoscheduler/internal/model"
	"dingoscheduler/pkg/config"
	"dingoscheduler/pkg/util"

	"github.com/bytedance/sonic"
	"go.uber.org/zap"
)

// FileInfo 扩展结构用于存储文件解析后的关键信息
type FileInfo struct {
	DataType      string
	Org           string
	Repo          string
	FileName      string // 包含JSON文件的文件夹名称
	Etag          string // 来自Lfs.Oid或Oid
	FileSize      int64  // 来自size字段
	OriginContent []byte // 解码后的content内容
}

type CacheContent struct {
	Content string `json:"content"`
}
type ContentItem struct {
	Type string   `json:"type"`
	Oid  string   `json:"oid"`
	Size int64    `json:"size"`
	Lfs  *LfsInfo `json:"lfs"`
	Path string   `json:"path"`
}
type LfsInfo struct {
	Oid  string `json:"oid"`
	Size int64  `json:"size"`
}

var (
	configPath    string
	repoPathParam string
	instanceID    string // 用户输入的InstanceID
	apiBaseURL    string // API基础地址，用于获取offset
	minFileSize   int64  // 最小文件大小阈值，单位为MB
)

func init() {
	// 解析命令行参数
	flag.StringVar(&configPath, "config", "./config/config.yaml", "配置文件路径")
	flag.StringVar(&repoPathParam, "repoPath", "/Users/shijie/yonyou/dingospeed/repos", "仓库路径")
	flag.StringVar(&instanceID, "instanceId", "mas", "实例ID（必填）")
	flag.StringVar(&apiBaseURL, "apiBase", "http://127.0.0.:8091", "获取offset的API基础地址")
	flag.Int64Var(&minFileSize, "minSize", 0, "最小文件大小阈值，单位为MB，小于此值的文件不录入数据库")
	flag.Parse()

	// 校验必填参数
	if instanceID == "" {
		zap.S().Fatal("必须提供instanceId参数，请使用 -instanceId 选项")
	}

	// 单位转换：MB -> 字节
	minFileSize *= 1024 * 1024
	zap.S().Infof("文件大小过滤阈值设置为: %d MB (%d 字节)", minFileSize/(1024*1024), minFileSize)
}

func main() {
	logger, err := zap.NewDevelopment()
	if err != nil {
		fmt.Printf("初始化zap日志失败: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()
	zap.ReplaceGlobals(logger)

	fileInfos, err := processDirectory(repoPathParam)
	if err != nil {
		zap.S().Fatalf("处理目录失败: %v", err)
	}

	if len(fileInfos) == 0 {
		zap.S().Warn("未找到任何符合条件的文件进行处理")
		return
	}
	zap.S().Infof("本次从路径 %s 读取到 %d 个符合条件的文件", repoPathParam, len(fileInfos))

	filteredFileInfos := make([]FileInfo, 0)
	for _, fileInfo := range fileInfos {
		if fileInfo.FileSize >= minFileSize {
			filteredFileInfos = append(filteredFileInfos, fileInfo)
		} else {
			zap.S().Debugf("文件 %s 大小为 %d 字节，小于阈值 %d 字节，将被过滤",
				fileInfo.FileName, fileInfo.FileSize, minFileSize)
		}
	}

	zap.S().Infof("过滤后，本次需处理的文件剩余 %d 个（大于等于 %d MB）",
		len(filteredFileInfos), minFileSize/(1024*1024))

	if len(filteredFileInfos) == 0 {
		zap.S().Warn("所有文件都小于指定的大小阈值，没有文件将被处理")
		return
	}

	allCandidateRecords := make([]model.ModelFileRecord, 0, len(filteredFileInfos))
	for _, item := range filteredFileInfos {
		m := model.ModelFileRecord{
			Datatype: item.DataType,
			Org:      item.Org,
			Repo:     item.Repo,
			Name:     item.FileName,
			Etag:     item.Etag,
			FileSize: item.FileSize,
		}
		allCandidateRecords = append(allCandidateRecords, m)
	}

	conf, err := config.Scan(configPath)
	if err != nil {
		zap.S().Fatalf("读取配置文件失败: %v", err)
	}
	baseData, _, err := data.NewBaseData(conf)
	if err != nil {
		zap.S().Fatalf("初始化基础数据失败: %v", err)
	}

	modelFileRecordDao := dao.NewModelFileRecordDao(baseData)

	const batchSize = 1000
	totalBatches := (len(allCandidateRecords) + batchSize - 1) / batchSize
	existingRecords := make([]model.ModelFileRecord, 0)

	zap.S().Infof("开始分批次查询记录，共%d条记录，分为%d批处理",
		len(allCandidateRecords), totalBatches)

	for i := 0; i < totalBatches; i++ {
		start := i * batchSize
		end := start + batchSize
		if end > len(allCandidateRecords) {
			end = len(allCandidateRecords)
		}

		batchRecords := allCandidateRecords[start:end]
		zap.S().Debugf("处理第%d/%d批记录查询，处理范围：%d-%d，数量：%d",
			i+1, totalBatches, start, end-1, len(batchRecords))

		batchExisting, err := modelFileRecordDao.ExistRecords(batchRecords)
		if err != nil {
			zap.S().Fatalf("查询第%d批记录失败: %v", i+1, err)
		}

		existingRecords = append(existingRecords, batchExisting...)
	}

	zap.S().Infof("所有批次记录查询完成，共找到%d个已存在的记录", len(existingRecords))
	existingRecordMap := make(map[string]struct{}, len(existingRecords))
	for _, r := range existingRecords {
		key := fmt.Sprintf("%s-%s-%s-%s", r.Etag, r.Name, r.Org, r.Repo)
		existingRecordMap[key] = struct{}{}
	}

	newRecords := make([]model.ModelFileRecord, 0)
	for _, r := range allCandidateRecords {
		key := fmt.Sprintf("%s-%s-%s-%s", r.Etag, r.Name, r.Org, r.Repo)
		if _, exists := existingRecordMap[key]; !exists {
			newRecords = append(newRecords, r)
		} else {
			zap.S().Infof("记录已存在 (etag: %s, name: %s, org: %s, repo: %s)，无需重复添加",
				r.Etag, r.Name, r.Org, r.Repo)
		}
	}

	if len(newRecords) > 0 {
		recordFile, err := os.OpenFile(
			"model_file_record.csv",
			os.O_CREATE|os.O_WRONLY|os.O_TRUNC,
			0644,
		)
		if err != nil {
			zap.S().Fatalf("创建ModelFileRecord CSV文件失败: %v", err)
		}
		defer func() {
			recordFile.Close()
			zap.S().Debugf("ModelFileRecord CSV文件句柄已关闭")
		}()

		recordWriter := csv.NewWriter(recordFile)
		recordWriter.Comma = ','
		recordWriter.UseCRLF = true
		defer func() {
			recordWriter.Flush()
			if err := recordWriter.Error(); err != nil {
				zap.S().Errorf("刷新ModelFileRecord CSV缓存失败: %v", err)
			}
		}()

		recordHeader := []string{"datatype", "org", "repo", "name", "etag", "file_size"}
		if err := recordWriter.Write(recordHeader); err != nil {
			zap.S().Fatalf("写入ModelFileRecord CSV表头失败: %v", err)
		}

		for _, record := range newRecords {
			dataRow := []string{
				record.Datatype,
				record.Org,
				record.Repo,
				record.Name,
				record.Etag,
				strconv.FormatInt(record.FileSize, 10),
			}
			if err := recordWriter.Write(dataRow); err != nil {
				zap.S().Fatalf("写入ModelFileRecord数据失败（Etag: %s）: %v", record.Etag, err)
			}
		}

		zap.S().Infof("成功生成ModelFileRecord CSV文件，包含 %d 条新记录: model_file_record.csv", len(newRecords))
		zap.S().Infof("=== MySQL导入命令参考 ===")
		zap.S().Infof("LOAD DATA INFILE '/path/to/model_file_record.csv'")
		zap.S().Infof("INTO TABLE model_file_record")
		zap.S().Infof("FIELDS TERMINATED BY ',' ENCLOSED BY '\"' ESCAPED BY '\\\\'")
		zap.S().Infof("LINES TERMINATED BY '\\r\\n'")
		zap.S().Infof("IGNORE 1 ROWS (datatype, org, repo, name, etag, file_size);")
		zap.S().Infof("==========================")
	} else {
		zap.S().Info("本次读取到的记录均已存在于ModelFileRecord，无需生成CSV文件")
	}
}

func processDirectory(rootPath string) ([]FileInfo, error) {
	var result []FileInfo

	if exists, err := fileExists(rootPath); err != nil {
		zap.S().Errorf("检查根目录存在性失败: %v", err)
		return nil, err
	} else if !exists {
		zap.S().Errorf("根目录不存在: %s", rootPath)
		return nil, os.ErrNotExist
	}

	err := filepath.WalkDir(rootPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			zap.S().Warnf("访问路径时出错 %s: %v", path, err)
			return nil
		}

		if path == rootPath {
			return nil
		}

		relPath, err := filepath.Rel(rootPath, path)
		if err != nil {
			zap.S().Warnf("获取相对路径失败 %s: %v", path, err)
			return nil
		}

		components := strings.Split(relPath, string(filepath.Separator))
		if len(components) < 5 ||
			components[0] != "api" ||
			!(components[1] == "models" || components[1] == "datasets" || components[1] == "spaces") ||
			components[4] != "paths-info" {
			return nil
		}

		if d.IsDir() {
			jsonFilePath := filepath.Join(path, "paths-info_post.json")
			if exists, err := fileExists(jsonFilePath); err != nil {
				zap.S().Warnf("检查文件存在性失败 %s: %v", jsonFilePath, err)
				return nil
			} else if exists {
				zap.S().Debugf("找到符合条件的文件: %s", jsonFilePath)
				var pathSegments []string
				if len(components) >= 7 {
					pathSegments = components[6:]
				} else {
					pathSegments = []string{}
				}

				fullPath := filepath.Join(pathSegments...)
				fileInfo, err := processJsonFile(
					jsonFilePath,
					components[1],
					components[2],
					components[3],
					fullPath,
				)
				if err != nil {
					zap.S().Warnf("处理JSON文件失败 %s: %v", jsonFilePath, err)
					return nil
				}

				if fileInfo != nil {
					result = append(result, *fileInfo)
				}
			}
		}

		return nil
	})

	return result, err
}

func processJsonFile(jsonPath, dataType, org, repo, fileName string) (*FileInfo, error) {
	bytes, err := util.ReadFileToBytes(jsonPath)
	if err != nil {
		return nil, err
	}

	var cacheContent CacheContent
	if err = sonic.Unmarshal(bytes, &cacheContent); err != nil {
		return nil, err
	}

	decodeByte, err := hex.DecodeString(cacheContent.Content)
	if err != nil {
		return nil, err
	}

	var contentItems []ContentItem
	if err = sonic.Unmarshal(decodeByte, &contentItems); err != nil {
		return nil, err
	}

	if len(contentItems) == 0 {
		zap.S().Warnf("JSON文件 %s 中的content数组为空", jsonPath)
		return nil, nil
	}

	item := contentItems[0]
	etag := item.Oid
	if item.Lfs != nil && item.Lfs.Oid != "" {
		etag = item.Lfs.Oid
	}

	return &FileInfo{
		DataType:      dataType,
		Org:           org,
		Repo:          repo,
		FileName:      fileName,
		Etag:          etag,
		FileSize:      item.Size,
		OriginContent: decodeByte,
	}, nil
}

func fileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}
