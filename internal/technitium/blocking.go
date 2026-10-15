package technitium

import (
	"fmt"

	"github.com/ilijamt/terraform-provider-technitium/internal/framework"
)

// The allowed and blocked zones reject a trailing dot.
const domainName = `^[^.]+(\.[^.]+)*$`

var (
	allowedZone = listedZone("allowed", "Allows a domain name and its subdomains, even when a block list blocks them.")
	blockedZone = listedZone("blocked", "Blocks a domain name and its subdomains, answering as `blocking_type` in `technitium_settings_blocking` says.")
)

// The list call returns the records at a domain, so an entry is found by
// the records named after it.
func listedZone(kind, description string) framework.Spec {
	return framework.Spec{
		Name:        kind + "_zone",
		Description: description + " Blocking must be on (`enable_blocking`).",
		Create:      fmt.Sprintf("/api/%s/add", kind),
		Read:        fmt.Sprintf("/api/%s/list", kind),
		Delete:      fmt.Sprintf("/api/%s/delete", kind),
		List:        "records",
		Attrs: []framework.Attr{
			{Name: "domain", Param: "domain", Type: framework.String, Key: "name", Required: true, Identity: true, Create: true, Pattern: domainName,
				Description: "The domain name, without a trailing dot. Its subdomains are " + kind + " too."},
		},
	}
}
