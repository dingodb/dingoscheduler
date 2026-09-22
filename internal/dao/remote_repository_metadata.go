package dao

import (
	"errors"
	"strings"
	"unicode/utf8"

	"dingoscheduler/internal/model"
	"github.com/go-sql-driver/mysql"
	"go.uber.org/zap"
)

// Preserve the normal legacy insert on databases that support the metadata.
// Some existing schemas have VARCHAR(10) tag IDs and INT repository_tag.repo_id.
// Those optional fields must not roll back a verified, complete repository.
// Retry only explicit column-capacity errors, after the transaction rolls back.
func saveRemoteRepositoryMetadata(repo *model.Repository, tags []*model.RepositoryTag, save func(*model.Repository, []*model.RepositoryTag) error) error {
	for {
		err := save(repo, tags)
		if err == nil {
			return nil
		}
		var sqlErr *mysql.MySQLError
		if !errors.As(err, &sqlErr) {
			return err
		}
		switch {
		case sqlErr.Number == 1406 && strings.Contains(sqlErr.Message, "'pipeline_tag_id'") && repo.PipelineTagId != "":
			if repo.PipelineTag == "" && utf8.RuneCountInString(repo.PipelineTagId) <= 100 {
				repo.PipelineTag = repo.PipelineTagId
			}
			repo.PipelineTagId = ""
		case len(tags) > 0 && ((sqlErr.Number == 1406 && strings.Contains(sqlErr.Message, "'tag_id'")) ||
			(sqlErr.Number == 1264 && strings.Contains(sqlErr.Message, "'repo_id'"))):
			tags = nil
		default:
			return err
		}
		zap.S().Warnf("remote repository %s optional tags exceed existing schema; retrying without unsupported tag fields: %v", repo.OrgRepo, err)
		// A rolled-back Create can still assign the generated ID to the Go value.
		repo.ID = 0
	}
}
