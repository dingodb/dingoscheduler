package nodehealth

import (
	pb "dingoscheduler/pkg/proto/manager"
	"testing"
	"time"
)

func fixture(now time.Time) *pb.NodeHealthSnapshot {
	return &pb.NodeHealthSnapshot{Version: 1, ProcessId: "process-1", StartedAt: now.Unix() - 60, CollectedAt: now.Unix(), HeartbeatPeriodSeconds: 5,
		Capabilities: []*pb.CapabilityObservation{
			{Capability: "metadata_read", State: 2, Unresolved: true, LastObservation: now.Unix(), LastFailure: now.Unix()},
			{Capability: "metadata_write", State: 1, LastObservation: now.Unix()},
		}, Errors: []*pb.StorageErrorCount{{Operation: "read", Kind: "mount_disconnected", Count: 12}},
	}
}

func TestPresentationTimeline(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	h := fixture(now)
	encoded, err := Encode(h)
	if err != nil {
		t.Fatal(err)
	}
	v := Present(encoded, now, now)
	if v.Communication != "connected" || v.Capabilities[0].Status != "unresolved" || v.Capabilities[1].Status != "observed_healthy" {
		t.Fatalf("unexpected view: %+v", v)
	}
	if v = Present(encoded, now, now.Add(31*time.Second)); v.Communication != "disconnected" || !v.Capabilities[0].Unresolved {
		t.Fatalf("lost failure on disconnect: %+v", v)
	}
	// New heartbeat after communication recovery still carries unresolved storage.
	h.CollectedAt = now.Add(time.Minute).Unix()
	encoded, _ = Encode(h)
	v = Present(encoded, now.Add(time.Minute), now.Add(time.Minute))
	if v.Communication != "connected" || !v.Capabilities[0].Unresolved {
		t.Fatal("communication recovery cleared storage failure")
	}
	// No business traffic: a fresh heartbeat cannot refresh business evidence.
	h.CollectedAt = now.Add(3 * time.Minute).Unix()
	encoded, _ = Encode(h)
	v = Present(encoded, now.Add(3*time.Minute), now.Add(3*time.Minute))
	if v.Capabilities[1].Status != "unknown" || v.Capabilities[0].Status != "unresolved" {
		t.Fatalf("stale evidence became healthy: %+v", v)
	}
	h.ProcessId = "process-2"
	h.StartedAt = h.CollectedAt
	h.Errors = nil
	for _, c := range h.Capabilities {
		c.State = 0
		c.Unresolved = false
		c.LastObservation = 0
		c.LastFailure = 0
	}
	encoded, _ = Encode(h)
	v = Present(encoded, now, now)
	if v.Capabilities[0].Status != "unknown" {
		t.Fatal("restart treated as recovery")
	}
	if v = Present(nil, now, now); v.Reported || v.Communication != "connected" {
		t.Fatal("legacy node not handled")
	}
	bad := "{broken"
	if !Present(&bad, now, now).ReportError {
		t.Fatal("corrupt storage silently accepted")
	}
}

func TestInvalidSnapshots(t *testing.T) {
	for _, mutate := range []func(*pb.NodeHealthSnapshot){
		func(h *pb.NodeHealthSnapshot) { h.Version = 2 },
		func(h *pb.NodeHealthSnapshot) { h.HeartbeatPeriodSeconds = 0 },
		func(h *pb.NodeHealthSnapshot) { h.Capabilities[0].State = 99 },
		func(h *pb.NodeHealthSnapshot) { h.Capabilities[1].Capability = h.Capabilities[0].Capability },
		func(h *pb.NodeHealthSnapshot) { h.Errors[0].Kind = "raw_secret_error" },
		func(h *pb.NodeHealthSnapshot) { h.Errors = append(h.Errors, h.Errors[0]) },
	} {
		h := fixture(time.Now())
		mutate(h)
		if _, err := Encode(h); err == nil {
			t.Fatal("invalid snapshot accepted")
		}
	}
}
