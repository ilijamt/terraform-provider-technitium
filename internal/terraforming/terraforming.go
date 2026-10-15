// Package terraforming exports the objects on a Technitium server as
// Terraform configuration, with import blocks that adopt them.
package terraforming

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/ilijamt/terraform-provider-technitium/internal/client"
	"github.com/ilijamt/terraform-provider-technitium/internal/framework"
	"github.com/ilijamt/terraform-provider-technitium/internal/technitium"
)

type Options struct {
	Host, Token, Username, Password string
	Insecure                        bool

	// Out is the directory the files are written to.
	Out string
	// Force overwrites existing files.
	Force bool
	// Types limits the export to these resource types, named without the
	// technitium_ prefix. A trailing "*" matches a prefix, as in "record_*".
	Types []string
	// Zones limits zones and their records to these zones.
	Zones []string

	// Version is the provider version the reads run with.
	Version string
	// HTTPClient talks to the server; nil uses a default one.
	HTTPClient *http.Client
	// Log gets warnings and progress.
	Log io.Writer
}

// files groups resource types into the files they are written to. Records
// go to a file per zone instead.
var files = []struct{ name, prefix string }{
	{"zones.tf", "zone"},
	{"zones.tf", "record_soa"},
	{"settings.tf", "settings_"},
	{"dhcp.tf", "dhcp_"},
	{"blocking.tf", "allowed_zone"},
	{"blocking.tf", "blocked_zone"},
	{"access.tf", "user"},
	{"access.tf", "group"},
	{"access.tf", "group_members"},
	{"access.tf", "permissions_"},
}

func fileFor(obj object) string {
	for _, f := range files {
		if obj.Type == f.prefix || strings.HasSuffix(f.prefix, "_") && strings.HasPrefix(obj.Type, f.prefix) {
			return f.name
		}
	}
	if strings.HasPrefix(obj.Type, "record_") {
		return "records_" + sanitize(obj.Zone) + ".tf"
	}
	return "other.tf"
}

// before orders objects as they are written: by file; in zones.tf each zone
// followed by its SOA; elsewhere by type, then label.
func before(a, b object) bool {
	if fa, fb := fileFor(a), fileFor(b); fa != fb {
		return fa < fb
	}
	if fileFor(a) == "zones.tf" && a.Zone != b.Zone {
		return a.Zone < b.Zone
	}
	if a.Type != b.Type {
		return a.Type == "zone" || a.Type < b.Type && b.Type != "zone"
	}
	return a.name < b.name
}

// Run exports the server into o.Out: files of resources by group, a file of
// records per zone, imports.tf with an import block for each resource, and
// variables.tf for the secrets the configuration does not hold.
func Run(ctx context.Context, o Options) error {
	if o.Log == nil {
		o.Log = io.Discard
	}
	warn := func(format string, args ...any) { fmt.Fprintf(o.Log, "warning: "+format+"\n", args...) }

	api := client.New(o.Host, o.Token, "tf-technitium/"+o.Version, o.Insecure, o.HTTPClient)
	if o.Token == "" {
		if err := api.Login(ctx, o.Username, o.Password); err != nil {
			return err
		}
	}
	specs := map[string]framework.Spec{}
	for _, s := range technitium.Specs() {
		specs[s.Name] = s
	}
	for _, t := range o.Types {
		if _, ok := specs[strings.TrimSuffix(t, "*")]; !ok && !strings.HasSuffix(t, "*") {
			return fmt.Errorf("unknown resource type %q", t)
		}
	}

	objects, err := (&lister{api: api, specs: specs, zones: o.Zones, warn: warn}).list(ctx)
	if err != nil {
		return err
	}
	objects = filter(objects, o.Types)

	// Labels are given out in a stable order, apart from the file layout.
	sort.SliceStable(objects, func(i, j int) bool {
		if objects[i].Type != objects[j].Type {
			return objects[i].Type < objects[j].Type
		}
		return objects[i].Label < objects[j].Label
	})
	used := map[string]labels{}
	for i, obj := range objects {
		kind := obj.Type
		if obj.Data {
			kind = "data." + kind
		}
		if used[kind] == nil {
			used[kind] = labels{}
		}
		objects[i].name = used[kind].next(obj.Label)
	}

	// Read everything first: only what was read is written and referred to.
	r, err := newReader(ctx, o.Version, o.HTTPClient, o)
	if err != nil {
		return fmt.Errorf("configure provider: %w", err)
	}
	states := map[string]map[string]tftypes.Value{}
	var kept []object
	for _, obj := range objects {
		if obj.Data {
			kept = append(kept, obj)
			continue
		}
		state, err := r.read(ctx, obj.Type, obj.ID)
		switch {
		case err != nil:
			warn("skipping technitium_%s %s: %s", obj.Type, obj.ID, err)
		case state == nil:
			warn("skipping technitium_%s %s: gone", obj.Type, obj.ID)
		default:
			states[obj.Type+"."+obj.name] = state
			kept = append(kept, obj)
		}
	}
	objects = kept
	refs := references(objects)

	sort.SliceStable(objects, func(i, j int) bool {
		if objects[i].Data != objects[j].Data {
			return objects[i].Data
		}
		return before(objects[i], objects[j])
	})
	out := map[string]*hclwrite.File{}
	body := func(name string) *hclwrite.Body {
		if out[name] == nil {
			out[name] = hclwrite.NewEmptyFile()
		}
		b := out[name].Body()
		if len(b.Blocks()) > 0 {
			b.AppendNewline()
		}
		return b
	}
	var vars []variable
	for _, obj := range objects {
		if obj.Data {
			dataBlock(body(fileFor(obj)), obj.Type, obj.name, obj.Identity)
			continue
		}
		vars = append(vars, resourceBlock(body(fileFor(obj)), obj.Type, obj.name, r.schemas["technitium_"+obj.Type], states[obj.Type+"."+obj.name], refs(obj.Type))...)
		importBlock(body("imports.tf"), obj.Type, obj.name, obj.ID)
	}
	for _, v := range vars {
		variableBlock(body("variables.tf"), v)
	}
	if len(out) == 0 {
		return errors.New("nothing to export")
	}
	return write(o.Out, out, o.Force, o.Log)
}

// references returns, for a resource type, the references to use instead of
// the names of exported zones, scopes, users and groups.
func references(objects []object) func(typ string) ref {
	type key struct{ kind, name string }
	index := map[key]hclwrite.Tokens{}
	traversal := func(names ...string) hclwrite.Tokens {
		t := hcl.Traversal{hcl.TraverseRoot{Name: names[0]}}
		for _, n := range names[1:] {
			t = append(t, hcl.TraverseAttr{Name: n})
		}
		return hclwrite.TokensForTraversal(t)
	}
	for _, obj := range objects {
		switch {
		case obj.Data && obj.Type == "group":
			index[key{"group", obj.ID}] = traversal("data", "technitium_group", obj.name, "name")
		case obj.Type == "group":
			index[key{"group", obj.ID}] = traversal("technitium_group", obj.name, "name")
		case obj.Type == "zone":
			index[key{"zone", strings.ToLower(obj.ID)}] = traversal("technitium_zone", obj.name, "zone")
		case obj.Type == "dhcp_scope":
			index[key{"scope", obj.ID}] = traversal("technitium_dhcp_scope", obj.name, "name")
		case obj.Data && obj.Type == "user":
			index[key{"user", obj.ID}] = traversal("data", "technitium_user", obj.name, "username")
		case obj.Type == "user":
			index[key{"user", obj.ID}] = traversal("technitium_user", obj.name, "username")
		}
	}
	return func(typ string) ref {
		return func(path []string, s string) (hclwrite.Tokens, bool) {
			var kind string
			switch {
			case strings.HasPrefix(typ, "record_") && slices.Equal(path, []string{"zone"}):
				kind, s = "zone", strings.ToLower(s)
			case typ == "dhcp_reserved_lease" && slices.Equal(path, []string{"scope"}):
				kind = "scope"
			case typ == "group_members" && slices.Equal(path, []string{"group"}):
				kind = "group"
			case typ == "group_members" && len(path) == 2 && path[0] == "members":
				kind = "user"
			case strings.HasPrefix(typ, "permissions_") && len(path) == 3 && path[0] == "group_permissions" && path[2] == "name":
				kind = "group"
			case strings.HasPrefix(typ, "permissions_") && len(path) == 3 && path[0] == "user_permissions" && path[2] == "username":
				kind = "user"
			default:
				return nil, false
			}
			t, ok := index[key{kind, s}]
			return t, ok
		}
	}
}

// filter keeps the objects whose type matches one of types, all when empty.
func filter(objects []object, types []string) []object {
	if len(types) == 0 {
		return objects
	}
	var out []object
	for _, obj := range objects {
		for _, t := range types {
			if obj.Type == t || strings.HasSuffix(t, "*") && strings.HasPrefix(obj.Type, strings.TrimSuffix(t, "*")) {
				out = append(out, obj)
				break
			}
		}
	}
	return out
}

func write(dir string, out map[string]*hclwrite.File, force bool, log io.Writer) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	names := sortedKeys(out)
	if !force {
		for _, name := range names {
			if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
				return fmt.Errorf("%s exists, use -force to overwrite", filepath.Join(dir, name))
			}
		}
	}
	for _, name := range names {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, hclwrite.Format(out[name].Bytes()), 0o644); err != nil {
			return err
		}
		fmt.Fprintf(log, "wrote %s\n", path)
	}
	return nil
}
