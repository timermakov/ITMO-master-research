package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/itmo-vkr/dwss/warmkit"
)

func (c *coordinator) putInstance(w http.ResponseWriter, r *http.Request) {
	if c.flags == nil {
		http.Error(w, "openfeature backend required", http.StatusNotImplemented)
		return
	}
	id := r.PathValue("id")
	var rec warmkit.InstanceRecord
	if err := json.NewDecoder(r.Body).Decode(&rec); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if rec.Addr == "" {
		http.Error(w, "addr required", http.StatusBadRequest)
		return
	}
	if rec.State == "" {
		rec.State = warmkit.StateRegistered
	}
	if rec.StartedAt == 0 {
		rec.StartedAt = time.Now().UnixMilli()
	}
	if err := c.flags.PutInstance(id, rec); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	c.log.Info("instance registered", slog.String("id", id), slog.String("state", string(rec.State)))
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(rec)
}

func (c *coordinator) putInstanceState(w http.ResponseWriter, r *http.Request) {
	if c.flags == nil {
		http.Error(w, "openfeature backend required", http.StatusNotImplemented)
		return
	}
	id := r.PathValue("id")
	var body struct {
		State warmkit.State `json:"state"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if body.State == "" {
		http.Error(w, "state required", http.StatusBadRequest)
		return
	}
	if err := c.flags.UpdateInstanceState(id, body.State); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	rec, err := c.flags.GetInstance(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(rec)
}

func (c *coordinator) getInstance(w http.ResponseWriter, r *http.Request) {
	if c.flags == nil {
		http.Error(w, "openfeature backend required", http.StatusNotImplemented)
		return
	}
	rec, err := c.flags.GetInstance(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(rec)
}

func (c *coordinator) listInstances(w http.ResponseWriter, _ *http.Request) {
	if c.flags == nil {
		http.Error(w, "openfeature backend required", http.StatusNotImplemented)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(c.flags.Instances())
}

func (c *coordinator) deleteInstance(w http.ResponseWriter, r *http.Request) {
	if c.flags == nil {
		http.Error(w, "openfeature backend required", http.StatusNotImplemented)
		return
	}
	c.flags.DeleteInstance(r.PathValue("id"))
	w.WriteHeader(http.StatusNoContent)
}

func (c *coordinator) getOpenFeatureFlags(w http.ResponseWriter, r *http.Request) {
	if c.flags == nil {
		http.Error(w, "openfeature backend required", http.StatusNotImplemented)
		return
	}
	doc, err := c.flags.Document()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	sum := sha256.Sum256(doc)
	etag := fmt.Sprintf(`"%x"`, sum[:8])
	if match := r.Header.Get("If-None-Match"); match == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("ETag", etag)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(doc)
}
