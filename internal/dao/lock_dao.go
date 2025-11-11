package dao

import (
	"fmt"
	"sync"
	"time"

	"dingoscheduler/internal/data"
)

type LockDao struct {
	baseData           *data.BaseData
	cacheJobReqMu      sync.Mutex
	cacheJobReqTimeout time.Duration
}

func NewLockDao(baseData *data.BaseData) *LockDao {
	return &LockDao{baseData: baseData, cacheJobReqTimeout: 60 * time.Second}
}

func (f *LockDao) GetCacheJobReqLock(orgRepoKey string) *sync.RWMutex {
	if val, ok := f.baseData.Cache.Get(orgRepoKey); ok {
		f.baseData.Cache.Set(orgRepoKey, val, f.cacheJobReqTimeout)
		return val.(*sync.RWMutex)
	}
	f.cacheJobReqMu.Lock()
	defer f.cacheJobReqMu.Unlock()
	if val, ok := f.baseData.Cache.Get(orgRepoKey); ok {
		f.baseData.Cache.Set(orgRepoKey, val, f.cacheJobReqTimeout)
		return val.(*sync.RWMutex)
	}
	newLock := &sync.RWMutex{}
	f.baseData.Cache.Set(orgRepoKey, newLock, f.cacheJobReqTimeout)
	return newLock
}

func GetCacheJobOrgRepoKey(orgRepo string) string {
	return fmt.Sprintf("cacheJob/%s", orgRepo)
}
