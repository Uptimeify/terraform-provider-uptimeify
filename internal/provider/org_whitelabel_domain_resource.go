package provider

import (
	"context"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptimeify/terraform-provider-uptimeify/internal/client"
)

var (
	_ resource.ResourceWithConfigure   = &whitelabelDomainResource{}
	_ resource.ResourceWithImportState = &whitelabelDomainResource{}
)

type whitelabelDomainResource struct{ client *client.Client }

type whitelabelDomainModel struct {
	ID                types.String `tfsdk:"id"`
	Hostname          types.String `tfsdk:"hostname"`
	Status            types.String `tfsdk:"status"`
	IsPrimary         types.Bool   `tfsdk:"is_primary"`
	VerifiedAt        types.String `tfsdk:"verified_at"`
	CreatedAt         types.String `tfsdk:"created_at"`
	DNSTxtName        types.String `tfsdk:"dns_txt_name"`
	DNSTxtValue       types.String `tfsdk:"dns_txt_value"`
	LastVerifyError   types.String `tfsdk:"last_verification_error"`
	LastVerifyErrorAt types.String `tfsdk:"last_verification_error_at"`
}

func (m *whitelabelDomainModel) fromAPI(d *client.WhitelabelDomain) {
	m.ID = types.StringValue(strconv.FormatInt(d.Domain.ID, 10))
	// The server stores the hostname lowercased; keep the configured spelling.
	if !strings.EqualFold(m.Hostname.ValueString(), d.Domain.Hostname) {
		m.Hostname = types.StringValue(d.Domain.Hostname)
	}
	m.Status = types.StringValue(d.Domain.Status)
	m.IsPrimary = types.BoolValue(d.Domain.IsPrimary)
	m.VerifiedAt = types.StringPointerValue(d.Domain.VerifiedAt)
	m.CreatedAt = types.StringValue(d.Domain.CreatedAt)
	m.DNSTxtName = types.StringValue(d.DNS.TxtName)
	m.DNSTxtValue = types.StringValue(d.DNS.TxtValue)
	m.LastVerifyError = rawJSONOrNull(d.Domain.LastVerificationError)
	m.LastVerifyErrorAt = types.StringPointerValue(d.Domain.LastVerificationErrorAt)
}

func NewWhitelabelDomainResource() resource.Resource { return &whitelabelDomainResource{} }

func (r *whitelabelDomainResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_whitelabel_domain"
}

func (r *whitelabelDomainResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	keep := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	resp.Schema = schema.Schema{
		Description: "A custom app domain (white label) of the organization. Terraform creates it as `pending` and outputs " +
			"the DNS TXT record that proves control. Verifying and activating are one-off actions, not managed here: " +
			"`POST /api/organization/whitelabel/domains/verify` with `{ \"domainId\": <id> }` once the record is live, then " +
			"`POST /api/organization/whitelabel/domains/activate` (optionally `makePrimary`). Organization admins only. " +
			"Import with the numeric id.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true, PlanModifiers: keep, Description: "Numeric domain id (`domainId`)."},
			"hostname": schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
				Validators:  []validator.String{stringvalidator.LengthBetween(3, 253)},
				Description: "Hostname, e.g. `status.example.com`. No wildcards, no platform hostnames."},
			"status":                     schema.StringAttribute{Computed: true, Description: "`pending`, `verified` or `active`."},
			"is_primary":                 schema.BoolAttribute{Computed: true, Description: "The first app domain becomes primary."},
			"verified_at":                schema.StringAttribute{Computed: true},
			"created_at":                 schema.StringAttribute{Computed: true, PlanModifiers: keep},
			"dns_txt_name":               schema.StringAttribute{Computed: true, PlanModifiers: keep, Description: "Name of the TXT record to publish."},
			"dns_txt_value":              schema.StringAttribute{Computed: true, PlanModifiers: keep, Description: "Value of the TXT record."},
			"last_verification_error":    lastVerificationErrorAttr(),
			"last_verification_error_at": lastVerificationErrorAtAttr(),
		},
	}
}

func (r *whitelabelDomainResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		r.client = c
	}
}

func (r *whitelabelDomainResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan whitelabelDomainModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	d, err := r.client.CreateWhitelabelDomain(ctx, plan.Hostname.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Creating white-label domain failed", err.Error())
		return
	}
	plan.fromAPI(d)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *whitelabelDomainResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state whitelabelDomainModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	d, err := r.client.GetWhitelabelDomain(ctx, state.ID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading white-label domain failed", err.Error())
		return
	}
	state.fromAPI(d)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is never reached: hostname forces replacement, everything else is computed.
func (r *whitelabelDomainResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan whitelabelDomainModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *whitelabelDomainResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state whitelabelDomainModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || state.ID.ValueString() == "" {
		return
	}
	if err := r.client.DeleteWhitelabelDomain(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Deleting white-label domain failed", err.Error())
	}
}

func (r *whitelabelDomainResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
