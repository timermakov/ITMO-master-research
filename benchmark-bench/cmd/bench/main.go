package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"
	"strings"

	"github.com/itmo-vkr/dwss/benchmark-bench/internal/experiment"
	"github.com/itmo-vkr/dwss/benchmark-bench/internal/profile"
	"github.com/itmo-vkr/dwss/benchmark-bench/internal/progress"
	"github.com/itmo-vkr/dwss/benchmark-bench/internal/report"
)

func main() {
	runtime.GOMAXPROCS(1)
	if len(os.Args) < 2 {
		log.Fatal("usage: bench run [--scenarios LIST] | bench run <scenario> | bench report --in <dir>")
	}
	switch os.Args[1] {
	case "run":
		runCmd(os.Args[2:])
	case "report":
		reportCmd(os.Args[2:])
	default:
		log.Fatal("unknown command")
	}
}

func runCmd(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	scenariosFlag := fs.String("scenarios", "", "comma-separated scenarios: s0-control,s-ref,s-dw,overhead,all")
	out := fs.String("out", "results", "output directory")
	target := fs.String("target-instance", "", "override warmup instance id")
	active := fs.String("active-instance", "", "override active instance id")
	ready := fs.Int("ready-after", 0, "override ready-after shadow count")
	if err := fs.Parse(args); err != nil {
		log.Fatal(err)
	}

	positional := ""
	if fs.NArg() > 0 {
		positional = fs.Arg(0)
	}

	selected, err := resolveScenarios(*scenariosFlag, positional)
	if err != nil {
		log.Fatal(err)
	}

	m, err := profile.LoadFromEnv()
	if err != nil {
		log.Fatal(err)
	}
	m.EnrichFromFlags(*target, *active, *ready)
	if err := m.ValidateWarmupReadyAfter(); err != nil {
		log.Fatal(err)
	}

	progress.LogPlan(selected, progress.EstimateTotalSec(m, selected))

	ctx := context.Background()
	results, hypo, err := runScenarios(ctx, m, selected)
	if err != nil {
		log.Fatal(err)
	}

	if err := report.WriteJSON(*out, m, results, hypo); err != nil {
		log.Fatal(err)
	}
	if err := report.WriteMarkdown(*out, m, results, hypo); err != nil {
		log.Fatal(err)
	}
	for _, r := range results {
		if !r.CVPass {
			log.Fatalf("CV check failed for %s: %s", r.Scenario, r.CVPassReason)
		}
	}
	fmt.Println("wrote results to", *out)
}

func resolveScenarios(flagValue, positional string) ([]string, error) {
	raw := strings.TrimSpace(flagValue)
	if raw == "" {
		raw = strings.TrimSpace(positional)
	}
	if raw == "" {
		return nil, fmt.Errorf("scenario required: use --scenarios or positional argument")
	}
	if raw == "all" {
		return []string{"s0-control", "s-ref", "s-dw"}, nil
	}

	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		name := normalizeScenario(strings.TrimSpace(part))
		if name == "" {
			continue
		}
		out = append(out, name)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no scenarios selected")
	}
	return out, nil
}

func normalizeScenario(name string) string {
	switch name {
	case "h4-overhead":
		return "overhead"
	default:
		return name
	}
}

func runScenarios(ctx context.Context, m profile.Manifest, names []string) ([]experiment.Result, map[string]string, error) {
	var results []experiment.Result
	hypo := map[string]string{}

	var s0, sRef, sDw experiment.Result
	ranS0, ranSRef, ranSDw := false, false, false

	for _, name := range names {
		switch name {
		case "s0-control":
			res, err := experiment.RunS0Control(m)
			if err != nil {
				return nil, nil, err
			}
			s0 = res
			results = append(results, res)
			ranS0 = true
		case "s-ref":
			res, err := experiment.RunSRef(m)
			if err != nil {
				return nil, nil, err
			}
			sRef = res
			results = append(results, res)
			ranSRef = true
		case "s-dw":
			res, err := experiment.RunSDw(ctx, m)
			if err != nil {
				return nil, nil, err
			}
			sDw = res
			results = append(results, res)
			ranSDw = true
		case "overhead":
			oh, err := experiment.RunOverhead(ctx, m)
			if err != nil {
				return nil, nil, err
			}
			results = append(results, oh...)
			if len(oh) == 2 {
				hypo["overhead"] = experiment.EvaluateOverhead(oh[0], oh[1])
			}
		default:
			return nil, nil, fmt.Errorf("unknown scenario %s", name)
		}
	}

	if ranS0 && ranSRef && ranSDw {
		for k, v := range experiment.EvaluateHypotheses(s0, sRef, sDw) {
			hypo[k] = v
		}
	}
	return results, hypo, nil
}

func reportCmd(args []string) {
	fs := flag.NewFlagSet("report", flag.ExitOnError)
	in := fs.String("in", "results", "input directory")
	_ = fs.Parse(args)
	payload, err := report.ReadPayload(*in + "/results.json")
	if err != nil {
		log.Fatal(err)
	}
	warnings, ok := report.ValidatePayload(payload)
	for _, warning := range warnings {
		fmt.Println("WARN:", warning)
	}
	if len(payload.Hypotheses) > 0 {
		fmt.Println("hypotheses:")
		for _, key := range []string{"H1", "H2", "H2_CI", "H3", "overhead", "H4"} {
			if value, found := payload.Hypotheses[key]; found {
				fmt.Printf("- %s: %s\n", key, value)
			}
		}
	}
	if !ok {
		log.Fatal("report validation failed")
	}
	fmt.Println("report validation passed")
}
