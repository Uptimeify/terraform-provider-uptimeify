package provider

import (
	"context"
	"fmt"
	"regexp"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptimeify/terraform-provider-uptimeify/internal/client"
)

var (
	_ resource.ResourceWithConfigure   = &statusPageDomainResource{}
	_ resource.ResourceWithImportState = &statusPageDomainResource{}
	_ resource.ResourceWithConfigure   = &statusPageDomainActivationResource{}
	_ resource.ResourceWithImportState = &statusPageDomainActivationResource{}

	// The server lowercases the hostname; only the normalized form is accepted.
	statusPageHostnameRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$`)
)

// ---- uptimeify_status_page_domain ---------------------------------------------------------------

type statusPageDomainResource struct{ client *client.Client }

var verificationRecordAttrTypes = map[string]attr.Type{"name": types.StringType, "value": types.StringType}

type statusPageDomainModel struct {
	ID                 types.String `tfsdk:"id"`
	StatusPageID       types.Int64  `tfsdk:"status_page_id"`
	Hostname           types.String `tfsdk:"hostname"`
	Status             types.String `tfsdk:"status"`
	VerifiedAt         types.String `tfsdk:"verified_at"`
	VerificationRecord types.Object `tfsdk:"verification_record"`
	CreatedAt          types.String `tfsdk:"created_at"`
}

func (m *statusPageDomainModel) fromAPI(d *client.StatusPageDomain, dns *client.DomainDNS) {
	m.ID = types.StringValue(strconv.FormatInt(d.ID, 10))
	m.Hostname = types.StringValue(d.Hostname)
	m.Status = types.StringValue(d.Status)
	m.VerifiedAt = types.StringPointerValue(d.VerifiedAt)
	m.VerificationRecord = types.ObjectValueMust(verificationRecordAttrTypes, map[string]attr.Value{
		"name": types.StringValue(dns.TxtName), "value": types.StringValue(dns.TxtValue)})
	m.CreatedAt = types.StringValue(d.CreatedAt)
}

func NewStatusPageDomainResource() resource.Resource { return &statusPageDomainResource{} }

func (r *statusPageDomainResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_status_page_domain"
}

func (r *statusPageDomainResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	keep := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	resp.Schema = schema.Schema{
		Description: "A custom domain of a status page (at most one per page). Creating it registers the hostname as " +
			"`pending`; nothing is verified or activated here. To go live:\n\n" +
			"1. Publish the TXT record `verification_record.name` with the value `verification_record.value` in your DNS zone, " +
			"and point the hostname (CNAME) at your Uptimeify status page host.\n" +
			"2. Verify and activate: `uptimeify_status_page_domain_activation` (runs verify + activate once), or " +
			"`POST /api/status-pages/domains/verify` and `.../activate` with `{\"domainId\": <id>}`.\n\n" +
			"`status` is read from the API: `pending`, `verified` or `active`. Deleting the status page deletes its domain. " +
			"Import with the numeric status page id.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true, PlanModifiers: keep, Description: "Numeric domain id."},
			"status_page_id": schema.Int64Attribute{Required: true, Description: "Numeric status page id. Changing it replaces the domain.",
				PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()},
				Validators:    []validator.Int64{int64validator.AtLeast(1)}},
			"hostname": schema.StringAttribute{Required: true, Description: "Lowercase hostname, e.g. `status.example.com`. Changing it replaces the domain.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
				Validators: []validator.String{stringvalidator.LengthBetween(3, 253),
					stringvalidator.RegexMatches(statusPageHostnameRe, "must be a lowercase hostname with at least one dot")}},
			"status":      schema.StringAttribute{Computed: true, Description: "`pending`, `verified` or `active`."},
			"verified_at": schema.StringAttribute{Computed: true},
			"verification_record": schema.SingleNestedAttribute{Computed: true,
				PlanModifiers: []planmodifier.Object{objectplanmodifier.UseStateForUnknown()},
				Description:   "DNS TXT record that proves control of the hostname.",
				Attributes: map[string]schema.Attribute{
					"name":  schema.StringAttribute{Computed: true, Description: "Name of the TXT record."},
					"value": schema.StringAttribute{Computed: true, Description: "Value of the TXT record."},
				}},
			"created_at": schema.StringAttribute{Computed: true, PlanModifiers: keep},
		},
	}
}

func (r *statusPageDomainResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		r.client = c
	}
}

func (r *statusPageDomainResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan statusPageDomainModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	d, dns, err := r.client.CreateStatusPageDomain(ctx, plan.StatusPageID.ValueInt64(), plan.Hostname.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Creating status page domain failed", err.Error())
		return
	}
	plan.fromAPI(d, dns)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *statusPageDomainResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state statusPageDomainModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id, _ := strconv.ParseInt(state.ID.ValueString(), 10, 64)
	d, dns, err := r.client.GetStatusPageDomain(ctx, id)
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading status page domain failed", err.Error())
		return
	}
	state.fromAPI(d, dns)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is never called with a change: every argument requires replacement.
func (r *statusPageDomainResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan statusPageDomainModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *statusPageDomainResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state statusPageDomainModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id, _ := strconv.ParseInt(state.ID.ValueString(), 10, 64)
	if id <= 0 { // nothing was created
		return
	}
	if err := r.client.DeleteStatusPageDomain(ctx, id); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Deleting status page domain failed", err.Error())
	}
}

// ImportState takes the status page id: GET /api/status-pages/domains/{id} does not name the page,
// the page names its domain.
func (r *statusPageDomainResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	pageID, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil || pageID <= 0 {
		resp.Diagnostics.AddError("Invalid import id", "expected the numeric status page id, got "+strconv.Quote(req.ID))
		return
	}
	p, err := r.client.GetStatusPage(ctx, req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Reading status page failed", err.Error())
		return
	}
	if p.CustomDomain == nil {
		resp.Diagnostics.AddError("Status page has no custom domain", fmt.Sprintf("status page %d has no custom domain to import", pageID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("status_page_id"), pageID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), strconv.FormatInt(p.CustomDomain.ID, 10))...)
}

// ---- uptimeify_status_page_domain_activation ----------------------------------------------------

type statusPageDomainActivationResource struct{ client *client.Client }

type statusPageDomainActivationModel struct {
	ID       types.String `tfsdk:"id"`
	DomainID types.Int64  `tfsdk:"domain_id"`
	Status   types.String `tfsdk:"status"`
}

func NewStatusPageDomainActivationResource() resource.Resource {
	return &statusPageDomainActivationResource{}
}

func (r *statusPageDomainActivationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_status_page_domain_activation"
}

func (r *statusPageDomainActivationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Verifies a status page domain's DNS TXT record and activates the domain, once. Create fails with the " +
			"API's answer when the TXT record is not visible yet; publish it (see `uptimeify_status_page_domain`) and apply again. " +
			"There is no waiting or retrying. Needs an organization admin token. When the domain is no longer `active` " +
			"(e.g. a later re-verification failed), the next plan runs verify + activate again. Destroying this resource " +
			"does not deactivate the domain; destroy the `uptimeify_status_page_domain` for that. Import with the domain id.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
				Description: "Same as `domain_id`."},
			"domain_id": schema.Int64Attribute{Required: true, Description: "`uptimeify_status_page_domain.<name>.id` as number.",
				PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()},
				Validators:    []validator.Int64{int64validator.AtLeast(1)}},
			"status": schema.StringAttribute{Computed: true, Description: "`active` after a successful run."},
		},
	}
}

func (r *statusPageDomainActivationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		r.client = c
	}
}

func (r *statusPageDomainActivationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan statusPageDomainActivationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id := plan.DomainID.ValueInt64()
	if err := r.client.VerifyStatusPageDomain(ctx, id); err != nil {
		hint := ""
		if _, dns, gerr := r.client.GetStatusPageDomain(ctx, id); gerr == nil {
			hint = fmt.Sprintf("\n\nPublish the DNS TXT record %q with the value %q, wait for DNS to propagate, then apply again.", dns.TxtName, dns.TxtValue)
		}
		resp.Diagnostics.AddError("Verifying status page domain failed", err.Error()+hint)
		return
	}
	d, err := r.client.ActivateStatusPageDomain(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError("Activating status page domain failed", "The TXT record was verified, activation failed: "+err.Error())
		return
	}
	plan.ID = types.StringValue(strconv.FormatInt(id, 10))
	plan.Status = types.StringValue(d.Status)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *statusPageDomainActivationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state statusPageDomainActivationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	d, _, err := r.client.GetStatusPageDomain(ctx, state.DomainID.ValueInt64())
	// Gone or no longer active: drop from state so the next apply verifies and activates again.
	if client.IsNotFound(err) || (err == nil && d.Status != "active") {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading status page domain failed", err.Error())
		return
	}
	state.ID = types.StringValue(strconv.FormatInt(d.ID, 10))
	state.Status = types.StringValue(d.Status)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is never called with a change: domain_id requires replacement.
func (r *statusPageDomainActivationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan statusPageDomainActivationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete only forgets the activation; the API has no deactivation.
func (r *statusPageDomainActivationResource) Delete(context.Context, resource.DeleteRequest, *resource.DeleteResponse) {
}

func (r *statusPageDomainActivationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil || id <= 0 {
		resp.Diagnostics.AddError("Invalid import id", "expected the numeric domain id, got "+strconv.Quote(req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("domain_id"), id)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}
