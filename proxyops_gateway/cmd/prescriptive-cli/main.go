package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/proxyops/internal/engine"
)

func main() {
	inputPath := flag.String("input", "", "Path to input JSON file (Assessment)")
	outputPath := flag.String("output", "", "Output file path (default: stdout)")
	flag.Parse()

	if *inputPath == "" {
		fmt.Fprintln(os.Stderr, "Usage: prescriptive-cli --input <assessment.json> [--output <report.json>]")
		flag.PrintDefaults()
		os.Exit(1)
	}

	data, err := os.ReadFile(*inputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
		os.Exit(1)
	}

	var a engine.Assessment
	if err := json.Unmarshal(data, &a); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing input JSON: %v\n", err)
		os.Exit(1)
	}

	store := engine.NewMemStore()
	store.AddAssessment(&a)

	report, err := engine.RunAssessment(store, a.ID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error running assessment: %v\n", err)
		os.Exit(1)
	}

	outData, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling report: %v\n", err)
		os.Exit(1)
	}

	if *outputPath != "" {
		if err := os.WriteFile(*outputPath, outData, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing output: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Report written to %s\n", *outputPath)
	} else {
		fmt.Println(string(outData))
	}
}
