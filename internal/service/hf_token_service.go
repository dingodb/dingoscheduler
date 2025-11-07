package service

import (
	"dingoscheduler/internal/dao"
)

type HfTokenService struct {
	hfTokenDao *dao.HfTokenDao
}

func NewHfTokenService(hfTokenDao *dao.HfTokenDao) *HfTokenService {
	return &HfTokenService{
		hfTokenDao: hfTokenDao,
	}
}

func (d *HfTokenService) RefreshToken() string {
	return d.hfTokenDao.RefreshToken()
}
