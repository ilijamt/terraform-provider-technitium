package terraforming

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
)

const (
	jsonProviders = `{"terraform": {"required_providers": {"technitium": {"source": "ilijamt/technitium"}}}}`
	hclProviders  = "terraform {\n  required_providers {\n    technitium = { source = \"ilijamt/technitium\" }\n  }\n}\n"
)

func TestDeclaresProvidersJSON(t *testing.T) {
	for _, name := range []string{"versions.tf.json", "versions.tofu.json"} {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(jsonProviders), 0o644))
		got, err := declaresProviders(dir, map[string]*hclwrite.File{})
		require.NoError(t, err)
		assert.Equal(t, name, got)
	}

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.tf.json"), []byte(`{"terraform": {"required_version": ">= 1.5"}, "locals": {"required_providers": 1}}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bad.tf.json"), []byte(`{"required_providers"`), 0o644))
	got, err := declaresProviders(dir, map[string]*hclwrite.File{})
	require.NoError(t, err)
	assert.Empty(t, got, "required_providers outside a terraform block, or in a broken file, does not count")
}

// export runs the part of Run that writes the files.
func export(dir string, force, prune bool) error {
	out := map[string]*hclwrite.File{"zones.tf": hclwrite.NewEmptyFile()}
	out["zones.tf"].Body().AppendNewBlock("resource", []string{"technitium_zone", "a"}).Body().SetAttributeValue("zone", cty.StringVal("a.test"))
	if err := addVersions(dir, out, force, io.Discard, func(string, ...any) {}); err != nil {
		return err
	}
	return write(dir, out, force, prune, io.Discard)
}

// stale is a directory an earlier export wrote technitium_versions.tf to,
// and where the user has since added required_providers of their own.
func stale(t *testing.T) string {
	dir := t.TempDir()
	require.NoError(t, export(dir, false, false))
	require.FileExists(t, filepath.Join(dir, "technitium_versions.tf"))
	require.NoError(t, os.Remove(filepath.Join(dir, "zones.tf")))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "versions.tf"), []byte(hclProviders), 0o644))
	return dir
}

func TestStaleVersions(t *testing.T) {
	t.Run("no force", func(t *testing.T) {
		dir := stale(t)
		err := export(dir, false, false)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "technitium_versions.tf from an earlier export repeats the required_providers in versions.tf, use -force to remove it")
		assert.FileExists(t, filepath.Join(dir, "technitium_versions.tf"))
		assert.NoFileExists(t, filepath.Join(dir, "zones.tf"), "nothing is written")
	})
	for _, c := range []struct {
		name  string
		prune bool
	}{{"force with a filter", false}, {"force", true}} {
		t.Run(c.name, func(t *testing.T) {
			dir := stale(t)
			require.NoError(t, export(dir, true, c.prune))
			assert.NoFileExists(t, filepath.Join(dir, "technitium_versions.tf"))
			assert.FileExists(t, filepath.Join(dir, "versions.tf"))
		})
	}
	t.Run("own file", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "versions.tf"), []byte(hclProviders), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "technitium_versions.tf"), []byte("# mine\n"), 0o644))
		require.NoError(t, export(dir, true, false))
		assert.FileExists(t, filepath.Join(dir, "technitium_versions.tf"), "a file without the marker is never removed")
	})
}

// TestVersionsValidate runs terraform and tofu validate on an exported
// directory, with the provider built from this tree.
func TestVersionsValidate(t *testing.T) {
	var bins []string
	for _, name := range []string{"terraform", "tofu"} {
		if p, err := exec.LookPath(name); err == nil {
			bins = append(bins, p)
		}
	}
	if len(bins) == 0 {
		t.Skip("neither terraform nor tofu in PATH")
	}
	bin := t.TempDir()
	b, err := exec.Command("go", "build", "-o", filepath.Join(bin, "terraform-provider-technitium"), "../../cmd/provider").CombinedOutput()
	require.NoError(t, err, string(b))
	rc := filepath.Join(bin, "terraformrc")
	require.NoError(t, os.WriteFile(rc, []byte(fmt.Sprintf("provider_installation {\n  dev_overrides {\n    \"ilijamt/technitium\" = %q\n  }\n  direct {}\n}\n", bin)), 0o644))

	validate := func(t *testing.T, dir string) {
		for _, tf := range bins {
			cmd := exec.Command(tf, "validate", "-no-color")
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), "TF_CLI_CONFIG_FILE="+rc)
			b, err := cmd.CombinedOutput()
			assert.NoError(t, err, "%s: %s", tf, b)
		}
	}

	t.Run("json", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "versions.tf.json"), []byte(jsonProviders), 0o644))
		require.NoError(t, export(dir, false, false))
		validate(t, dir)
	})
	t.Run("stale", func(t *testing.T) {
		dir := stale(t)
		require.NoError(t, export(dir, true, false))
		validate(t, dir)
	})
}
