package warmkit

import (
	"encoding/json"
)

const flagdSchema = "https://flagd.dev/schema/v0/flags.json"

const flagVariantCurrent = "current"

// MirrorFlagKey is the OpenFeature flag that carries MirrorConfig for a service.
func MirrorFlagKey(service string) string {
	return "dwss." + service + ".mirror"
}

// InstancesFlagKey is the OpenFeature object flag that carries the instance map.
func InstancesFlagKey(service string) string {
	return "dwss." + service + ".instances"
}

type flagdDocument struct {
	Schema string               `json:"$schema"`
	Flags  map[string]flagdFlag `json:"flags"`
}

type flagdFlag struct {
	State          string         `json:"state"`
	Variants       map[string]any `json:"variants"`
	DefaultVariant string         `json:"defaultVariant"`
}

// BuildFlagdDocument renders a flagd v0 flag set for one DWSS service.
func BuildFlagdDocument(service string, mirror MirrorConfig, instances map[string]InstanceRecord) ([]byte, error) {
	if instances == nil {
		instances = map[string]InstanceRecord{}
	}
	doc := flagdDocument{
		Schema: flagdSchema,
		Flags: map[string]flagdFlag{
			MirrorFlagKey(service): {
				State:          "ENABLED",
				DefaultVariant: flagVariantCurrent,
				Variants: map[string]any{
					flagVariantCurrent: mirror,
				},
			},
			InstancesFlagKey(service): {
				State:          "ENABLED",
				DefaultVariant: flagVariantCurrent,
				Variants: map[string]any{
					flagVariantCurrent: instances,
				},
			},
		},
	}
	return json.MarshalIndent(doc, "", "  ")
}

func decodeJSON[T any](raw json.RawMessage) (T, error) {
	var out T
	if len(raw) == 0 || string(raw) == "null" {
		return out, nil
	}
	err := json.Unmarshal(raw, &out)
	return out, err
}
