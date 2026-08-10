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
	"github.com/zlrrr/mutil-agent-system/internal/reasoner"
	"github.com/zlrrr/mutil-agent-system/internal/reasoner/strategy"
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
	rsnCfg := strategy.Register(fs)
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

	// A comma-separated selection runs the whole matrix once per adapter, so a single
	// invocation produces the comparison rather than two runs a reader has to align by
	// hand — and nothing would check that those two ran over the same cases (REQ-0104).
	selections, err := strategy.Split(*rsnCfg)
	if err != nil {
		fatal(err)
	}
	opts := make([]eval.Options, 0, len(selections))
	for _, sel := range selections {
		sel := sel
		opts = append(opts, eval.Options{
			Reasoner: sel.Name(),
			NewReasoner: func(c *catalog.Catalog) reasoner.Reasoner {
				r, err := sel.Build(c, reasoner.DefaultConfig())
				if err != nil {
					fatal(err)
				}
				if r == nil {
					return reasoner.NewRuleReasoner(c, reasoner.DefaultConfig())
				}
				return r
			},
		})
	}

	rep, err := eval.RunAllWith(context.Background(), cat, ids, opts)
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
