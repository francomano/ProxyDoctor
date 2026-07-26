package routetrace

import "testing"

func TestParseTracerouteOutput(t *testing.T) {
	raw := `traceroute to example.com (93.184.216.34), 20 hops max
 1  192.168.1.1  1.123 ms  1.221 ms  1.320 ms
 2  10.0.0.1  5.500 ms
 3  8.8.8.8  20.100 ms
 4  * * *
`
	hops := parseTracerouteOutput(raw)
	if len(hops) != 4 {
		t.Fatalf("expected 4 hops, got %d", len(hops))
	}
	if hops[0].Address != "192.168.1.1" || !hops[0].Private {
		t.Fatalf("unexpected first hop: %+v", hops[0])
	}
	if hops[2].Address != "8.8.8.8" || hops[2].Private {
		t.Fatalf("unexpected third hop: %+v", hops[2])
	}
	if !hops[3].TimedOut {
		t.Fatalf("expected fourth hop to be timed out: %+v", hops[3])
	}
}

func TestCountryFlag(t *testing.T) {
	if got := countryFlag("IT"); got != "🇮🇹" {
		t.Fatalf("got %q", got)
	}
	if got := countryFlag("bad"); got != "" {
		t.Fatalf("expected empty flag, got %q", got)
	}
}

func TestCompareRoutes(t *testing.T) {
	changed, summary := CompareRoutes(
		map[string]interface{}{"route_countries": []string{"Italy", "Germany"}},
		map[string]interface{}{"route_countries": []string{"Italy", "France"}},
	)
	if !changed || summary == "" {
		t.Fatalf("expected route difference, got changed=%v summary=%q", changed, summary)
	}
}
