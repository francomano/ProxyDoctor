package engine

import (
	"fmt"
	"sync"
	"time"

	"github.com/francomano/proxydoctor/core/check"
)

// DiagnosisRequest represents the input for a diagnosis
type DiagnosisRequest struct {
	URL         string
	ProxyConfig check.ProxyConfig
	CheckIDs    []string // empty = all checks
	Timeout     time.Duration
}

// DiagnosisReport represents the final output report
type DiagnosisReport struct {
	ID               string              `json:"id"`
	RequestMetadata  RequestMetadata     `json:"request_metadata"`
	Results          []check.CheckResult `json:"results"`
	ExecutionTime    time.Duration       `json:"execution_time"`
	ChecksExecuted   int                 `json:"checks_executed"`
	ChecksFailed     int                 `json:"checks_failed"`
	CriticalFindings int                 `json:"critical_findings"`
	WarningFindings  int                 `json:"warning_findings"`
}

// ComparisonReport contains direct and proxied diagnosis results plus their diff.
type ComparisonReport struct {
	ID            string                 `json:"id"`
	URL           string                 `json:"url"`
	DirectReport  *DiagnosisReport       `json:"direct_report"`
	ProxyReport   *DiagnosisReport       `json:"proxy_report"`
	Differences   []ComparisonDifference `json:"differences"`
	ExecutionTime time.Duration          `json:"execution_time"`
}

// ComparisonDifference describes a field that changed between direct and proxied runs.
type ComparisonDifference struct {
	CheckID     string      `json:"check_id"`
	Field       string      `json:"field"`
	DirectValue interface{} `json:"direct_value"`
	ProxyValue  interface{} `json:"proxy_value"`
	Summary     string      `json:"summary"`
}

// RequestMetadata contains information about the diagnosis request
type RequestMetadata struct {
	URL         string          `json:"url"`
	ProxyType   check.ProxyType `json:"proxy_type"`
	Timeout     time.Duration   `json:"timeout"`
	StartedAt   time.Time       `json:"started_at"`
	CompletedAt time.Time       `json:"completed_at"`
	UserAgent   string          `json:"user_agent,omitempty"`
}

// DefaultDiagnosisTimeout is the fallback timeout when none is specified.
const DefaultDiagnosisTimeout = 30 * time.Second

// DiagnosisOrchestrator orchestrates the execution of checks
type DiagnosisOrchestrator struct {
	registry   *CheckRegistry
	adapters   AdapterFactory
	maxWorkers int
}

// NewDiagnosisOrchestrator creates a new orchestrator
func NewDiagnosisOrchestrator(registry *CheckRegistry, adapters AdapterFactory, maxWorkers int) *DiagnosisOrchestrator {
	if maxWorkers <= 0 {
		maxWorkers = 4
	}
	return &DiagnosisOrchestrator{
		registry:   registry,
		adapters:   adapters,
		maxWorkers: maxWorkers,
	}
}

// Execute runs the diagnosis with the given request
func (o *DiagnosisOrchestrator) Execute(req DiagnosisRequest) (*DiagnosisReport, error) {
	startTime := time.Now()

	// Validate input
	if err := o.validateRequest(req); err != nil {
		return nil, err
	}
	if req.Timeout == 0 {
		req.Timeout = DefaultDiagnosisTimeout
	}

	// Get checks to execute
	checksToRun, err := o.getChecksToRun(req.CheckIDs)
	if err != nil {
		return nil, err
	}
	if len(checksToRun) == 0 {
		return nil, fmt.Errorf("no checks to execute")
	}

	// Create network adapters
	directAdapter := o.adapters.CreateAdapter(check.ProxyTypeDirect, check.ProxyConfig{})
	proxyAdapter := o.adapters.CreateAdapter(req.ProxyConfig.Type, req.ProxyConfig)

	// Create execution context
	ctx := NewExecutionContext(req.URL, req.ProxyConfig, directAdapter, proxyAdapter, req.Timeout)

	// Build dependency graph
	dag := o.buildDependencyGraph(checksToRun)

	// Execute checks
	results := o.executeChecksParallel(ctx, dag, checksToRun)

	// Generate report
	report := o.generateReport(req, results, startTime)

	return report, nil
}

// ExecuteComparison runs the same diagnosis directly and through the configured proxy.
func (o *DiagnosisOrchestrator) ExecuteComparison(req DiagnosisRequest) (*ComparisonReport, error) {
	startTime := time.Now()
	if req.ProxyConfig.Type == check.ProxyTypeDirect {
		return nil, fmt.Errorf("comparison requires a proxy configuration")
	}

	directReq := req
	directReq.ProxyConfig = check.ProxyConfig{Type: check.ProxyTypeDirect}

	directReport, err := o.Execute(directReq)
	if err != nil {
		return nil, fmt.Errorf("direct diagnosis failed: %w", err)
	}

	proxyReport, err := o.Execute(req)
	if err != nil {
		return nil, fmt.Errorf("proxy diagnosis failed: %w", err)
	}

	return &ComparisonReport{
		ID:            fmt.Sprintf("compare_%d", time.Now().Unix()),
		URL:           req.URL,
		DirectReport:  directReport,
		ProxyReport:   proxyReport,
		Differences:   CompareReports(directReport, proxyReport),
		ExecutionTime: time.Since(startTime),
	}, nil
}

// CompareReports returns user-facing differences between direct and proxied reports.
func CompareReports(directReport, proxyReport *DiagnosisReport) []ComparisonDifference {
	if directReport == nil || proxyReport == nil {
		return nil
	}

	proxyResults := make(map[string]check.CheckResult, len(proxyReport.Results))
	for _, result := range proxyReport.Results {
		proxyResults[result.ID] = result
	}

	differences := make([]ComparisonDifference, 0)
	for _, directResult := range directReport.Results {
		proxyResult, ok := proxyResults[directResult.ID]
		if !ok {
			differences = append(differences, ComparisonDifference{
				CheckID:     directResult.ID,
				Field:       "result",
				DirectValue: "present",
				ProxyValue:  "missing",
				Summary:     fmt.Sprintf("%s ran directly but not through the proxy", directResult.ID),
			})
			continue
		}

		differences = appendResultDifferences(differences, directResult, proxyResult)
	}

	directResults := make(map[string]check.CheckResult, len(directReport.Results))
	for _, result := range directReport.Results {
		directResults[result.ID] = result
	}
	for _, proxyResult := range proxyReport.Results {
		if _, ok := directResults[proxyResult.ID]; !ok {
			differences = append(differences, ComparisonDifference{
				CheckID:     proxyResult.ID,
				Field:       "result",
				DirectValue: "missing",
				ProxyValue:  "present",
				Summary:     fmt.Sprintf("%s ran through the proxy but not directly", proxyResult.ID),
			})
		}
	}

	return differences
}

func appendResultDifferences(
	differences []ComparisonDifference,
	directResult check.CheckResult,
	proxyResult check.CheckResult,
) []ComparisonDifference {
	if directResult.Status != proxyResult.Status {
		differences = append(differences, ComparisonDifference{
			CheckID:     directResult.ID,
			Field:       "status",
			DirectValue: directResult.Status,
			ProxyValue:  proxyResult.Status,
			Summary:     fmt.Sprintf("%s status changed from %s to %s", directResult.ID, directResult.Status, proxyResult.Status),
		})
	}

	if directResult.Severity != proxyResult.Severity {
		differences = append(differences, ComparisonDifference{
			CheckID:     directResult.ID,
			Field:       "severity",
			DirectValue: directResult.Severity,
			ProxyValue:  proxyResult.Severity,
			Summary:     fmt.Sprintf("%s severity changed from %s to %s", directResult.ID, directResult.Severity, proxyResult.Severity),
		})
	}

	if directIP, ok := directResult.Evidence["ip_address"]; ok {
		if proxyIP, ok := proxyResult.Evidence["ip_address"]; ok && directIP != proxyIP {
			differences = append(differences, ComparisonDifference{
				CheckID:     directResult.ID,
				Field:       "ip_address",
				DirectValue: directIP,
				ProxyValue:  proxyIP,
				Summary:     fmt.Sprintf("%s IP changed from %v to %v", directResult.ID, directIP, proxyIP),
			})
		}
	}

	if directResult.ID == "route_trace" {
		directRoute := routeCountriesFromEvidence(directResult.Evidence)
		proxyRoute := routeCountriesFromEvidence(proxyResult.Evidence)
		if fmt.Sprint(directRoute) != fmt.Sprint(proxyRoute) {
			differences = append(differences, ComparisonDifference{
				CheckID:     directResult.ID,
				Field:       "route_countries",
				DirectValue: directRoute,
				ProxyValue:  proxyRoute,
				Summary:     fmt.Sprintf("%s countries changed from %v to %v", directResult.ID, directRoute, proxyRoute),
			})
		}
	}

	if directResult.ExecutionTime != proxyResult.ExecutionTime {
		differences = append(differences, ComparisonDifference{
			CheckID:     directResult.ID,
			Field:       "execution_time",
			DirectValue: directResult.ExecutionTime,
			ProxyValue:  proxyResult.ExecutionTime,
			Summary:     fmt.Sprintf("%s latency changed by %s", directResult.ID, proxyResult.ExecutionTime-directResult.ExecutionTime),
		})
	}

	return differences
}

func (o *DiagnosisOrchestrator) validateRequest(req DiagnosisRequest) error {
	if req.URL == "" {
		return fmt.Errorf("URL is required")
	}
	return nil
}

func (o *DiagnosisOrchestrator) getChecksToRun(checkIDs []string) (map[string]check.Checker, error) {
	if len(checkIDs) == 0 {
		return o.registry.ListChecks(), nil
	}

	result := make(map[string]check.Checker)
	for _, id := range checkIDs {
		c, ok := o.registry.GetCheck(id)
		if !ok {
			return nil, fmt.Errorf("unknown check ID %q", id)
		}
		result[id] = c
	}
	return result, nil
}

func (o *DiagnosisOrchestrator) buildDependencyGraph(checks map[string]check.Checker) DependencyDAG {
	dag := NewDependencyDAG()
	for id, checker := range checks {
		dag.AddNode(id, checker)
		for _, depID := range checker.DependsOn() {
			dag.AddEdge(depID, id)
		}
	}
	return dag
}

func (o *DiagnosisOrchestrator) executeChecksParallel(
	ctx check.ExecutionContext,
	dag DependencyDAG,
	checks map[string]check.Checker,
) []check.CheckResult {
	results := make([]check.CheckResult, 0)
	resultsChan := make(chan check.CheckResult, len(checks))
	var wg sync.WaitGroup

	// Get execution order from DAG
	executionOrder := dag.TopologicalSort()

	// Semaphore for limiting concurrent executions
	semaphore := make(chan struct{}, o.maxWorkers)

	for _, id := range executionOrder {
		if checker, ok := checks[id]; ok {
			wg.Add(1)
			go func(checkID string, c check.Checker) {
				defer wg.Done()
				semaphore <- struct{}{}        // Acquire
				defer func() { <-semaphore }() // Release

				if execCtx, ok := ctx.(*DefaultExecutionContext); ok {
					if execCtx.IsCancelled() {
						return
					}
				}

				startTime := time.Now()
				result := c.Execute(ctx)
				result.ExecutionTime = time.Since(startTime)
				result.Timestamp = time.Now()

				resultsChan <- result
			}(id, checker)
		}
	}

	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	// Collect results
	for result := range resultsChan {
		results = append(results, result)
	}

	return results
}

func (o *DiagnosisOrchestrator) generateReport(
	req DiagnosisRequest,
	results []check.CheckResult,
	startTime time.Time,
) *DiagnosisReport {
	report := &DiagnosisReport{
		ID: fmt.Sprintf("diag_%d", time.Now().Unix()),
		RequestMetadata: RequestMetadata{
			URL:         req.URL,
			ProxyType:   req.ProxyConfig.Type,
			Timeout:     req.Timeout,
			StartedAt:   startTime,
			CompletedAt: time.Now(),
		},
		Results:        results,
		ExecutionTime:  time.Since(startTime),
		ChecksExecuted: len(results),
	}

	// Count failures and findings
	for _, r := range results {
		if r.IsFailed() {
			report.ChecksFailed++
		}
		if r.Severity == check.SeverityCritical {
			report.CriticalFindings++
		} else if r.Severity == check.SeverityWarning {
			report.WarningFindings++
		}
	}

	return report
}

func routeCountriesFromEvidence(evidence map[string]interface{}) []string {
	value, ok := evidence["route_countries"]
	if !ok {
		return nil
	}
	switch typed := value.(type) {
	case []string:
		return typed
	case []interface{}:
		countries := make([]string, 0, len(typed))
		for _, item := range typed {
			if country, ok := item.(string); ok && country != "" {
				countries = append(countries, country)
			}
		}
		return countries
	default:
		return nil
	}
}
