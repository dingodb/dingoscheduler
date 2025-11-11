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

package config

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/labstack/gommon/log"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

var SysConfig *Config

type Config struct {
	Server      ServerConfig      `json:"server" yaml:"server"`
	BizDBConfig DBConfig          `json:"bizDB" yaml:"bizDB"`
	Cache       Cache             `json:"cache" yaml:"cache"`
	Scheduler   Scheduler         `json:"scheduler" yaml:"scheduler"`
	Retry       Retry             `json:"retry" yaml:"retry"`
	Log         LogConfig         `json:"log" yaml:"log"`
	Avatar      Avatar            `json:"avatar" yaml:"avatar"`
	Oss         Oss               `json:"oss" yaml:"oss"`
	Proxy       Proxy             `json:"proxy" yaml:"proxy"`
	Aidc        map[string]string `json:"aidc" yaml:"aidc"`
}

type ServerConfig struct {
	Mode      string `json:"mode" yaml:"mode"`
	Host      string `json:"host" yaml:"host"`
	Port      int    `json:"port" yaml:"port"`
	PProf     bool   `json:"pprof" yaml:"pprof"`
	PProfPort int    `json:"pprofPort" yaml:"pprofPort"`
	Metrics   bool   `json:"metrics" yaml:"metrics"`
	HfNetLoc  string `json:"hfNetLoc" yaml:"hfNetLoc"`
	HfScheme  string `json:"hfScheme" yaml:"hfScheme" validate:"oneof=https http"`
	Ssl       SSL    `json:"ssl" yaml:"ssl"`
}

type SSL struct {
	EnableCA bool   `yaml:"enableCA"`
	CrtFile  string `json:"crtFile" yaml:"crtFile" `
	KeyFile  string `json:"keyFile" yaml:"keyFile" `
	CaFile   string `json:"caFile" yaml:"caFile" `
}

type Retry struct {
	Delay    int  `json:"delay" yaml:"delay" validate:"min=0,max=60"`
	Attempts uint `json:"attempts" yaml:"attempts" validate:"min=1,max=5"`
}

type LogConfig struct {
	MaxSize    int `json:"maxSize" yaml:"maxSize"`
	MaxBackups int `json:"maxBackups" yaml:"maxBackups"`
	MaxAge     int `json:"maxAge" yaml:"maxAge"`
}

type Avatar struct {
	Path string `yaml:"path"`
}
type Cache struct {
	DefaultExpiration int `json:"defaultExpiration" yaml:"defaultExpiration" `
	CleanupInterval   int `json:"cleanupInterval" yaml:"cleanupInterval"`
}

type Oss struct {
	Path       string `yaml:"path"`
	BucketName string `yaml:"bucketName"`
	Region     string `yaml:"region"`
}

type Proxy struct {
	Enabled   bool   `json:"enabled" yaml:"enabled"`
	HttpProxy string `json:"httpProxy" yaml:"httpProxy"`
}

type Scheduler struct {
	Port          int32       `json:"port" yaml:"port"`
	PersistRepo   PersistRepo `json:"persistRepo" yaml:"persistRepo"`
	GlobalHfToken string      `json:"globalHfToken" yaml:"globalHfToken"`
}

type PersistRepo struct {
	Enabled     bool   `json:"enabled" yaml:"enabled"`
	Cron        string `json:"cron" yaml:"cron"`
	InstanceIds string `json:"instanceIds" yaml:"instanceIds"`
}

type DBConfig struct {
	Type        string `yaml:"type"`
	User        string `yaml:"user"`
	Password    string `yaml:"password"`
	Host        string `yaml:"host"`
	Port        int    `yaml:"port"`
	Database    string `yaml:"database"`
	Timeout     string `yaml:"timeout"`
	MaxConn     int    `yaml:"maxConn"`
	MaxIdleConn int    `yaml:"maxIdleConn"`
}

func (c *Config) GetHFURLBase() string {
	return fmt.Sprintf("%s://%s", c.GetHfScheme(), c.GetHfNetLoc())
}
func (c *Config) GetHfScheme() string {
	return c.Server.HfScheme
}
func (c *Config) GetHfNetLoc() string {
	return c.Server.HfNetLoc
}

func (c *Config) GetHost() string {
	return c.Server.Host
}

func (c *Config) EnableMetric() bool {
	return c.Server.Metrics
}

func (c *Config) SetDefaults() {
	if c.Server.Port == 0 {
		c.Server.Port = 8090
	}
	if c.Server.Host == "" {
		c.Server.Host = "localhost"
	}
	if c.Server.PProfPort == 0 {
		c.Server.PProfPort = 6060
	}
	c.Server.Ssl.EnableCA = true
}

func (c *Config) GetDefaultExpiration() time.Duration {
	if c.Cache.DefaultExpiration == 0 {
		c.Cache.DefaultExpiration = 5
	}
	return time.Duration(c.Cache.DefaultExpiration) * time.Hour
}

func (c *Config) GetCleanupInterval() time.Duration {
	if c.Cache.CleanupInterval == 0 {
		c.Cache.CleanupInterval = 10
	}
	return time.Duration(c.Cache.CleanupInterval) * time.Hour
}

func (c *Config) GetEnablePersistRepo() bool {
	return c.Scheduler.PersistRepo.Enabled
}

func (c *Config) GetPersistRepoCron() string {
	return c.Scheduler.PersistRepo.Cron
}

func (c *Config) GetSpeedExpiration() time.Duration {
	return time.Duration(5) * time.Minute
}

func (c *Config) GetCacheExpiration() time.Duration {
	return time.Duration(30) * time.Minute
}

func Scan(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var c Config
	err = yaml.Unmarshal(b, &c)
	if err != nil {
		return nil, err
	}
	c.SetDefaults()

	validate := validator.New()
	err = validate.Struct(&c)
	if err != nil {
		var invalidValidationError *validator.InvalidValidationError
		if errors.As(err, &invalidValidationError) {
			zap.S().Errorf("Invalid validation error: %v\n", err)
		}
		return nil, err
	}
	SysConfig = &c // 设置全局配置变量

	marshal, err := yaml.Marshal(c)
	if err != nil {
		return nil, err
	}
	log.Info(string(marshal))
	return &c, nil
}

func (c *Config) GetHttpProxy() string {
	return c.Proxy.HttpProxy
}
