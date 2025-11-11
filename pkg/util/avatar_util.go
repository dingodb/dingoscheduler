package util

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	myerr "dingoscheduler/pkg/error"

	"github.com/aliyun/alibabacloud-oss-go-sdk-v2/oss"
	"github.com/aliyun/alibabacloud-oss-go-sdk-v2/oss/credentials"
	"go.uber.org/zap"
	"golang.org/x/net/html"
)

func FetchAvatarURL(orgName string) (string, error) {
	orgUri := fmt.Sprintf("/%s", orgName)
	zap.S().Debugf("开始请求Hugging Face组织页面：%s", orgUri) // 调试级日志，记录请求地址
	// 这里无需设置token，设了反而会出现401
	headers := make(map[string]string)
	headers["User-Agent"] = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"
	headers["Accept"] = "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8"
	resp, err := Get(orgUri, headers)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", myerr.New(fmt.Sprintf("组织页面状态异常（%s）：状态码 %d", orgName, resp.StatusCode))
	}
	reader := bytes.NewReader(resp.Body)
	doc, err := html.Parse(reader)
	if err != nil {
		return "", myerr.New(fmt.Sprintf("解析HTML页面（%s）失败：%v", orgUri, err))
	}

	var avatarURL string
	var findAvatar func(*html.Node)
	findAvatar = func(n *html.Node) {
		if avatarURL != "" {
			return // 已找到，提前退出
		}
		if n.Type == html.ElementNode && n.Data == "div" {
			for _, attr := range n.Attr {
				if attr.Key == "class" &&
					strings.Contains(attr.Val, "relative") &&
					strings.Contains(attr.Val, "mr-4") &&
					strings.Contains(attr.Val, "flex") &&
					(strings.Contains(attr.Val, "size-16") || strings.Contains(attr.Val, "size-20")) {
					for c := n.FirstChild; c != nil; c = c.NextSibling {
						if c.Type == html.ElementNode && c.Data == "a" {
							isTargetOrgLink := false
							for _, aAttr := range c.Attr {
								if (aAttr.Key == "href" && strings.Trim(aAttr.Val, "/") == orgName) ||
									(aAttr.Key == "title" && strings.Contains(strings.ToLower(aAttr.Val), strings.ToLower(orgName))) {
									isTargetOrgLink = true
									break
								}
							}
							if !isTargetOrgLink {
								continue
							}
							for imgNode := c.FirstChild; imgNode != nil; imgNode = imgNode.NextSibling {
								if imgNode.Type == html.ElementNode && imgNode.Data == "img" {
									for _, imgAttr := range imgNode.Attr {
										if imgAttr.Key == "src" && imgAttr.Val != "" {
											if strings.Contains(imgAttr.Val, "cdn-avatars.huggingface.co") ||
												strings.Contains(imgAttr.Val, "/avatars/") ||
												strings.Contains(imgAttr.Val, orgName) {
												avatarURL = imgAttr.Val
												zap.S().Debugf("成功匹配头像img，src：%s", avatarURL)
												return
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			findAvatar(c)
			if avatarURL != "" {
				return
			}
		}
	}
	findAvatar(doc)
	if avatarURL == "" {
		return "", fmt.Errorf("未在组织页面（%s）中找到头像元素", orgUri)
	}
	return avatarURL, nil
}

func DownloadAvatar(
	avatarURL, saveRoot, org, ossBucketName string,
	ossOption *ImageUploadOption, // 引用UploadImageToOSS的ImageUploadOption类型
) (string, error) {
	localFileName := fmt.Sprintf("_%s_avatar.jpg", org)
	localSavePath := filepath.Join(saveRoot, localFileName)
	resp, err := GetForURL(avatarURL, nil)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("组织页面状态异常（%s）：状态码 %d", avatarURL, resp.StatusCode)
	}
	if err = MakeDirs(localSavePath); err != nil {
		zap.S().Errorf("create %s dir err.%v", localSavePath, err)
		return "", err
	}
	localFile, err := os.Create(localSavePath)
	if err != nil {
		return "", fmt.Errorf("创建本地暂存文件（路径=%s）失败：%w", localSavePath, err)
	}
	defer localFile.Close()

	if _, err := io.Copy(localFile, bytes.NewReader(resp.Body)); err != nil {
		return "", fmt.Errorf("写入本地暂存文件（路径=%s）失败：%w", localSavePath, err)
	}
	ossObjectKey := fmt.Sprintf("assets/static/bp/model/%s", localFileName)
	uploadResult, err := UploadImageToOSS(
		ossBucketName,
		ossObjectKey,
		localSavePath,
		ossOption,
	)
	if err != nil {
		return "", fmt.Errorf("OSS上传失败（OSS路径=%s）：%w", ossObjectKey, err)
	}
	zap.S().Debugf("头像OSS上传成功：桶名=%s，OSS文件名称=%s，ETag=%s",
		ossBucketName, uploadResult.ObjectKey, uploadResult.ETag)
	if err := os.Remove(localSavePath); err != nil {
		zap.S().Warnf("删除本地暂存文件（路径=%s）失败：%v", localSavePath, err)
	}
	return uploadResult.ObjectKey, nil
}

type ImageUploadOption struct {
	Region        string
	StorageClass  oss.StorageClassType
	ObjectACL     oss.ObjectACLType
	Timeout       time.Duration
	CustomHeaders map[string]string
}

type ImageUploadResult struct {
	BucketName string
	ObjectKey  string
	ETag       string
	RequestId  string
	StatusCode int
	FileSize   int64
}

func isValidImageFormat(fileExt string) bool {
	validExts := []string{".jpg", ".jpeg", ".png", ".gif", ".bmp", ".webp"}
	fileExt = strings.ToLower(fileExt)
	for _, ext := range validExts {
		if fileExt == ext {
			return true
		}
	}
	return false
}

func UploadImageToOSS(
	bucketName, objectKey, localImagePath string,
	option *ImageUploadOption,
) (*ImageUploadResult, error) {
	if bucketName == "" {
		return nil, errors.New("bucket名称不能为空")
	}
	if objectKey == "" {
		return nil, errors.New("OSS对象名（objectKey）不能为空")
	}
	if localImagePath == "" {
		return nil, errors.New("本地图片路径不能为空")
	}

	fileInfo, err := os.Stat(localImagePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errors.New("本地图片文件不存在：" + localImagePath)
		}
		return nil, errors.New("获取图片文件信息失败：" + err.Error())
	}
	if fileInfo.IsDir() {
		return nil, errors.New("本地路径是目录，不是图片文件：" + localImagePath)
	}
	fileExt := filepath.Ext(localImagePath)
	if !isValidImageFormat(fileExt) {
		return nil, errors.New("不支持的图片格式：" + fileExt + "，支持格式：jpg/jpeg/png/gif/bmp/webp")
	}

	if option == nil {
		option = &ImageUploadOption{
			Region:       "cn-beijing",             // 默认华北2（北京）地域
			StorageClass: oss.StorageClassStandard, // 标准存储（oss根包常量）
			ObjectACL:    oss.ObjectACLPrivate,     // 私有访问（oss根包常量）
			Timeout:      10 * time.Second,         // 默认10秒超时
		}
	} else {
		if option.Region == "" {
			option.Region = "cn-beijing"
		}
		if option.StorageClass == "" {
			option.StorageClass = oss.StorageClassStandard
		}
		if option.ObjectACL == "" {
			option.ObjectACL = oss.ObjectACLPrivate
		}
		if option.Timeout <= 0 {
			option.Timeout = 10 * time.Second
		}
	}

	cfg := oss.LoadDefaultConfig().
		WithCredentialsProvider(credentials.NewEnvironmentVariableCredentialsProvider()).
		WithRegion(option.Region)

	client := oss.NewClient(cfg)
	file, err := os.Open(localImagePath)
	if err != nil {
		return nil, errors.New("打开图片文件失败：" + err.Error())
	}
	defer file.Close()

	ctx, cancel := context.WithTimeout(context.Background(), option.Timeout)
	defer cancel()

	putReq := &oss.PutObjectRequest{
		Bucket:        oss.Ptr(bucketName),
		Key:           oss.Ptr(objectKey),
		Body:          file,
		ContentLength: oss.Ptr(fileInfo.Size()),
	}
	result, err := client.PutObject(ctx, putReq)
	if err != nil {
		return nil, errors.New("OSS图片上传失败：" + err.Error())
	}

	return &ImageUploadResult{
		BucketName: bucketName,
		ObjectKey:  *putReq.Key,
		ETag:       *result.ETag,
		RequestId:  result.ResultCommon.Headers.Get("X-Oss-Request-Id"),
		StatusCode: result.StatusCode,
		FileSize:   fileInfo.Size(),
	}, nil
}
