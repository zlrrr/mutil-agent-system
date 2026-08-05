// Command faultctl drives the demo stack's fault injection: it applies, inspects and
// reverses the configuration change a fault case describes.
//
// It talks to the demo order-api over HTTP and never to a real system — the actuator
// safety model (ARC-010) applies to the arena service, and this command is the operator
// side of the same boundary.
//
// Usage:
//
//	faultctl inject  --case C1 [--target http://localhost:8081]
//	faultctl restore --case C1 [--target http://localhost:8081]
//	faultctl status         [--target http://localhost:8081]
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/zlrrr/mutil-agent-system/internal/catalog"
)

// sdd:impl DLD-1075

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd := os.Args[1]
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	target := fs.String("target", env("FAULTCTL_TARGET", "http://localhost:8081"),
		"demo order-api base URL")
	caseID := fs.String("case", "C1", "fault case identifier")
	if err := fs.Parse(os.Args[2:]); err != nil {
		os.Exit(2)
	}

	var err error
	switch cmd {
	case "inject":
		err = apply(*target, *caseID, false)
	case "restore":
		err = apply(*target, *caseID, true)
	case "status":
		err = status(*target)
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "faultctl: unknown command %q\n\n", cmd)
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "faultctl: %v\n", err)
		os.Exit(1)
	}
}

// apply pushes the fault case's configuration change to the demo service, or its
// inverse when restoring.
func apply(target, caseID string, restore bool) error {
	cat, err := catalog.Load()
	if err != nil {
		return err
	}
	fc, ok := cat.Case(caseID)
	if !ok {
		return fmt.Errorf("unknown case %q (have %v)", caseID, cat.CaseIDs())
	}
	if len(fc.Changes) == 0 {
		return fmt.Errorf("case %s declares no configuration change to inject", caseID)
	}

	ch := fc.Changes[0]
	value := ch.New
	verb := "injecting"
	if restore {
		value = ch.Old
		verb = "restoring"
	}
	fmt.Printf("%s %s: %s=%s on %s\n", verb, caseID, ch.Key, value, target)

	body, _ := json.Marshal(map[string]string{"key": ch.Key, "value": value})
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		strings.TrimRight(target, "/")+"/admin/config", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("reach %s: %w (is the demo stack up?)", target, err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 300 {
		return fmt.Errorf("%s returned %s: %s", target, res.Status, strings.TrimSpace(string(raw)))
	}
	fmt.Printf("%s\n", strings.TrimSpace(string(raw)))
	return nil
}

func status(target string) error {
	client := &http.Client{Timeout: 10 * time.Second}
	res, err := client.Get(strings.TrimRight(target, "/") + "/admin/config")
	if err != nil {
		return fmt.Errorf("reach %s: %w (is the demo stack up?)", target, err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	fmt.Printf("%s\n", strings.TrimSpace(string(raw)))
	return nil
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func usage() {
	fmt.Fprint(os.Stderr, strings.TrimLeft(`
faultctl — drive the demo stack's fault injection

Commands:
  inject  --case C1   apply the case's configuration change to the demo service
  restore --case C1   put the previous value back
  status              print the demo service's current configuration

Flags:
  --target  demo order-api base URL   (env FAULTCTL_TARGET, default http://localhost:8081)

This command drives the demo stack only. The arena service itself reaches a target
system exclusively through its policy-gated executor.
`, "\n"))
}
