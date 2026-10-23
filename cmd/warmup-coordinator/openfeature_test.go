package main

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/itmo-vkr/dwss/warmkit"
)

func testCoordinator() *coordinator {
	flags := warmkit.NewFlagSource("loaded-service")
	c := &coordinator{
		log:         slog.New(slog.NewTextHandler(os.Stdout, nil)),
		service:     "loaded-service",
		sessions:    map[string]*session{},
		flags:       flags,
		writeMirror: flags.WriteMirror,
		lookup:      flags.GetInstance,
	}
	return c
}

func TestOpenFeatureFlagsAndInstances(t *testing.T) {
	c := testCoordinator()
	srv := httptest.NewServer(c.routes())
	defer srv.Close()

	rec := warmkit.InstanceRecord{Addr: "warmup:8081", State: warmkit.StateRegistered, Role: "warmup"}
	body, _ := json.Marshal(rec)
	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/v1/instances/warmup-1", bytes.NewReader(body))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("register %d", resp.StatusCode)
	}
	_ = resp.Body.Close()

	if err := c.writeMirror(warmkit.MirrorConfig{Enabled: true, Ratio: 0.35, TargetInstanceID: "warmup-1"}); err != nil {
		t.Fatal(err)
	}

	flagsResp, err := http.Get(srv.URL + warmkit.PathOpenFeatureFlags)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = flagsResp.Body.Close() }()
	if flagsResp.StatusCode != http.StatusOK {
		t.Fatalf("flags %d", flagsResp.StatusCode)
	}
	var doc struct {
		Flags map[string]struct {
			DefaultVariant string         `json:"defaultVariant"`
			Variants       map[string]any `json:"variants"`
		} `json:"flags"`
	}
	if err := json.NewDecoder(flagsResp.Body).Decode(&doc); err != nil {
		t.Fatal(err)
	}
	mirrorFlag, ok := doc.Flags[warmkit.MirrorFlagKey("loaded-service")]
	if !ok {
		t.Fatal("missing mirror flag")
	}
	current, ok := mirrorFlag.Variants["current"].(map[string]any)
	if !ok {
		t.Fatalf("variants %+v", mirrorFlag.Variants)
	}
	if current["ratio"] != 0.35 {
		t.Fatalf("ratio %v", current["ratio"])
	}

	stateBody, _ := json.Marshal(map[string]string{"state": string(warmkit.StateReady)})
	stReq, _ := http.NewRequest(http.MethodPut, srv.URL+"/v1/instances/warmup-1/state", bytes.NewReader(stateBody))
	stResp, err := http.DefaultClient.Do(stReq)
	if err != nil {
		t.Fatal(err)
	}
	if stResp.StatusCode != http.StatusOK {
		t.Fatalf("state %d", stResp.StatusCode)
	}
	_ = stResp.Body.Close()
	if !c.instanceReady("warmup-1") {
		t.Fatal("expected ready")
	}
}

func TestOpenFeatureFlagsETag(t *testing.T) {
	c := testCoordinator()
	srv := httptest.NewServer(c.routes())
	defer srv.Close()

	resp, err := http.Get(srv.URL + warmkit.PathOpenFeatureFlags)
	if err != nil {
		t.Fatal(err)
	}
	etag := resp.Header.Get("ETag")
	_ = resp.Body.Close()
	if etag == "" {
		t.Fatal("missing ETag")
	}
	req, _ := http.NewRequest(http.MethodGet, srv.URL+warmkit.PathOpenFeatureFlags, nil)
	req.Header.Set("If-None-Match", etag)
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp2.Body.Close()
	if resp2.StatusCode != http.StatusNotModified {
		t.Fatalf("status %d", resp2.StatusCode)
	}
}

func TestMirrorConfigHTTP(t *testing.T) {
	c := testCoordinator()
	srv := httptest.NewServer(c.routes())
	defer srv.Close()

	body, _ := json.Marshal(warmkit.MirrorConfig{Enabled: false, Ratio: 0, ActiveInstanceID: "active-1"})
	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/v1/mirror/config", bytes.NewReader(body))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	got := c.flags.ReadMirror()
	if got.ActiveInstanceID != "active-1" || got.Enabled {
		t.Fatalf("mirror %+v", got)
	}
}
