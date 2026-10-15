package technitium

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"

	"github.com/ilijamt/terraform-provider-technitium/internal/framework"
)

// The allowed and blocked zones reject a trailing dot.
const domainName = `^[^.]+(\.[^.]+)*$`

var (
	allowedZone = listedZone("allowed", "Allows a domain name and its subdomains, even when a block list blocks them.")
	blockedZone = listedZone("blocked", "Blocks a domain name and its subdomains, answering as `blocking_type` in `technitium_settings_blocking` says.")
)

// listedNameValidator refuses IP addresses and networks: the server stores
// an IP address under its reverse name, so the entry could not be read back.
type listedNameValidator struct{}

func (listedNameValidator) Description(context.Context) string {
	return "must be a domain name, not an IP address or network"
}

func (v listedNameValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (listedNameValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if hint := reverseZoneHint(req.ConfigValue.ValueString()); hint != "" {
		resp.Diagnostics.AddAttributeError(req.Path, "Domain is an IP address",
			"The server stores an IP address under its reverse name: "+hint+".")
	}
}

// The list call returns the records at a domain, so an entry is found by
// the records named after it. The add call is the Update rather than the
// Create so the framework checks first that the entry does not exist: the
// server accepts an existing entry without an error, and destroying the
// resource would then delete it.
func listedZone(kind, description string) framework.Spec {
	return framework.Spec{
		Name: kind + "_zone",
		Description: description + " Blocking must be on (`enable_blocking`). " +
			"Creating an entry that already exists fails: import it instead.",
		Read:   fmt.Sprintf("/api/%s/list", kind),
		Update: fmt.Sprintf("/api/%s/add", kind),
		Delete: fmt.Sprintf("/api/%s/delete", kind),
		List:   "records",
		Attrs: []framework.Attr{
			{Name: "domain", Param: "domain", Type: framework.String, Key: "name", Required: true, Identity: true, Create: true, Pattern: domainName,
				Validators:  []validator.String{listedNameValidator{}},
				Description: "The domain name, without a trailing dot. Its subdomains are " + kind + " too. An IP address or network is refused, as the server would store it under its reverse name, such as `5.1.168.192.in-addr.arpa` for `192.168.1.5`."},
		},
	}
}
