package catalog

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/zlrrr/mutil-agent-system/internal/domain"
)

// sdd:verify TC-0082
func TestCatalogLoad(t *testing.T) {
	cat, err := Load()
	if err != nil {
		t.Fatalf("the embedded catalog must load: %v", err)
	}

	if len(cat.Signatures) < 5 {
		t.Errorf("catalog has %d signatures, the task breakdown requires at least 5",
			len(cat.Signatures))
	}
	if len(cat.Cases) < 3 {
		t.Errorf("catalog has %d fault cases, the task breakdown requires at least 3",
			len(cat.Cases))
	}

	for _, fc := range cat.Cases {
		if _, ok := cat.Signature(fc.ExpectedSignature); !ok {
			t.Errorf("case %s expects signature %q, which is not in the catalog",
				fc.ID, fc.ExpectedSignature)
		}
		if len(fc.Series) == 0 || len(fc.Logs) == 0 {
			t.Errorf("case %s declares no signals", fc.ID)
		}
		if len(fc.DefaultSeries) == 0 {
			t.Errorf("case %s declares no default series", fc.ID)
		}
		if fc.Alert.StartsAt.IsZero() {
			t.Errorf("case %s has no alert start time", fc.ID)
		}
	}
	for _, sig := range cat.Signatures {
		if sig.Claim == "" || sig.Mechanism == "" {
			t.Errorf("signature %s must carry a claim and a mechanism", sig.ID)
		}
		for _, id := range sig.RunbookIDs {
			if _, ok := cat.Runbook(id); !ok {
				t.Errorf("signature %s references unknown runbook %q", sig.ID, id)
			}
		}
	}
}

// sdd:verify TC-0082
func TestCatalogIsExtendedByDataAlone(t *testing.T) {
	// Adding a case is a data change: the loader discovers it with no code change.
	fsys := fstest.MapFS{
		"data/signatures.json": &fstest.MapFile{Data: []byte(`{"signatures":[
			{"id":"sig-new","claim":"c","mechanism":"m",
			 "requires":[{"kind":"log","match":["boom"],"label":"boom in the logs"}]}]}`)},
		"data/case-new.json": &fstest.MapFile{Data: []byte(`{
			"id":"CX","title":"a new scenario","default_series":["s"],
			"alert":{"alert_name":"A","service":"svc","starts_at":"2026-07-26T10:00:00Z"},
			"series":[{"name":"s","points":[{"at":"2026-07-26T10:00:00Z","value":1}]}],
			"logs":[{"at":"2026-07-26T10:00:00Z","service":"svc","message":"boom"}],
			"expected_signature":"sig-new"}`)},
	}
	cat, err := loadFS(fsys, "data")
	if err != nil {
		t.Fatalf("a well-formed data-only catalog must load: %v", err)
	}
	if _, ok := cat.Case("CX"); !ok {
		t.Errorf("the added case was not discovered; have %v", cat.CaseIDs())
	}
}

// sdd:verify TC-0082
func TestCatalogRefusesBrokenData(t *testing.T) {
	cases := []struct {
		name string
		fs   fstest.MapFS
		want string
	}{
		{
			name: "dangling expected signature",
			fs: fstest.MapFS{
				"data/case-x.json": &fstest.MapFile{Data: []byte(`{
					"id":"CX","alert":{"alert_name":"A","service":"s","starts_at":"2026-07-26T10:00:00Z"},
					"expected_signature":"sig-absent"}`)},
			},
			want: "unknown signature",
		},
		{
			name: "duplicate signature identifier",
			fs: fstest.MapFS{
				"data/signatures.json": &fstest.MapFile{Data: []byte(`{"signatures":[
					{"id":"sig-a","requires":[{"kind":"log","match":["x"]}]},
					{"id":"sig-a","requires":[{"kind":"log","match":["y"]}]}]}`)},
			},
			want: "duplicate signature id",
		},
		{
			name: "signature requiring nothing",
			fs: fstest.MapFS{
				"data/signatures.json": &fstest.MapFile{Data: []byte(
					`{"signatures":[{"id":"sig-a","requires":[]}]}`)},
			},
			want: "requires no evidence",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := loadFS(tc.fs, "data")
			if err == nil {
				t.Fatal("a broken catalog must fail to load, so it cannot run")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to mention %q", err, tc.want)
			}
		})
	}
}

// sdd:verify TC-0020
func TestPatternMatching(t *testing.T) {
	ev := []domain.Evidence{
		{ID: "e-1", Kind: domain.KindMetric, Summary: "db_pool_saturation reached capacity",
			Facts: map[string]string{"series": "db_pool_saturation", "saturated": "true"}},
		{ID: "e-2", Kind: domain.KindMetric, Summary: "http_requests_total rose",
			Facts: map[string]string{"series": "http_requests_total", "saturated": "false"}},
		{ID: "e-3", Kind: domain.KindLog, Summary: "db connection timeout after #ms"},
	}

	t.Run("saturation is required when declared", func(t *testing.T) {
		sig := Signature{ID: "s", Requires: []Pattern{
			{Kind: domain.KindMetric, Match: []string{"http_requests_total"}, Saturated: true},
		}}
		if Match(sig, ev).Any() {
			t.Error("a non-saturated series must not satisfy a saturation pattern")
		}
	})
	t.Run("matching is case-insensitive across summary and facts", func(t *testing.T) {
		sig := Signature{ID: "s", Requires: []Pattern{
			{Kind: domain.KindMetric, Match: []string{"DB_POOL_SATURATION"}, Saturated: true},
		}}
		m := Match(sig, ev)
		if !m.Any() || m.Matched[0].EvidenceID != "e-1" {
			t.Errorf("expected e-1 to match, got %+v", m.Matched)
		}
	})
	t.Run("every term must appear", func(t *testing.T) {
		sig := Signature{ID: "s", Requires: []Pattern{
			{Kind: domain.KindLog, Match: []string{"connection", "refused"}},
		}}
		if Match(sig, ev).Any() {
			t.Error("a pattern requiring two terms must not match on one")
		}
	})
	t.Run("fraction reflects partial satisfaction", func(t *testing.T) {
		sig := Signature{ID: "s", Requires: []Pattern{
			{Kind: domain.KindMetric, Match: []string{"db_pool_saturation"}, Saturated: true},
			{Kind: domain.KindMetric, Match: []string{"absent_series"}},
		}}
		m := Match(sig, ev)
		if got := m.Fraction(domain.KindMetric); got != 0.5 {
			t.Errorf("fraction = %v, want 0.5", got)
		}
		if len(m.Unmatched) != 1 {
			t.Errorf("unmatched = %d, want 1", len(m.Unmatched))
		}
	})
	t.Run("a kind the signature does not require scores zero", func(t *testing.T) {
		sig := Signature{ID: "s", Requires: []Pattern{
			{Kind: domain.KindMetric, Match: []string{"db_pool_saturation"}, Saturated: true},
		}}
		if got := Match(sig, ev).Fraction(domain.KindLog); got != 0 {
			t.Errorf("log fraction = %v; a signature with no log requirement is genuinely "+
				"unsupported by logs", got)
		}
	})
}

// sdd:verify TC-0082
func TestRecoveryTriggerMatching(t *testing.T) {
	trigger := &RecoveryTrigger{
		Tool: "set_config",
		Args: map[string]string{"service": "order-api", "key": "DB_POOL_SIZE", "value": "20"},
	}
	if !trigger.Matches(domain.ActionCall{Tool: "set_config", Args: map[string]string{
		"service": "order-api", "key": "DB_POOL_SIZE", "value": "20", "extra": "ignored",
	}}) {
		t.Error("a call carrying every required argument must match")
	}
	if trigger.Matches(domain.ActionCall{Tool: "set_config", Args: map[string]string{
		"service": "order-api", "key": "DB_POOL_SIZE", "value": "2",
	}}) {
		t.Error("a different value must not trigger recovery")
	}
	if trigger.Matches(domain.ActionCall{Tool: "restart_service"}) {
		t.Error("a different tool must not trigger recovery")
	}
	var nilTrigger *RecoveryTrigger
	if nilTrigger.Matches(domain.ActionCall{Tool: "set_config"}) {
		t.Error("an absent trigger must never match")
	}
}
