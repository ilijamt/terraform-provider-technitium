package technitium_test

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ilijamt/terraform-provider-technitium/internal/terraforming"
	"github.com/ilijamt/terraform-provider-technitium/internal/testserver"
)

func TestAccExportValidates(t *testing.T) {
	acceptance(t)
	tf, err := exec.LookPath("terraform")
	if p := os.Getenv("TF_ACC_TERRAFORM_PATH"); p != "" {
		tf, err = p, nil
	}
	if err != nil {
		t.Skip("terraform not in PATH")
	}
	c := api(t)
	call(t, c, "/api/zones/create", url.Values{"zone": {"validate.test"}})
	t.Cleanup(func() {
		_, _ = c.Call(context.Background(), "/api/zones/delete", url.Values{"zone": {"validate.test"}})
	})
	call(t, c, "/api/zones/records/add", url.Values{"zone": {"validate.test"}, "domain": {"validate.test"}, "type": {"TXT"}, "text": {"a/b"}})

	out := t.TempDir()
	require.NoError(t, terraforming.Run(context.Background(), terraforming.Options{
		Host: os.Getenv("TECHNITIUM_HOST"), Username: testserver.Username, Password: testserver.Password,
		Out: out, Version: "test", Zones: []string{"validate.test"}, Types: []string{"zone", "record_*"},
	}))

	bin := t.TempDir()
	build := exec.Command("go", "build", "-o", filepath.Join(bin, "terraform-provider-technitium"), "../../cmd/provider")
	b, err := build.CombinedOutput()
	require.NoError(t, err, string(b))
	rc := filepath.Join(bin, "terraformrc")
	require.NoError(t, os.WriteFile(rc, []byte(fmt.Sprintf("provider_installation {\n  dev_overrides {\n    \"ilijamt/technitium\" = %q\n  }\n  direct {}\n}\n", bin)), 0o644))

	validate := exec.Command(tf, "validate", "-no-color")
	validate.Dir = out
	validate.Env = append(os.Environ(), "TF_CLI_CONFIG_FILE="+rc)
	b, err = validate.CombinedOutput()
	require.NoError(t, err, string(b))
}
