// Command evalctl runs every fault case in all three comparison modes and reports the
// result, so the multi-agent claim is computed rather than asserted (REQ-0081).
//
// Usage:
//
//	evalctl run [--cases C1,C2] [--out eval.md] [--json]
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/zlrrr/mutil-agent-system/internal/catalog"
	"github.com/zlrrr/mutil-agent-system/internal/eval"
)

// sdd:impl DLD-1075

func main() {
	if len(os.Args) < 2 || os.Args[1] == "help" || os.Args[1] == "-h" || os.Args[1] == "--help" {
		usage()
		if len(os.Args) < 2 {
			os.Exit(2)
		}
		return
	}
	if os.Args[1] != "run" {
		fmt.Fprintf(os.Stderr, "evalctl: unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}

	fs := flag.NewFlagSet("run", flag.ExitOnError)
	cases := fs.String("cases", "", "comma-separated case identifiers (default: all)")
	out := fs.String("out", "", "write the report to a file instead of stdout")
	asJSON := fs.Bool("json", false, "emit machine-readable JSON")
	if err := fs.Parse(os.Args[2:]); err != nil {
		os.Exit(2)
	}

	cat, err := catalog.Load()
	if err != nil {
		fatal(err)
	}
	var ids []string
	if *cases != "" {
		for _, id := range strings.Split(*cases, ",") {
			if id = strings.TrimSpace(id); id != "" {
				ids = append(ids, id)
			}
		}
	}

	rep, err := eval.RunAll(context.Background(), cat, ids)
	if err != nil {
		fatal(err)
	}

	var content string
	if *asJSON {
		raw, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			fatal(err)
		}
		content = string(raw) + "\n"
	} else {
		content = rep.Markdown()
	}

	if *out == "" {
		fmt.Print(content)
		return
	}
	if err := os.WriteFile(*out, []byte(content), 0o644); err != nil {
		fatal(err)
	}
	fmt.Fprintf(os.Stderr, "wrote %s\n", *out)
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "evalctl: %v\n", err)
	os.Exit(1)
}

func usage() {
	fmt.Fprint(os.Stderr, strings.TrimLeft(`
evalctl — compare single-agent and multi-agent flows over the same fixtures

Usage:
  evalctl run [--cases C1,C2] [--out FILE] [--json]

All three modes see identical inputs, so a difference in the result is attributable
to the flow rather than to the data. Every rate is reported with its sample size.
`, "\n"))
}
