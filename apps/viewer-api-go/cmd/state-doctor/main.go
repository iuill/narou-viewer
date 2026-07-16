package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"narou-viewer/apps/viewer-api-go/internal/statedoctor"
)

type stringList []string

func (values *stringList) String() string {
	return strings.Join(*values, ",")
}

func (values *stringList) Set(value string) error {
	*values = append(*values, value)
	return nil
}

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	defaultDataDir := filepath.Clean("../../data")
	if value := strings.TrimSpace(os.Getenv("VIEWER_API_DATA_DIR")); value != "" {
		defaultDataDir = value
	} else if value := strings.TrimSpace(os.Getenv("DATA_DIR")); value != "" {
		defaultDataDir = value
	}
	flags := flag.NewFlagSet("state-doctor", flag.ContinueOnError)
	flags.SetOutput(stderr)
	dataDir := flags.String("data-dir", defaultDataDir, "viewer data directory")
	format := flags.String("format", "human", "output format: human or json")
	apply := flags.Bool("apply", false, "apply explicitly selected derived-state repairs")
	var findingIDs stringList
	flags.Var(&findingIDs, "finding", "repairable finding ID to apply; repeat for multiple findings")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(stderr, "state-doctor does not accept positional arguments")
		return 2
	}
	if *format != "human" && *format != "json" {
		fmt.Fprintln(stderr, "--format must be human or json")
		return 2
	}
	if !*apply && len(findingIDs) > 0 {
		fmt.Fprintln(stderr, "--finding requires --apply")
		return 2
	}
	if *apply && len(findingIDs) == 0 {
		fmt.Fprintln(stderr, "--apply requires at least one --finding ID")
		return 2
	}

	var report statedoctor.Report
	var err error
	if *apply {
		report, err = statedoctor.Apply(ctx, *dataDir, findingIDs)
	} else {
		report, err = statedoctor.Scan(ctx, *dataDir)
	}
	if err != nil {
		fmt.Fprintf(stderr, "state-doctor: %v\n", err)
		return 2
	}
	if *format == "json" {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(report); err != nil {
			fmt.Fprintf(stderr, "state-doctor: encode report: %v\n", err)
			return 2
		}
	} else {
		if _, err := io.WriteString(stdout, statedoctor.Human(report)); err != nil {
			fmt.Fprintf(stderr, "state-doctor: write report: %v\n", err)
			return 2
		}
	}
	if report.HasIssues() {
		return 1
	}
	return 0
}
