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
	"github.com/itmo-vkr/dwss/benchmark-bench/internal/report"
)

func main() {
	runtime.GOMAXPROCS(1)
	if len(os.Args) < 2 {
		log.Fatal("usage: bench run <scenario> | bench report --in <dir>")
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
	var scenario string
	flagArgs := make([]string, 0, len(args))
	for _, a := range args {
		if scenario == "" && !strings.HasPrefix(a, "-") {
			scenario = a
			continue
		}
		flagArgs = append(flagArgs, a)
	}
	if scenario == "" {
		log.Fatal("scenario required")
	}

	fs := flag.NewFlagSet("run", flag.ExitOnError)
	out := fs.String("out", "results", "output directory")
	target := fs.String("target-instance", "", "override warmup instance id")
	active := fs.String("active-instance", "", "override active instance id")
	ready := fs.Int("ready-after", 0, "override ready-after shadow count")
	if err := fs.Parse(flagArgs); err != nil {
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

	ctx := context.Background()
	var results []experiment.Result
	var hypo map[string]string

	switch scenario {
	case "s0-control":
		res, err := experiment.RunS0Control(m)
		if err != nil {
			log.Fatal(err)
		}
		results = []experiment.Result{res}
	case "s-ref":
		res, err := experiment.RunSRef(m)
		if err != nil {
			log.Fatal(err)
		}
		results = []experiment.Result{res}
	case "s-dw":
		res, err := experiment.RunSDw(ctx, m)
		if err != nil {
			log.Fatal(err)
		}
		results = []experiment.Result{res}
	case "h4-overhead":
		results, err = experiment.RunH4Overhead(ctx, m)
		if err != nil {
			log.Fatal(err)
		}
		if len(results) == 2 {
			hypo = map[string]string{"H4": experiment.EvaluateH4(results[0], results[1])}
		}
	case "all":
		s0, err := experiment.RunS0Control(m)
		if err != nil {
			log.Fatal(err)
		}
		sRef, err := experiment.RunSRef(m)
		if err != nil {
			log.Fatal(err)
		}
		sDw, err := experiment.RunSDw(ctx, m)
		if err != nil {
			log.Fatal(err)
		}
		results = []experiment.Result{s0, sRef, sDw}
		hypo = experiment.EvaluateHypotheses(s0, sRef, sDw)
	default:
		log.Fatalf("unknown scenario %s", scenario)
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
		for _, key := range []string{"H1", "H2", "H2_CI", "H3", "H4"} {
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
