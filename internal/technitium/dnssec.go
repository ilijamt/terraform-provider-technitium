package technitium

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ilijamt/terraform-provider-technitium/internal/client"
	"github.com/ilijamt/terraform-provider-technitium/internal/framework"
)

var (
	dnssecKeyType = types.ObjectType{AttrTypes: map[string]attr.Type{
		"key_tag": types.Int64Type, "key_type": types.StringType, "algorithm": types.StringType,
		"state": types.StringType, "rollover_days": types.Int64Type,
	}}
	dnssecDSType = types.ObjectType{AttrTypes: map[string]attr.Type{
		"key_tag": types.Int64Type, "algorithm": types.StringType, "algorithm_number": types.Int64Type,
		"digest_type": types.StringType, "digest_type_number": types.Int64Type, "digest": types.StringType,
	}}
)

type dnssecKey struct {
	KeyTag       int64  `tfsdk:"key_tag"`
	KeyType      string `tfsdk:"key_type"`
	Algorithm    string `tfsdk:"algorithm"`
	State        string `tfsdk:"state"`
	RolloverDays int64  `tfsdk:"rollover_days"`
}

type dnssecDS struct {
	KeyTag           int64  `tfsdk:"key_tag"`
	Algorithm        string `tfsdk:"algorithm"`
	AlgorithmNumber  int64  `tfsdk:"algorithm_number"`
	DigestType       string `tfsdk:"digest_type"`
	DigestTypeNumber int64  `tfsdk:"digest_type_number"`
	Digest           string `tfsdk:"digest"`
}

type dnssecInfo struct {
	signed                 bool
	algorithm, curve, hash string
	nxProof                string
	iterations, saltLength types.Int64
	dnsKeyTTL, zskRollover types.Int64
	kskSize, zskSize       types.Int64
	status                 string
	keys                   []dnssecKey
	ds                     []dnssecDS
	zskTags                []int64
}

func readDNSSEC(ctx context.Context, api framework.API, zone string) (*dnssecInfo, error) {
	props, err := api.Call(ctx, "/api/zones/dnssec/properties/get", url.Values{"zone": {zone}})
	if client.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	info := &dnssecInfo{status: str(props["dnssecStatus"]), iterations: types.Int64Null(), saltLength: types.Int64Null(), zskRollover: types.Int64Null(),
		kskSize: types.Int64Null(), zskSize: types.Int64Null()}
	switch info.status {
	case "SignedWithNSEC":
		info.signed, info.nxProof = true, "NSEC"
	case "SignedWithNSEC3":
		info.signed, info.nxProof = true, "NSEC3"
		info.iterations, info.saltLength = num(props["nsec3Iterations"]), num(props["nsec3SaltLength"])
	default:
		return info, nil
	}
	info.dnsKeyTTL = num(props["dnsKeyTtl"])
	keys, _ := props["dnssecPrivateKeys"].([]any)
	active := map[string]int{}
	for _, k := range keys {
		m, _ := k.(map[string]any)
		key := dnssecKey{KeyTag: num(m["keyTag"]).ValueInt64(), KeyType: str(m["keyType"]), Algorithm: str(m["algorithm"]),
			State: str(m["state"]), RolloverDays: num(m["rolloverDays"]).ValueInt64()}
		info.keys = append(info.keys, key)
		if info.algorithm == "" {
			info.algorithm, info.curve, info.hash = splitAlgorithm(key.Algorithm)
		}
		if key.KeyType == "ZoneSigningKey" && m["isRetiring"] != true {
			info.zskTags = append(info.zskTags, key.KeyTag)
			info.zskRollover = types.Int64Value(key.RolloverDays)
		}
		// The active key of each type, else the last one listed.
		size := &info.kskSize
		if key.KeyType == "ZoneSigningKey" {
			size = &info.zskSize
		}
		if bits := keySize(key.Algorithm); !bits.IsNull() && (key.State == "Active" || active[key.KeyType] == 0) {
			*size = bits
			if key.State == "Active" {
				active[key.KeyType]++
			}
		}
	}

	view, err := api.Call(ctx, "/api/zones/dnssec/viewDS", url.Values{"zone": {zone}})
	if err != nil {
		return nil, err
	}
	records, _ := view["dsRecords"].([]any)
	for _, r := range records {
		m, _ := r.(map[string]any)
		digests, _ := m["digests"].([]any)
		for _, d := range digests {
			dm, _ := d.(map[string]any)
			info.ds = append(info.ds, dnssecDS{
				KeyTag: num(m["keyTag"]).ValueInt64(), Algorithm: str(m["algorithm"]), AlgorithmNumber: num(m["algorithmNumber"]).ValueInt64(),
				DigestType: str(dm["digestType"]), DigestTypeNumber: num(dm["digestTypeNumber"]).ValueInt64(), Digest: str(dm["digest"]),
			})
		}
	}
	return info, nil
}

func splitAlgorithm(name string) (algorithm, curve, hash string) {
	switch name {
	case "ECDSAP256SHA256":
		return "ECDSA", "P256", ""
	case "ECDSAP384SHA384":
		return "ECDSA", "P384", ""
	case "ED25519", "ED448":
		return "EDDSA", name, ""
	}
	if rest, ok := strings.CutPrefix(name, "RSA"); ok {
		switch {
		case strings.HasPrefix(rest, "MD5"):
			return "RSA", "", "MD5"
		case strings.HasPrefix(rest, "SHA256"):
			return "RSA", "", "SHA256"
		case strings.HasPrefix(rest, "SHA512"):
			return "RSA", "", "SHA512"
		case strings.HasPrefix(rest, "SHA1"):
			return "RSA", "", "SHA1"
		}
	}
	return name, "", ""
}

var rsaBits = regexp.MustCompile(`\((\d+) bits\)$`)

// keySize reads the size of an RSA key from its algorithm, which the server
// writes as "RSASHA256 (2048 bits)"; other algorithms have none.
func keySize(algorithm string) types.Int64 {
	m := rsaBits.FindStringSubmatch(algorithm)
	if m == nil {
		return types.Int64Null()
	}
	return num(m[1])
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func num(v any) types.Int64 {
	s := fmt.Sprint(v)
	if v == nil {
		return types.Int64Null()
	}
	i, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return types.Int64Null()
	}
	return types.Int64Value(i)
}

func optString(s string) types.String {
	if s == "" {
		return types.StringNull()
	}
	return types.StringValue(s)
}

type zoneDNSSEC struct{ api framework.API }

type zoneDNSSECModel struct {
	ID              types.String `tfsdk:"id"`
	Zone            types.String `tfsdk:"zone"`
	Algorithm       types.String `tfsdk:"algorithm"`
	Curve           types.String `tfsdk:"curve"`
	HashAlgorithm   types.String `tfsdk:"hash_algorithm"`
	KSKKeySize      types.Int64  `tfsdk:"ksk_key_size"`
	ZSKKeySize      types.Int64  `tfsdk:"zsk_key_size"`
	NxProof         types.String `tfsdk:"nx_proof"`
	NSEC3Iterations types.Int64  `tfsdk:"nsec3_iterations"`
	NSEC3SaltLength types.Int64  `tfsdk:"nsec3_salt_length"`
	DNSKeyTTL       types.Int64  `tfsdk:"dns_key_ttl"`
	ZSKRolloverDays types.Int64  `tfsdk:"zsk_rollover_days"`
	Status          types.String `tfsdk:"status"`
	Keys            types.List   `tfsdk:"keys"`
	DSRecords       types.List   `tfsdk:"ds_records"`
}

var (
	_ resource.ResourceWithConfigure      = (*zoneDNSSEC)(nil)
	_ resource.ResourceWithImportState    = (*zoneDNSSEC)(nil)
	_ resource.ResourceWithValidateConfig = (*zoneDNSSEC)(nil)
)

func newZoneDNSSEC() resource.Resource { return &zoneDNSSEC{} }

func (r *zoneDNSSEC) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_zone_dnssec"
}

func (r *zoneDNSSEC) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	api, ok := req.ProviderData.(framework.API)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("expected framework.API, got %T", req.ProviderData))
		return
	}
	r.api = api
}

const dnssecReplace = "Changing this unsigns and signs the zone again with new keys, which breaks validation until the parent zone has the new DS records."

func (r *zoneDNSSEC) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	keep := []planmodifier.Int64{int64planmodifier.UseStateForUnknown()}
	keySize := []planmodifier.Int64{int64planmodifier.UseStateForUnknown(), int64planmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		Description: "Signs a primary zone with DNSSEC. The server manages the keys and rolls them over; `ds_records` are what the parent zone needs. " +
			"Destroying the resource unsigns the zone. Replacing it signs the zone with new keys, which breaks validation until the parent zone has the new DS records, " +
			"so consider `prevent_destroy` once the DS records are registered.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true, Description: "The zone name.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"zone": schema.StringAttribute{Required: true, Description: "The primary zone to sign.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"algorithm": schema.StringAttribute{Required: true, Description: "The signing algorithm: `RSA`, `ECDSA` or `EDDSA`. " + dnssecReplace,
				Validators:    []validator.String{stringvalidator.OneOf("RSA", "ECDSA", "EDDSA")},
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"curve": schema.StringAttribute{Optional: true, Computed: true,
				Description:   "The curve, for `ECDSA` (`P256` or `P384`) and `EDDSA` (`ED25519` or `ED448`). " + dnssecReplace,
				Validators:    []validator.String{stringvalidator.OneOf("P256", "P384", "ED25519", "ED448")},
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown(), stringplanmodifier.RequiresReplace()}},
			"hash_algorithm": schema.StringAttribute{Optional: true, Computed: true,
				Description:   "The hash, for `RSA`: `MD5`, `SHA1`, `SHA256` or `SHA512`. " + dnssecReplace,
				Validators:    []validator.String{stringvalidator.OneOf("MD5", "SHA1", "SHA256", "SHA512")},
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown(), stringplanmodifier.RequiresReplace()}},
			"ksk_key_size": schema.Int64Attribute{Optional: true, Computed: true,
				Description:   "The key signing key size in bits, for `RSA`, read from the active key signing key. Null for other algorithms. " + dnssecReplace,
				PlanModifiers: keySize},
			"zsk_key_size": schema.Int64Attribute{Optional: true, Computed: true,
				Description:   "The zone signing key size in bits, for `RSA`, read from the active zone signing key. Null for other algorithms. " + dnssecReplace,
				PlanModifiers: keySize},
			"nx_proof": schema.StringAttribute{Optional: true, Computed: true,
				Description:   "How non-existence is proven: `NSEC` (Technitium's default) or `NSEC3`. Changes in place.",
				Validators:    []validator.String{stringvalidator.OneOf("NSEC", "NSEC3")},
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			// Not kept from state: switching to NSEC clears them.
			"nsec3_iterations": schema.Int64Attribute{Optional: true, Computed: true,
				Description: "The NSEC3 hash iterations, with `nx_proof` `NSEC3`. Technitium's default is `0`. Changes in place."},
			"nsec3_salt_length": schema.Int64Attribute{Optional: true, Computed: true,
				Description: "The NSEC3 salt length in bytes, with `nx_proof` `NSEC3`. Technitium's default is `0`. Changes in place."},
			"dns_key_ttl": schema.Int64Attribute{Optional: true, Computed: true, PlanModifiers: keep,
				Description: "The TTL of the DNSKEY records. Technitium's default is `86400`. Changes in place, but the server refuses while a key is still being published, for some hours after signing."},
			"zsk_rollover_days": schema.Int64Attribute{Optional: true, Computed: true, PlanModifiers: keep,
				Description: "The days after which the server rolls the zone signing key over; `0` turns it off. Technitium's default is `30`. Changes in place."},
			"status": schema.StringAttribute{Computed: true, Description: "The signing status: `SignedWithNSEC` or `SignedWithNSEC3`."},
			"keys": schema.ListNestedAttribute{Computed: true, Description: "The zone's keys. The server rolls them over on its own.",
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"key_tag":       schema.Int64Attribute{Computed: true, Description: "The key tag."},
					"key_type":      schema.StringAttribute{Computed: true, Description: "`KeySigningKey` or `ZoneSigningKey`."},
					"algorithm":     schema.StringAttribute{Computed: true, Description: "The DNSKEY algorithm, for example `ECDSAP256SHA256`."},
					"state":         schema.StringAttribute{Computed: true, Description: "The key state, for example `Published`, `Ready` or `Active`."},
					"rollover_days": schema.Int64Attribute{Computed: true, Description: "The days between rollovers; `0` is never."},
				}}},
			"ds_records": schema.ListNestedAttribute{Computed: true, Description: "The DS records to add at the parent zone, one per key signing key and digest.",
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"key_tag":            schema.Int64Attribute{Computed: true, Description: "The key tag."},
					"algorithm":          schema.StringAttribute{Computed: true, Description: "The algorithm name."},
					"algorithm_number":   schema.Int64Attribute{Computed: true, Description: "The algorithm number."},
					"digest_type":        schema.StringAttribute{Computed: true, Description: "The digest type, for example `SHA256`."},
					"digest_type_number": schema.Int64Attribute{Computed: true, Description: "The digest type number."},
					"digest":             schema.StringAttribute{Computed: true, Description: "The digest as a hex string."},
				}}},
		},
	}
}

// ValidateConfig refuses a key size for an algorithm without one, which
// would read back as null and replace the zone's keys on every apply.
func (r *zoneDNSSEC) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var m zoneDNSSECModel
	if resp.Diagnostics.Append(req.Config.Get(ctx, &m)...); resp.Diagnostics.HasError() {
		return
	}
	if m.Algorithm.IsUnknown() || m.Algorithm.IsNull() || m.Algorithm.ValueString() == "RSA" {
		return
	}
	for name, v := range map[string]types.Int64{"ksk_key_size": m.KSKKeySize, "zsk_key_size": m.ZSKKeySize} {
		if !v.IsNull() && !v.IsUnknown() {
			resp.Diagnostics.AddAttributeError(path.Root(name), "Invalid key size", name+" only applies to RSA.")
		}
	}
}

func (r *zoneDNSSEC) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan zoneDNSSECModel
	if resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...); resp.Diagnostics.HasError() {
		return
	}
	params := url.Values{"zone": {plan.Zone.ValueString()}, "algorithm": {plan.Algorithm.ValueString()}}
	setString(params, "curve", plan.Curve)
	setString(params, "hashAlgorithm", plan.HashAlgorithm)
	setString(params, "nxProof", plan.NxProof)
	setInt(params, "kskKeySize", plan.KSKKeySize)
	setInt(params, "zskKeySize", plan.ZSKKeySize)
	setInt(params, "iterations", plan.NSEC3Iterations)
	setInt(params, "saltLength", plan.NSEC3SaltLength)
	setInt(params, "dnsKeyTtl", plan.DNSKeyTTL)
	setInt(params, "zskRolloverDays", plan.ZSKRolloverDays)
	if _, err := r.api.Call(ctx, "/api/zones/dnssec/sign", params); err != nil {
		resp.Diagnostics.AddError("Sign zone", err.Error())
		return
	}
	r.refresh(ctx, &plan, &resp.State, &resp.Diagnostics)
}

func (r *zoneDNSSEC) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state zoneDNSSECModel
	if resp.Diagnostics.Append(req.State.Get(ctx, &state)...); resp.Diagnostics.HasError() {
		return
	}
	info, err := readDNSSEC(ctx, r.api, state.Zone.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Read zone DNSSEC", err.Error())
		return
	}
	if info == nil || !info.signed {
		resp.State.RemoveResource(ctx)
		return
	}
	resp.Diagnostics.Append(state.fill(ctx, info)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *zoneDNSSEC) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, prior zoneDNSSECModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	zone := url.Values{"zone": {plan.Zone.ValueString()}}
	call := func(path string, extra url.Values) bool {
		params := url.Values{}
		for k, v := range zone {
			params[k] = v
		}
		for k, v := range extra {
			params[k] = v
		}
		if _, err := r.api.Call(ctx, path, params); err != nil {
			resp.Diagnostics.AddError("Update zone DNSSEC", err.Error())
			return false
		}
		return true
	}
	changed := func(a, b attr.Value) bool { return !a.IsUnknown() && !a.IsNull() && !a.Equal(b) }

	if changed(plan.NxProof, prior.NxProof) {
		path := "/api/zones/dnssec/properties/convertToNSEC"
		if plan.NxProof.ValueString() == "NSEC3" {
			path = "/api/zones/dnssec/properties/convertToNSEC3"
		}
		if !call(path, nil) {
			return
		}
	}
	if plan.NxProof.ValueString() == "NSEC3" && (changed(plan.NSEC3Iterations, prior.NSEC3Iterations) || changed(plan.NSEC3SaltLength, prior.NSEC3SaltLength)) {
		// The server resets a parameter left out, so one not configured keeps its value.
		params := url.Values{}
		setInt(params, "iterations", known(plan.NSEC3Iterations, prior.NSEC3Iterations))
		setInt(params, "saltLength", known(plan.NSEC3SaltLength, prior.NSEC3SaltLength))
		if !call("/api/zones/dnssec/properties/updateNSEC3Params", params) {
			return
		}
	}
	if changed(plan.DNSKeyTTL, prior.DNSKeyTTL) {
		if !call("/api/zones/dnssec/properties/updateDnsKeyTtl", url.Values{"ttl": {strconv.FormatInt(plan.DNSKeyTTL.ValueInt64(), 10)}}) {
			return
		}
	}
	if changed(plan.ZSKRolloverDays, prior.ZSKRolloverDays) {
		info, err := readDNSSEC(ctx, r.api, plan.Zone.ValueString())
		if err != nil || info == nil {
			resp.Diagnostics.AddError("Update zone DNSSEC", fmt.Sprintf("read keys: %v", err))
			return
		}
		for _, tag := range info.zskTags {
			if !call("/api/zones/dnssec/properties/updatePrivateKey", url.Values{
				"keyTag": {strconv.FormatInt(tag, 10)}, "rolloverDays": {strconv.FormatInt(plan.ZSKRolloverDays.ValueInt64(), 10)},
			}) {
				return
			}
		}
	}
	r.refresh(ctx, &plan, &resp.State, &resp.Diagnostics)
}

func (r *zoneDNSSEC) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state zoneDNSSECModel
	if resp.Diagnostics.Append(req.State.Get(ctx, &state)...); resp.Diagnostics.HasError() {
		return
	}
	if _, err := r.api.Call(ctx, "/api/zones/dnssec/unsign", url.Values{"zone": {state.Zone.ValueString()}}); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Unsign zone", err.Error())
	}
}

func (r *zoneDNSSEC) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("zone"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

func (r *zoneDNSSEC) refresh(ctx context.Context, m *zoneDNSSECModel, state *tfsdk.State, diags *diag.Diagnostics) {
	info, err := readDNSSEC(ctx, r.api, m.Zone.ValueString())
	if err == nil && (info == nil || !info.signed) {
		err = fmt.Errorf("zone %s is not signed", m.Zone.ValueString())
	}
	if err != nil {
		diags.AddError("Read zone DNSSEC", err.Error())
		return
	}
	diags.Append(m.fill(ctx, info)...)
	diags.Append(state.Set(ctx, m)...)
}

func (m *zoneDNSSECModel) fill(ctx context.Context, info *dnssecInfo) diag.Diagnostics {
	m.ID = m.Zone
	m.Algorithm = types.StringValue(info.algorithm)
	m.Curve = optString(info.curve)
	m.HashAlgorithm = optString(info.hash)
	m.KSKKeySize, m.ZSKKeySize = info.kskSize, info.zskSize
	m.NxProof = types.StringValue(info.nxProof)
	m.NSEC3Iterations, m.NSEC3SaltLength = info.iterations, info.saltLength
	m.DNSKeyTTL, m.ZSKRolloverDays = info.dnsKeyTTL, info.zskRollover
	m.Status = types.StringValue(info.status)
	var diags diag.Diagnostics
	var d diag.Diagnostics
	m.Keys, d = types.ListValueFrom(ctx, dnssecKeyType, info.keys)
	diags.Append(d...)
	m.DSRecords, d = types.ListValueFrom(ctx, dnssecDSType, info.ds)
	diags.Append(d...)
	return diags
}

func setString(p url.Values, name string, v types.String) {
	if !v.IsNull() && !v.IsUnknown() {
		p.Set(name, v.ValueString())
	}
}

func known(v, fallback types.Int64) types.Int64 {
	if v.IsUnknown() {
		return fallback
	}
	return v
}

func setInt(p url.Values, name string, v types.Int64) {
	if !v.IsNull() && !v.IsUnknown() {
		p.Set(name, strconv.FormatInt(v.ValueInt64(), 10))
	}
}

type zoneDNSSECData struct{ api framework.API }

type zoneDNSSECDataModel struct {
	Zone            types.String `tfsdk:"zone"`
	Signed          types.Bool   `tfsdk:"signed"`
	Algorithm       types.String `tfsdk:"algorithm"`
	Curve           types.String `tfsdk:"curve"`
	HashAlgorithm   types.String `tfsdk:"hash_algorithm"`
	NxProof         types.String `tfsdk:"nx_proof"`
	NSEC3Iterations types.Int64  `tfsdk:"nsec3_iterations"`
	NSEC3SaltLength types.Int64  `tfsdk:"nsec3_salt_length"`
	DNSKeyTTL       types.Int64  `tfsdk:"dns_key_ttl"`
	ZSKRolloverDays types.Int64  `tfsdk:"zsk_rollover_days"`
	Status          types.String `tfsdk:"status"`
	Keys            types.List   `tfsdk:"keys"`
	DSRecords       types.List   `tfsdk:"ds_records"`
}

func newZoneDNSSECData() datasource.DataSource { return &zoneDNSSECData{} }

func (d *zoneDNSSECData) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_zone_dnssec"
}

func (d *zoneDNSSECData) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	api, ok := req.ProviderData.(framework.API)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("expected framework.API, got %T", req.ProviderData))
		return
	}
	d.api = api
}

func (d *zoneDNSSECData) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	str := func(desc string) dschema.Attribute { return dschema.StringAttribute{Computed: true, Description: desc} }
	num := func(desc string) dschema.Attribute { return dschema.Int64Attribute{Computed: true, Description: desc} }
	resp.Schema = dschema.Schema{
		Description: "Reads the DNSSEC signing of a zone, for example its DS records for the parent zone.",
		Attributes: map[string]dschema.Attribute{
			"zone":              dschema.StringAttribute{Required: true, Description: "The zone."},
			"signed":            dschema.BoolAttribute{Computed: true, Description: "Whether the zone is signed."},
			"algorithm":         str("The signing algorithm: `RSA`, `ECDSA` or `EDDSA`."),
			"curve":             str("The curve, for `ECDSA` and `EDDSA`."),
			"hash_algorithm":    str("The hash, for `RSA`."),
			"nx_proof":          str("`NSEC` or `NSEC3`."),
			"nsec3_iterations":  num("The NSEC3 hash iterations."),
			"nsec3_salt_length": num("The NSEC3 salt length in bytes."),
			"dns_key_ttl":       num("The TTL of the DNSKEY records."),
			"zsk_rollover_days": num("The days between zone signing key rollovers."),
			"status":            str("The signing status: `Unsigned`, `SignedWithNSEC` or `SignedWithNSEC3`."),
			"keys": dschema.ListNestedAttribute{Computed: true, Description: "The zone's keys.",
				NestedObject: dschema.NestedAttributeObject{Attributes: map[string]dschema.Attribute{
					"key_tag": num("The key tag."), "key_type": str("`KeySigningKey` or `ZoneSigningKey`."),
					"algorithm": str("The DNSKEY algorithm."), "state": str("The key state."), "rollover_days": num("The days between rollovers."),
				}}},
			"ds_records": dschema.ListNestedAttribute{Computed: true, Description: "The DS records for the parent zone.",
				NestedObject: dschema.NestedAttributeObject{Attributes: map[string]dschema.Attribute{
					"key_tag": num("The key tag."), "algorithm": str("The algorithm name."), "algorithm_number": num("The algorithm number."),
					"digest_type": str("The digest type."), "digest_type_number": num("The digest type number."), "digest": str("The digest."),
				}}},
		},
	}
}

func (d *zoneDNSSECData) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var m zoneDNSSECDataModel
	if resp.Diagnostics.Append(req.Config.Get(ctx, &m)...); resp.Diagnostics.HasError() {
		return
	}
	info, err := readDNSSEC(ctx, d.api, m.Zone.ValueString())
	if err == nil && info == nil {
		err = fmt.Errorf("zone %s not found", m.Zone.ValueString())
	}
	if err != nil {
		resp.Diagnostics.AddError("Read zone DNSSEC", err.Error())
		return
	}
	var r zoneDNSSECModel
	r.Zone = m.Zone
	resp.Diagnostics.Append(r.fill(ctx, info)...)
	m.Signed = types.BoolValue(info.signed)
	m.Algorithm, m.Curve, m.HashAlgorithm, m.NxProof = optString(info.algorithm), r.Curve, r.HashAlgorithm, optString(info.nxProof)
	m.NSEC3Iterations, m.NSEC3SaltLength, m.DNSKeyTTL, m.ZSKRolloverDays = r.NSEC3Iterations, r.NSEC3SaltLength, r.DNSKeyTTL, r.ZSKRolloverDays
	m.Status, m.Keys, m.DSRecords = r.Status, r.Keys, r.DSRecords
	if !info.signed {
		m.DNSKeyTTL = types.Int64Null()
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}
