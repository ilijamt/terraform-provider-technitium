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
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/zclconf/go-cty/cty"

	"github.com/ilijamt/terraform-provider-technitium/internal/client"
	"github.com/ilijamt/terraform-provider-technitium/internal/framework"
	"github.com/ilijamt/terraform-provider-technitium/internal/technitium"
)

type Options struct {
	Host, Token, Username, Password string
	Insecure                        bool

	Out   string
	Force bool
	// Types are named without the technitium_ prefix; a trailing "*"
	// matches a prefix, as in "record_*".
	Types []string
	Zones []string

	Version    string
	HTTPClient *http.Client
	Log        io.Writer
}

var files = []struct{ name, prefix string }{
	{"zones.tf", "zone"},
	{"zones.tf", "record_soa"},
	{"zones.tf", "zone_permissions"},
	{"zones.tf", "zone_dnssec"},
	{"settings.tf", "settings_"},
	{"dhcp.tf", "dhcp_"},
	{"blocking.tf", "allowed_zone"},
	{"blocking.tf", "blocked_zone"},
	{"access.tf", "user"},
	{"access.tf", "group"},
	{"access.tf", "group_members"},
	{"access.tf", "permissions_"},
	{"apps.tf", "app"},
	{"apps.tf", "app_config"},
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

// In zones.tf each zone is followed by its SOA.
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
	objects, states, failed := readAll(ctx, objects, r.read, warn)
	prune := o.prune(failed)
	if failed && o.prune(false) {
		warn("files an earlier export wrote are not removed, since some objects could not be read")
	}
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
		spec := specs[obj.Type]
		b := body(fileFor(obj))
		if note := keptNote(spec); note != "" {
			b.AppendUnstructuredTokens(hclwrite.Tokens{{Type: hclsyntax.TokenComment, Bytes: []byte(note)}})
		}
		if obj.Type == "app_config" {
			var config string
			_ = states[obj.Type+"."+obj.name]["config"].As(&config)
			if keys := secretKeys(config); len(keys) > 0 {
				warn("technitium_app_config.%s: the config is written out in full and has keys that look like secrets (%s); keep %s private", obj.name, strings.Join(keys, ", "), fileFor(obj))
				b.AppendUnstructuredTokens(hclwrite.Tokens{{Type: hclsyntax.TokenComment, Bytes: []byte(
					fmt.Sprintf("# This config has keys that look like secrets (%s), written out as they are.\n", strings.Join(keys, ", ")))}})
			}
		}
		isJSON := func(attr string) bool {
			return slices.ContainsFunc(spec.Attrs, func(a framework.Attr) bool { return a.Name == attr && a.JSON })
		}
		vars = append(vars, resourceBlock(b, obj.Type, obj.name, r.schemas["technitium_"+obj.Type], states[obj.Type+"."+obj.name], refs(obj.Type), isJSON)...)
		importBlock(body("imports.tf"), obj.Type, obj.name, obj.ID)
	}
	for _, v := range vars {
		variableBlock(body("variables.tf"), v)
	}
	if len(out) == 0 {
		return errors.New("nothing to export")
	}
	if err := addVersions(o.Out, out, warn); err != nil {
		return err
	}
	return write(o.Out, out, o.Force, prune, o.Log)
}

func readAll(ctx context.Context, objects []object, read func(ctx context.Context, typ, id string) (map[string]tftypes.Value, error), warn func(string, ...any)) ([]object, map[string]map[string]tftypes.Value, bool) {
	states := map[string]map[string]tftypes.Value{}
	var kept []object
	failed := false
	for _, obj := range objects {
		if obj.Data {
			kept = append(kept, obj)
			continue
		}
		state, err := read(ctx, obj.Type, obj.ID)
		switch {
		case err != nil:
			failed = true
			warn("skipping technitium_%s %s: %s", obj.Type, obj.ID, err)
		case state == nil:
			warn("skipping technitium_%s %s: gone", obj.Type, obj.ID)
		default:
			states[obj.Type+"."+obj.name] = state
			kept = append(kept, obj)
		}
	}
	return kept, states, failed
}

// addVersions adds technitium_versions.tf unless the module already has
// required_providers: Terraform allows only one such block per module.
func addVersions(dir string, out map[string]*hclwrite.File, warn func(string, ...any)) error {
	name, err := declaresProviders(dir, out)
	if err != nil {
		return err
	}
	if name != "" {
		warn("%s already has required_providers, so technitium_versions.tf is not written; add technitium = { source = \"ilijamt/technitium\" } there", filepath.Join(dir, name))
		return nil
	}
	out["technitium_versions.tf"] = versions()
	return nil
}

func declaresProviders(dir string, out map[string]*hclwrite.File) (string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	} else if err != nil {
		return "", err
	}
	for _, e := range entries {
		name := e.Name()
		if !e.Type().IsRegular() || out[name] != nil || !strings.HasSuffix(name, ".tf") {
			continue
		}
		src, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return "", err
		}
		if strings.HasPrefix(string(src), marker) || !strings.Contains(string(src), "required_providers") {
			continue
		}
		// A file with errors still yields the blocks that parsed.
		f, _ := hclsyntax.ParseConfig(src, name, hcl.InitialPos)
		body, ok := f.Body.(*hclsyntax.Body)
		if !ok {
			continue
		}
		for _, b := range body.Blocks {
			if b.Type != "terraform" {
				continue
			}
			for _, inner := range b.Body.Blocks {
				if inner.Type == "required_providers" {
					return name, nil
				}
			}
		}
	}
	return "", nil
}

// prune reports whether files an earlier export wrote and this one does not
// are removed: a filtered export, or one where a read failed, writes only
// part of the configuration.
func (o Options) prune(failed bool) bool {
	return o.Force && len(o.Types) == 0 && len(o.Zones) == 0 && !failed
}

// versions names the provider's source, without which Terraform looks for
// hashicorp/technitium. Import blocks need Terraform 1.5.
func versions() *hclwrite.File {
	f := hclwrite.NewEmptyFile()
	tf := f.Body().AppendNewBlock("terraform", nil).Body()
	tf.SetAttributeValue("required_version", cty.StringVal(">= 1.5"))
	tf.AppendNewBlock("required_providers", nil).Body().SetAttributeValue("technitium",
		cty.ObjectVal(map[string]cty.Value{"source": cty.StringVal("ilijamt/technitium")}))
	return f
}

func keptNote(spec framework.Spec) string {
	for _, a := range spec.Attrs {
		switch len(a.Kept) {
		case 0:
		case 1:
			return fmt.Sprintf("# %s always has full rights here: the server adds it, so it is not listed.\n", a.Kept[0])
		default:
			return fmt.Sprintf("# %s always have full rights here: the server adds them, so they are not listed.\n", strings.Join(a.Kept, " and "))
		}
	}
	return ""
}

func permissions(typ string) bool {
	return strings.HasPrefix(typ, "permissions_") || typ == "zone_permissions"
}

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
		case obj.Type == "app":
			index[key{"app", obj.ID}] = traversal("technitium_app", obj.name, "name")
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
			case (strings.HasPrefix(typ, "record_") || typ == "zone_permissions" || typ == "zone_dnssec") && slices.Equal(path, []string{"zone"}):
				kind, s = "zone", strings.ToLower(s)
			case typ == "app_config" && slices.Equal(path, []string{"name"}),
				typ == "record_app" && slices.Equal(path, []string{"app_name"}):
				kind = "app"
			case typ == "dhcp_reserved_lease" && slices.Equal(path, []string{"scope"}):
				kind = "scope"
			case typ == "group_members" && slices.Equal(path, []string{"group"}):
				kind = "group"
			case typ == "group_members" && len(path) == 2 && path[0] == "members":
				kind = "user"
			case permissions(typ) && len(path) == 3 && path[0] == "group_permissions" && path[2] == "name":
				kind = "group"
			case permissions(typ) && len(path) == 3 && path[0] == "user_permissions" && path[2] == "username":
				kind = "user"
			case (typ == "settings_sso" || typ == "settings_ldap") && len(path) == 3 && path[0] == "group_map" && path[2] == "local_group":
				kind = "group"
			default:
				return nil, false
			}
			t, ok := index[key{kind, s}]
			return t, ok
		}
	}
}

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

// marker starts every file Run writes; only a file with it is removed.
const marker = "# Generated by tf-technitium. -force overwrites it, or removes it once nothing is exported to it.\n"

func write(dir string, out map[string]*hclwrite.File, force, prune bool, log io.Writer) error {
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
		if err := os.WriteFile(path, append([]byte(marker), hclwrite.Format(out[name].Bytes())...), 0o644); err != nil {
			return err
		}
		fmt.Fprintf(log, "wrote %s\n", path)
	}
	if !prune {
		return nil
	}
	// A file left from an earlier run, such as the records of a deleted
	// zone, would import objects that are gone.
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		name := e.Name()
		if !e.Type().IsRegular() || out[name] != nil || !strings.HasSuffix(name, ".tf") {
			continue
		}
		path := filepath.Join(dir, name)
		if ok, err := generated(path); err != nil {
			return err
		} else if !ok {
			continue
		}
		if err := os.Remove(path); err != nil {
			return err
		}
		fmt.Fprintf(log, "removed %s\n", path)
	}
	return nil
}

func generated(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	head := make([]byte, len(marker))
	n, _ := io.ReadFull(f, head)
	return string(head[:n]) == marker, nil
}
