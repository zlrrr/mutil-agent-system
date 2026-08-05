package httpapi_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/zlrrr/mutil-agent-system/internal/arena"
	"github.com/zlrrr/mutil-agent-system/internal/catalog"
	"github.com/zlrrr/mutil-agent-system/internal/domain"
	"github.com/zlrrr/mutil-agent-system/internal/eventbus"
	"github.com/zlrrr/mutil-agent-system/internal/httpapi"
	"github.com/zlrrr/mutil-agent-system/internal/policy"
	"github.com/zlrrr/mutil-agent-system/internal/reasoner"
	"github.com/zlrrr/mutil-agent-system/internal/store"
)

func newServer(t *testing.T) (*httptest.Server, *arena.Registry) {
	t.Helper()
	cat, err := catalog.Load()
	if err != nil {
		t.Fatal(err)
	}
	reg, err := arena.NewRegistry(store.NewMemory(), eventbus.New(),
		reasoner.DefaultConfig(), policy.DefaultConfig(), cat)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(httpapi.NewServer(reg, cat, "test").Handler())
	t.Cleanup(srv.Close)
	return srv, reg
}

func post(t *testing.T, url, body string) (*http.Response, map[string]any) {
	t.Helper()
	res, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var decoded map[string]any
	_ = json.NewDecoder(res.Body).Decode(&decoded)
	return res, decoded
}

func get(t *testing.T, url string) (*http.Response, []byte) {
	t.Helper()
	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(res.Body)
	return res, buf.Bytes()
}

// sdd:verify TC-0063
func TestAPIValidation(t *testing.T) {
	srv, _ := newServer(t)

	cases := []struct {
		name      string
		body      string
		wantCode  int
		wantField string
	}{
		{"missing service", `{"alert_name":"A","starts_at":"2026-07-26T10:07:00Z"}`,
			http.StatusBadRequest, "service"},
		{"missing alert name", `{"service":"order-api","starts_at":"2026-07-26T10:07:00Z"}`,
			http.StatusBadRequest, "alert_name"},
		{"missing start time", `{"alert_name":"A","service":"order-api"}`,
			http.StatusBadRequest, "starts_at"},
		{"unknown field", `{"alert_name":"A","service":"order-api",` +
			`"starts_at":"2026-07-26T10:07:00Z","nonsense":1}`, http.StatusBadRequest, ""},
		{"malformed body", `{`, http.StatusBadRequest, ""},
		{"unknown case reference", `{"case_ref":"C99"}`, http.StatusBadRequest, "case_ref"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, body := post(t, srv.URL+"/api/cases", tc.body)
			if res.StatusCode != tc.wantCode {
				t.Fatalf("status = %d, want %d (body %v)", res.StatusCode, tc.wantCode, body)
			}
			if msg, _ := body["error"].(string); msg == "" {
				t.Error("the error response carries no message")
			}
			if tc.wantField != "" {
				if got, _ := body["field"].(string); got != tc.wantField {
					t.Errorf("field = %q, want %q", got, tc.wantField)
				}
			}
		})
	}

	res, _ := get(t, srv.URL+"/api/cases/inc-does-not-exist")
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("an unknown case returned %d, want 404", res.StatusCode)
	}
}

// sdd:verify TC-0063
func TestAPILifecycle(t *testing.T) {
	srv, _ := newServer(t)

	res, created := post(t, srv.URL+"/api/cases", `{"case_ref":"C1"}`)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create returned %d: %v", res.StatusCode, created)
	}
	caseID, _ := created["id"].(string)
	if caseID == "" {
		t.Fatalf("the created case has no identifier: %v", created)
	}
	if status, _ := created["status"].(string); status != string(domain.StatusAwaitingApproval) {
		t.Errorf("status = %q, want awaiting_approval", status)
	}

	// The case appears in the listing.
	res, body := get(t, srv.URL+"/api/cases")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("list returned %d", res.StatusCode)
	}
	var headers []store.CaseHeader
	if err := json.Unmarshal(body, &headers); err != nil {
		t.Fatal(err)
	}
	if len(headers) != 1 || headers[0].ID != caseID {
		t.Fatalf("listing = %+v", headers)
	}

	// Events are contiguous and ordered.
	res, body = get(t, srv.URL+"/api/cases/"+caseID+"/events")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("events returned %d", res.StatusCode)
	}
	var events []domain.Event
	if err := json.Unmarshal(body, &events); err != nil {
		t.Fatal(err)
	}
	for i, e := range events {
		if e.Seq != i+1 {
			t.Fatalf("event %d has sequence %d", i, e.Seq)
		}
	}
	// The `from` parameter resumes.
	_, body = get(t, srv.URL+"/api/cases/"+caseID+"/events?from=5")
	var tail []domain.Event
	if err := json.Unmarshal(body, &tail); err != nil {
		t.Fatal(err)
	}
	if len(tail) == 0 || tail[0].Seq != 6 {
		t.Fatalf("resuming from 5 returned %d events starting at %v", len(tail), firstSeq(tail))
	}

	// The decision endpoint validates its input.
	var caseView map[string]any
	_, body = get(t, srv.URL+"/api/cases/"+caseID)
	_ = json.Unmarshal(body, &caseView)
	actions, _ := caseView["actions"].([]any)
	if len(actions) == 0 {
		t.Fatal("the case has no action to decide on")
	}
	actionID, _ := actions[0].(map[string]any)["id"].(string)
	decideURL := srv.URL + "/api/cases/" + caseID + "/actions/" + actionID + "/decision"

	res, errBody := post(t, decideURL, `{"decision":"maybe","by":"operator"}`)
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("an invalid decision returned %d", res.StatusCode)
	}
	if got, _ := errBody["field"].(string); got != "decision" {
		t.Errorf("field = %q, want decision", got)
	}
	res, errBody = post(t, decideURL, `{"decision":"approved"}`)
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("an approval with no approver returned %d", res.StatusCode)
	}
	if got, _ := errBody["field"].(string); got != "by" {
		t.Errorf("field = %q, want by", got)
	}

	res, decided := post(t, decideURL, `{"decision":"approved","by":"demo-operator","comment":"ok"}`)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("approval returned %d: %v", res.StatusCode, decided)
	}
	if status, _ := decided["status"].(string); status != string(domain.StatusClosed) {
		t.Errorf("status after approval = %q, want closed", status)
	}

	// Reports render in both formats with the documented content types.
	res, body = get(t, srv.URL+"/api/cases/"+caseID+"/report.md")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("report.md returned %d", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); ct != "text/markdown; charset=utf-8" {
		t.Errorf("report.md content type = %q", ct)
	}
	if !bytes.Contains(body, []byte("# Root cause report")) {
		t.Error("report.md does not look like the report")
	}

	res, body = get(t, srv.URL+"/api/cases/"+caseID+"/report.json")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("report.json returned %d", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("report.json content type = %q", ct)
	}
	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Errorf("report.json does not parse: %v", err)
	}

	// Health and version are available for a container probe.
	if res, _ := get(t, srv.URL+"/healthz"); res.StatusCode != http.StatusOK {
		t.Errorf("healthz returned %d", res.StatusCode)
	}
	if res, _ := get(t, srv.URL+"/api/version"); res.StatusCode != http.StatusOK {
		t.Errorf("version returned %d", res.StatusCode)
	}
}

func firstSeq(events []domain.Event) any {
	if len(events) == 0 {
		return "none"
	}
	return events[0].Seq
}

// sdd:verify TC-0064
func TestStreamResume(t *testing.T) {
	srv, _ := newServer(t)
	res, created := post(t, srv.URL+"/api/cases", `{"case_ref":"C1"}`)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create returned %d: %v", res.StatusCode, created)
	}
	caseID, _ := created["id"].(string)

	// Read the first part of the stream, then disconnect.
	firstHalf := readStream(t, srv.URL+"/api/cases/"+caseID+"/stream", "", 6)
	if len(firstHalf) != 6 {
		t.Fatalf("read %d events before disconnecting", len(firstHalf))
	}
	for i, seq := range firstHalf {
		if seq != i+1 {
			t.Fatalf("event %d has sequence %d; the backlog must replay in order", i, seq)
		}
	}

	// Reconnect with Last-Event-ID: no gap, no duplicate.
	last := firstHalf[len(firstHalf)-1]
	secondHalf := readStream(t, srv.URL+"/api/cases/"+caseID+"/stream",
		strconv.Itoa(last), 4)
	if len(secondHalf) == 0 {
		t.Fatal("the resumed stream delivered nothing")
	}
	if secondHalf[0] != last+1 {
		t.Errorf("resumed at %d, want %d: the client would have a gap", secondHalf[0], last+1)
	}
	for i := 1; i < len(secondHalf); i++ {
		if secondHalf[i] != secondHalf[i-1]+1 {
			t.Errorf("the resumed stream is not contiguous: %v", secondHalf)
			break
		}
	}
}

// sdd:verify TC-0064
func TestStreamDisconnectDoesNotStallTheCase(t *testing.T) {
	srv, reg := newServer(t)

	// Subscribe with a buffer of one and never read, then run a case to completion.
	ch, cancel := reg.Bus().Subscribe("", 1)
	defer cancel()
	_ = ch

	done := make(chan struct{})
	go func() {
		defer close(done)
		post(t, srv.URL+"/api/cases", `{"case_ref":"C1"}`)
	}()

	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("a stalled subscriber blocked the investigation")
	}

	res, body := get(t, srv.URL+"/api/cases")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("list returned %d", res.StatusCode)
	}
	var headers []store.CaseHeader
	_ = json.Unmarshal(body, &headers)
	if len(headers) != 1 {
		t.Fatalf("the case did not complete: %+v", headers)
	}
}

// readStream reads up to n server-sent events and returns their sequence numbers.
func readStream(t *testing.T, url, lastEventID string, n int) []int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	if lastEventID != "" {
		req.Header.Set("Last-Event-ID", lastEventID)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if ct := res.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("stream content type = %q", ct)
	}

	var out []int
	sc := bufio.NewScanner(res.Body)
	for sc.Scan() && len(out) < n {
		line := sc.Text()
		if !strings.HasPrefix(line, "id: ") {
			continue
		}
		seq, err := strconv.Atoi(strings.TrimPrefix(line, "id: "))
		if err != nil {
			t.Fatalf("malformed event id line %q", line)
		}
		out = append(out, seq)
	}
	return out
}

// sdd:verify TC-0065
func TestConsoleRenders(t *testing.T) {
	srv, _ := newServer(t)
	res, created := post(t, srv.URL+"/api/cases", `{"case_ref":"C1"}`)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create returned %d: %v", res.StatusCode, created)
	}
	caseID, _ := created["id"].(string)

	res, body := get(t, srv.URL+"/cases/"+caseID)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("the console page returned %d", res.StatusCode)
	}
	html := string(body)

	for _, want := range []string{
		"Agent timeline",
		"Evidence",
		"Ranked root cause hypotheses",
		"Adversarial review",
		"Evidence the critic demanded",
		"Remediation and approval",
		"Report",
		"metric_alignment", // the score breakdown is visible
		"Approve",          // the approval control is present
		"sig-db-pool-exhaustion",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("the console page does not contain %q", want)
		}
	}
	// At least one critique with its verdict must be visible.
	if !strings.Contains(html, "verdict-") {
		t.Error("no critique verdict is rendered")
	}
	// The medium-risk action's approval form must target the decision endpoint.
	if !strings.Contains(html, "/actions/") || !strings.Contains(html, "/decision") {
		t.Error("the approval control does not target the decision endpoint")
	}

	// Static assets are served, and nothing else is.
	if res, _ := get(t, srv.URL+"/static/style.css"); res.StatusCode != http.StatusOK {
		t.Errorf("the stylesheet returned %d", res.StatusCode)
	}
	if res, _ := get(t, srv.URL+"/static/app.js"); res.StatusCode != http.StatusOK {
		t.Errorf("the script returned %d", res.StatusCode)
	}
	if res, _ := get(t, srv.URL+"/static/case.html"); res.StatusCode == http.StatusOK {
		t.Error("the static handler served a template; it must not be a file browser")
	}
	if res, _ := get(t, srv.URL+"/static/../console.go"); res.StatusCode == http.StatusOK {
		t.Error("the static handler served a source file")
	}

	// The index lists both the open case and the reproducible fault cases.
	res, body = get(t, srv.URL+"/")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("the index returned %d", res.StatusCode)
	}
	index := string(body)
	if !strings.Contains(index, caseID) {
		t.Error("the index does not list the open case")
	}
	for _, id := range []string{"C1", "C2", "C3"} {
		if !strings.Contains(index, id) {
			t.Errorf("the index does not offer fault case %s", id)
		}
	}
}
