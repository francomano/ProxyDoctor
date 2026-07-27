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
	"github.com/francomano/proxydoctor/core/plugin"
	"github.com/francomano/proxydoctor/core/plugins"
	"github.com/francomano/proxydoctor/core/utils"
)

var (
	url         string
	proxyStr    string
	proxyType   string
	exportFmt   string
	output      string
	compare     bool
	timeout     string
	checks      string
	pluginNames string
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
	diagnoseCmd.Flags().StringVar(&pluginNames, "plugins", "", "Comma-separated plugin IDs to load (e.g., route_trace) or 'all'")

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

	if pluginNames != "" {
		mgr := plugin.NewManager()
		names := strings.Split(pluginNames, ",")
		for i := range names {
			names[i] = strings.TrimSpace(names[i])
		}
		ctx := &plugin.Context{Registry: registry, Config: map[string]interface{}{}}
		if err := plugins.Load(names, mgr, ctx); err != nil {
			fmt.Printf("❌ Plugin load failed: %v\n", err)
			return err
		}
		defer mgr.ShutdownAll()
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

const htmlReportStyle = `
  :root {
    color-scheme: light dark;
    --bg: #f4f6f8;
    --card-bg: #ffffff;
    --text: #1a1f27;
    --muted: #5b6673;
    --border: #e2e6ea;
    --pass: #1e8e5a;
    --fail: #d64545;
    --error: #c9820a;
    --critical: #d64545;
    --warning: #c9820a;
    --info: #3b6fd6;
    --accent: #2b5fa8;
  }
  * { box-sizing: border-box; }
  body {
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
    background: var(--bg);
    color: var(--text);
    margin: 0;
    padding: 32px 16px;
    line-height: 1.5;
  }
  .container { max-width: 880px; margin: 0 auto; }
  header.report-header {
    background: var(--card-bg);
    border: 1px solid var(--border);
    border-radius: 10px;
    padding: 24px 28px;
    margin-bottom: 20px;
  }
  header.report-header h1 { margin: 0 0 6px; font-size: 1.6em; }
  header.report-header .meta { color: var(--muted); font-size: 0.92em; }
  .summary-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(120px, 1fr));
    gap: 12px;
    margin-top: 18px;
  }
  .summary-card {
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 12px 14px;
    text-align: center;
  }
  .summary-card .value { font-size: 1.5em; font-weight: 700; }
  .summary-card .label { color: var(--muted); font-size: 0.8em; text-transform: uppercase; letter-spacing: 0.03em; }
  .summary-card.critical .value { color: var(--critical); }
  .summary-card.failed .value { color: var(--fail); }
  .results { display: flex; flex-direction: column; gap: 12px; }
  .result-card {
    background: var(--card-bg);
    border: 1px solid var(--border);
    border-left: 5px solid var(--muted);
    border-radius: 8px;
    padding: 16px 20px;
  }
  .result-card.status-passed { border-left-color: var(--pass); }
  .result-card.status-failed { border-left-color: var(--fail); }
  .result-card.status-error { border-left-color: var(--error); }
  .result-card.status-skipped { border-left-color: var(--border); }
  .result-title {
    display: flex;
    align-items: center;
    justify-content: space-between;
    flex-wrap: wrap;
    gap: 8px;
  }
  .result-title h3 { margin: 0; font-size: 1.05em; }
  .badges { display: flex; gap: 6px; flex-wrap: wrap; }
  .badge {
    display: inline-block;
    padding: 2px 9px;
    border-radius: 999px;
    font-size: 0.75em;
    font-weight: 600;
    color: #fff;
    background: var(--muted);
    white-space: nowrap;
  }
  .badge.status-passed { background: var(--pass); }
  .badge.status-failed { background: var(--fail); }
  .badge.status-error { background: var(--error); }
  .badge.status-skipped { background: var(--muted); }
  .badge.severity-critical { background: var(--critical); }
  .badge.severity-warning { background: var(--warning); }
  .badge.severity-info { background: var(--info); }
  .badge.confidence { background: var(--accent); }
  .explanation { margin: 10px 0 0; color: var(--text); }
  details.evidence { margin-top: 10px; }
  details.evidence summary {
    cursor: pointer;
    color: var(--accent);
    font-size: 0.88em;
    font-weight: 600;
  }
  details.evidence table {
    width: 100%;
    border-collapse: collapse;
    margin-top: 8px;
    font-size: 0.85em;
  }
  details.evidence td {
    border-top: 1px solid var(--border);
    padding: 6px 8px;
    vertical-align: top;
  }
  details.evidence td.evidence-key {
    color: var(--muted);
    white-space: nowrap;
    width: 1%;
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  }
  ul.list-plain { margin: 8px 0 0; padding-left: 20px; }
  footer.report-footer {
    text-align: center;
    color: var(--muted);
    font-size: 0.82em;
    margin-top: 24px;
  }
`

func formatHTML(report *engine.DiagnosisReport) string {
	var out strings.Builder
	out.WriteString("<!doctype html>\n<html lang=\"en\"><head>\n")
	out.WriteString("<meta charset=\"utf-8\">\n<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n")
	out.WriteString("<title>ProxyDoctor Diagnosis Report</title>\n")
	out.WriteString("<style>" + htmlReportStyle + "</style>\n</head><body>\n")
	out.WriteString("<div class=\"container\">\n")

	out.WriteString("<header class=\"report-header\">\n")
	out.WriteString("<h1>🩺 ProxyDoctor Diagnosis Report</h1>\n")
	targetURL := report.RequestMetadata.URL
	generatedAt := report.RequestMetadata.CompletedAt
	if generatedAt.IsZero() {
		generatedAt = time.Now()
	}
	out.WriteString("<div class=\"meta\">\n")
	if targetURL != "" {
		out.WriteString(fmt.Sprintf("Target: <strong>%s</strong><br>\n", html.EscapeString(targetURL)))
	}
	out.WriteString(fmt.Sprintf("Generated: %s\n", html.EscapeString(generatedAt.Format("2006-01-02 15:04:05 MST"))))
	out.WriteString("</div>\n")

	out.WriteString("<div class=\"summary-grid\">\n")
	out.WriteString(summaryCard("Checks Executed", fmt.Sprintf("%d", report.ChecksExecuted), ""))
	out.WriteString(summaryCard("Failed", fmt.Sprintf("%d", report.ChecksFailed), "failed"))
	out.WriteString(summaryCard("Critical", fmt.Sprintf("%d", report.CriticalFindings), "critical"))
	out.WriteString(summaryCard("Warnings", fmt.Sprintf("%d", report.WarningFindings), ""))
	out.WriteString(summaryCard("Total Time", report.ExecutionTime.String(), ""))
	out.WriteString("</div>\n")
	out.WriteString("</header>\n")

	out.WriteString("<section class=\"results\">\n")
	for _, result := range report.Results {
		out.WriteString(formatHTMLResultCard(result))
	}
	out.WriteString("</section>\n")

	out.WriteString("<footer class=\"report-footer\">Generated by ProxyDoctor &mdash; github.com/francomano/ProxyDoctor</footer>\n")
	out.WriteString("</div>\n</body></html>\n")
	return out.String()
}

func summaryCard(label, value, extraClass string) string {
	class := "summary-card"
	if extraClass != "" {
		class += " " + extraClass
	}
	return fmt.Sprintf("<div class=\"%s\"><div class=\"value\">%s</div><div class=\"label\">%s</div></div>\n",
		class, html.EscapeString(value), html.EscapeString(label))
}

func formatHTMLResultCard(result check.CheckResult) string {
	statusClass := "status-" + strings.ToLower(string(result.Status))
	severityClass := "severity-" + strings.ToLower(string(result.Severity))

	var out strings.Builder
	out.WriteString(fmt.Sprintf("<article class=\"result-card %s\">\n", statusClass))
	out.WriteString("<div class=\"result-title\">\n")
	out.WriteString(fmt.Sprintf("<h3>%s</h3>\n", html.EscapeString(result.ID)))
	out.WriteString("<div class=\"badges\">\n")
	out.WriteString(fmt.Sprintf("<span class=\"badge %s\">%s</span>\n", statusClass, html.EscapeString(string(result.Status))))
	out.WriteString(fmt.Sprintf("<span class=\"badge %s\">%s</span>\n", severityClass, html.EscapeString(string(result.Severity))))
	out.WriteString(fmt.Sprintf("<span class=\"badge confidence\">%.0f%% confidence</span>\n", result.Confidence*100))
	out.WriteString("</div>\n</div>\n")

	if result.Explanation != "" {
		out.WriteString(fmt.Sprintf("<p class=\"explanation\">%s</p>\n", html.EscapeString(result.Explanation)))
	}

	if len(result.ProbableCauses) > 0 {
		out.WriteString("<div><strong>Probable causes</strong>" + htmlList(result.ProbableCauses) + "</div>\n")
	}
	if len(result.SuggestedActions) > 0 {
		out.WriteString("<div><strong>Suggested actions</strong>" + htmlList(result.SuggestedActions) + "</div>\n")
	}

	if len(result.Evidence) > 0 {
		out.WriteString("<details class=\"evidence\"><summary>Evidence</summary>\n<table>\n")
		keys := make([]string, 0, len(result.Evidence))
		for k := range result.Evidence {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			out.WriteString(fmt.Sprintf("<tr><td class=\"evidence-key\">%s</td><td>%s</td></tr>\n",
				html.EscapeString(k), html.EscapeString(formatEvidenceValue(result.Evidence[k]))))
		}
		out.WriteString("</table>\n</details>\n")
	}

	out.WriteString("</article>\n")
	return out.String()
}

func htmlList(items []string) string {
	var out strings.Builder
	out.WriteString("<ul class=\"list-plain\">\n")
	for _, item := range items {
		out.WriteString(fmt.Sprintf("<li>%s</li>\n", html.EscapeString(item)))
	}
	out.WriteString("</ul>\n")
	return out.String()
}

func formatEvidenceValue(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return typed
	case []string:
		return strings.Join(typed, ", ")
	case []interface{}:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			parts = append(parts, fmt.Sprint(item))
		}
		return strings.Join(parts, ", ")
	case nil:
		return ""
	default:
		return fmt.Sprint(typed)
	}
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
