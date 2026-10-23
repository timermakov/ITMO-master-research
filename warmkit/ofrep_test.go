package warmkit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestOFREPView(t *testing.T) {
	mirror := MirrorConfig{Enabled: true, Ratio: 0.6, TargetInstanceID: "warmup-1", ActiveInstanceID: "active-1"}
	instances := map[string]InstanceRecord{
		"active-1": {Addr: "a:8080", State: StateActive, Role: "active"},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method %s", r.Method)
		}
		key := r.URL.Path[len("/ofrep/v1/evaluate/flags/"):]
		var value any
		switch key {
		case MirrorFlagKey("loaded-service"):
			value = mirror
		case InstancesFlagKey("loaded-service"):
			value = instances
		default:
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ofrepEvalResponse{
			Key:     key,
			Value:   mustRaw(t, value),
			Reason:  "STATIC",
			Variant: flagVariantCurrent,
		})
	}))
	defer srv.Close()

	c := newOFREPClient(srv.URL, time.Second)
	view, err := c.view(context.Background(), "loaded-service")
	if err != nil {
		t.Fatal(err)
	}
	if view.Mirror.Ratio != 0.6 {
		t.Fatalf("ratio %v", view.Mirror.Ratio)
	}
	if view.Instances["active-1"].Addr != "a:8080" {
		t.Fatalf("instances %+v", view.Instances)
	}
}

func TestOFRegistryRegisterAndWatch(t *testing.T) {
	flags := NewFlagSource("loaded-service")
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /v1/instances/{id}", func(w http.ResponseWriter, r *http.Request) {
		var rec InstanceRecord
		if err := json.NewDecoder(r.Body).Decode(&rec); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := flags.PutInstance(r.PathValue("id"), rec); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("PUT /v1/instances/{id}/state", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			State State `json:"state"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := flags.UpdateInstanceState(r.PathValue("id"), body.State); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("DELETE /v1/instances/{id}", func(w http.ResponseWriter, r *http.Request) {
		flags.DeleteInstance(r.PathValue("id"))
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /ofrep/v1/evaluate/flags/{key}", func(w http.ResponseWriter, r *http.Request) {
		doc, err := flags.Document()
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		var parsed flagdDocument
		if err := json.Unmarshal(doc, &parsed); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		flag, ok := parsed.Flags[r.PathValue("key")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ofrepEvalResponse{
			Key:     r.PathValue("key"),
			Value:   mustRaw(t, flag.Variants[flagVariantCurrent]),
			Variant: flagVariantCurrent,
			Reason:  "STATIC",
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	cfg := OFConfig{
		OFREPURL:       srv.URL,
		CoordinatorURL: srv.URL,
		PollInterval:   20 * time.Millisecond,
		HTTPTimeout:    time.Second,
	}
	reg := NewOFRegistry(cfg, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := reg.RegisterInstance(ctx, "loaded-service", "warmup-1", InstanceRecord{
		Addr: "w:8081", State: StateRegistered, Role: "warmup",
	}); err != nil {
		t.Fatal(err)
	}
	if err := flags.WriteMirror(MirrorConfig{Enabled: true, Ratio: 0.15, TargetInstanceID: "warmup-1"}); err != nil {
		t.Fatal(err)
	}
	ch, err := reg.WatchMirror(ctx, "loaded-service")
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.After(time.Second)
	for {
		select {
		case cfg := <-ch:
			if cfg.Ratio == 0.15 && cfg.TargetInstanceID == "warmup-1" {
				if err := reg.UpdateInstanceState(ctx, "loaded-service", "warmup-1", StateWarming); err != nil {
					t.Fatal(err)
				}
				rec, err := flags.GetInstance("warmup-1")
				if err != nil {
					t.Fatal(err)
				}
				if rec.State != StateWarming {
					t.Fatalf("state %s", rec.State)
				}
				if err := reg.Close(); err != nil {
					t.Fatal(err)
				}
				if _, err := flags.GetInstance("warmup-1"); err == nil {
					t.Fatal("expected deregister")
				}
				return
			}
		case <-deadline:
			t.Fatal("did not observe mirror flag")
		}
	}
}
