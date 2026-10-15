package technitium

import "testing"

func TestReverseZoneHint(t *testing.T) {
	for in, want := range map[string]string{
		"example.com":            "",
		".":                      "",
		"1.168.192.in-addr.arpa": "",
		"192.168.1.0/24":         `use the reverse zone name "1.168.192.in-addr.arpa" for 192.168.1.0/24`,
		"10.0.0.0/8":             `use the reverse zone name "10.in-addr.arpa" for 10.0.0.0/8`,
		"192.168.1.5":            `use the reverse zone name "5.1.168.192.in-addr.arpa" for 192.168.1.5`,
		"192.168.16.0/20":        `use the reverse zone name "168.192.in-addr.arpa" for 192.168.0.0/16, or one zone per /24 such as "16.168.192.in-addr.arpa"`,
		"2001:db8::/32":          `use the reverse zone name "8.b.d.0.1.0.0.2.ip6.arpa" for 2001:db8::/32`,
		"2001:db8::/30":          `use the reverse zone name "b.d.0.1.0.0.2.ip6.arpa" for 2001:db0::/28, or one zone per /32 such as "8.b.d.0.1.0.0.2.ip6.arpa"`,
		"::1":                    `use the reverse zone name "1.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.ip6.arpa" for ::1`,
		"192.168":                "the server reads 192.168 as an IP address; use its reverse zone name",
	} {
		if got := reverseZoneHint(in); got != want {
			t.Errorf("%s: got %q, want %q", in, got, want)
		}
	}
}
