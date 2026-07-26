package publicip

import "testing"

func TestParseIP(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
		ok   bool
	}{
		{name: "ipv4 trimmed", in: " 203.0.113.7\n", want: "203.0.113.7", ok: true},
		{name: "ipv6", in: "2001:db8::1", want: "2001:db8::1", ok: true},
		{name: "html is rejected", in: "<html>error</html>", ok: false},
		{name: "empty is rejected", in: "\n", ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseIP(tt.in)
			if tt.ok && err != nil {
				t.Fatalf("expected valid IP, got error: %v", err)
			}
			if !tt.ok && err == nil {
				t.Fatalf("expected error, got %q", got)
			}
			if tt.ok && got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}
