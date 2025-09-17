package dao

import (
	"fmt"

	"dingoscheduler/internal/data"
	"dingoscheduler/internal/model"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

type RepositoryTagDao struct {
	baseData *data.BaseData
}

func NewRepositoryTagDao(data *data.BaseData) *RepositoryTagDao {
	return &RepositoryTagDao{
		baseData: data,
	}
}

func (r *RepositoryTagDao) SaveBySql(tx *gorm.DB, repo *model.RepositoryTag) (int64, error) {
	recordSql := fmt.Sprintf("INSERT INTO repository_tag (repo_id, tag_id) VALUES(%d, '%s')",
		repo.RepoId, repo.TagId)
	db, err := tx.DB()
	if err != nil {
		return 0, err
	}
	result, err := db.Exec(recordSql)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (r *RepositoryTagDao) BatchSave(tx *gorm.DB, repositoryTags []*model.RepositoryTag) error {
	for _, repositoryTag := range repositoryTags {
		recordSql := fmt.Sprintf("INSERT INTO repository_tag (repo_id, tag_id) VALUES(%d, '%s')",
			repositoryTag.RepoId, repositoryTag.TagId)
		result := tx.Exec(recordSql)
		if result.Error != nil {
			// 出错回滚事务
			zap.S().Error("批量插入失败: %v", result.Error)
			return result.Error
		}
	}
	return nil
}
