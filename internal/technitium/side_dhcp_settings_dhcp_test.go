package technitium_test

import (
	"context"
	"fmt"
	"net/url"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/stretchr/testify/require"

	"github.com/ilijamt/terraform-provider-technitium/internal/client"
	"github.com/ilijamt/terraform-provider-technitium/internal/technitium"
	"github.com/ilijamt/terraform-provider-technitium/internal/testserver"
)

// addresses returns the A addresses at domain, or nil when the zone is gone.
func addresses(c *client.Client, zone, domain string) ([]string, error) {
	resp, err := c.Call(context.Background(), "/api/zones/records/get", url.Values{"zone": {zone}, "domain": {domain}})
	if client.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []string
	for _, r := range resp["records"].([]any) {
		if rec := r.(map[string]any); rec["type"] == "A" {
			out = append(out, rec["rData"].(map[string]any)["ipAddress"].(string))
		}
	}
	slices.Sort(out)
	return out, nil
}

func expectAddresses(zone, domain string, want ...string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		c, err := login()
		if err != nil {
			return err
		}
		got, err := addresses(c, zone, domain)
		if err != nil {
			return err
		}
		if !slices.Equal(got, want) {
			return fmt.Errorf("A at %s is %v, want %v", domain, got, want)
		}
		return nil
	}
}

func zoneTypeIs(zone, typ string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		c, err := login()
		if err != nil {
			return err
		}
		resp, err := c.Call(context.Background(), "/api/zones/options/get", url.Values{"zone": {zone}})
		if err != nil {
			return fmt.Errorf("zone %s: %w", zone, err)
		}
		if resp["type"] != typ {
			return fmt.Errorf("zone %s is %v, want %s", zone, resp["type"], typ)
		}
		return nil
	}
}

func deleteZones(t *testing.T, zones ...string) {
	t.Cleanup(func() {
		c, err := login()
		if err != nil {
			return
		}
		for _, z := range zones {
			_, _ = c.Call(context.Background(), "/api/zones/delete", url.Values{"zone": {z}})
		}
	})
}

// TFTECH-107: a reserved lease creates the zones it needs, so a later
// technitium_zone for them fails.
func TestAccSideDHCPReservedLeaseCreatesZones(t *testing.T) {
	deleteZones(t, "side107.test", "107.168.192.in-addr.arpa")
	config := `
resource "technitium_dhcp_scope" "side" {
  name             = "side107"
  starting_address = "192.168.107.100"
  ending_address   = "192.168.107.200"
  subnet_mask      = "255.255.255.0"
  domain_name      = "side107.test"
  dns_updates      = true
}

resource "technitium_dhcp_reserved_lease" "nas" {
  scope            = technitium_dhcp_scope.side.name
  hardware_address = "00-11-22-00-01-07"
  ip_address       = "192.168.107.10"
  host_name        = "nas"
}
`
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeTestCheckFunc(
					zoneTypeIs("side107.test", "Primary"),
					zoneTypeIs("107.168.192.in-addr.arpa", "Primary"),
					expectAddresses("side107.test", "nas.side107.test", "192.168.107.10"),
				),
			},
			{
				Config:      config + `resource "technitium_zone" "side" { zone = "side107.test" }`,
				ExpectError: regexp.MustCompile(`already exists`),
			},
		},
	})
}

// TFTECH-107: a reserved lease replaces the A set at its name, and removing
// it deletes its address there, whoever added it.
func TestAccSideDHCPReservedLeaseOverwritesRecords(t *testing.T) {
	const base = `
resource "technitium_zone" "fwd" {
  zone = "side117.test"
}

resource "technitium_zone" "rev" {
  zone = "117.168.192.in-addr.arpa"
}

resource "technitium_dhcp_scope" "side" {
  name             = "side117"
  starting_address = "192.168.117.100"
  ending_address   = "192.168.117.200"
  subnet_mask      = "255.255.255.0"
  domain_name      = technitium_zone.fwd.zone
  dns_updates      = true
  dns_ttl          = 3600
  depends_on       = [technitium_zone.rev]
}

resource "technitium_record_a" "nas" {
  zone       = technitium_zone.fwd.zone
  domain     = "nas.side117.test"
  ip_address = "192.168.117.50"
  ttl        = 3600
}

resource "technitium_record_a" "cam" {
  zone       = technitium_zone.fwd.zone
  domain     = "cam.side117.test"
  ip_address = "192.168.117.11"
  ttl        = 3600
}
`
	lease := func(name, ip string) string {
		return fmt.Sprintf(`
resource "technitium_dhcp_reserved_lease" "%[1]s" {
  scope            = technitium_dhcp_scope.side.name
  hardware_address = "00-11-22-00-01-%[2]s"
  ip_address       = "192.168.117.%[2]s"
  host_name        = "%[1]s"
}
`, name, ip)
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("side117.test", "117.168.192.in-addr.arpa"),
		Steps: []resource.TestStep{
			{
				Config: base,
				Check:  expectAddresses("side117.test", "nas.side117.test", "192.168.117.50"),
			},
			{
				// technitium_record_a.nas is gone, so the plan after apply recreates it.
				Config:             base + lease("nas", "10") + lease("cam", "11"),
				Check:              expectAddresses("side117.test", "nas.side117.test", "192.168.117.10"),
				ExpectNonEmptyPlan: true,
			},
			{
				// Removing the cam lease deletes technitium_record_a.cam.
				Config: base + lease("nas", "10"),
				Check: resource.ComposeTestCheckFunc(
					expectAddresses("side117.test", "cam.side117.test"),
					expectAddresses("side117.test", "nas.side117.test", "192.168.117.10", "192.168.117.50"),
				),
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

// TFTECH-115: disabling or deleting a scope leaves the records of its
// reserved leases with a host name; deleting it drops the reservations.
func TestAccSideDHCPScopeReservedLeaseRecords(t *testing.T) {
	config := func(enabled bool) string {
		return fmt.Sprintf(`
resource "technitium_zone" "fwd" {
  zone = "side115.test"
}

resource "technitium_zone" "rev" {
  zone = "115.168.192.in-addr.arpa"
}

resource "technitium_dhcp_scope" "side" {
  name             = "side115"
  starting_address = "192.168.115.100"
  ending_address   = "192.168.115.200"
  subnet_mask      = "255.255.255.0"
  domain_name      = technitium_zone.fwd.zone
  dns_updates      = true
  enabled          = %t
  depends_on       = [technitium_zone.rev]
}

resource "technitium_dhcp_reserved_lease" "nas" {
  scope            = technitium_dhcp_scope.side.name
  hardware_address = "00-11-22-00-01-15"
  ip_address       = "192.168.115.10"
  host_name        = "nas"
}
`, enabled)
	}
	nas := expectAddresses("side115.test", "nas.side115.test", "192.168.115.10")
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("side115.test", "115.168.192.in-addr.arpa"),
		Steps: []resource.TestStep{
			{Config: config(true), Check: nas},
			{Config: config(false), Check: nas},
			{
				PreConfig: func() {
					c := api(t)
					call(t, c, "/api/dhcp/scopes/delete", url.Values{"name": {"side115"}})
					require.NoError(t, nas(nil), "deleting the scope keeps the reservation's records")
				},
				Config: config(false),
				Check:  nas,
			},
		},
	})
}

// dhcpClient sends a DHCP DISCOVER and REQUEST from a container on the
// Docker bridge, as the server image has perl but no DHCP client.
const dhcpClient = `
use strict;
use IO::Socket::INET;
use Socket;
my ($mac, $host) = @ARGV;
my @m = map { hex } split /-/, $mac;
my $xid = pack("N", int(rand(2**31)));
my $s = IO::Socket::INET->new(Proto => 'udp', LocalPort => 68, Broadcast => 1, ReuseAddr => 1) or die "socket: $!";
sub packet {
    my ($type, @opts) = @_;
    my $p = pack("CCCC", 1, 1, 6, 0) . $xid . pack("nn", 0, 0x8000) . ("\0" x 16) . pack("C6", @m) . ("\0" x 202);
    $p .= pack("N", 0x63825363) . pack("CCC", 53, 1, $type) . pack("CC", 12, length $host) . $host;
    return $p . join("", @opts) . pack("C", 255);
}
sub reply {
    my $want = shift;
    for (1 .. 10) {
        my ($buf, $rin) = ('', '');
        vec($rin, fileno($s), 1) = 1;
        next unless select($rin, undef, undef, 1);
        $s->recv($buf, 1500);
        next unless substr($buf, 4, 4) eq $xid && index($buf, pack("CCC", 53, 1, $want)) >= 240;
        my ($sid) = $buf =~ /\x36\x04(.{4})/s;
        return (substr($buf, 16, 4), $sid);
    }
    die "no DHCP reply $want";
}
my $to = sockaddr_in(67, INADDR_BROADCAST);
$s->send(packet(1), 0, $to);
my ($ip, $sid) = reply(2);
$s->send(packet(3, pack("CC", 50, 4) . $ip, pack("CC", 54, 4) . $sid), 0, $to);
reply(5);
print inet_ntoa($ip), "\n";
`

// TFTECH-115: disabling a scope removes the records of its dynamic leases,
// enabling adds them back, and deleting it removes them. It runs on its own
// server, as the scope must be on the Docker network to hand out leases.
func TestAccSideDHCPScopeDynamicLeaseRecords(t *testing.T) {
	acceptance(t)
	if !ownServer {
		t.Skip("needs a test server in Docker; TECHNITIUM_TESTACC_HOST is set")
	}
	host, ip, stop, err := testserver.Run(context.Background())
	require.NoError(t, err)
	t.Cleanup(stop)
	c, err := loginTo(host)
	require.NoError(t, err)

	net := ip[:strings.LastIndex(ip, ".")]
	call(t, c, "/api/dhcp/scopes/set", url.Values{
		"name": {"dyn"}, "startingAddress": {net + ".200"}, "endingAddress": {net + ".210"}, "subnetMask": {"255.255.255.0"},
		"domainName": {"dyn.test"}, "dnsUpdates": {"true"},
	})
	call(t, c, "/api/dhcp/scopes/addReservedLease", url.Values{
		"name": {"dyn"}, "hardwareAddress": {"00-11-22-00-02-01"}, "ipAddress": {net + ".220"}, "hostName": {"nas"},
	})
	cmd := exec.Command("docker", "run", "--interactive", "--rm", "--label", testserver.Label, "--entrypoint", "perl",
		"technitium/dns-server:"+technitium.Version, "-", "00-11-22-00-02-02", "laptop")
	cmd.Stdin = strings.NewReader(dhcpClient)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	leased := strings.TrimSpace(string(out))
	t.Log("leased", leased)

	expect := func(domain string, want ...string) {
		t.Helper()
		got, err := addresses(c, "dyn.test", domain)
		require.NoError(t, err)
		require.Equal(t, want, got, domain)
	}
	expect("laptop.dyn.test", leased)
	expect("nas.dyn.test", net+".220")

	call(t, c, "/api/dhcp/scopes/disable", url.Values{"name": {"dyn"}})
	expect("laptop.dyn.test")
	expect("nas.dyn.test", net+".220")

	call(t, c, "/api/dhcp/scopes/enable", url.Values{"name": {"dyn"}})
	expect("laptop.dyn.test", leased)

	call(t, c, "/api/dhcp/scopes/delete", url.Values{"name": {"dyn"}})
	expect("laptop.dyn.test")
	expect("nas.dyn.test", net+".220")
}
