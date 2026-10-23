package warmkit

import (
	"fmt"
	"strings"
)

// Backend is the distributed coordination implementation.
type Backend string

const (
	// BackendZooKeeper stores instance records and mirror config in ZooKeeper.
	BackendZooKeeper Backend = "zookeeper"
	// BackendOpenFeature stores coordination state as OpenFeature flags served by flagd.
	BackendOpenFeature Backend = "openfeature"
)

// ParseBackend parses DWSS_COORDINATION.
func ParseBackend(s string) (Backend, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "zookeeper", "zk":
		return BackendZooKeeper, nil
	case "openfeature", "flagd", "of":
		return BackendOpenFeature, nil
	default:
		return "", fmt.Errorf("unknown coordination backend %q (want zookeeper or openfeature)", s)
	}
}

// CoordinationView is a consistent snapshot of mirror config and instance registry.
type CoordinationView struct {
	Mirror    MirrorConfig
	Instances map[string]InstanceRecord
}

const (
	// PathOpenFeatureFlags is the flagd-compatible document served by warmup-coordinator.
	PathOpenFeatureFlags = "/v1/openfeature/flags"
	// PathInstances is the instance registry prefix on warmup-coordinator.
	PathInstances = "/v1/instances/"
)

// InstancePath is PUT/GET/DELETE /v1/instances/{id}.
func InstancePath(instanceID string) string {
	return PathInstances + instanceID
}

// InstanceStatePath is PUT /v1/instances/{id}/state.
func InstanceStatePath(instanceID string) string {
	return PathInstances + instanceID + "/state"
}
