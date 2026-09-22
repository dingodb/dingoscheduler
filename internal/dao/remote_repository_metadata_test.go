package dao

import (
	"dingoscheduler/internal/model"
	"errors"
	"github.com/go-sql-driver/mysql"
	"testing"
)

func TestRemoteMetadataCapacityFallback(t *testing.T) {
	repo := &model.Repository{OrgRepo: "sshleifer/tiny-gpt2", PipelineTagId: "text-generation"}
	tags := []*model.RepositoryTag{{TagId: "pytorch"}}
	calls := 0
	err := saveRemoteRepositoryMetadata(repo, tags, func(r *model.Repository, ts []*model.RepositoryTag) error {
		calls++
		if r.ID != 0 {
			t.Fatal("rolled-back ID reused")
		}
		switch calls {
		case 1:
			r.ID = 9007199254740993
			return &mysql.MySQLError{Number: 1406, Message: "Data too long for column 'pipeline_tag_id' at row 1"}
		case 2:
			if r.PipelineTagId != "" || r.PipelineTag != "text-generation" {
				t.Fatal("pipeline label lost")
			}
			r.ID = 9007199254740994
			return &mysql.MySQLError{Number: 1264, Message: "Out of range value for column 'repo_id' at row 1"}
		case 3:
			if len(ts) != 0 {
				t.Fatal("unsupported associations retained")
			}
			return nil
		default:
			t.Fatal("unbounded retry")
			return nil
		}
	})
	if err != nil || calls != 3 {
		t.Fatalf("calls=%d error=%v", calls, err)
	}
}

func TestRemoteMetadataDoesNotHideOtherFailures(t *testing.T) {
	for _, failure := range []error{errors.New("connection failed"), &mysql.MySQLError{Number: 1406, Message: "Data too long for column 'repo' at row 1"}} {
		calls := 0
		err := saveRemoteRepositoryMetadata(&model.Repository{}, nil, func(*model.Repository, []*model.RepositoryTag) error { calls++; return failure })
		if err != failure || calls != 1 {
			t.Fatalf("failure masked: %v", err)
		}
	}
}
