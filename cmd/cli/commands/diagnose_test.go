package commands

import (
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/francomano/proxydoctor/core/check"
	checkspkg "github.com/francomano/proxydoctor/core/checks"
	"github.com/francomano/proxydoctor/core/engine"
	"github.com/francomano/proxydoctor/core/plugin"
	"github.com/francomano/proxydoctor/core/plugins"
)

func TestParseDiagnosisTimeout(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    time.Duration
		wantErr bool
	}{
		{
			name:  "empty uses default",
			value: "",
			want:  engine.DefaultDiagnosisTimeout,
		},
		{
			name:  "seconds",
			value: "10s",
			want:  10 * time.Second,
		},
		{
			name:  "minutes",
			value: "5m",
			want:  5 * time.Minute,
		},
		{
			name:    "below minimum",
			value:   "500ms",
			wantErr: true,
		},
		{
			name:    "above maximum",
			value:   "6m",
			wantErr: true,
		},
		{
			name:    "invalid duration",
			value:   "slow",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseDiagnosisTimeout(tt.value)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseCheckFiltersByID(t *testing.T) {
	got, err := parseCheckFilters(" public_ip, dns_resolve ", newTestRegistry())
	if err != nil {
		t.Fatalf("parseCheckFilters returned error: %v", err)
	}

	want := []string{"dns_resolve", "public_ip"}
	assertStringSlicesEqual(t, got, want)
}

func TestParseCheckFiltersByCategory(t *testing.T) {
	got, err := parseCheckFilters("network", newTestRegistry())
	if err != nil {
		t.Fatalf("parseCheckFilters returned error: %v", err)
	}

	want := []string{"dns_resolve", "port_connectivity", "public_ip"}
	assertStringSlicesEqual(t, got, want)
}

func TestParseCheckFiltersAll(t *testing.T) {
	got, err := parseCheckFilters("all", newTestRegistry())
	if err != nil {
		t.Fatalf("parseCheckFilters returned error: %v", err)
	}
	if got != nil {
		t.Fatalf("got %v, want nil for all checks", got)
	}
}

func TestParseCheckFiltersRejectsUnknownFilter(t *testing.T) {
	if _, err := parseCheckFilters("missing", newTestRegistry()); err == nil {
		t.Fatal("expected an error for an unknown check filter")
	}
}

func TestFormatComparisonOutputsDifferences(t *testing.T) {
	report := &engine.ComparisonReport{
		ID: "compare_test",
		DirectReport: &engine.DiagnosisReport{
			ChecksExecuted:   1,
			ChecksFailed:     0,
			CriticalFindings: 0,
			ExecutionTime:    time.Second,
		},
		ProxyReport: &engine.DiagnosisReport{
			ChecksExecuted:   1,
			ChecksFailed:     1,
			CriticalFindings: 1,
			ExecutionTime:    2 * time.Second,
		},
		Differences: []engine.ComparisonDifference{
			{
				CheckID:     "public_ip",
				Field:       "ip_address",
				DirectValue: "203.0.113.10",
				ProxyValue:  "198.51.100.20",
				Summary:     "public_ip IP changed from 203.0.113.10 to 198.51.100.20",
			},
		},
		ExecutionTime: 3 * time.Second,
	}

	text := formatComparisonText(report)
	if !strings.Contains(text, "Direct Connection") ||
		!strings.Contains(text, "Proxied Connection") ||
		!strings.Contains(text, "public_ip IP changed") {
		t.Fatalf("text comparison output missing expected content:\n%s", text)
	}

	markdown := formatComparisonMarkdown(report)
	if !strings.Contains(markdown, "# ProxyDoctor Comparison Report") ||
		!strings.Contains(markdown, "**public_ip**") {
		t.Fatalf("markdown comparison output missing expected content:\n%s", markdown)
	}

	html := formatComparisonHTML(report)
	if !strings.Contains(html, "<h1>ProxyDoctor Comparison Report</h1>") ||
		!strings.Contains(html, "public_ip IP changed") {
		t.Fatalf("html comparison output missing expected content:\n%s", html)
	}

	json := formatComparisonJSON(report)
	if !strings.Contains(json, `"direct_report"`) ||
		!strings.Contains(json, `"differences"`) ||
		!strings.Contains(json, `"proxy_value": "198.51.100.20"`) {
		t.Fatalf("json comparison output missing expected content:\n%s", json)
	}
}

func TestFormatComparisonOutputsNoDifferences(t *testing.T) {
	report := &engine.ComparisonReport{
		DirectReport: &engine.DiagnosisReport{
			Results: []check.CheckResult{},
		},
		ProxyReport: &engine.DiagnosisReport{
			Results: []check.CheckResult{},
		},
	}

	if got := formatComparisonText(report); !strings.Contains(got, "No differences detected") {
		t.Fatalf("expected no differences message, got:\n%s", got)
	}
}

func TestFormatHTMLIncludesSummaryAndEvidence(t *testing.T) {
	report := &engine.DiagnosisReport{
		RequestMetadata: engine.RequestMetadata{
			URL: "https://example.com",
		},
		ChecksExecuted:   2,
		ChecksFailed:     1,
		CriticalFindings: 1,
		WarningFindings:  0,
		ExecutionTime:    2 * time.Second,
		Results: []check.CheckResult{
			{
				ID:          "public_ip",
				Status:      check.StatusPassed,
				Severity:    check.SeverityInfo,
				Confidence:  0.95,
				Explanation: "Public IP resolved via proxy.",
				Evidence: map[string]interface{}{
					"ip": "203.0.113.10",
				},
			},
			{
				ID:               "dns_leak",
				Status:           check.StatusFailed,
				Severity:         check.SeverityCritical,
				Confidence:       0.8,
				Explanation:      "DNS requests bypassed the proxy.",
				ProbableCauses:   []string{"System resolver used instead of proxy"},
				SuggestedActions: []string{"Configure DNS over the proxy"},
			},
		},
	}

	out := formatHTML(report)

	mustContain := []string{
		"<!doctype html>",
		"ProxyDoctor Diagnosis Report",
		"example.com",
		"Checks Executed",
		"public_ip",
		"dns_leak",
		"<details class=\"evidence\">",
		"203.0.113.10",
		"Probable causes",
		"Suggested actions",
	}
	for _, want := range mustContain {
		if !strings.Contains(out, want) {
			t.Fatalf("html output missing %q:\n%s", want, out)
		}
	}
}

func newTestRegistry() *engine.CheckRegistry {
	registry := engine.NewCheckRegistry()
	if err := checkspkg.RegisterDefaults(registry); err != nil {
		panic(err)
	}
	return registry
}

func assertStringSlicesEqual(t *testing.T, got []string, want []string) {
	t.Helper()
	sort.Strings(got)
	sort.Strings(want)

	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestDefaultRegistryDoesNotContainRouteTrace(t *testing.T) {
	registry := newTestRegistry()
	if _, ok := registry.GetCheck("route_trace"); ok {
		t.Fatal("route_trace should not be in the default registry (it is now a plugin)")
	}
}

func TestLoadRouteTracePluginAddsCheck(t *testing.T) {
	registry := newTestRegistry()
	mgr := plugin.NewManager()
	ctx := &plugin.Context{Registry: registry, Config: map[string]interface{}{}}

	if err := plugins.Load([]string{"route_trace"}, mgr, ctx); err != nil {
		t.Fatalf("Load: %v", err)
	}
	defer mgr.ShutdownAll()

	check, ok := registry.GetCheck("route_trace")
	if !ok {
		t.Fatal("route_trace should be registered after loading plugin")
	}
	if check.Category() != "network" {
		t.Fatalf("route_trace category: got %q, want %q", check.Category(), "network")
	}
}

func TestLoadAllPluginsAddsRouteTrace(t *testing.T) {
	registry := newTestRegistry()
	mgr := plugin.NewManager()
	ctx := &plugin.Context{Registry: registry, Config: map[string]interface{}{}}

	if err := plugins.Load([]string{"all"}, mgr, ctx); err != nil {
		t.Fatalf("Load all: %v", err)
	}
	defer mgr.ShutdownAll()

	if _, ok := registry.GetCheck("route_trace"); !ok {
		t.Fatal("route_trace should be registered after loading all plugins")
	}
}

func TestLoadUnknownPluginIsNoOp(t *testing.T) {
	registry := newTestRegistry()
	mgr := plugin.NewManager()
	ctx := &plugin.Context{Registry: registry, Config: map[string]interface{}{}}

	if err := plugins.Load([]string{"nonexistent"}, mgr, ctx); err != nil {
		t.Fatalf("Load unknown plugin should not error: %v", err)
	}
	if _, ok := registry.GetCheck("route_trace"); ok {
		t.Fatal("route_trace should not be registered when only unknown plugin requested")
	}
}

func TestParseCheckFiltersWithPluginCheck(t *testing.T) {
	registry := newTestRegistry()
	mgr := plugin.NewManager()
	ctx := &plugin.Context{Registry: registry, Config: map[string]interface{}{}}
	if err := plugins.Load([]string{"route_trace"}, mgr, ctx); err != nil {
		t.Fatalf("Load: %v", err)
	}
	defer mgr.ShutdownAll()

	got, err := parseCheckFilters("route_trace", registry)
	if err != nil {
		t.Fatalf("parseCheckFilters: %v", err)
	}
	if len(got) != 1 || got[0] != "route_trace" {
		t.Fatalf("got %v, want [route_trace]", got)
	}
}

func TestParseCheckFiltersNetworkIncludesPluginCheck(t *testing.T) {
	registry := newTestRegistry()
	mgr := plugin.NewManager()
	ctx := &plugin.Context{Registry: registry, Config: map[string]interface{}{}}
	if err := plugins.Load([]string{"route_trace"}, mgr, ctx); err != nil {
		t.Fatalf("Load: %v", err)
	}
	defer mgr.ShutdownAll()

	got, err := parseCheckFilters("network", registry)
	if err != nil {
		t.Fatalf("parseCheckFilters: %v", err)
	}

	want := []string{"dns_resolve", "port_connectivity", "public_ip", "route_trace"}
	assertStringSlicesEqual(t, got, want)
}
