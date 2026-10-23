package warmkit

import (
	"encoding/json"
	"testing"
)

func TestParseBackend(t *testing.T) {
	cases := []struct {
		in   string
		want Backend
	}{
		{"zookeeper", BackendZooKeeper},
		{"ZK", BackendZooKeeper},
		{"openfeature", BackendOpenFeature},
		{"flagd", BackendOpenFeature},
	}
	for _, tc := range cases {
		got, err := ParseBackend(tc.in)
		if err != nil {
			t.Fatalf("%s: %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("%s: got %s want %s", tc.in, got, tc.want)
		}
	}
	if _, err := ParseBackend("etcd"); err == nil {
		t.Fatal("expected error")
	}
}

func TestBuildFlagdDocumentRoundTrip(t *testing.T) {
	mirror := MirrorConfig{
		Enabled:            true,
		Ratio:              0.35,
		TargetInstanceID:   "warmup-1",
		ActiveInstanceID:   "active-1",
		SessionID:          "s1",
		UpdatedAtUnixMilli: 42,
	}
	instances := map[string]InstanceRecord{
		"active-1": {Addr: "active:8080", State: StateActive, Role: "active", StartedAt: 1},
		"warmup-1": {Addr: "warmup:8081", State: StateWarming, Role: "warmup", StartedAt: 2},
	}
	raw, err := BuildFlagdDocument("loaded-service", mirror, instances)
	if err != nil {
		t.Fatal(err)
	}
	var doc flagdDocument
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Schema != flagdSchema {
		t.Fatalf("schema %s", doc.Schema)
	}
	mirrorFlag, ok := doc.Flags[MirrorFlagKey("loaded-service")]
	if !ok || mirrorFlag.DefaultVariant != flagVariantCurrent {
		t.Fatal("missing mirror flag")
	}
	gotMirror, err := decodeJSON[MirrorConfig](mustRaw(t, mirrorFlag.Variants[flagVariantCurrent]))
	if err != nil {
		t.Fatal(err)
	}
	if gotMirror.Ratio != 0.35 || gotMirror.TargetInstanceID != "warmup-1" {
		t.Fatalf("mirror %+v", gotMirror)
	}
	instFlag := doc.Flags[InstancesFlagKey("loaded-service")]
	gotInst, err := decodeJSON[map[string]InstanceRecord](mustRaw(t, instFlag.Variants[flagVariantCurrent]))
	if err != nil {
		t.Fatal(err)
	}
	if gotInst["warmup-1"].State != StateWarming {
		t.Fatalf("instances %+v", gotInst)
	}
}

func mustRaw(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestFlagSourceDocumentAndLookup(t *testing.T) {
	s := NewFlagSource("loaded-service")
	if err := s.WriteMirror(MirrorConfig{Enabled: true, Ratio: 1, ActiveInstanceID: "active-1"}); err != nil {
		t.Fatal(err)
	}
	if err := s.PutInstance("warmup-1", InstanceRecord{Addr: "w:8081", State: StateRegistered, Role: "warmup"}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateInstanceState("warmup-1", StateReady); err != nil {
		t.Fatal(err)
	}
	rec, err := s.GetInstance("warmup-1")
	if err != nil {
		t.Fatal(err)
	}
	if rec.State != StateReady {
		t.Fatalf("state %s", rec.State)
	}
	raw, err := s.Document()
	if err != nil {
		t.Fatal(err)
	}
	var doc flagdDocument
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Flags) != 2 {
		t.Fatalf("flags %d", len(doc.Flags))
	}
	s.DeleteInstance("warmup-1")
	if _, err := s.GetInstance("warmup-1"); err == nil {
		t.Fatal("expected missing instance")
	}
}
