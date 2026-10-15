package terraforming

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/ilijamt/terraform-provider-technitium/internal/framework"
)

// builtinGroups exist on every server and cannot be deleted or renamed, so
// they are exported as data sources.
var builtinGroups = []string{"Administrators", "DHCP Administrators", "DNS Administrators", "Everyone"}

type object struct {
	Type  string
	Label string
	ID    string
	Zone  string
	// Data objects cannot be deleted, so they are written as data sources
	// found by Identity.
	Data     bool
	Identity map[string]string

	name string
}

type caller interface {
	Call(ctx context.Context, path string, params url.Values) (map[string]any, error)
}

type lister struct {
	api   caller
	specs map[string]framework.Spec
	zones []string
	warn  func(format string, args ...any)
}

func (l *lister) list(ctx context.Context) ([]object, error) {
	var out []object
	for _, step := range []func(context.Context) ([]object, error){
		l.listZones, l.listSettings, l.listDHCP, l.listBlocking, l.listAccess, l.listApps,
	} {
		objs, err := step(ctx)
		if err != nil {
			return nil, err
		}
		out = append(out, objs...)
	}
	return out, nil
}

// Only zones whose records are kept on this server have their records listed.
func (l *lister) listZones(ctx context.Context) ([]object, error) {
	resp, err := l.api.Call(ctx, "/api/zones/list", nil)
	if err != nil {
		return nil, err
	}
	var out []object
	for _, z := range items(resp, "zones") {
		name, _ := z["name"].(string)
		typ, _ := z["type"].(string)
		if name == "" || (len(l.zones) > 0 && !slices.ContainsFunc(l.zones, func(s string) bool { return strings.EqualFold(s, name) })) {
			continue
		}
		out = append(out, object{Type: "zone", Label: name, ID: name, Zone: name},
			object{Type: "zone_permissions", Label: name, ID: name, Zone: name})
		// Other zone types are copied from elsewhere, not edited here.
		if typ != "Primary" && typ != "Forwarder" {
			continue
		}
		out = append(out, object{Type: "record_soa", Label: name, ID: name + "/" + name, Zone: name})
		if status, _ := z["dnssecStatus"].(string); strings.HasPrefix(status, "Signed") {
			out = append(out, object{Type: "zone_dnssec", Label: name, ID: name, Zone: name})
		}

		records, err := l.api.Call(ctx, "/api/zones/records/get", url.Values{"zone": {name}, "domain": {name}, "listZone": {"true"}})
		if err != nil {
			return nil, fmt.Errorf("records of %s: %w", name, err)
		}
		for _, r := range items(records, "records") {
			rtype, _ := r["type"].(string)
			// The DNSSEC records come with signing, which zone_dnssec owns.
			switch rtype {
			case "SOA", "DNSKEY", "RRSIG", "NSEC", "NSEC3", "NSEC3PARAM":
				continue
			}
			spec, ok := l.specs["record_"+strings.ToLower(rtype)]
			if !ok {
				l.warn("skipping %s record %v in %s: no resource for the type", rtype, r["name"], name)
				continue
			}
			id, err := recordID(spec, name, r)
			if err != nil {
				return nil, err
			}
			domain, _ := r["name"].(string)
			out = append(out, object{Type: spec.Name, Label: domain, ID: id, Zone: name})
		}
	}
	return out, nil
}

func recordID(spec framework.Spec, zone string, record map[string]any) (string, error) {
	id := map[string]any{"zone": zone}
	parts := []string{zone}
	short := true
	for _, a := range spec.Attrs {
		if !a.Identity || a.Key == "" {
			continue
		}
		v, ok := lookup(record, a.Key)
		if !ok || v == nil {
			short = false
			continue
		}
		id[a.Name] = v
		switch v := v.(type) {
		case string:
			// An empty part means unset, and only the last may hold "/".
			short = short && v != "" && !strings.Contains(v, "/")
			parts = append(parts, v)
		case json.Number:
			parts = append(parts, v.String())
		case bool:
			parts = append(parts, fmt.Sprint(v))
		default:
			short = false
		}
	}
	if short {
		return strings.Join(parts, "/"), nil
	}
	b, err := json.Marshal(id)
	return string(b), err
}

func (l *lister) listSettings(context.Context) ([]object, error) {
	var out []object
	for name := range l.specs {
		if strings.HasPrefix(name, "settings_") {
			out = append(out, object{Type: name, Label: "this", ID: name})
		}
	}
	return out, nil
}

func (l *lister) listDHCP(ctx context.Context) ([]object, error) {
	resp, err := l.api.Call(ctx, "/api/dhcp/scopes/list", nil)
	if err != nil {
		return nil, err
	}
	var out []object
	for _, s := range items(resp, "scopes") {
		name, _ := s["name"].(string)
		out = append(out, object{Type: "dhcp_scope", Label: name, ID: name})
		scope, err := l.api.Call(ctx, "/api/dhcp/scopes/get", url.Values{"name": {name}})
		if err != nil {
			return nil, fmt.Errorf("DHCP scope %s: %w", name, err)
		}
		for _, lease := range items(scope, "reservedLeases") {
			mac, _ := lease["hardwareAddress"].(string)
			label := mac
			if host, _ := lease["hostName"].(string); host != "" {
				label = host
			}
			out = append(out, object{Type: "dhcp_reserved_lease", Label: name + "_" + label, ID: name + "/" + mac})
		}
	}
	return out, nil
}

// The list call returns only one level of the tree, so it is walked.
func (l *lister) listBlocking(ctx context.Context) ([]object, error) {
	var out []object
	for _, kind := range []string{"allowed", "blocked"} {
		seen := map[string]bool{}
		var walk func(domain string) error
		walk = func(domain string) error {
			resp, err := l.api.Call(ctx, "/api/"+kind+"/list", url.Values{"domain": {domain}})
			if err != nil {
				return fmt.Errorf("%s zones at %q: %w", kind, domain, err)
			}
			at, _ := resp["domain"].(string)
			if seen[at] {
				return nil
			}
			seen[at] = true
			for _, r := range items(resp, "records") {
				if name, _ := r["name"].(string); name != "" && strings.EqualFold(name, at) {
					out = append(out, object{Type: kind + "_zone", Label: at, ID: at})
					break
				}
			}
			zones, _ := resp["zones"].([]any)
			for _, z := range zones {
				child, _ := z.(string)
				if m, ok := z.(map[string]any); ok {
					child, _ = m["name"].(string)
				}
				if child == "" || seen[child] {
					continue
				}
				if err := walk(child); err != nil {
					return err
				}
			}
			return nil
		}
		if err := walk(""); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (l *lister) listAccess(ctx context.Context) ([]object, error) {
	var out []object
	for _, c := range []struct{ path, list, key, typ string }{
		{"/api/admin/users/list", "users", "username", "user"},
		{"/api/admin/groups/list", "groups", "name", "group"},
		{"/api/admin/permissions/list", "permissions", "section", "permissions"},
	} {
		resp, err := l.api.Call(ctx, c.path, nil)
		if err != nil {
			return nil, err
		}
		for _, it := range items(resp, c.list) {
			v, _ := it[c.key].(string)
			if v == "" {
				continue
			}
			// The SSO provider owns these accounts, so they are read, not
			// managed.
			if sso, _ := it["isSsoUser"].(bool); c.typ == "user" && sso {
				out = append(out, object{Type: c.typ, Label: v, ID: v, Data: true, Identity: map[string]string{"username": v}})
				continue
			}
			if c.typ == "group" {
				// Everyone holds every user without a member list.
				if v != "Everyone" {
					out = append(out, object{Type: "group_members", Label: v, ID: v})
				}
				if slices.Contains(builtinGroups, v) {
					out = append(out, object{Type: c.typ, Label: v, ID: v, Data: true, Identity: map[string]string{"name": v}})
					continue
				}
			}
			if c.typ == "permissions" {
				if name, ok := l.permissionsFor(v); ok {
					out = append(out, object{Type: name, Label: "this", ID: name})
				} else {
					l.warn("skipping permissions of section %s: no resource for it", v)
				}
				continue
			}
			out = append(out, object{Type: c.typ, Label: v, ID: v})
		}
		// Everyone is not listed, but permissions refer to it.
		if c.typ == "group" {
			out = append(out, object{Type: c.typ, Label: "Everyone", ID: "Everyone", Data: true, Identity: map[string]string{"name": "Everyone"}})
		}
	}
	return out, nil
}

// The server does not return the URL an app came from, so apps are written
// without one, which adopts them as they are.
func (l *lister) listApps(ctx context.Context) ([]object, error) {
	resp, err := l.api.Call(ctx, "/api/apps/list", nil)
	if err != nil {
		return nil, err
	}
	var out []object
	for _, a := range items(resp, "apps") {
		name, _ := a["name"].(string)
		if name == "" {
			continue
		}
		out = append(out, object{Type: "app", Label: name, ID: name}, object{Type: "app_config", Label: name, ID: name})
	}
	return out, nil
}

func (l *lister) permissionsFor(section string) (string, bool) {
	for name, s := range l.specs {
		if strings.HasPrefix(name, "permissions_") && s.Fixed["section"] == section {
			return name, true
		}
	}
	return "", false
}

func items(resp map[string]any, key string) []map[string]any {
	list, _ := resp[key].([]any)
	var out []map[string]any
	for _, it := range list {
		if m, ok := it.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func lookup(obj map[string]any, key string) (any, bool) {
	var cur any = obj
	for _, k := range strings.Split(key, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		if cur, ok = m[k]; !ok {
			return nil, false
		}
	}
	return cur, true
}
