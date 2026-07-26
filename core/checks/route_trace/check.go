package routetrace

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os/exec"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/francomano/proxydoctor/core/check"
)

const (
	maxTraceHops       = 20
	traceHopTimeoutSec = 2
	geoLookupTimeout   = 6 * time.Second
)

var hopLinePattern = regexp.MustCompile(`^\s*(\d+)\s+(.*)$`)
var ipPattern = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b|\b[0-9a-fA-F:]{2,}\b`)

// RouteTraceCheck runs a best-effort traceroute and enriches each public hop with country metadata.
type RouteTraceCheck struct{}

func NewRouteTraceCheck() check.Checker { return &RouteTraceCheck{} }

func (c *RouteTraceCheck) ID() string   { return "route_trace" }
func (c *RouteTraceCheck) Name() string { return "Route Trace" }
func (c *RouteTraceCheck) Description() string {
	return "Traces network hops to the target and annotates public hops with country information"
}
func (c *RouteTraceCheck) Category() check.CheckCategory { return check.CategoryNetwork }
func (c *RouteTraceCheck) DependsOn() []string           { return []string{"dns_resolve"} }

func (c *RouteTraceCheck) Execute(ctx check.ExecutionContext) check.CheckResult {
	result := check.NewCheckResult(c.ID(), c.Category())
	startTime := time.Now()

	parsed, err := url.Parse(ctx.GetURL())
	if err != nil {
		result.SetExecutionTime(time.Since(startTime))
		return *result.WithStatus(check.StatusError, check.SeverityCritical).
			WithExplanation(fmt.Sprintf("Invalid URL: %v", err)).
			WithConfidence(0)
	}

	hostname := parsed.Hostname()
	if hostname == "" {
		result.SetExecutionTime(time.Since(startTime))
		return *result.WithStatus(check.StatusError, check.SeverityCritical).
			WithExplanation("Invalid URL: missing hostname").
			WithConfidence(0)
	}

	if ctx.GetProxyConfig().Type != check.ProxyTypeDirect {
		result.AddEvidence("note", "Traceroute is executed from the local host; HTTP/SOCKS proxies usually cannot expose packet-level route hops.")
	}

	traceCtx, cancel := context.WithTimeout(context.Background(), ctx.GetTimeout())
	defer cancel()

	hops, command, rawOutput, err := runTraceroute(traceCtx, hostname)
	result.SetExecutionTime(time.Since(startTime))
	result.AddEvidence("hostname", hostname).
		AddEvidence("command", command).
		AddEvidence("raw_output", rawOutput)
	if err != nil {
		return *result.WithStatus(check.StatusError, check.SeverityWarning).
			WithExplanation(fmt.Sprintf("Traceroute failed for %s: %v", hostname, err)).
			WithConfidence(0).
			AddProbableCause("Traceroute command is unavailable or blocked by the network/firewall").
			AddSuggestedAction("Install traceroute/tracepath or allow ICMP/UDP probing from this host")
	}
	if len(hops) == 0 {
		return *result.WithStatus(check.StatusFailed, check.SeverityWarning).
			WithExplanation(fmt.Sprintf("Traceroute returned no hops for %s", hostname)).
			WithConfidence(0.4)
	}

	enrichHopsWithCountries(hops)
	countries := routeCountryNames(hops)
	flags := routeCountryFlags(hops)

	result.AddEvidence("hops", hops).
		AddEvidence("hop_count", len(hops)).
		AddEvidence("route_countries", countries).
		AddEvidence("route_flags", flags)
	ctx.SetSharedData("route_trace_hops", hops)
	ctx.SetSharedData("route_trace_countries", countries)

	explanation := fmt.Sprintf("Route to %s has %d hop(s)", hostname, len(hops))
	if len(countries) > 0 {
		explanation = fmt.Sprintf("%s across %s", explanation, strings.Join(countries, " → "))
	}

	return *result.WithStatus(check.StatusPassed, check.SeverityInfo).
		WithExplanation(explanation).
		WithConfidence(0.75)
}

// Hop describes one traceroute hop. It is kept JSON-friendly for CLI and GUI rendering.
type Hop struct {
	Number      int     `json:"number"`
	Address     string  `json:"address,omitempty"`
	Host        string  `json:"host,omitempty"`
	LatencyMS   float64 `json:"latency_ms,omitempty"`
	CountryCode string  `json:"country_code,omitempty"`
	CountryName string  `json:"country_name,omitempty"`
	Flag        string  `json:"flag,omitempty"`
	Private     bool    `json:"private"`
	TimedOut    bool    `json:"timed_out"`
}

func runTraceroute(ctx context.Context, hostname string) ([]Hop, string, string, error) {
	candidates := tracerouteCommands(hostname)
	var lastErr error
	for _, args := range candidates {
		cmd := exec.CommandContext(ctx, args[0], args[1:]...)
		out, err := cmd.CombinedOutput()
		raw := string(out)
		if ctx.Err() != nil {
			return nil, strings.Join(args, " "), raw, ctx.Err()
		}
		if err != nil {
			lastErr = fmt.Errorf("%s: %w", strings.Join(args, " "), err)
			continue
		}
		hops := parseTracerouteOutput(raw)
		return hops, strings.Join(args, " "), raw, nil
	}
	if lastErr != nil {
		return nil, "", "", lastErr
	}
	return nil, "", "", fmt.Errorf("no traceroute command candidate available")
}

func tracerouteCommands(hostname string) [][]string {
	if runtime.GOOS == "windows" {
		return [][]string{{"tracert", "-d", "-h", strconv.Itoa(maxTraceHops), hostname}}
	}
	commands := [][]string{
		{"traceroute", "-n", "-m", strconv.Itoa(maxTraceHops), "-w", strconv.Itoa(traceHopTimeoutSec), hostname},
		{"tracepath", "-n", "-m", strconv.Itoa(maxTraceHops), hostname},
	}
	return commands
}

func parseTracerouteOutput(raw string) []Hop {
	hops := make([]Hop, 0)
	for _, line := range strings.Split(raw, "\n") {
		match := hopLinePattern.FindStringSubmatch(line)
		if len(match) != 3 {
			continue
		}
		number, err := strconv.Atoi(match[1])
		if err != nil {
			continue
		}
		body := strings.TrimSpace(match[2])
		hop := Hop{Number: number, TimedOut: strings.Contains(body, "*")}
		if ip := firstIP(body); ip != "" {
			hop.Address = ip
			hop.Private = isPrivateIP(ip)
		}
		if latency := firstLatencyMS(body); latency > 0 {
			hop.LatencyMS = latency
		}
		hops = append(hops, hop)
	}
	return hops
}

func firstIP(s string) string {
	for _, candidate := range ipPattern.FindAllString(s, -1) {
		candidate = strings.Trim(candidate, "()[]")
		if addr, err := netip.ParseAddr(candidate); err == nil {
			return addr.String()
		}
	}
	return ""
}

func firstLatencyMS(s string) float64 {
	fields := strings.Fields(s)
	for i, field := range fields {
		if strings.EqualFold(strings.TrimSpace(field), "ms") && i > 0 {
			value, err := strconv.ParseFloat(strings.Trim(fields[i-1], "<>"), 64)
			if err == nil {
				return value
			}
		}
		if strings.HasSuffix(field, "ms") {
			value, err := strconv.ParseFloat(strings.TrimSuffix(field, "ms"), 64)
			if err == nil {
				return value
			}
		}
	}
	return 0
}

func isPrivateIP(ip string) bool {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}
	return addr.IsPrivate() || addr.IsLoopback() || addr.IsLinkLocalUnicast() || addr.IsMulticast() || addr.IsUnspecified()
}

func enrichHopsWithCountries(hops []Hop) {
	ips := make([]string, 0)
	seen := map[string]struct{}{}
	for _, hop := range hops {
		if hop.Address == "" || hop.Private {
			continue
		}
		if _, ok := seen[hop.Address]; ok {
			continue
		}
		seen[hop.Address] = struct{}{}
		ips = append(ips, hop.Address)
	}
	if len(ips) == 0 {
		return
	}
	countries := lookupCountries(ips)
	for i := range hops {
		if geo, ok := countries[hops[i].Address]; ok {
			hops[i].CountryCode = geo.CountryCode
			hops[i].CountryName = geo.CountryName
			hops[i].Flag = countryFlag(geo.CountryCode)
		}
	}
}

type geoInfo struct {
	CountryCode string
	CountryName string
}

func lookupCountries(ips []string) map[string]geoInfo {
	result := make(map[string]geoInfo)
	client := &http.Client{Timeout: geoLookupTimeout}
	for _, ip := range ips {
		info, err := lookupCountry(client, ip)
		if err == nil && info.CountryCode != "" {
			result[ip] = info
		}
	}
	return result
}

func lookupCountry(client *http.Client, ip string) (geoInfo, error) {
	endpoint := "https://ipapi.co/" + url.PathEscape(ip) + "/json/"
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return geoInfo{}, err
	}
	req.Header.Set("User-Agent", "ProxyDoctor/0.1")
	resp, err := client.Do(req)
	if err != nil {
		return geoInfo{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return geoInfo{}, fmt.Errorf("geo lookup returned HTTP %d", resp.StatusCode)
	}
	var body struct {
		CountryCode string `json:"country_code"`
		CountryName string `json:"country_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return geoInfo{}, err
	}
	return geoInfo{CountryCode: strings.ToUpper(body.CountryCode), CountryName: body.CountryName}, nil
}

func countryFlag(code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	if len(code) != 2 {
		return ""
	}
	runes := []rune(code)
	if runes[0] < 'A' || runes[0] > 'Z' || runes[1] < 'A' || runes[1] > 'Z' {
		return ""
	}
	return string([]rune{0x1F1E6 + runes[0] - 'A', 0x1F1E6 + runes[1] - 'A'})
}

func routeCountryNames(hops []Hop) []string {
	return uniqueHopValues(hops, func(h Hop) string { return h.CountryName })
}

func routeCountryFlags(hops []Hop) []string {
	return uniqueHopValues(hops, func(h Hop) string { return h.Flag })
}

func uniqueHopValues(hops []Hop, pick func(Hop) string) []string {
	seen := map[string]struct{}{}
	values := []string{}
	for _, hop := range hops {
		value := strings.TrimSpace(pick(hop))
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		values = append(values, value)
	}
	return values
}

func routeSignatureFromEvidence(evidence map[string]interface{}) []string {
	value, ok := evidence["route_countries"]
	if !ok {
		return nil
	}
	switch v := value.(type) {
	case []string:
		return append([]string(nil), v...)
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

// CompareRoutes returns a human-readable comparison between two route_trace results.
func CompareRoutes(directEvidence, proxyEvidence map[string]interface{}) (bool, string) {
	direct := routeSignatureFromEvidence(directEvidence)
	proxy := routeSignatureFromEvidence(proxyEvidence)
	if len(direct) == 0 && len(proxy) == 0 {
		return false, ""
	}
	if strings.Join(direct, "|") == strings.Join(proxy, "|") {
		return false, ""
	}
	return true, fmt.Sprintf("route countries changed from %s to %s", formatRoute(direct), formatRoute(proxy))
}

func formatRoute(route []string) string {
	if len(route) == 0 {
		return "unknown"
	}
	return strings.Join(route, " → ")
}

// StableCountryList is used by tests and output formatters that need deterministic country labels.
func StableCountryList(hops []Hop) []string {
	values := routeCountryNames(hops)
	sort.Strings(values)
	return values
}

var _ = net.IP{}
