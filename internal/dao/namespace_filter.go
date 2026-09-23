package dao

import (
	"dingoscheduler/pkg/repository"
	"gorm.io/gorm"
)

// column is supplied only by DAO code, never by an HTTP request. Prefixes are
// literal; arbitrary hosted namespaces use equality rather than SQL wildcards.
func filterNamespace(db *gorm.DB, column, namespace string) (*gorm.DB, error) {
	if namespace == "" {
		return db, nil
	}
	if err := repository.ValidatePath(namespace, false); err != nil {
		return nil, err
	}
	switch namespace {
	case "modelscope":
		return db.Where(column+" LIKE ?", "modelscope/%"), nil
	case "huggingface":
		return db.Where(column+" NOT LIKE ? AND "+column+" NOT LIKE ?", "modelscope/%", "dingo-local/%"), nil
	default:
		return db.Where(column+" = ?", "dingo-local/"+namespace), nil
	}
}
