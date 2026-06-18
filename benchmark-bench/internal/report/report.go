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
	sb.WriteString("| Scenario | P50 workload (µs) | P95 workload (µs) | CV % | CV pass |\n")
	sb.WriteString("|----------|-------------------|-------------------|------|--------|\n")
	for _, r := range results {
		pass := "no"
		if r.CVPass {
			pass = "yes"
		}
		sb.WriteString(fmt.Sprintf("| %s | %.1f | %.1f | %.2f | %s |\n",
			r.Scenario, r.Summary.P50/1000, r.Summary.P95/1000, r.Summary.CV, pass))
	}
	if len(extra) > 0 {
		sb.WriteString("\n## Hypotheses\n\n")
		for k, v := range extra {
			sb.WriteString(fmt.Sprintf("- **%s**: %s\n", k, v))
		}
	}
	return os.WriteFile(filepath.Join(outDir, "results.md"), []byte(sb.String()), 0o644)
}
