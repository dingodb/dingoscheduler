package service

import (
	"dingoscheduler/internal/dao"
)

type OrganizationService struct {
	organizationDao *dao.OrganizationDao
}

func NewOrganizationService(organizationDao *dao.OrganizationDao) *OrganizationService {
	return &OrganizationService{
		organizationDao: organizationDao,
	}
}
