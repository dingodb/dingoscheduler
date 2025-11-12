package dao

import (
	"fmt"
	"sync"

	"dingoscheduler/internal/data"
	"dingoscheduler/internal/model"

	"go.uber.org/zap"
)

type HfTokenDao struct {
	baseData     *data.BaseData
	defaultToken string
	mu           sync.Mutex
}

func NewHfTokenDao(data *data.BaseData) *HfTokenDao {
	return &HfTokenDao{
		baseData: data,
	}
}

func (d *HfTokenDao) getDefaultToken() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.defaultToken == "" {
		var hfTokens []*model.HfToken
		if err := d.baseData.BizDB.Table("hf_token t1").Where("enabled = ?", true).Limit(1).Find(&hfTokens).Error; err != nil {
			zap.S().Errorf("getDefaultToken err.%v", err)
			return ""
		}
		if len(hfTokens) > 0 {
			d.defaultToken = hfTokens[0].Token
		}
	}
	return d.defaultToken
}

func (d *HfTokenDao) RefreshToken() string {
	d.defaultToken = ""
	return d.getDefaultToken()
}

func (d *HfTokenDao) GetHeaders() map[string]string {
	m := make(map[string]string)
	token := d.getDefaultToken()
	if token != "" {
		m["Authorization"] = fmt.Sprintf("Bearer %s", token)
	}
	return m
}
