package dao

import (
	"net/http"
	"testing"

	"dingoscheduler/internal/model"
	"dingoscheduler/internal/model/query"
	"dingoscheduler/pkg/consts"
)

func TestCacheJobSizeReportAndResumeStatus(t *testing.T) {
	_, jobs, db := remoteTestDAO(t, func(w http.ResponseWriter, r *http.Request) {})
	create := func(status int32) *model.CacheJob {
		j := &model.CacheJob{Type: 1, InstanceId: "node", Datatype: "datasets", Org: "org", Repo: "data", UsedStorage: 999, Commit: "c", Status: status}
		if err := db.Create(j).Error; err != nil {
			t.Fatal(err)
		}
		return j
	}
	get := func(id int64) model.CacheJob {
		var j model.CacheJob
		if err := db.First(&j, id).Error; err != nil {
			t.Fatal(err)
		}
		return j
	}
	report := func(id int64, status int32, size int64) {
		if err := jobs.UpdateStatusAndRepo(&query.UpdateJobStatusReq{Id: id, InstanceId: "node", Status: status, UsedStorage: size}); err != nil {
			t.Fatal(err)
		}
	}

	// The measured size replaces the create-time estimate; 0 keeps it.
	running := create(consts.RunningStatusJobIng)
	report(running.ID, consts.RunningStatusJobIng, 42)
	if j := get(running.ID); j.UsedStorage != 42 || j.Status != consts.RunningStatusJobIng {
		t.Fatalf("size report: %+v", j)
	}
	report(running.ID, consts.RunningStatusJobBreak, 0)
	if j := get(running.ID); j.UsedStorage != 42 || j.Status != consts.RunningStatusJobBreak {
		t.Fatalf("zero size overwrote: %+v", j)
	}

	// A late "running" report must not regress a stopped job.
	stopping := create(consts.RunningStatusJobStopping)
	report(stopping.ID, consts.RunningStatusJobIng, 7)
	if j := get(stopping.ID); j.Status != consts.RunningStatusJobStopping {
		t.Fatalf("running report regressed stopping job: %+v", j)
	}

	// Resume shows the job as running, but only from a resumable state.
	for status, want := range map[int32]int32{
		consts.RunningStatusJobBreak:    consts.RunningStatusJobIng,
		consts.RunningStatusJobStop:     consts.RunningStatusJobIng,
		consts.RunningStatusJobWait:     consts.RunningStatusJobIng,
		consts.RunningStatusJobComplete: consts.RunningStatusJobComplete,
	} {
		j := create(status)
		if err := db.Model(j).Update("error_msg", `{"msg":"interrupted"}`).Error; err != nil {
			t.Fatal(err)
		}
		if err := jobs.MarkCacheJobResumed(j.ID); err != nil {
			t.Fatal(err)
		}
		got := get(j.ID)
		if got.Status != want || (want == consts.RunningStatusJobIng && got.ErrorMsg != "") {
			t.Fatalf("resume from %d: %+v", status, got)
		}
	}
}
