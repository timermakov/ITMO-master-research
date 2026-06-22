package report

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/itmo-vkr/dwss/benchmark-bench/internal/experiment"
	"github.com/itmo-vkr/dwss/benchmark-bench/internal/profile"
)

// Payload is the persisted benchmark result file format.
type Payload struct {
	Manifest   profile.Manifest    `json:"manifest"`
	Results    []experiment.Result `json:"results"`
	Hypotheses map[string]string   `json:"hypotheses,omitempty"`
}

// WriteJSON writes results to out dir.
func WriteJSON(outDir string, manifest profile.Manifest, results []experiment.Result, extra map[string]string) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	payload := map[string]any{
		"manifest":   manifest,
		"results":    results,
		"hypotheses": extra,
	}
	b, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(outDir, "results.json"), b, 0o644)
}

// WriteMarkdown writes human-readable summary.
func WriteMarkdown(outDir string, manifest profile.Manifest, results []experiment.Result, extra map[string]string) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	var sb strings.Builder
	sb.WriteString("# Benchmark results\n\n")
	sb.WriteString(fmt.Sprintf("Protocol v%s | commit %s | GOMAXPROCS=%d | %s\n\n",
		manifest.ProtocolVersion, manifest.GitCommit, manifest.GOMAXPROCS,
		manifest.StartedAt.Format(time.RFC3339)))
	writeMetadata(&sb, manifest)
	sb.WriteString("| Scenario | N | N filt | Primary P50 (µs) | P50 CI (µs) | Primary P95 (µs) | CV % | CV pass |\n")
	sb.WriteString("|----------|---|--------|------------------|-------------|------------------|------|--------|\n")
	for _, r := range results {
		pass := "no"
		if r.CVPass {
			pass = "yes"
		}
		ci := fmt.Sprintf("[%.1f, %.1f]", r.Summary.P50CILow/1000, r.Summary.P50CIHigh/1000)
		name := resultName(r)
		sb.WriteString(fmt.Sprintf("| %s | %d | %d | %.1f | %s | %.1f | %.2f | %s |\n",
			name, r.Summary.N, r.Summary.NFiltered,
			r.Summary.P50/1000, ci, r.Summary.P95/1000, r.Summary.CV, pass))
		if r.CVPassReason != "" && !r.CVPass {
			sb.WriteString(fmt.Sprintf("| _%s_ | | | _%s_ | | | | |\n", name, r.CVPassReason))
		}
		if len(r.Summary.Outliers) > 0 {
			sb.WriteString(fmt.Sprintf("| _%s outliers_ | | | _indexes %v_ | | | | |\n", name, r.Summary.Outliers))
		}
	}
	if len(extra) > 0 {
		sb.WriteString("\n## Hypotheses\n\n")
		for _, key := range []string{"H1", "H2", "H2_CI", "H3", "H4"} {
			if v, ok := extra[key]; ok {
				sb.WriteString(fmt.Sprintf("- **%s**: %s\n", key, v))
			}
		}
		for k, v := range extra {
			if k == "H1" || k == "H2" || k == "H2_CI" || k == "H3" || k == "H4" {
				continue
			}
			sb.WriteString(fmt.Sprintf("- **%s**: %s\n", k, v))
		}
	}
	for _, r := range results {
		expected := expectedRuns(manifest, r)
		if r.Summary.NFiltered < expected {
			sb.WriteString(fmt.Sprintf("\n> Warning: %s has N_filtered=%d < configured runs=%d\n",
				resultName(r), r.Summary.NFiltered, expected))
		}
		if r.Summary.CV > 5 && r.CVPass {
			sb.WriteString(fmt.Sprintf("\n> Warning: %s CV=%.2f%% is above thesis ideal 5%% but within configured threshold\n",
				resultName(r), r.Summary.CV))
		}
	}
	return os.WriteFile(filepath.Join(outDir, "results.md"), []byte(sb.String()), 0o644)
}

func writeMetadata(sb *strings.Builder, manifest profile.Manifest) {
	sb.WriteString("## Reproducibility metadata\n\n")
	sb.WriteString(fmt.Sprintf("- Go: `%s`, platform: `%s/%s`, CPUs: `%d`\n",
		manifest.GoVersion, manifest.OS, manifest.Arch, manifest.NumCPU))
	sb.WriteString(fmt.Sprintf("- Git dirty: `%t`", manifest.GitDirty))
	if manifest.GitStatus != "" {
		sb.WriteString(fmt.Sprintf(" (`%s`)", strings.ReplaceAll(manifest.GitStatus, "\n", "; ")))
	}
	sb.WriteString("\n")
	if manifest.EnvProfileName != "" {
		sb.WriteString(fmt.Sprintf("- Environment profile: `%s`\n", manifest.EnvProfileName))
	}
	if manifest.DockerVersion != "" {
		sb.WriteString(fmt.Sprintf("- Docker: `%s`\n", manifest.DockerVersion))
	}
	if len(manifest.DockerImages) > 0 {
		sb.WriteString(fmt.Sprintf("- Docker image IDs: `%s`\n", strings.Join(manifest.DockerImages, "`, `")))
	}
	sb.WriteString(fmt.Sprintf("- CV thresholds: global %.2f%%, cold %.2f%%, manual %.2f%%, dynamic %.2f%%\n",
		manifest.CVThreshold, manifest.CVThresholdS0, manifest.CVThresholdSRef, manifest.CVThresholdSDw))
	sb.WriteString(fmt.Sprintf("- H4 runs: target `%d`, max `%d`; readyAfter actual `%d`\n\n",
		manifest.H4Runs, manifest.H4MaxRuns, manifest.ReadyAfterActual))
}

func resultName(r experiment.Result) string {
	if r.DisplayName != "" {
		return r.DisplayName
	}
	name, _ := experiment.ScenarioInfo(r.Scenario)
	return name
}

func expectedRuns(manifest profile.Manifest, r experiment.Result) int {
	switch r.Scenario {
	case "h4-mirror-off", "h4-mirror-on":
		return manifest.H4Runs
	default:
		return manifest.Runs
	}
}

// ReadPayload reads a benchmark results.json file.
func ReadPayload(path string) (Payload, error) {
	var payload Payload
	b, err := os.ReadFile(path)
	if err != nil {
		return payload, err
	}
	if err := json.Unmarshal(b, &payload); err != nil {
		return payload, err
	}
	return payload, nil
}

// ValidatePayload returns warnings and whether the payload is report-ready.
func ValidatePayload(payload Payload) ([]string, bool) {
	warnings := make([]string, 0)
	ok := true
	for _, r := range payload.Results {
		if !r.CVPass {
			ok = false
			warnings = append(warnings, fmt.Sprintf("%s CV failed: %s", resultName(r), r.CVPassReason))
		}
		expected := expectedRuns(payload.Manifest, r)
		if r.Summary.NFiltered < expected {
			warnings = append(warnings, fmt.Sprintf("%s has N_filtered=%d < configured runs=%d",
				resultName(r), r.Summary.NFiltered, expected))
		}
	}
	return warnings, ok
}
