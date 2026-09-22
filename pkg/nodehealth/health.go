// Package nodehealth validates transported observations, not node readiness.
package nodehealth

import (
	"encoding/json"
	"fmt"
	"time"

	pb "dingoscheduler/pkg/proto/manager"
	"google.golang.org/protobuf/encoding/protojson"
)

func Encode(h *pb.NodeHealthSnapshot) (*string, error) {
	if h == nil {
		return nil, nil
	}
	if h.Version != 1 || h.ProcessId == "" || len(h.ProcessId) > 128 || h.StartedAt <= 0 || h.CollectedAt < h.StartedAt || h.HeartbeatPeriodSeconds == 0 || h.HeartbeatPeriodSeconds > 86400 {
		return nil, fmt.Errorf("invalid health snapshot header")
	}
	if len(h.Capabilities) != 2 || len(h.Errors) > 16 {
		return nil, fmt.Errorf("invalid health observation count")
	}
	seen := map[string]bool{}
	for _, c := range h.Capabilities {
		if c == nil || (c.Capability != "metadata_read" && c.Capability != "metadata_write") || seen[c.Capability] || c.State < 0 || c.State > 4 || c.LastObservation < 0 || c.LastFailure < 0 || c.LastObservation > h.CollectedAt || c.LastFailure > h.CollectedAt {
			return nil, fmt.Errorf("invalid capability observation")
		}
		seen[c.Capability] = true
	}
	seen = map[string]bool{}
	for _, e := range h.Errors {
		if e == nil {
			return nil, fmt.Errorf("invalid error count")
		}
		if e.Operation != "stat" && e.Operation != "read" && e.Operation != "mkdir" && e.Operation != "write" {
			return nil, fmt.Errorf("invalid error operation")
		}
		if e.Kind != "permission_denied" && e.Kind != "mount_disconnected" && e.Kind != "io_failure" && e.Kind != "path_unavailable" {
			return nil, fmt.Errorf("invalid error kind")
		}
		key := e.Operation + ":" + e.Kind
		if seen[key] {
			return nil, fmt.Errorf("duplicate error count")
		}
		seen[key] = true
	}
	encoded, err := (protojson.MarshalOptions{UseProtoNames: true, EmitUnpopulated: true}).Marshal(h)
	if err != nil {
		return nil, err
	}
	value := string(encoded)
	return &value, nil
}

type CapabilityView struct {
	Capability      string `json:"capability"`
	Status          string `json:"status"`
	Unresolved      bool   `json:"unresolved"`
	LastObservation int64  `json:"lastObservation"`
	LastFailure     int64  `json:"lastFailure"`
}

type View struct {
	Communication string           `json:"communication"`
	ReceivedAt    time.Time        `json:"receivedAt"`
	Reported      bool             `json:"reported"`
	ReportError   bool             `json:"reportError"`
	Snapshot      json.RawMessage  `json:"snapshot,omitempty"`
	Capabilities  []CapabilityView `json:"capabilities"`
}

func Present(encoded *string, received, now time.Time) View {
	v := View{Communication: "connected", ReceivedAt: received, Capabilities: []CapabilityView{}}
	maxAge := 5 * time.Minute
	if encoded != nil {
		h := &pb.NodeHealthSnapshot{}
		err := protojson.Unmarshal([]byte(*encoded), h)
		if err == nil {
			_, err = Encode(h)
		}
		if err != nil {
			v.ReportError = true
		} else {
			v.Reported, v.Snapshot = true, json.RawMessage(*encoded)
			maxAge = max(30*time.Second, time.Duration(h.HeartbeatPeriodSeconds)*3*time.Second)
			for _, c := range h.Capabilities {
				status := "unknown"
				// Transport age uses scheduler time; evidence age uses speed-relative
				// timestamps, avoiding dependence on synchronized node clocks.
				evidenceAge := h.CollectedAt - c.LastObservation
				fresh := evidenceAge <= 120 && max(time.Duration(0), now.Sub(received)) <= time.Duration(120-evidenceAge)*time.Second
				if c.Unresolved {
					status = "unresolved"
				} else if c.LastObservation > 0 && fresh && c.State == 1 {
					status = "observed_healthy"
				}
				v.Capabilities = append(v.Capabilities, CapabilityView{c.Capability, status, c.Unresolved, c.LastObservation, c.LastFailure})
			}
		}
	}
	if received.IsZero() || now.Sub(received) > maxAge {
		v.Communication = "disconnected"
	}
	return v
}
