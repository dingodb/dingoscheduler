//  Copyright (c) 2025 dingodb.com, Inc. All Rights Reserved
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http:www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.

package util

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"dingoscheduler/pkg/common"
	"dingoscheduler/pkg/config"
	"dingoscheduler/pkg/consts"

	"github.com/avast/retry-go"
	"go.uber.org/zap"
)

var (
	reqTimeout   = 0 * time.Second
	simpleClient *http.Client
	proxyClient  *http.Client
	simpleOnce   sync.Once
	proxyOnce    sync.Once
)

func RetryRequest(f func() (*common.Response, error)) (*common.Response, error) {
	var resp *common.Response
	err := retry.Do(
		func() error {
			var err error
			resp, err = f()
			return err
		},
		retry.Delay(time.Duration(config.SysConfig.Retry.Delay)*time.Second),
		retry.Attempts(config.SysConfig.Retry.Attempts),
		retry.DelayType(retry.FixedDelay),
	)
	return resp, err
}

func NewHTTPClient() (*http.Client, error) {
	simpleOnce.Do(
		func() {
			simpleClient = &http.Client{Timeout: reqTimeout}
		})
	return simpleClient, nil
}

func NewHTTPClientWithProxy() (*http.Client, error) {
	proxyOnce.Do(func() {
		proxyClient = &http.Client{Timeout: reqTimeout}
		if config.SysConfig.GetHttpProxy() == "" {
			return
		}
		proxyURL, err := url.Parse(config.SysConfig.GetHttpProxy())
		if err != nil {
			zap.S().Errorf("代理地址解析失败: %v", err)
			return
		}
		transport := &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout:   10 * time.Second,
			ForceAttemptHTTP2:     false,
			ResponseHeaderTimeout: 10 * time.Second,
			IdleConnTimeout:       90 * time.Second,
		}
		proxyClient.Transport = transport
	})
	return proxyClient, nil
}

func constructClient() (string, *http.Client, error) {
	var (
		client *http.Client
		err    error
	)
	// 代理不可用，且允许代理切换到备用，使用直联。
	if config.SysConfig.Proxy.Enabled {
		client, err = NewHTTPClientWithProxy()
	} else {
		client, err = NewHTTPClient()
	}
	return config.SysConfig.GetHFURLBase(), client, err
}

func GetForDomain(domain, requestUri string, headers map[string]string) (*common.Response, error) {
	client, err := NewHTTPClient()
	if err != nil {
		return nil, fmt.Errorf("construct http client err: %v", err)
	}
	requestURL := fmt.Sprintf("%s%s", domain, requestUri)
	return doGet(client, requestURL, headers)
}

func GetForURL(requestURL string, headers map[string]string) (*common.Response, error) {
	_, client, err := constructClient()
	if err != nil {
		return nil, fmt.Errorf("construct http client err: %v", err)
	}
	return doGet(client, requestURL, headers)
}

func Get(requestUri string, headers map[string]string) (*common.Response, error) {
	domain, client, err := constructClient()
	if err != nil {
		return nil, fmt.Errorf("construct http client err: %v", err)
	}
	requestURL := fmt.Sprintf("%s%s", domain, requestUri)
	return doGet(client, requestURL, headers)
}

func doGet(client *http.Client, targetURL string, headers map[string]string) (*common.Response, error) {
	req, err := http.NewRequest("GET", targetURL, nil)
	if err != nil {
		return nil, fmt.Errorf("创建GET请求失败: %v", err)
	}
	if headers != nil {
		for key, value := range headers {
			req.Header.Set(key, value)
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		zap.S().Warnf("URL请求失败: %s, 错误: %v", targetURL, err)
		return nil, fmt.Errorf("执行GET请求失败: %v", err)
	}

	defer func() {
		if r := recover(); r != nil {
			zap.S().Errorf("关闭响应体资源时出现异常: %v", r)
		}
		resp.Body.Close()
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应体失败: %v", err)
	}

	respHeaders := make(map[string]interface{})
	for key, values := range resp.Header {
		respHeaders[key] = values
	}

	return &common.Response{
		StatusCode: resp.StatusCode,
		Headers:    respHeaders,
		Body:       body,
	}, nil
}

func GetStream(domain, uri string, headers map[string]string, f func(r *http.Response) error) error {
	var (
		client *http.Client
		err    error
	)
	if IsInnerDomain(domain) {
		client, err = NewHTTPClient()
	} else {
		domain, client, err = constructClient()
	}
	if err != nil {
		return fmt.Errorf("construct http client err: %v", err)
	}
	requestURL := fmt.Sprintf("%s%s", domain, uri)
	return doGetStream(client, requestURL, headers, f)
}

func doGetStream(client *http.Client, targetURL string, headers map[string]string, f func(r *http.Response) error) error {
	escapedURL := strings.ReplaceAll(targetURL, "#", "%23")
	req, err := http.NewRequest("GET", escapedURL, nil)
	if err != nil {
		return fmt.Errorf("创建GET请求失败: %v", err)
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respHeaders := make(map[string]interface{})
	for key, value := range resp.Header {
		respHeaders[key] = value
	}
	return f(resp)
}

func PostForDomain(domain, requestUri string, contentType string, data []byte, headers map[string]string) (*common.Response, error) {
	client, err := NewHTTPClient()
	if err != nil {
		return nil, fmt.Errorf("construct http client err: %v", err)
	}
	requestURL := fmt.Sprintf("%s%s", domain, requestUri)
	return doPost(client, requestURL, contentType, data, headers)
}

func Post(requestUri string, contentType string, data []byte, headers map[string]string) (*common.Response, error) {
	domain, client, err := constructClient()
	if err != nil {
		return nil, fmt.Errorf("construct http client err: %v", err)
	}
	requestURL := fmt.Sprintf("%s%s", domain, requestUri)
	return doPost(client, requestURL, contentType, data, headers)
}

func doPost(client *http.Client, targetURL string, contentType string, data []byte, headers map[string]string) (*common.Response, error) {
	req, err := http.NewRequest("POST", targetURL, bytes.NewBuffer(data))
	if err != nil {
		return nil, fmt.Errorf("创建POST请求失败: %v", err)
	}

	req.Header.Set("Content-Type", contentType)
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := client.Do(req)
	if err != nil {
		zap.S().Warnf("URL请求失败: %s, 错误: %v", targetURL, err)
		return nil, fmt.Errorf("执行POST请求失败: %v", err)
	}

	defer func() {
		if r := recover(); r != nil {
			zap.S().Errorf("关闭响应体资源时出现异常: %v", r)
		}
		resp.Body.Close()
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应体失败: %v", err)
	}

	respHeaders := make(map[string]interface{})
	for key, values := range resp.Header {
		respHeaders[key] = values
	}

	return &common.Response{
		StatusCode: resp.StatusCode,
		Headers:    respHeaders,
		Body:       body,
	}, nil
}

func IsInnerDomain(url string) bool {
	return !strings.Contains(url, consts.Huggingface) && !strings.Contains(url, consts.Hfmirror)
}

func GetHeaders() map[string]string {
	m := make(map[string]string)
	m["Authorization"] = fmt.Sprintf("Bearer %s", config.SysConfig.GetGlobalHfToken())
	return m
}
