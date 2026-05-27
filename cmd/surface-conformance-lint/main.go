// surface-conformance-lint is the CLI wrapper around
// github.com/dakasa-yggdrasil/surface-conformance/pkg/conformance. Given a path
// to a surface repository, it walks the source tree, applies the six
// SURFACE_CONTRACT.md §3 invariant checks, prints findings, and exits with
// code 1 when any finding is SeverityError.
//
//	surface-conformance-lint /path/to/surface
//	surface-conformance-lint -format=json /path/to/surface
//	surface-conformance-lint -skip=lego,multi-tenant /path/to/surface
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/dakasa-yggdrasil/surface-conformance/pkg/conformance"
)

func main() {
	format := flag.String("format", "text", "output format: text | json")
	skipFlag := flag.String("skip", "", "comma-separated invariant ids to skip (stateless,auth,backend-agnostic,multi-tenant,lego,no-business-decisions)")
	allowPaths := flag.String("allow", "", "comma-separated glob paths to exclude from scanning (e.g. legacy/**)")
	allowPrefixes := flag.String("allow-url-prefix", "", "comma-separated additional URL prefixes treated as core-canonical (rare)")
	flag.Parse()

	if flag.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: surface-conformance-lint [flags] <surface-repo-path>")
		flag.PrintDefaults()
		os.Exit(2)
	}
	target := flag.Arg(0)

	cfg := conformance.Config{}
	if *skipFlag != "" {
		for _, s := range strings.Split(*skipFlag, ",") {
			cfg.Skip = append(cfg.Skip, conformance.Invariant(strings.TrimSpace(s)))
		}
	}
	if *allowPaths != "" {
		for _, p := range strings.Split(*allowPaths, ",") {
			cfg.AllowPaths = append(cfg.AllowPaths, strings.TrimSpace(p))
		}
	}
	if *allowPrefixes != "" {
		for _, p := range strings.Split(*allowPrefixes, ",") {
			cfg.AllowedURLPrefixes = append(cfg.AllowedURLPrefixes, strings.TrimSpace(p))
		}
	}

	linter := conformance.NewLinter(cfg)
	findings, err := linter.Lint(target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "surface-conformance-lint: %v\n", err)
		os.Exit(2)
	}

	switch *format {
	case "json":
		out, err := conformance.FormatJSON(findings)
		if err != nil {
			fmt.Fprintf(os.Stderr, "surface-conformance-lint: %v\n", err)
			os.Exit(2)
		}
		fmt.Println(string(out))
	default:
		if text := conformance.FormatText(findings); text != "" {
			fmt.Println(text)
		}
	}

	max := conformance.MaxSeverity(findings)
	if max == conformance.SeverityWarn {
		fmt.Fprintln(os.Stderr, "surface-conformance-lint: warnings only (exit 0); see SURFACE_CONTRACT.md §5 for exception path")
	} else if max == conformance.SeverityError {
		fmt.Fprintln(os.Stderr, "surface-conformance-lint: hard-line violation(s) — SURFACE_CONTRACT.md §6")
	}
	os.Exit(conformance.ExitCode(max))
}
