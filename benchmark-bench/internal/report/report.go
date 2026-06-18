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
	sb.WriteString("| Scenario | N | N filt | P50 workload (µs) | P50 CI (µs) | P95 (µs) | CV % | CV pass |\n")
	sb.WriteString("|----------|---|--------|-------------------|-------------|----------|------|--------|\n")
	for _, r := range results {
		pass := "no"
		if r.CVPass {
			pass = "yes"
		}
		ci := fmt.Sprintf("[%.1f, %.1f]", r.Summary.P50CILow/1000, r.Summary.P50CIHigh/1000)
		sb.WriteString(fmt.Sprintf("| %s | %d | %d | %.1f | %s | %.1f | %.2f | %s |\n",
			r.Scenario, r.Summary.N, r.Summary.NFiltered,
			r.Summary.P50/1000, ci, r.Summary.P95/1000, r.Summary.CV, pass))
		if r.CVPassReason != "" && !r.CVPass {
			sb.WriteString(fmt.Sprintf("| _%s_ | | | _%s_ | | | | |\n", r.Scenario, r.CVPassReason))
		}
		if len(r.Summary.Outliers) > 0 {
			sb.WriteString(fmt.Sprintf("| _%s outliers_ | | | _runs %v_ | | | | |\n", r.Scenario, r.Summary.Outliers))
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
		if r.Summary.NFiltered < manifest.Runs {
			sb.WriteString(fmt.Sprintf("\n> Warning: %s has N_filtered=%d < configured runs=%d\n",
				r.Scenario, r.Summary.NFiltered, manifest.Runs))
		}
	}
	return os.WriteFile(filepath.Join(outDir, "results.md"), []byte(sb.String()), 0o644)
}
