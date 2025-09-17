package main

import (
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"dingoscheduler/internal/dao"
	"dingoscheduler/internal/data"
	"dingoscheduler/internal/model"
	"dingoscheduler/pkg/config"
	"dingoscheduler/pkg/util"

	"github.com/bytedance/sonic"
	"go.uber.org/zap"
)

type FileInfo struct {
	DataType      string
	Org           string
	Repo          string
	FileName      string
	Etag          string
	FileSize      int64
	OriginContent []byte
}

type offsetResult struct {
	RecordID int64
	Offset   int64
	Err      error
}

var (
	configPath     string
	repoPathParam  string
	instanceID     string
	apiBaseURL     string
	minFileSize    int64
	maxConcurrency = 50
)

func init() {
	flag.StringVar(&configPath, "config", "./config/config.yaml", "配置文件路径")
	flag.StringVar(&repoPathParam, "repoPath", "/Users/shijie/yonyou/dingospeed/repos", "仓库路径")
	flag.StringVar(&instanceID, "instanceId", "mas", "实例ID（必填）")
	flag.StringVar(&apiBaseURL, "apiBase", "http://127.0.0.1:8091", "获取offset的API基础地址（修复格式错误）")
	flag.Int64Var(&minFileSize, "minSize", 0, "最小文件大小阈值（MB），小于此值的文件不录入")
	flag.IntVar(&maxConcurrency, "concurrency", 50, "获取offset的最大并发数（建议50-200）")
	flag.Parse()

	if instanceID == "" {
		zap.S().Fatal("必须提供 -instanceId 参数")
	}

	minFileSize *= 1024 * 1024 // 转换为字节
	zap.S().Infof("文件大小阈值：%d MB（%d 字节），并发数：%d", minFileSize/(1024*1024), minFileSize, maxConcurrency)
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
		zap.S().Warn("未找到符合条件的文件")
		return
	}
	zap.S().Infof("从 %s 读取到 %d 个文件", repoPathParam, len(fileInfos))

	filteredFileInfos := make([]FileInfo, 0)
	for _, fi := range fileInfos {
		if fi.FileSize >= minFileSize {
			filteredFileInfos = append(filteredFileInfos, fi)
		} else {
			zap.S().Debugf("文件 %s（%d 字节）小于阈值，过滤", fi.FileName, fi.FileSize)
		}
	}
	if len(filteredFileInfos) == 0 {
		zap.S().Warn("所有文件均小于阈值，无需处理")
		return
	}
	zap.S().Infof("过滤后剩余 %d 个文件（≥%d MB）", len(filteredFileInfos), minFileSize/(1024*1024))

	etagSet := make(map[string]struct{})
	for _, fi := range filteredFileInfos {
		if fi.Etag != "" {
			etagSet[fi.Etag] = struct{}{}
		} else {
			zap.S().Debugf("文件 %s ETag为空，跳过", fi.FileName)
		}
	}
	etags := make([]string, 0, len(etagSet))
	for etag := range etagSet {
		etags = append(etags, etag)
	}
	if len(etags) == 0 {
		zap.S().Warn("无有效ETag，程序退出")
		return
	}
	zap.S().Infof("提取到 %d 个唯一ETag", len(etags))

	conf, err := config.Scan(configPath)
	if err != nil {
		zap.S().Fatalf("读取配置失败: %v", err)
	}
	baseData, _, err := data.NewBaseData(conf)
	if err != nil {
		zap.S().Fatalf("初始化基础数据失败: %v", err)
	}

	modelFileRecordDao := dao.NewModelFileRecordDao(baseData)
	batchSize := 100
	totalCount := len(etags)
	totalBatches := (totalCount + batchSize - 1) / batchSize
	allRecords := make([]model.ModelFileRecord, 0)

	zap.S().Infof("分 %d 批查询ModelFileRecord（每批%d个ETag）", totalBatches, batchSize)
	for i := 0; i < totalBatches; i++ {
		start := i * batchSize
		end := start + batchSize
		if end > totalCount {
			end = totalCount
		}
		batchEtags := etags[start:end]

		zap.S().Debugf("查询第 %d/%d 批（ETag范围：%d-%d）", i+1, totalBatches, start, end-1)
		batchRecords, err := modelFileRecordDao.BatchQueryByEtags(batchEtags)
		if err != nil {
			zap.S().Fatalf("第 %d 批查询失败: %v", i+1, err)
		}
		allRecords = append(allRecords, batchRecords...)
	}

	if len(allRecords) == 0 {
		zap.S().Info("无匹配的ModelFileRecord，无需生成Process记录")
		return
	}
	zap.S().Infof("查询完成，共获取 %d 条ModelFileRecord", len(allRecords))

	startOffsetTime := time.Now()
	modelFileProcessDao := dao.NewModelFileProcessDao(baseData)
	processRecords := make([]model.ModelFileProcess, 0, len(allRecords))
	semaphore := make(chan struct{}, maxConcurrency)       // 信号量：控制并发数
	resultChan := make(chan offsetResult, len(allRecords)) // 结果通道：缓冲避免阻塞

	var wg sync.WaitGroup
	wg.Add(len(allRecords))
	for _, record := range allRecords {
		go func() {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() {
				<-semaphore
			}()

			offset, err := getOffsetValue(
				record.Datatype,
				record.Org,
				record.Repo,
				record.Etag,
				record.FileSize,
			)

			resultChan <- offsetResult{
				RecordID: record.ID,
				Offset:   offset,
				Err:      err,
			}
		}()
	}

	go func() {
		wg.Wait()
		close(resultChan)
		zap.S().Infof("所有Offset请求完成，耗时：%v", time.Since(startOffsetTime))
	}()

	recordMap := make(map[int64]model.ModelFileRecord, len(allRecords))
	for _, rec := range allRecords {
		recordMap[rec.ID] = rec
	}

	for result := range resultChan {
		_, ok := recordMap[result.RecordID]
		if !ok {
			zap.S().Warnf("RecordID %d 不存在，跳过", result.RecordID)
			continue
		}

		offset := result.Offset
		if result.Err != nil {
			zap.S().Warnf("RecordID %d 获取Offset失败: %v，使用默认值0", result.RecordID, result.Err)
			offset = 0
		}

		zap.S().Infof("RecordID %d 获取Offset成功: %v", result.RecordID, result.Offset)
		processRecords = append(processRecords, model.ModelFileProcess{
			RecordID:   result.RecordID,
			InstanceID: instanceID,
			OffsetNum:  offset,
			Status:     3,
		})
	}
	allRecordIDs := make([]int64, 0, len(allRecords))
	for _, rec := range allRecords {
		allRecordIDs = append(allRecordIDs, rec.ID)
	}

	var existingIDs []int64
	processBatchSize := 100
	totalIDCount := len(allRecordIDs)
	totalIDBatches := (totalIDCount + processBatchSize - 1) / processBatchSize
	zap.S().Infof("分 %d 批查询已存在的ModelFileProcess记录（每批最多100个ID）", totalIDBatches)

	for i := 0; i < totalIDBatches; i++ {
		startIdx := i * processBatchSize
		endIdx := startIdx + processBatchSize
		if endIdx > totalIDCount {
			endIdx = totalIDCount
		}
		batchIDs := allRecordIDs[startIdx:endIdx]

		zap.S().Debugf("查询已存在记录第 %d/%d 批（ID范围：%d-%d）", i+1, totalIDBatches, startIdx, endIdx-1)
		batchExisting, err := modelFileProcessDao.ExistRecordIDs(instanceID, batchIDs)
		if err != nil {
			zap.S().Fatalf("第 %d 批查询已存在Process记录失败: %v", i+1, err)
		}
		existingIDs = append(existingIDs, batchExisting...)
	}

	existingIDMap := make(map[int64]struct{}, len(existingIDs))
	for _, id := range existingIDs {
		existingIDMap[id] = struct{}{}
	}

	newProcessRecords := make([]model.ModelFileProcess, 0)
	for _, p := range processRecords {
		if _, exists := existingIDMap[p.RecordID]; !exists {
			newProcessRecords = append(newProcessRecords, p)
		} else {
			zap.S().Infof("RecordID %d 已存在Process记录，跳过", p.RecordID)
		}
	}

	if len(newProcessRecords) > 0 {
		file, err := os.OpenFile("model_file_process.csv", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			zap.S().Fatalf("创建CSV文件失败: %v", err)
		}
		defer func() {
			file.Close()
			zap.S().Debugf("CSV文件句柄已关闭")
		}()

		writer := csv.NewWriter(file)
		writer.Comma = ','
		writer.UseCRLF = true
		defer func() {
			writer.Flush()
			if err := writer.Error(); err != nil {
				zap.S().Errorf("刷新CSV缓存失败: %v", err)
			}
		}()

		if err := writer.Write([]string{"record_id", "instance_id", "offset_num", "status"}); err != nil {
			zap.S().Fatalf("写入CSV表头失败: %v", err)
		}

		for _, p := range newProcessRecords {
			row := []string{
				strconv.FormatInt(p.RecordID, 10),
				p.InstanceID,
				strconv.FormatInt(p.OffsetNum, 10),
				strconv.Itoa(int(p.Status)),
			}
			if err := writer.Write(row); err != nil {
				zap.S().Fatalf("写入RecordID %d 失败: %v", p.RecordID, err)
			}
		}

		zap.S().Infof("生成CSV文件：model_file_process.csv（%d 条新记录）", len(newProcessRecords))
		zap.S().Infof("MySQL导入命令：\nLOAD DATA INFILE '/path/to/model_file_process.csv'\nINTO TABLE model_file_process\nFIELDS TERMINATED BY ',' ENCLOSED BY '\"' ESCAPED BY '\\\\'\nLINES TERMINATED BY '\\r\\n'\nIGNORE 1 ROWS (record_id, instance_id, offset_num, status);")
	} else {
		zap.S().Info("所有Process记录均已存在，无需生成CSV")
	}

	zap.S().Infof("程序执行完成：处理 %d 个文件，生成 %d 条Process记录", len(filteredFileInfos), len(newProcessRecords))
}

func getOffsetValue(dataType, org, repo, etag string, fileSize int64) (int64, error) {
	url := fmt.Sprintf("%s/api/fileOffset/%s/%s/%s/%s/%d", apiBaseURL, dataType, org, repo, etag, fileSize)
	resp, err := http.Get(url)
	if err != nil {
		return 0, fmt.Errorf("API请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("API返回错误状态码: %d（URL: %s）", resp.StatusCode, url)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("读取响应失败: %w", err)
	}

	var offset int64
	if err := json.Unmarshal(body, &offset); err != nil {
		return 0, fmt.Errorf("解析Offset失败: %w（响应: %s, URL: %s）", err, string(body), url)
	}

	return offset, nil
}

func processDirectory(rootPath string) ([]FileInfo, error) {
	var result []FileInfo
	if exists, err := fileExists(rootPath); err != nil {
		return nil, fmt.Errorf("检查目录存在性失败: %w", err)
	} else if !exists {
		return nil, fmt.Errorf("目录不存在: %s", rootPath)
	}

	err := filepath.WalkDir(rootPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			zap.S().Warnf("访问路径 %s 失败: %v", path, err)
			return nil
		}
		if path == rootPath || !d.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(rootPath, path)
		if err != nil {
			zap.S().Warnf("获取相对路径失败: %v", err)
			return nil
		}
		components := strings.Split(relPath, string(filepath.Separator))
		if len(components) < 5 || components[0] != "api" ||
			!(components[1] == "models" || components[1] == "datasets" || components[1] == "spaces") ||
			components[4] != "paths-info" {
			return nil
		}

		jsonPath := filepath.Join(path, "paths-info_post.json")
		if exists, err := fileExists(jsonPath); err != nil || !exists {
			return nil
		}

		var pathSegments []string
		if len(components) >= 7 {
			pathSegments = components[6:]
		}
		fileName := filepath.Join(pathSegments...)
		fi, err := processJsonFile(jsonPath, components[1], components[2], components[3], fileName)
		if err != nil {
			zap.S().Warnf("处理JSON %s 失败: %v", jsonPath, err)
			return nil
		}
		if fi != nil {
			result = append(result, *fi)
		}
		return nil
	})

	return result, err
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

func processJsonFile(jsonPath, dataType, org, repo, fileName string) (*FileInfo, error) {
	bytes, err := util.ReadFileToBytes(jsonPath)
	if err != nil {
		return nil, err
	}

	var cacheContent CacheContent
	if err := sonic.Unmarshal(bytes, &cacheContent); err != nil {
		return nil, err
	}

	decodeByte, err := hex.DecodeString(cacheContent.Content)
	if err != nil {
		return nil, err
	}

	var contentItems []ContentItem
	if err := sonic.Unmarshal(decodeByte, &contentItems); err != nil {
		return nil, err
	}
	if len(contentItems) == 0 {
		zap.S().Warnf("JSON %s 中content为空", jsonPath)
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
