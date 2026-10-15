package framework

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/stretchr/testify/assert"
)

func TestCompare(t *testing.T) {
	for _, c := range []struct {
		compare Compare
		x, y    string
		same    bool
	}{
		{Exact, "Hello", "Hello", true},
		{Exact, "Hello", "hello", false},
		{CaseInsensitive, "ab01", "AB01", true},
		{DomainName, "WWW.Example.com.", "www.example.com", true},
		{DomainName, "www.example.com", "www.example.org", false},
		{DomainName, "www.Bücher.test", "www.xn--bcher-kva.test", true},
		{DomainName, "_sip._tcp.example.com", "_SIP._tcp.example.com", true},
		{IPAddress, "2001:DB8:0::1", "2001:db8::1", true},
		{IPAddress, "10.0.0.1", "10.0.0.2", false},
		{IPAddress, "request-ip-address", "10.0.0.1", false},
		{URI, "HTTPS://Example.com", "https://example.com/", true},
		{URI, "https://example.com:443/a", "https://example.com/a", true},
		{URI, "https://example.com/A", "https://example.com/a", false},
		{URI, "ftp://ftp.example.com/public", "ftp://ftp.example.com/public", true},
		{Mailbox, "hostmaster.example.com.", "hostmaster@example.com", true},
		{Mailbox, "Admin@Example.com", "admin@example.com", true},
		{Mailbox, "admin.example.com", "other@example.com", false},
	} {
		assert.Equal(t, c.same, c.compare.same(c.x, c.y), "%v %q %q", c.compare, c.x, c.y)
	}

	for _, c := range []struct {
		key, x, y string
		same      bool
	}{
		{"ipv6hint", "2001:DB8::1,2001:db8:0:0::2", "2001:db8::1,2001:db8::2", true},
		{"ipv6hint", "2001:db8::2,2001:db8::1", "2001:db8::1,2001:db8::2", false},
		{"ipv4hint", "10.0.0.1, 10.0.0.2", "10.0.0.1,10.0.0.2", true},
		{"ipv4hint", "10.0.0.1", "10.0.0.1,10.0.0.2", false},
		{"port", "0443", "443", true},
		{"alpn", "h2, h3", "h2,h3", true},
		{"alpn", "H2", "h2", false},
		{"mandatory", "ALPN,port", "alpn,port", true},
		{"mandatory", "port,alpn", "alpn,port", false},
		{"ech", "aabb", "AA:BB", true},
		{"dohpath", "/Q", "/q", false},
	} {
		assert.Equal(t, c.same, svcParam(c.key, c.x, c.y), "%s %q %q", c.key, c.x, c.y)
	}

	assert.Equal(t, DomainName, Attr{Identity: true}.compare(), "identity values are names unless said otherwise")
	assert.Equal(t, Exact, Attr{}.compare())
	assert.Equal(t, Exact, Attr{Identity: true, Compare: Exact}.compare())
}

func TestFindKeepsCase(t *testing.T) {
	spec := Spec{Name: "record_txt", List: "records", Attrs: []Attr{
		{Name: "domain", Param: "domain", Type: String, Key: "name", Identity: true, Compare: DomainName},
		{Name: "text", Param: "text", Type: String, Key: "rData.text", Identity: true, Compare: Exact},
	}}
	resp := map[string]any{"records": []any{
		map[string]any{"name": "example.com", "rData": map[string]any{"text": "HELLO"}},
		map[string]any{"name": "example.com", "rData": map[string]any{"text": "hello"}},
	}}
	r := &Resource{spec: spec}
	got := r.find(resp, "records", map[string]tftypes.Value{"domain": str("Example.COM."), "text": str("hello")})
	assert.Equal(t, "hello", got["rData"].(map[string]any)["text"])
}
