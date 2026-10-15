package framework

import (
	"context"
	"net/netip"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"

	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"golang.org/x/net/idna"
)

type Compare int

const (
	// Auto compares an identity attribute as a DomainName and any other
	// attribute as Exact.
	Auto Compare = iota
	Exact
	// CaseInsensitive is for values such as hex the server writes in a case
	// of its own.
	CaseInsensitive
	// DomainName ignores case and the trailing dot, and matches an
	// internationalized name with its punycode.
	DomainName
	// ZoneName is a DomainName sent without the trailing dot, for a zone
	// the server looks up by a name it rejects with one.
	ZoneName
	// Mailbox is a DomainName whose first label is the local part of an
	// email address, which the server writes with "@".
	Mailbox
	// IPAddress compares addresses, not spellings. Values must be addresses.
	IPAddress
	// URI ignores the spelling differences the server normalizes, such as
	// the case of the scheme and host, a default port and an empty path.
	URI
	// SvcParams compares a Map of SVCB service parameters by value, for those
	// the server rewrites: hint addresses, ports, spaces in lists, the case
	// of mandatory keys and the spelling of ech hex.
	SvcParams
)

func (a Attr) compare() Compare {
	if a.Compare != Auto {
		return a.Compare
	}
	if a.Identity {
		return DomainName
	}
	return Exact
}

func (c Compare) same(x, y string) bool {
	if x == y {
		return true
	}
	switch c {
	case CaseInsensitive:
		return strings.EqualFold(x, y)
	case DomainName, ZoneName:
		return domainName(x) == domainName(y)
	case Mailbox:
		return mailbox(x) == mailbox(y)
	case IPAddress:
		a, aerr := netip.ParseAddr(x)
		b, berr := netip.ParseAddr(y)
		return aerr == nil && berr == nil && a == b
	case URI:
		return uri(x) == uri(y)
	}
	return false
}

func domainName(s string) string {
	s = strings.ToLower(strings.TrimSuffix(s, "."))
	if ascii, err := idna.Punycode.ToASCII(s); err == nil {
		return ascii
	}
	return s
}

func mailbox(s string) string {
	if !strings.Contains(s, "@") {
		s = strings.Replace(s, ".", "@", 1)
	}
	return domainName(s)
}

var defaultPorts = map[string]string{"http": "80", "https": "443", "ftp": "21", "ws": "80", "wss": "443"}

func uri(s string) string {
	u, err := url.Parse(s)
	if err != nil {
		return s
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	if p := u.Port(); p != "" && p == defaultPorts[u.Scheme] {
		u.Host = strings.TrimSuffix(u.Host, ":"+p)
	}
	if u.Host != "" && u.Path == "" && u.Opaque == "" {
		u.Path = "/"
	}
	return u.String()
}

func (a Attr) matches(got, prior tftypes.Value) bool {
	if a.Type == Map && a.compare() == SvcParams {
		return svcParams(got, prior)
	}
	g, _ := a.param(got)
	p, _ := a.param(prior)
	if a.Type != String {
		return g == p
	}
	return a.compare().same(g, p)
}

func svcParams(x, y tftypes.Value) bool {
	var a, b map[string]tftypes.Value
	if !set(x) || !set(y) || x.As(&a) != nil || y.As(&b) != nil || len(a) != len(b) {
		return false
	}
	for k, v := range a {
		w, ok := b[k]
		var s, t string
		if !ok || v.As(&s) != nil || w.As(&t) != nil || !svcParam(k, s, t) {
			return false
		}
	}
	return true
}

func svcParam(key, x, y string) bool {
	if x == y {
		return true
	}
	switch key {
	case "ipv4hint", "ipv6hint":
		return slices.EqualFunc(strings.Split(x, ","), strings.Split(y, ","), func(x, y string) bool {
			return IPAddress.same(strings.TrimSpace(x), strings.TrimSpace(y))
		})
	case "port":
		a, aerr := strconv.ParseUint(strings.TrimSpace(x), 10, 16)
		b, berr := strconv.ParseUint(strings.TrimSpace(y), 10, 16)
		return aerr == nil && berr == nil && a == b
	case "alpn", "mandatory":
		return slices.EqualFunc(strings.Split(x, ","), strings.Split(y, ","), func(x, y string) bool {
			x, y = strings.TrimSpace(x), strings.TrimSpace(y)
			return x == y || key == "mandatory" && strings.EqualFold(x, y)
		})
	case "ech":
		hex := strings.NewReplacer(":", "", " ", "")
		return strings.EqualFold(hex.Replace(x), hex.Replace(y))
	}
	return false
}

type ipValidator struct{}

func (ipValidator) Description(context.Context) string { return "must be an IP address" }

func (v ipValidator) MarkdownDescription(ctx context.Context) string { return v.Description(ctx) }

func (ipValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if _, err := netip.ParseAddr(req.ConfigValue.ValueString()); err != nil {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid IP address",
			"The value must be an IP address, got "+req.ConfigValue.String()+".")
	}
}
