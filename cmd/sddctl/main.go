// Command sddctl is the specification-driven-development governance tool for this
// repository. It parses the artifact chain, enforces bilingual parity, validates
// traceability from goal to source line, and computes the cascading impact of any
// upstream change.
//
// Usage:
//
//	sddctl validate [--root DIR] [--json] [--strict]
//	sddctl lint     [--root DIR] [--json]
//	sddctl trace    [--root DIR] [--json]
//	sddctl drift    [--root DIR] [--json] [--fail-on-stale]
//	sddctl seal     [--root DIR]
//	sddctl gate --stage STAGE [--root DIR] [--json]
//	sddctl impact ID [ID...] [--root DIR] [--json]
//	sddctl matrix   [--root DIR] [--out FILE]
//	sddctl graph    [--root DIR] [--out FILE]
//	sddctl stats    [--root DIR]
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/zlrrr/mutil-agent-system/internal/sdd"
)

// sdd:impl DLD-0109

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd := os.Args[1]
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	root := fs.String("root", ".", "repository root")
	asJSON := fs.Bool("json", false, "emit machine-readable JSON")
	strict := fs.Bool("strict", false, "treat warnings as errors")
	failStale := fs.Bool("fail-on-stale", false, "exit non-zero when the tree is stale")
	stage := fs.String("stage", "", "stage name for `gate`")
	out := fs.String("out", "", "write output to a file instead of stdout")

	args := os.Args[2:]
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}

	switch cmd {
	case "help", "-h", "--help":
		usage()
		return
	}

	model, err := sdd.Load(*root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sddctl: %v\n", err)
		os.Exit(2)
	}

	switch cmd {
	case "validate":
		os.Exit(emitReport(model.Validate(), model, *asJSON, *strict, "validate"))

	case "lint":
		os.Exit(emitReport(model.Lint(), model, *asJSON, *strict, "lint"))

	case "trace":
		os.Exit(emitReport(model.Trace(), model, *asJSON, *strict, "trace"))

	case "drift":
		d, err := model.Drift()
		if err != nil {
			fatal(err)
		}
		if *asJSON {
			writeJSON(d)
			if *failStale && !d.Clean() {
				os.Exit(1)
			}
			return
		}
		printDrift(d)
		if *failStale && !d.Clean() {
			os.Exit(1)
		}

	case "seal":
		lock, err := model.Seal()
		if err != nil {
			fatal(err)
		}
		fmt.Printf("sealed %d item(s) at %s -> %s\n",
			len(lock.Items), lock.SealedAt, model.Config.LockFile)

	case "gate":
		if *stage == "" {
			fatal(fmt.Errorf("gate requires --stage"))
		}
		res, err := model.Gate(*stage)
		if err != nil {
			fatal(err)
		}
		if *asJSON {
			writeJSON(res)
		} else {
			printGate(res)
		}
		if !res.Passed {
			os.Exit(1)
		}

	case "impact":
		ids := fs.Args()
		if len(ids) == 0 {
			fatal(fmt.Errorf("impact requires at least one item id"))
		}
		items, files := model.ImpactOf(ids...)
		if *asJSON {
			writeJSON(map[string]any{"roots": ids, "items": items, "files": files})
			return
		}
		fmt.Printf("Changing %s would require revisiting:\n\n", strings.Join(ids, ", "))
		if len(items) == 0 && len(files) == 0 {
			fmt.Println("  (nothing — this item has no descendants)")
			return
		}
		for _, it := range items {
			fmt.Printf("  spec  %s\n", it)
		}
		for _, f := range files {
			fmt.Printf("  code  %s\n", f)
		}
		fmt.Printf("\n%d specification item(s), %d source file(s)\n", len(items), len(files))

	case "matrix":
		write(*out, model.RenderMatrix())

	case "graph":
		write(*out, model.RenderGraph())

	case "stats":
		fmt.Println(model.Stats())

	default:
		fmt.Fprintf(os.Stderr, "sddctl: unknown command %q\n\n", cmd)
		usage()
		os.Exit(2)
	}
}

func emitReport(rep *sdd.Report, m *sdd.Model, asJSON, strict bool, label string) int {
	if asJSON {
		writeJSON(rep)
	} else {
		fmt.Printf("sddctl %s — %s\n", label, m.Stats())
		if len(rep.Findings) == 0 {
			fmt.Println("\n  no findings")
		} else {
			fmt.Println()
			for _, f := range rep.Sorted() {
				fmt.Printf("  %s\n", f)
			}
		}
		fmt.Printf("\n%s\n", rep.Summary())
	}
	if rep.HasErrors() {
		return 1
	}
	if strict && rep.Count(sdd.SeverityWarn) > 0 {
		return 1
	}
	return 0
}

func printDrift(d *sdd.DriftResult) {
	if !d.Sealed {
		fmt.Println("specification tree has never been sealed; run `sddctl seal`")
		return
	}
	fmt.Printf("baseline sealed at %s\n\n", d.SealedAt)
	if d.Clean() {
		fmt.Println("  clean — every artifact matches its sealed baseline")
		return
	}
	section := func(title string, ids []string, note string) {
		if len(ids) == 0 {
			return
		}
		fmt.Printf("  %s (%d)%s\n", title, len(ids), note)
		for _, id := range ids {
			fmt.Printf("    %s\n", id)
		}
		fmt.Println()
	}
	section("MODIFIED", d.Modified, " — changed since seal")
	section("ADDED", d.Added, " — not yet sealed")
	section("REMOVED", d.Removed, " — sealed but gone")
	section("STALE", d.Stale, " — upstream changed, must be revisited")
	section("STALE FILES", d.StaleFile, " — source implementing changed design")
	fmt.Println("resolve by updating each stale artifact, then run `sddctl seal`")
}

func printGate(res *sdd.GateResult) {
	verdict := "PASS"
	if !res.Passed {
		verdict = "FAIL"
	}
	fmt.Printf("gate %s: %s\n\n", res.Stage, verdict)
	for _, c := range res.Checks {
		mark := "ok  "
		if !c.Passed {
			mark = "FAIL"
		}
		fmt.Printf("  [%s] %-28s %s\n", mark, c.Name, c.Detail)
		for _, b := range c.Blocked {
			fmt.Printf("           %s\n", b)
		}
	}
	for _, u := range res.Unknowns {
		fmt.Printf("  [FAIL] %-28s unknown gate requirement\n", u)
	}
}

func writeJSON(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		fatal(err)
	}
}

func write(path, content string) {
	if path == "" {
		fmt.Print(content)
		return
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		fatal(err)
	}
	fmt.Fprintf(os.Stderr, "wrote %s\n", path)
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "sddctl: %v\n", err)
	os.Exit(2)
}

func usage() {
	fmt.Fprint(os.Stderr, `sddctl — specification-driven-development governance

Commands:
  validate    lint + trace + drift; non-zero exit on any error finding
  lint        bilingual document parity (CON-004)
  trace       derivation graph and code anchor integrity (CON-001, CON-002, CON-006)
  drift       compare against the sealed baseline and print the stale set (CON-003)
  seal        record current content hashes as the accepted baseline
  gate        assert the preconditions for entering a stage  --stage NAME
  impact      print the downstream blast radius of changing an item  ID [ID...]
  matrix      requirement-to-source traceability matrix (markdown)
  graph       artifact derivation graph (mermaid)
  stats       one-line tree summary

Common flags:
  --root DIR        repository root (default ".")
  --json            machine-readable output
  --strict          treat warnings as errors (validate/lint/trace)
  --fail-on-stale   non-zero exit when drift is not clean
  --out FILE        write to a file (matrix/graph)
`)
}
