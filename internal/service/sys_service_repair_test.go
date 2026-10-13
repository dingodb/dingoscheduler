package service

import (
	"testing"
	"time"

	"dingoscheduler/internal/dao"
	"dingoscheduler/internal/data"
	"dingoscheduler/internal/model"
	"dingoscheduler/pkg/config"
	"dingoscheduler/pkg/consts"

	"github.com/glebarez/sqlite"
	"github.com/patrickmn/go-cache"
	"gorm.io/gorm"
)

func TestRepairMovesStoppingJobsToStopped(t *testing.T) {
	previous := config.SysConfig
	config.SysConfig = &config.Config{}
	t.Cleanup(func() { config.SysConfig = previous })
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { sqlDB.Close() })
	if err := db.AutoMigrate(&model.CacheJob{}, &model.Repository{}); err != nil {
		t.Fatal(err)
	}
	// 2026-10-13: a job left in "stopping" during a database outage made every
	// scheduler restart panic with "cache job does not match instance".
	job := &model.CacheJob{Type: 1, InstanceId: "bj-12", Datatype: "models", Org: "Qwen", Repo: "demo", Commit: "c", Status: consts.RunningStatusJobStopping}
	if err := db.Create(job).Error; err != nil {
		t.Fatal(err)
	}
	d := &data.BaseData{BizDB: db, Cache: cache.New(time.Minute, time.Minute)}
	repos := dao.NewRepositoryDao(d, dao.NewRepositoryTagDao(d), dao.NewTagDao(d), dao.NewDingospeedDao(d), dao.NewOrganizationDao(d), dao.NewHfTokenDao(d))
	s := SysService{repositoryDao: repos, cacheJobDao: dao.NewCacheJobDao(d, repos)}
	if err := s.repairJobRunStatus(); err != nil {
		t.Fatal(err)
	}
	var got model.CacheJob
	if err := db.First(&got, job.ID).Error; err != nil || got.Status != consts.RunningStatusJobStop {
		t.Fatalf("stopping job not repaired: %+v %v", got, err)
	}
}
