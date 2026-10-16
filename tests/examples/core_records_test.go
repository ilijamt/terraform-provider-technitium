//go:build integration

package examples

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestIntegration_TXTImportWithSlash(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("txt.test"),
		Steps: []resource.TestStep{
			{
				Config: ReadFixture(t, "txt_import_with_slash", "main.tf"),
				Check:  resource.TestCheckResourceAttr("technitium_record_txt.spf", "text", "v=spf1 ip4:10.0.0.0/8 -all"),
			},
			{
				ResourceName:      "technitium_record_txt.spf",
				ImportState:       true,
				ImportStateId:     `{"zone": "txt.test", "domain": "txt.test", "text": "v=spf1 ip4:10.0.0.0/8 -all"}`,
				ImportStateVerify: true,
			},
		},
	})
}

func TestIntegration_NAPTRAndRPRecords(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("naptr.test"),
		Steps: []resource.TestStep{
			{
				Config: ReadFixture(t, "naptr_and_rp_records", "main.tf"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("technitium_record_naptr.sip", "naptr_regexp", "!^.*$!sip:info@naptr.test!"),
					resource.TestCheckResourceAttr("technitium_record_rp.admin", "mailbox", "admin@naptr.test"),
				),
			},
			{
				Config:           ReadFixture(t, "naptr_and_rp_records", "update.tf"),
				ConfigPlanChecks: expectUpdate("technitium_record_naptr.sip"),
				Check:            resource.TestCheckResourceAttr("technitium_record_naptr.sip", "ttl", "600"),
			},
			{
				ResourceName:      "technitium_record_naptr.sip",
				ImportState:       true,
				ImportStateId:     `{"zone": "naptr.test", "domain": "naptr.test", "naptr_order": 100, "naptr_preference": 10, "naptr_flags": "U", "naptr_services": "E2U+sip", "naptr_regexp": "!^.*$!sip:info@naptr.test!"}`,
				ImportStateVerify: true,
			},
			{
				ResourceName:      "technitium_record_rp.admin",
				ImportState:       true,
				ImportStateId:     "naptr.test/naptr.test/admin@naptr.test/info.naptr.test",
				ImportStateVerify: true,
			},
		},
	})
}

func TestIntegration_SOARecord(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("soa.test"),
		Steps: []resource.TestStep{
			{
				Config: ReadFixture(t, "soa_record", "main.tf"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("technitium_record_soa.test", "refresh", "7200"),
					resource.TestCheckResourceAttr("technitium_record_soa.test", "minimum", "300"),
					resource.TestCheckResourceAttrSet("technitium_record_soa.test", "serial"),
					resource.TestCheckResourceAttrSet("technitium_record_soa.test", "primary_name_server"),
				),
			},
			{
				Config:           ReadFixture(t, "soa_record", "update.tf"),
				ConfigPlanChecks: expectUpdate("technitium_record_soa.test"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("technitium_record_soa.test", "minimum", "600"),
					resource.TestCheckResourceAttr("technitium_record_soa.test", "refresh", "7200"),
				),
			},
			{
				ResourceName:      "technitium_record_soa.test",
				ImportState:       true,
				ImportStateId:     "soa.test/soa.test",
				ImportStateVerify: true,
				// The record added after the last refresh bumped it.
				ImportStateVerifyIgnore: []string{"serial"},
			},
			{
				Config: ReadFixture(t, "soa_record", "zone_only.tf"),
				Check: func(*terraform.State) error {
					for _, rec := range records(t, "soa.test", "soa.test", "SOA") {
						if got := fmt.Sprint(rec["rData"].(map[string]any)["minimum"]); got != "600" {
							return fmt.Errorf("minimum is %s", got)
						}
					}
					return nil
				},
			},
		},
	})
}

// recordTypes are the record resources in testdata/record_types. APP needs
// an installed DNS app, so it is tested on its own.
var recordTypes = []string{
	"aaaa", "aname", "caa", "dname", "ds", "https", "mx", "ns", "ptr", "srv", "sshfp", "svcb", "tlsa", "uri",
}

func TestIntegration_RecordTypes(t *testing.T) {
	var updates []plancheck.PlanCheck
	var imports []resource.TestStep
	for _, name := range recordTypes {
		addr := "technitium_record_" + name + ".test"
		updates = append(updates, plancheck.ExpectResourceAction(addr, plancheck.ResourceActionUpdate))
		imports = append(imports, resource.TestStep{ResourceName: addr, ImportState: true, ImportStateVerify: true})
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("types.test", "2.0.192.in-addr.arpa"),
		Steps: append([]resource.TestStep{
			{Config: ReadFixture(t, "record_types", "main.tf")},
			{
				Config:           ReadFixture(t, "record_types", "update.tf"),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: updates},
				Check:            resource.TestCheckResourceAttr("technitium_record_mx.test", "ttl", "600"),
			},
		}, imports...),
	})
}

func TestIntegration_ServiceBindingSelf(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("self.test"),
		Steps: []resource.TestStep{
			{
				Config: ReadFixture(t, "service_binding_self", "main.tf"),
				Check:  resource.TestCheckResourceAttr("technitium_record_https.self", "svc_target_name", "."),
			},
			{
				Config:           ReadFixture(t, "service_binding_self", "update.tf"),
				ConfigPlanChecks: expectUpdate("technitium_record_https.self"),
			},
			{
				ResourceName:      "technitium_record_https.self",
				ImportState:       true,
				ImportStateId:     `{"zone": "self.test", "domain": "self.test", "svc_priority": 1, "svc_target_name": ".", "svc_params": {}}`,
				ImportStateVerify: true,
			},
		},
	})
}
