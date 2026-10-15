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

// object is one thing on the server to import.
type object struct {
	// Type is the resource name without the technitium_ prefix.
	Type string
	// Label is the name to derive the resource label from.
	Label string
	// ID is the import ID.
	ID string
	// Zone is the zone a zone, SOA or record object belongs to.
	Zone string
	// Data marks an object Terraform cannot delete, written as a data
	// source found by Identity.
	Data     bool
	Identity map[string]string

	// name is the resource label, given out before writing.
	name string
}

// caller is what listing needs from the API client.
type caller interface {
	Call(ctx context.Context, path string, params url.Values) (map[string]any, error)
}

type lister struct {
	api   caller
	specs map[string]framework.Spec
	zones []string
	warn  func(format string, args ...any)
}

// list returns every object on the server, zones filtered to l.zones when set.
func (l *lister) list(ctx context.Context) ([]object, error) {
	var out []object
	for _, step := range []func(context.Context) ([]object, error){
		l.listZones, l.listSettings, l.listDHCP, l.listBlocking, l.listAccess,
	} {
		objs, err := step(ctx)
		if err != nil {
			return nil, err
		}
		out = append(out, objs...)
	}
	return out, nil
}

// listZones returns the zones, and the SOA and other records of the zones
// whose records are kept on this server.
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
		out = append(out, object{Type: "zone", Label: name, ID: name, Zone: name})
		// Other zone types are copied from elsewhere, not edited here.
		if typ != "Primary" && typ != "Forwarder" {
			continue
		}
		out = append(out, object{Type: "record_soa", Label: name, ID: name + "/" + name, Zone: name})

		records, err := l.api.Call(ctx, "/api/zones/records/get", url.Values{"zone": {name}, "domain": {name}, "listZone": {"true"}})
		if err != nil {
			return nil, fmt.Errorf("records of %s: %w", name, err)
		}
		for _, r := range items(records, "records") {
			rtype, _ := r["type"].(string)
			if rtype == "SOA" {
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

// recordID is the import ID of a record: its identity attributes, taken from
// the record as the read call returns it. It is the short form, the values
// joined by "/", when that is unambiguous, and a JSON object otherwise.
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

// listBlocking walks the allowed and blocked zone trees: the list call
// returns the records at a domain and the subdomains below it.
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

// permissionsFor returns the permissions_<name> resource of a section.
func (l *lister) permissionsFor(section string) (string, bool) {
	for name, s := range l.specs {
		if strings.HasPrefix(name, "permissions_") && s.Fixed["section"] == section {
			return name, true
		}
	}
	return "", false
}

// items returns the objects in the list at key.
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

// lookup follows a dotted key through nested objects.
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
