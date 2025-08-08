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
	"os"

	"dingoscheduler/internal/model"

	"github.com/go-playground/validator/v10"
	"github.com/labstack/gommon/log"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

var SysConfig *Config
var SystemInfo *model.SystemInfo

type Config struct {
	Server      ServerConfig `json:"server" yaml:"server"`
	BizDBConfig DBConfig     `json:"bizDB" yaml:"bizDB"`
	Scheduler   Scheduler    `json:"scheduler" yaml:"scheduler"`
	Retry       Retry        `json:"retry" yaml:"retry"`
	Log         LogConfig    `json:"log" yaml:"log"`
}

type ServerConfig struct {
	Mode      string `json:"mode" yaml:"mode"`
	Host      string `json:"host" yaml:"host"`
	Port      int    `json:"port" yaml:"port"`
	PProf     bool   `json:"pprof" yaml:"pprof"`
	PProfPort int    `json:"pprofPort" yaml:"pprofPort"`
	Metrics   bool   `json:"metrics" yaml:"metrics"`
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

type Scheduler struct {
	Port int32 `json:"port" yaml:"port"`
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
	SystemInfo = &model.SystemInfo{}
	return &c, nil
}
