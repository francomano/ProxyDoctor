package ipv6leak

import (
	"reflect"
	"testing"

	"github.com/francomano/proxydoctor/core/check"
)

func TestTargetIPv6AddressesFiltersFromSharedData(t *testing.T) {
	got := targetIPv6Addresses([]string{
		"203.0.113.7",
		"2001:db8::1",
		"::ffff:203.0.113.7", // IPv4-mapped IPv6, should be excluded
		"2001:db8::1",        // duplicate, should be deduplicated
	}, "example.com")

	want := []string{"2001:db8::1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestTargetIPv6AddressesHandlesInterfaceSlice(t *testing.T) {
	got := targetIPv6Addresses([]interface{}{"2001:db8::2", "198.51.100.1"}, "example.com")
	want := []string{"2001:db8::2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestTargetIPv6AddressesNoIPv6(t *testing.T) {
	got := targetIPv6Addresses([]string{"203.0.113.7", "198.51.100.1"}, "example.com")
	if len(got) != 0 {
		t.Fatalf("expected no IPv6 addresses, got %v", got)
	}
}

func TestParseIPResponseJSON(t *testing.T) {
	got, err := parseIPResponse([]byte(`{"ip":"2001:db8::1"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "2001:db8::1" {
		t.Fatalf("got %q", got)
	}
}

func TestParseIPResponsePlainText(t *testing.T) {
	got, err := parseIPResponse([]byte("2001:db8::1\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "2001:db8::1" {
		t.Fatalf("got %q", got)
	}
}

func TestParseIPResponseEmpty(t *testing.T) {
	if _, err := parseIPResponse([]byte("  \n")); err == nil {
		t.Fatal("expected error for empty response")
	}
}

func TestEvaluateIPv6LeakNoSystemIPv6(t *testing.T) {
	v := evaluateIPv6Leak(false, false, true, "example.com", "", nil)
	if v.status != check.StatusPassed || v.severity != check.SeverityInfo {
		t.Fatalf("unexpected verdict: %+v", v)
	}
}

func TestEvaluateIPv6LeakProxyForwardsIPv6(t *testing.T) {
	v := evaluateIPv6Leak(true, true, true, "example.com", "2001:db8::1", []string{"2001:db8::99"})
	if v.status != check.StatusPassed || v.severity != check.SeverityInfo {
		t.Fatalf("unexpected verdict: %+v", v)
	}
}

func TestEvaluateIPv6LeakDetectedWithIPv6Target(t *testing.T) {
	v := evaluateIPv6Leak(true, false, true, "example.com", "2001:db8::1", []string{"2001:db8::99"})
	if v.status != check.StatusFailed || v.severity != check.SeverityCritical {
		t.Fatalf("unexpected verdict: %+v", v)
	}
	if len(v.causes) == 0 || len(v.actions) == 0 {
		t.Fatalf("expected probable causes and suggested actions, got %+v", v)
	}
}

func TestEvaluateIPv6LeakRiskWithoutIPv6Target(t *testing.T) {
	v := evaluateIPv6Leak(true, false, false, "example.com", "2001:db8::1", nil)
	if v.status != check.StatusFailed || v.severity != check.SeverityWarning {
		t.Fatalf("unexpected verdict: %+v", v)
	}
}
