package commands

import (
	"encoding/json"
	"fmt"
	"html"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/francomano/proxydoctor/core/adapters"
	"github.com/francomano/proxydoctor/core/check"
	checkspkg "github.com/francomano/proxydoctor/core/checks"
	"github.com/francomano/proxydoctor/core/engine"
	"github.com/francomano/proxydoctor/core/utils"
)

var (
	url       string
	proxyStr  string
	proxyType string
	exportFmt string
	output    string
	compare   bool
	timeout   string
	checks    string
)

const (
	minDiagnosisTimeout = time.Second
	maxDiagnosisTimeout = 5 * time.Minute
)

// RootCmd is the main command
var RootCmd = &cobra.Command{
	Use:   "proxyctl",
	Short: "ProxyDoctor - Comprehensive proxy diagnostics tool",
	Long: `ProxyDoctor is a command-line tool for comprehensive proxy diagnostics.
It analyzes connectivity through proxies and identifies issues.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		if cmd.Name() != "diagnose" {
			return
		}
		fmt.Println()
		fmt.Println("  ┏━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┓")
		fmt.Println("  ┃  🩺 ProxyDoctor                                      ┃")
		fmt.Println("  ┃  Comprehensive proxy diagnostics tool                ┃")
		fmt.Println("  ┗━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┛")
		fmt.Println("  by Marco Francomano — github.com/francomano/ProxyDoctor")
		fmt.Println()
	},
}

// diagnoseCmd is the main diagnose command
var diagnoseCmd = &cobra.Command{
	Use:   "diagnose",
	Short: "Run a comprehensive diagnosis on a URL",
	Long: `Run a comprehensive diagnosis on the given URL through your proxy or direct connection.
The diagnosis includes multiple checks for network connectivity, security, and leaks.`,
	RunE: runDiagnose,
}

func init() {
	RootCmd.AddCommand(diagnoseCmd)
	RootCmd.AddCommand(listChecksCmd)
	RootCmd.AddCommand(versionCmd)

	diagnoseCmd.Flags().StringVarP(&url, "url", "u", "", "URL to diagnose (required)")
	diagnoseCmd.Flags().StringVarP(&proxyStr, "proxy", "p", "", "Proxy URL (e.g., http://localhost:8080, socks5://localhost:1080)")
	diagnoseCmd.Flags().StringVar(&proxyType, "proxy-type", "auto", "Proxy type: auto, http, https, socks4, socks5")
	diagnoseCmd.Flags().StringVarP(&exportFmt, "export", "e", "text", "Export format: text, json, html, markdown")
	diagnoseCmd.Flags().StringVarP(&output, "output", "o", "", "Output file (empty = stdout)")
	diagnoseCmd.Flags().BoolVar(&compare, "compare", false, "Compare with direct connection")
	diagnoseCmd.Flags().StringVar(&timeout, "timeout", engine.DefaultDiagnosisTimeout.String(), "Diagnosis timeout (1s to 5m, e.g., 10s, 2m)")
	diagnoseCmd.Flags().StringVar(&checks, "checks", "", "Comma-separated check IDs or categories to run (empty/all = all checks)")

	diagnoseCmd.MarkFlagRequired("url")
}

func runDiagnose(cmd *cobra.Command, args []string) error {
	fmt.Printf("🔍 ProxyDoctor v0.2.1 - Proxy Diagnostics Tool\n")
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	diagnosisTimeout, err := parseDiagnosisTimeout(timeout)
	if err != nil {
		fmt.Printf("❌ Invalid timeout: %v\n", err)
		return err
	}

	proxyConfig, err := utils.ParseProxyConfig(proxyStr, proxyType)
	if err != nil {
		fmt.Printf("❌ Invalid proxy configuration: %v\n", err)
		return err
	}

	registry := engine.NewCheckRegistry()
	if err := checkspkg.RegisterDefaults(registry); err != nil {
		return err
	}

	checkIDs, err := parseCheckFilters(checks, registry)
	if err != nil {
		fmt.Printf("❌ Invalid checks: %v\n", err)
		return err
	}

	adapterFactory := adapters.NewAdapterFactory()
	orchestrator := engine.NewDiagnosisOrchestrator(registry, adapterFactory, 4)

	diagRequest := engine.DiagnosisRequest{
		URL:         url,
		ProxyConfig: proxyConfig,
		CheckIDs:    checkIDs,
		Timeout:     diagnosisTimeout,
	}

	fmt.Printf("📋 Running diagnosis for: %s\n", url)
	if len(checkIDs) > 0 {
		fmt.Printf("🧪 Checks: %s\n", strings.Join(checkIDs, ", "))
	}
	if proxyConfig.Type != check.ProxyTypeDirect {
		fmt.Printf("🔗 Via proxy: %s://%s:%d\n", proxyConfig.Type, proxyConfig.Host, proxyConfig.Port)
	}
	fmt.Printf("\n")

	if compare {
		comparisonReport, err := orchestrator.ExecuteComparison(diagRequest)
		if err != nil {
			fmt.Printf("❌ Comparison failed: %v\n", err)
			return err
		}

		if err := formatComparisonResults(comparisonReport, exportFmt, output); err != nil {
			fmt.Printf("❌ Failed to save output: %v\n", err)
		} else if output != "" {
			fmt.Printf("✅ Results saved to %s\n", output)
		}

		return nil
	}

	report, err := orchestrator.Execute(diagRequest)
	if err != nil {
		fmt.Printf("❌ Diagnosis failed: %v\n", err)
		return err
	}

	if err := formatResults(report, exportFmt, output); err != nil {
		fmt.Printf("❌ Failed to save output: %v\n", err)
	} else if output != "" {
		fmt.Printf("✅ Results saved to %s\n", output)
	}

	return nil
}

func parseDiagnosisTimeout(value string) (time.Duration, error) {
	if value == "" {
		return engine.DefaultDiagnosisTimeout, nil
	}

	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("must be a valid duration such as 10s, 5m, or 1h: %w", err)
	}
	if parsed < minDiagnosisTimeout {
		return 0, fmt.Errorf("must be at least %s", minDiagnosisTimeout)
	}
	if parsed > maxDiagnosisTimeout {
		return 0, fmt.Errorf("must be at most %s", maxDiagnosisTimeout)
	}

	return parsed, nil
}

func parseCheckFilters(value string, registry *engine.CheckRegistry) ([]string, error) {
	value = strings.TrimSpace(value)
	if value == "" || strings.EqualFold(value, "all") {
		return nil, nil
	}

	availableChecks := registry.ListChecks()
	selected := make(map[string]struct{})
	var checkIDs []string

	for _, raw := range strings.Split(value, ",") {
		filter := strings.TrimSpace(raw)
		if filter == "" {
			continue
		}

		if _, ok := registry.GetCheck(filter); ok {
			if _, seen := selected[filter]; !seen {
				selected[filter] = struct{}{}
				checkIDs = append(checkIDs, filter)
			}
			continue
		}

		categoryMatched := false
		category := check.CheckCategory(filter)
		for id, checker := range availableChecks {
			if checker.Category() != category {
				continue
			}
			categoryMatched = true
			if _, seen := selected[id]; !seen {
				selected[id] = struct{}{}
				checkIDs = append(checkIDs, id)
			}
		}
		if categoryMatched {
			continue
		}

		return nil, fmt.Errorf("unknown check ID or category %q (available: %s)", filter, availableCheckFilters(availableChecks))
	}

	sort.Strings(checkIDs)
	return checkIDs, nil
}

func availableCheckFilters(checks map[string]check.Checker) string {
	values := make(map[string]struct{}, len(checks))
	for id, checker := range checks {
		values[id] = struct{}{}
		values[string(checker.Category())] = struct{}{}
	}

	labels := make([]string, 0, len(values))
	for value := range values {
		labels = append(labels, value)
	}
	sort.Strings(labels)
	return strings.Join(labels, ", ")
}

func formatComparisonResults(report *engine.ComparisonReport, format string, outPath string) error {
	var result string
	switch format {
	case "json":
		result = formatComparisonJSON(report)
	case "text":
		result = formatComparisonText(report)
	case "markdown", "md":
		result = formatComparisonMarkdown(report)
	case "html":
		result = formatComparisonHTML(report)
	default:
		result = formatComparisonText(report)
	}

	if outPath != "" {
		return os.WriteFile(outPath, []byte(result), 0644)
	}

	fmt.Print(result)
	return nil
}

func formatResults(report *engine.DiagnosisReport, format string, outPath string) error {
	var result string
	switch format {
	case "json":
		result = formatJSON(report)
	case "text":
		result = formatText(report)
	case "markdown", "md":
		result = formatMarkdown(report)
	case "html":
		result = formatHTML(report)
	default:
		result = formatText(report)
	}

	if outPath != "" {
		return os.WriteFile(outPath, []byte(result), 0644)
	}

	fmt.Print(result)
	return nil
}

func formatText(report *engine.DiagnosisReport) string {
	var out string
	out += "📊 Diagnosis Results\n"
	out += "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n"

	for i, result := range report.Results {
		status := "✅"
		if result.IsFailed() {
			status = "❌"
		} else if result.IsError() {
			status = "⚠️"
		}

		out += fmt.Sprintf("%d. %s %s\n", i+1, status, result.ID)
		out += fmt.Sprintf("   Status: %s | Severity: %s | Confidence: %.0f%%\n",
			result.Status, result.Severity, result.Confidence*100)
		out += fmt.Sprintf("   %s\n\n", result.Explanation)
	}

	out += "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n"
	out += fmt.Sprintf("Checks Executed: %d | Failed: %d | Critical: %d\n",
		report.ChecksExecuted, report.ChecksFailed, report.CriticalFindings)
	out += fmt.Sprintf("Total Time: %s\n", report.ExecutionTime)
	return out
}

func formatJSON(report *engine.DiagnosisReport) string {
	b, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Sprintf("{\"error\": \"%s\"}\n", err.Error())
	}
	return string(b) + "\n"
}

func formatComparisonJSON(report *engine.ComparisonReport) string {
	b, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Sprintf("{\"error\": \"%s\"}\n", err.Error())
	}
	return string(b) + "\n"
}

func formatHTML(report *engine.DiagnosisReport) string {
	var out string
	out += "<!doctype html>\n<html><head><meta charset=\"utf-8\"><title>ProxyDoctor Diagnosis Report</title></head><body>\n"
	out += "<h1>ProxyDoctor Diagnosis Report</h1>\n"
	out += "<ol>\n"
	for _, result := range report.Results {
		out += fmt.Sprintf("<li><strong>%s</strong><br>Status: %s<br>Severity: %s<br>Confidence: %.0f%%<p>%s</p></li>\n",
			html.EscapeString(result.ID),
			html.EscapeString(string(result.Status)),
			html.EscapeString(string(result.Severity)),
			result.Confidence*100,
			html.EscapeString(result.Explanation))
	}
	out += "</ol>\n"
	out += fmt.Sprintf("<p>Checks Executed: %d | Failed: %d | Critical: %d</p>\n",
		report.ChecksExecuted, report.ChecksFailed, report.CriticalFindings)
	out += fmt.Sprintf("<p>Total Time: %s</p>\n", html.EscapeString(report.ExecutionTime.String()))
	out += "</body></html>\n"
	return out
}

func formatMarkdown(report *engine.DiagnosisReport) string {
	var out string
	out += "# ProxyDoctor Diagnosis Report\n\n"
	for i, result := range report.Results {
		status := "✅"
		if result.IsFailed() {
			status = "❌"
		} else if result.IsError() {
			status = "⚠️"
		}
		out += fmt.Sprintf("## %d. %s %s\n\n", i+1, status, result.ID)
		out += fmt.Sprintf("- **Status**: %s\n", result.Status)
		out += fmt.Sprintf("- **Severity**: %s\n", result.Severity)
		out += fmt.Sprintf("- **Confidence**: %.0f%%\n\n", result.Confidence*100)
		out += fmt.Sprintf("%s\n\n", result.Explanation)
	}
	out += "---\n\n"
	out += fmt.Sprintf("- **Checks Executed**: %d\n", report.ChecksExecuted)
	out += fmt.Sprintf("- **Failed**: %d\n", report.ChecksFailed)
	out += fmt.Sprintf("- **Critical**: %d\n", report.CriticalFindings)
	out += fmt.Sprintf("- **Total Time**: %s\n", report.ExecutionTime)
	return out
}

func formatComparisonText(report *engine.ComparisonReport) string {
	var out string
	out += "📊 ProxyDoctor Comparison Results\n"
	out += "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n"
	out += "Direct Connection\n"
	out += formatResultSummary(report.DirectReport)
	out += "\nProxied Connection\n"
	out += formatResultSummary(report.ProxyReport)
	out += "\nDifferences\n"
	if len(report.Differences) == 0 {
		out += "   No differences detected.\n"
	} else {
		for _, diff := range report.Differences {
			out += fmt.Sprintf("   - %s: %v → %v\n", diff.Summary, diff.DirectValue, diff.ProxyValue)
		}
	}
	out += "\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n"
	out += fmt.Sprintf("Total Time: %s\n", report.ExecutionTime)
	return out
}

func formatComparisonMarkdown(report *engine.ComparisonReport) string {
	var out string
	out += "# ProxyDoctor Comparison Report\n\n"
	out += "## Direct Connection\n\n"
	out += formatMarkdownSummary(report.DirectReport)
	out += "\n## Proxied Connection\n\n"
	out += formatMarkdownSummary(report.ProxyReport)
	out += "\n## Differences\n\n"
	if len(report.Differences) == 0 {
		out += "No differences detected.\n"
	} else {
		for _, diff := range report.Differences {
			out += fmt.Sprintf("- **%s** `%s`: `%v` → `%v`\n", diff.CheckID, diff.Field, diff.DirectValue, diff.ProxyValue)
		}
	}
	out += fmt.Sprintf("\n---\n\n- **Total Time**: %s\n", report.ExecutionTime)
	return out
}

func formatComparisonHTML(report *engine.ComparisonReport) string {
	var out string
	out += "<!doctype html>\n<html><head><meta charset=\"utf-8\"><title>ProxyDoctor Comparison Report</title></head><body>\n"
	out += "<h1>ProxyDoctor Comparison Report</h1>\n"
	out += "<h2>Direct Connection</h2>\n"
	out += formatHTMLSummary(report.DirectReport)
	out += "<h2>Proxied Connection</h2>\n"
	out += formatHTMLSummary(report.ProxyReport)
	out += "<h2>Differences</h2>\n"
	if len(report.Differences) == 0 {
		out += "<p>No differences detected.</p>\n"
	} else {
		out += "<ul>\n"
		for _, diff := range report.Differences {
			out += fmt.Sprintf("<li><strong>%s</strong> %s: <code>%s</code> to <code>%s</code><br>%s</li>\n",
				html.EscapeString(diff.CheckID),
				html.EscapeString(diff.Field),
				html.EscapeString(fmt.Sprint(diff.DirectValue)),
				html.EscapeString(fmt.Sprint(diff.ProxyValue)),
				html.EscapeString(diff.Summary))
		}
		out += "</ul>\n"
	}
	out += fmt.Sprintf("<p>Total Time: %s</p>\n", html.EscapeString(report.ExecutionTime.String()))
	out += "</body></html>\n"
	return out
}

func formatResultSummary(report *engine.DiagnosisReport) string {
	return fmt.Sprintf("   Checks Executed: %d | Failed: %d | Critical: %d | Time: %s\n",
		report.ChecksExecuted, report.ChecksFailed, report.CriticalFindings, report.ExecutionTime)
}

func formatMarkdownSummary(report *engine.DiagnosisReport) string {
	return fmt.Sprintf("- **Checks Executed**: %d\n- **Failed**: %d\n- **Critical**: %d\n- **Total Time**: %s\n",
		report.ChecksExecuted, report.ChecksFailed, report.CriticalFindings, report.ExecutionTime)
}

func formatHTMLSummary(report *engine.DiagnosisReport) string {
	return fmt.Sprintf("<p>Checks Executed: %d | Failed: %d | Critical: %d | Time: %s</p>\n",
		report.ChecksExecuted, report.ChecksFailed, report.CriticalFindings, html.EscapeString(report.ExecutionTime.String()))
}

func formatRouteTraceCLI(evidence map[string]interface{}) string {
	var b strings.Builder
	countries := stringSliceEvidence(evidence["route_countries"])
	if len(countries) > 0 {
		b.WriteString(fmt.Sprintf("Route countries: %s\n", strings.Join(countries, " -> ")))
	}
	if hopCount, ok := evidence["hop_count"]; ok {
		b.WriteString(fmt.Sprintf("Hop count: %v\n", hopCount))
	}
	if len(countries) == 0 {
		b.WriteString("Route countries: unavailable\n")
	}
	return b.String()
}

func stringSliceEvidence(value interface{}) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []interface{}:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}
