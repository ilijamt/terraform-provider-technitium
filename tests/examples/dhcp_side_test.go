//go:build integration

package examples

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

// dhcpAddresses returns the A addresses at domain, or nil when the zone is gone.
func dhcpAddresses(c *client.Client, zone, domain string) ([]string, error) {
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

func expectDHCPAddresses(zone, domain string, want ...string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		got, err := dhcpAddresses(api(), zone, domain)
		if err != nil {
			return err
		}
		if !slices.Equal(got, want) {
			return fmt.Errorf("A at %s is %v, want %v", domain, got, want)
		}
		return nil
	}
}

func dhcpZoneTypeIs(zone, typ string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		resp, err := api().Call(context.Background(), "/api/zones/options/get", url.Values{"zone": {zone}})
		if err != nil {
			return fmt.Errorf("zone %s: %w", zone, err)
		}
		if resp["type"] != typ {
			return fmt.Errorf("zone %s is %v, want %s", zone, resp["type"], typ)
		}
		return nil
	}
}

func dhcpDeleteZones(t *testing.T, zones ...string) {
	t.Cleanup(func() {
		for _, z := range zones {
			_, _ = api().Call(context.Background(), "/api/zones/delete", url.Values{"zone": {z}})
		}
	})
}

// TFTECH-107: a reserved lease creates the zones it needs, so a later
// technitium_zone for them fails.
func TestIntegration_SideDHCPReservedLeaseCreatesZones(t *testing.T) {
	dhcpDeleteZones(t, "side107.test", "107.168.192.in-addr.arpa")
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: ReadFixture(t, "side_dhcp_reserved_lease_creates_zones", "main.tf"),
				Check: resource.ComposeTestCheckFunc(
					dhcpZoneTypeIs("side107.test", "Primary"),
					dhcpZoneTypeIs("107.168.192.in-addr.arpa", "Primary"),
					expectDHCPAddresses("side107.test", "nas.side107.test", "192.168.107.10"),
				),
			},
			{
				Config:      ReadFixture(t, "side_dhcp_reserved_lease_creates_zones", "zone.tf"),
				ExpectError: regexp.MustCompile(`already exists`),
			},
		},
	})
}

// TFTECH-107: a reserved lease replaces the A set at its name, and removing
// it deletes its address there, whoever added it.
func TestIntegration_SideDHCPReservedLeaseOverwritesRecords(t *testing.T) {
	const name = "side_dhcp_reserved_lease_overwrites_records"
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("side117.test", "117.168.192.in-addr.arpa"),
		Steps: []resource.TestStep{
			{
				Config: ReadFixture(t, name, "main.tf"),
				Check:  expectDHCPAddresses("side117.test", "nas.side117.test", "192.168.117.50"),
			},
			{
				// technitium_record_a.nas is gone, so the plan after apply recreates it.
				Config:             ReadFixture(t, name, "leases.tf"),
				Check:              expectDHCPAddresses("side117.test", "nas.side117.test", "192.168.117.10"),
				ExpectNonEmptyPlan: true,
			},
			{
				// Removing the cam lease deletes technitium_record_a.cam.
				Config: ReadFixture(t, name, "remove_cam.tf"),
				Check: resource.ComposeTestCheckFunc(
					expectDHCPAddresses("side117.test", "cam.side117.test"),
					expectDHCPAddresses("side117.test", "nas.side117.test", "192.168.117.10", "192.168.117.50"),
				),
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

// TFTECH-115: disabling or deleting a scope leaves the records of its
// reserved leases with a host name; deleting it drops the reservations.
func TestIntegration_SideDHCPScopeReservedLeaseRecords(t *testing.T) {
	const name = "side_dhcp_scope_reserved_lease_records"
	nas := expectDHCPAddresses("side115.test", "nas.side115.test", "192.168.115.10")
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("side115.test", "115.168.192.in-addr.arpa"),
		Steps: []resource.TestStep{
			{Config: ReadFixture(t, name, "main.tf"), Check: nas},
			{Config: ReadFixture(t, name, "disabled.tf"), Check: nas},
			{
				PreConfig: func() {
					call(t, "/api/dhcp/scopes/delete", url.Values{"name": {"side115"}})
					require.NoError(t, nas(nil), "deleting the scope keeps the reservation's records")
				},
				Config: ReadFixture(t, name, "disabled.tf"),
				Check:  nas,
			},
		},
	})
}

// dhcpClientScript sends a DHCP DISCOVER and REQUEST from a container on the
// Docker bridge, as the server image has perl but no DHCP client.
const dhcpClientScript = `
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
func TestIntegration_SideDHCPScopeDynamicLeaseRecords(t *testing.T) {
	_, ip, c := ownTestServerWith(t)

	net := ip[:strings.LastIndex(ip, ".")]
	callWith(t, c, "/api/dhcp/scopes/set", url.Values{
		"name": {"dyn"}, "startingAddress": {net + ".200"}, "endingAddress": {net + ".210"}, "subnetMask": {"255.255.255.0"},
		"domainName": {"dyn.test"}, "dnsUpdates": {"true"},
	})
	callWith(t, c, "/api/dhcp/scopes/addReservedLease", url.Values{
		"name": {"dyn"}, "hardwareAddress": {"00-11-22-00-02-01"}, "ipAddress": {net + ".220"}, "hostName": {"nas"},
	})
	cmd := exec.Command("docker", "run", "--interactive", "--rm", "--label", testserver.Label, "--entrypoint", "perl",
		"technitium/dns-server:"+technitium.Version, "-", "00-11-22-00-02-02", "laptop")
	cmd.Stdin = strings.NewReader(dhcpClientScript)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	leased := strings.TrimSpace(string(out))
	t.Log("leased", leased)

	expect := func(domain string, want ...string) {
		t.Helper()
		got, err := dhcpAddresses(c, "dyn.test", domain)
		require.NoError(t, err)
		require.Equal(t, want, got, domain)
	}
	expect("laptop.dyn.test", leased)
	expect("nas.dyn.test", net+".220")

	callWith(t, c, "/api/dhcp/scopes/disable", url.Values{"name": {"dyn"}})
	expect("laptop.dyn.test")
	expect("nas.dyn.test", net+".220")

	callWith(t, c, "/api/dhcp/scopes/enable", url.Values{"name": {"dyn"}})
	expect("laptop.dyn.test", leased)

	callWith(t, c, "/api/dhcp/scopes/delete", url.Values{"name": {"dyn"}})
	expect("laptop.dyn.test")
	expect("nas.dyn.test", net+".220")
}
