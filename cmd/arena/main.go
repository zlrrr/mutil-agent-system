// Command arena runs the IncidentOps Arena service and its demo scenarios.
//
// Usage:
//
//	arena serve  [--addr :8080] [--store memory|file] [--data ./data]
//	             [--signal-profile fixture|live] [--prometheus-url URL]
//	             [--container-host HOST] [--series-map FILE]
//	             [--reasoner rule|model] [--model-endpoint URL] [--model-name NAME]
//	arena demo   [--case C1] [--mode multi_with_critic] [--approve] [--out report.md]
//	arena cases
//	arena version
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/zlrrr/mutil-agent-system/internal/arena"
	"github.com/zlrrr/mutil-agent-system/internal/catalog"
	"github.com/zlrrr/mutil-agent-system/internal/domain"
	"github.com/zlrrr/mutil-agent-system/internal/eventbus"
	"github.com/zlrrr/mutil-agent-system/internal/httpapi"
	"github.com/zlrrr/mutil-agent-system/internal/policy"
	"github.com/zlrrr/mutil-agent-system/internal/reasoner"
	"github.com/zlrrr/mutil-agent-system/internal/reasoner/model"
	"github.com/zlrrr/mutil-agent-system/internal/report"
	"github.com/zlrrr/mutil-agent-system/internal/signal/profile"
	"github.com/zlrrr/mutil-agent-system/internal/store"
)

// sdd:impl DLD-1075

// Version is the build identity, overridable at link time.
var Version = "0.1.0-mvp"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "serve":
		err = serve(os.Args[2:])
	case "demo":
		err = demo(os.Args[2:])
	case "cases":
		err = listCases()
	case "version":
		fmt.Println(Version)
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "arena: unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "arena: %v\n", err)
		// HLD-018: a configuration error exits 2, so an operator's start-up script can
		// tell "you asked for something impossible" apart from "it broke while running".
		var ce *configError
		if errors.As(err, &ce) {
			os.Exit(2)
		}
		os.Exit(1)
	}
}

// configError marks a failure caused by what the operator asked for, rather than by
// something that went wrong at runtime.
type configError struct {
	key string
	err error
}

func (e *configError) Error() string { return e.key + ": " + e.err.Error() }
func (e *configError) Unwrap() error { return e.err }

func badConfig(key string, format string, args ...any) error {
	return &configError{key: key, err: fmt.Errorf(format, args...)}
}

func serve(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := fs.String("addr", env("ARENA_ADDR", ":8080"), "listen address")
	kind := fs.String("store", env("ARENA_STORE", "memory"), "store adapter: memory|file")
	dir := fs.String("data", env("ARENA_DATA", "./data"), "data directory for the file store")
	prof := fs.String("signal-profile", env("ARENA_SIGNAL_PROFILE", string(profile.Fixture)),
		"signal adapters: "+strings.Join(profile.Names(), "|"))
	promURL := fs.String("prometheus-url", env("ARENA_PROMETHEUS_URL", ""),
		"Prometheus base URL, required by the live profile")
	runtimeHost := fs.String("container-host", env("ARENA_CONTAINER_HOST", "unix:///var/run/docker.sock"),
		"container runtime host, used by the live profile")
	seriesMap := fs.String("series-map", env("ARENA_SERIES_MAP", ""),
		"path to a JSON file mapping series names to PromQL, used by the live profile")
	reasonerKind := fs.String("reasoner", env("ARENA_REASONER", "rule"), "reasoner adapter: rule|model")
	modelEndpoint := fs.String("model-endpoint", env("ARENA_MODEL_ENDPOINT", ""),
		"chat-completions URL, required by the model reasoner")
	modelName := fs.String("model-name", env("ARENA_MODEL_NAME", ""),
		"model identifier, required by the model reasoner")
	if err := fs.Parse(args); err != nil {
		return err
	}

	st, err := buildStore(*kind, *dir)
	if err != nil {
		return err
	}
	cat, err := catalog.Load()
	if err != nil {
		return fmt.Errorf("load catalog: %w", err)
	}

	signals, err := buildSignalConfig(*prof, *promURL, *runtimeHost, *seriesMap)
	if err != nil {
		return err
	}
	rsn, err := buildReasoner(*reasonerKind, *modelEndpoint, *modelName, cat)
	if err != nil {
		return err
	}
	registry, err := arena.NewRegistryWith(arena.RegistryOptions{
		Store: st, Bus: eventbus.New(),
		Config: reasoner.DefaultConfig(), Policy: policy.DefaultConfig(),
		Catalog: cat, Signals: signals, Reasoner: rsn,
	})
	if err != nil {
		return err
	}

	srv := &http.Server{
		Addr:              *addr,
		Handler:           httpapi.NewServer(registry, cat, Version).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		// No write timeout: the event stream is a long-lived response.
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()

	fmt.Printf("IncidentOps Arena %s listening on %s (store=%s, signals=%s, reasoner=%s, cases=%v)\n",
		Version, *addr, *kind, signals.Profile, *reasonerKind, cat.CaseIDs())
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func demo(args []string) error {
	fs := flag.NewFlagSet("demo", flag.ExitOnError)
	caseID := fs.String("case", "C1", "fault case identifier")
	mode := fs.String("mode", string(domain.ModeMultiWithCritic),
		"single|multi_no_critic|multi_with_critic")
	approve := fs.Bool("approve", true, "approve the proposed action when the gate is reached")
	out := fs.String("out", "", "write the report to a file instead of stdout")
	quiet := fs.Bool("quiet", false, "print only the report")
	if err := fs.Parse(args); err != nil {
		return err
	}

	ctx := context.Background()
	build, err := arena.NewFixtureBuild(arena.Params{CaseID: *caseID})
	if err != nil {
		return err
	}

	c, err := build.Engine.Create(ctx, build.Alert(), domain.Mode(*mode))
	if err != nil {
		return err
	}
	c, err = build.Engine.Run(ctx, c.ID)
	if err != nil {
		return err
	}

	if c.Status == domain.StatusAwaitingApproval {
		action, ok := c.PendingAction()
		if !ok {
			return fmt.Errorf("case %s awaits approval but no action is pending", c.ID)
		}
		if !*approve {
			if !*quiet {
				fmt.Printf("\ncase %s is holding at the approval gate for %q (%s risk).\n",
					c.ID, action.Title, action.Risk)
				fmt.Printf("actuator invocations so far: %d\n", len(build.State.Calls()))
			}
			return emit(*out, report.Markdown(c))
		}
		if !*quiet {
			fmt.Printf("\napproving %q (%s risk): %s\n", action.Title, action.Risk, action.ID)
		}
		c, err = build.Engine.Decide(ctx, c.ID, action.ID, domain.ApprovalDecision{
			Decision: "approved", By: "arena-demo",
			Comment: "approved by the demo command",
		})
		if err != nil {
			return err
		}
	}

	if !*quiet {
		printSummary(c, build)
	}
	return emit(*out, report.Markdown(c))
}

func printSummary(c *domain.Case, build *arena.Build) {
	fmt.Printf("\ncase %s finished in state %s after %d collection round(s)\n",
		c.ID, c.Status, c.Round)
	fmt.Printf("  evidence %d across %d kind(s), hypotheses %d, critiques %d, demands %d\n",
		len(c.Evidence), len(domain.DistinctKinds(c.Evidence)),
		len(c.Hypotheses), len(c.Critiques), len(c.Demands))
	for i, h := range c.Hypotheses {
		mark := " "
		if h.SignatureID == build.Case.ExpectedSignature {
			mark = "*"
		}
		fmt.Printf("  %s #%d %-32s %.2f  verdict=%s\n",
			mark, i+1, h.SignatureID, h.Breakdown.Total, orPending(string(h.Verdict)))
	}
	fmt.Printf("  actuator invocations: %d\n", len(build.State.Calls()))
	fmt.Printf("  (* marks the root cause this case declares as correct)\n\n")
}

func orPending(s string) string {
	if s == "" {
		return "pending"
	}
	return s
}

func listCases() error {
	cat, err := catalog.Load()
	if err != nil {
		return err
	}
	fmt.Printf("%-5s %-45s %-12s %s\n", "CASE", "TITLE", "SERVICE", "EXPECTED ROOT CAUSE")
	for _, fc := range cat.Cases {
		fmt.Printf("%-5s %-45s %-12s %s\n",
			fc.ID, truncate(fc.Title, 45), fc.Alert.Service, fc.ExpectedSignature)
	}
	return nil
}

// buildSignalConfig resolves the adapter profile from flags. A configuration error names
// the offending key and exits 2 (HLD-018), because a deployment that meant to read live
// signals and silently read fixtures instead would look healthy while proving nothing.
func buildSignalConfig(name, promURL, runtimeHost, seriesMapPath string) (profile.Config, error) {
	cfg := profile.Config{
		Profile:       profile.Profile(name),
		PrometheusURL: promURL,
		ContainerHost: runtimeHost,
	}
	if cfg.Profile != profile.Live {
		// Validated here too, so a mistyped profile fails at start-up rather than at
		// the first case.
		if err := cfg.Validate(); err != nil {
			return cfg, badConfig("-signal-profile", "%w", err)
		}
		return cfg, nil
	}

	if seriesMapPath == "" {
		return cfg, badConfig("-series-map", "required by the %s profile: without it no series can be resolved", profile.Live)
	}
	raw, err := os.ReadFile(seriesMapPath)
	if err != nil {
		return cfg, badConfig("-series-map", "%w", err)
	}
	var file struct {
		Series     map[string]string  `json:"series"`
		Units      map[string]string  `json:"units"`
		Capacities map[string]float64 `json:"capacities"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		return cfg, badConfig("-series-map", "%s: %w", seriesMapPath, err)
	}
	if len(file.Series) == 0 {
		return cfg, badConfig("-series-map", "%s declares no series", seriesMapPath)
	}
	cfg.SeriesMap, cfg.Units, cfg.Capacities = file.Series, file.Units, file.Capacities

	if err := cfg.Validate(); err != nil {
		return cfg, badConfig("-signal-profile", "%w", err)
	}
	return cfg, nil
}

// buildReasoner resolves the reasoner adapter. Nil means the deterministic rule engine,
// which arena treats as the default (ADR-002) — so "rule" and an unset flag agree.
func buildReasoner(kind, endpoint, name string, cat *catalog.Catalog) (reasoner.Reasoner, error) {
	switch kind {
	case "rule", "":
		return nil, nil
	case "model":
		// No provider has been chosen (charter Q1), so there is no default endpoint to
		// fall back to and guessing one would be a decision this code cannot make.
		if endpoint == "" {
			return nil, badConfig("-model-endpoint", "required by the model reasoner")
		}
		if name == "" {
			return nil, badConfig("-model-name", "required by the model reasoner")
		}
		return model.New(endpoint, name, cat, reasoner.DefaultConfig(), model.Options{
			APIKey: os.Getenv("ARENA_MODEL_API_KEY"),
		}), nil
	default:
		return nil, badConfig("-reasoner", "unknown adapter %q: use rule or model", kind)
	}
}

func buildStore(kind, dir string) (store.Store, error) {
	switch kind {
	case "memory", "":
		return store.NewMemory(), nil
	case "file":
		return store.NewFile(dir)
	default:
		return nil, badConfig("-store", "unknown adapter %q: use memory or file", kind)
	}
}

func emit(path, content string) error {
	if path == "" {
		fmt.Print(content)
		return nil
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "wrote %s\n", path)
	return nil
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func usage() {
	fmt.Fprint(os.Stderr, strings.TrimLeft(`
arena — IncidentOps Arena, a multi-agent incident localisation system

Commands:
  serve     run the API, event stream and console
  demo      run one fault case end to end and print its report
  cases     list the reproducible fault cases in the catalog
  version   print the build version

serve flags:
  --addr    listen address                       (env ARENA_ADDR,  default :8080)
  --store   memory|file                          (env ARENA_STORE, default memory)
  --data    data directory for the file store    (env ARENA_DATA,  default ./data)

demo flags:
  --case    fault case identifier                (default C1)
  --mode    single|multi_no_critic|multi_with_critic
  --approve approve the proposed action at the gate (default true)
  --out     write the report to a file
  --quiet   print only the report

Everything runs offline: no network, no credentials and no model provider.
`, "\n"))
}
