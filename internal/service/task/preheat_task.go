package task

import (
	"fmt"
	"io"
	"net/http"

	"dingoscheduler/internal/model"
	"dingoscheduler/pkg/util"

	"go.uber.org/zap"
)

type DownloadTask struct {
	Job      *model.PreheatJob
	Domain   string
	FileName string
}

func (d *DownloadTask) DoTask() {
	uri := fmt.Sprintf("/%s/%s/%s/resolve/%s/%s", d.Job.Datatype, d.Job.Org, d.Job.Repo, d.Job.Revision, d.FileName)
	headers := make(map[string]string)
	if d.Job.Token != "" {
		headers["authorization"] = fmt.Sprintf("Bearer %s", d.Job.Token)
	}
	headers["preheat"] = "1"
	err := util.GetStream(d.Domain, uri, headers, func(resp *http.Response) error {
		for {
			select {
			default:
				chunk := make([]byte, 1)
				_, err := resp.Body.Read(chunk)
				if err != nil {
					if err == io.EOF {
						return nil
					}
					zap.S().Errorf("req remote err.%v", err)
					return err
				}
			}
		}
	})
	if err != nil {
		return
	}
}
