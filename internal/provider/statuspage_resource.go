package provider

import (
	"context"
	"fmt"
	"regexp"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptimeify/terraform-provider-uptimeify/internal/client"
)

var (
	_ resource.ResourceWithConfigure   = &statusPageResource{}
	_ resource.ResourceWithImportState = &statusPageResource{}

	// The server trims name and description and normalizes the slug (lowercase, [a-z0-9-]);
	// only already-normalized values are accepted so the state always equals the config.
	statusPageTrimmed = stringvalidator.RegexMatches(regexp.MustCompile(`^(\S(.*\S)?)?$`), "must not start or end with whitespace")
	statusPageSlugRe  = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
)

type statusPageResource struct{ client *client.Client }

type monitorRefModel struct {
	Type types.String `tfsdk:"type"`
	ID   types.Int64  `tfsdk:"id"`
}

var monitorRefAttrTypes = map[string]attr.Type{"type": types.StringType, "id": types.Int64Type}

// statusPageModel is shared by the uptimeify_status_page resource and data source.
type statusPageModel struct {
	ID                       types.String `tfsdk:"id"`
	PublicID                 types.String `tfsdk:"public_id"`
	OrganizationID           types.Int64  `tfsdk:"organization_id"`
	CustomerID               types.Int64  `tfsdk:"customer_id"`
	CustomerName             types.String `tfsdk:"customer_name"`
	Name                     types.String `tfsdk:"name"`
	Slug                     types.String `tfsdk:"slug"`
	Description              types.String `tfsdk:"description"`
	Visibility               types.String `tfsdk:"visibility"`
	IsPublished              types.Bool   `tfsdk:"is_published"`
	SubscriptionsEnabled     types.Bool   `tfsdk:"subscriptions_enabled"`
	SubscriptionDelayMinutes types.Int64  `tfsdk:"subscription_delay_minutes"`
	IncludeCustomerMonitors  types.Bool   `tfsdk:"include_customer_monitors"`
	HiddenMonitors           types.Set    `tfsdk:"hidden_monitors"`
	HasPassword              types.Bool   `tfsdk:"has_password"`
	AccessPasswordSetAt      types.String `tfsdk:"access_password_set_at"`
	CreatedAt                types.String `tfsdk:"created_at"`
	UpdatedAt                types.String `tfsdk:"updated_at"`
}

type statusPageResourceModel struct {
	statusPageModel
	AccessPasswordWO        types.String `tfsdk:"access_password_wo"`
	AccessPasswordWOVersion types.Int64  `tfsdk:"access_password_wo_version"`
}

func (m *statusPageModel) fromAPI(p *client.StatusPage) {
	m.ID = types.StringValue(strconv.FormatInt(p.ID, 10))
	m.PublicID = types.StringValue(p.PublicID)
	m.OrganizationID = types.Int64Value(p.OrganizationID)
	m.CustomerID = types.Int64Value(p.CustomerID)
	m.CustomerName = types.StringValue(p.CustomerName)
	m.Name = types.StringValue(p.Name)
	m.Slug = types.StringValue(p.Slug)
	m.Description = types.StringPointerValue(p.Description)
	m.Visibility = types.StringValue(p.Visibility)
	m.IsPublished = types.BoolValue(p.IsPublished)
	m.SubscriptionsEnabled = types.BoolValue(p.SubscriptionsEnabled)
	m.SubscriptionDelayMinutes = types.Int64Value(p.SubscriptionDelayMinutes)
	m.IncludeCustomerMonitors = types.BoolValue(p.IncludeCustomerMonitors)
	elems := []attr.Value{} // never null: the attribute defaults to an empty set
	for _, h := range p.HiddenMonitors {
		elems = append(elems, types.ObjectValueMust(monitorRefAttrTypes, map[string]attr.Value{
			"type": types.StringValue(h.Type), "id": types.Int64Value(h.ID)}))
	}
	m.HiddenMonitors = types.SetValueMust(types.ObjectType{AttrTypes: monitorRefAttrTypes}, elems)
	m.HasPassword = types.BoolValue(p.HasPassword)
	m.AccessPasswordSetAt = types.StringPointerValue(p.AccessPasswordSetAt)
	m.CreatedAt = types.StringValue(p.CreatedAt)
	m.UpdatedAt = types.StringValue(p.UpdatedAt)
}

// hiddenMonitorsToAPI reads a known set (plans at apply time are known).
func hiddenMonitorsToAPI(ctx context.Context, s types.Set) []client.MonitorRef {
	var in []monitorRefModel
	s.ElementsAs(ctx, &in, false)
	out := []client.MonitorRef{}
	for _, h := range in {
		out = append(out, client.MonitorRef{Type: h.Type.ValueString(), ID: h.ID.ValueInt64()})
	}
	return out
}

func NewStatusPageResource() resource.Resource { return &statusPageResource{} }

func (r *statusPageResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_status_page"
}

func (r *statusPageResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	keep := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	resp.Schema = schema.Schema{
		Description: "A status page of one customer. It shows its components (`uptimeify_status_page_component`) and, with " +
			"`include_customer_monitors`, every monitor of the customer except `hidden_monitors`. " +
			"Design, custom domain and IM rules are separate resources (`uptimeify_status_page_design`, " +
			"`uptimeify_status_page_domain`, `uptimeify_im_statuspage_rule`). Deleting the page also deletes its custom domain. " +
			"Import with the numeric id or the public id.",
		Attributes: map[string]schema.Attribute{
			"id":              schema.StringAttribute{Computed: true, PlanModifiers: keep, Description: "Numeric status page id."},
			"public_id":       schema.StringAttribute{Computed: true, PlanModifiers: keep, Description: "Public UUID of the page."},
			"organization_id": schema.Int64Attribute{Computed: true, PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()}},
			"customer_id": schema.Int64Attribute{Required: true, Description: "Customer whose monitors the page shows. Changing it moves the page in place.",
				Validators: []validator.Int64{int64validator.AtLeast(1)}},
			"customer_name": schema.StringAttribute{Computed: true},
			"name": schema.StringAttribute{Required: true, Description: "1 to 120 characters, no surrounding whitespace.",
				Validators: []validator.String{stringvalidator.LengthBetween(1, 120), statusPageTrimmed}},
			"slug": schema.StringAttribute{Optional: true, Computed: true, PlanModifiers: keep,
				Description: "URL slug, lowercase letters, digits and single hyphens, 2 to 120 characters. Unset = derived from " +
					"`name` once at creation (with a numeric suffix on collision). A set slug that is taken fails the create.",
				Validators: []validator.String{stringvalidator.LengthBetween(2, 120),
					stringvalidator.RegexMatches(statusPageSlugRe, "must be lowercase letters, digits and single hyphens")}},
			"description": schema.StringAttribute{Optional: true, Description: "Up to 1000 characters, no surrounding whitespace.",
				Validators: []validator.String{stringvalidator.LengthAtMost(1000), statusPageTrimmed}},
			"visibility": schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString("public"),
				Description: "`public` (default), `customer_members_only` or `password_protected`. `password_protected` needs a " +
					"password: set `access_password_wo`, or keep one set earlier.",
				Validators: []validator.String{stringvalidator.OneOf(client.StatusPageVisibilities...)}},
			"access_password_wo": schema.StringAttribute{Optional: true, WriteOnly: true, Sensitive: true,
				Description: "Access password, 8 to 200 characters. Write-only: sent on create and whenever " +
					"`access_password_wo_version` changes; never stored in state. Removing it does not remove the password.",
				Validators: []validator.String{stringvalidator.LengthBetween(8, 200)}},
			"access_password_wo_version": schema.Int64Attribute{Optional: true, Description: "Change to resend `access_password_wo`."},
			"has_password":               schema.BoolAttribute{Computed: true, Description: "Whether a password is set (it is never returned)."},
			"access_password_set_at":     schema.StringAttribute{Computed: true},
			"is_published": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true),
				Description: "Default `true`."},
			"subscriptions_enabled": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true),
				Description: "Visitors may subscribe to e-mail updates. Default `true`."},
			"subscription_delay_minutes": schema.Int64Attribute{Optional: true, Computed: true, Default: int64default.StaticInt64(5),
				Description: "Flap window before subscribers are mailed, 1 to 120 minutes. Default `5`.",
				Validators:  []validator.Int64{int64validator.Between(1, 120)}},
			"include_customer_monitors": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true),
				Description: "Show every monitor of the customer (minus `hidden_monitors`) after the explicit components " +
					"(`uptimeify_status_page_component`). `false` = only the components. Default `true`."},
			"hidden_monitors": schema.SetNestedAttribute{
				Optional: true, Computed: true,
				Default:     setdefault.StaticValue(types.SetValueMust(types.ObjectType{AttrTypes: monitorRefAttrTypes}, nil)),
				Description: "Monitors of the customer the page does not show, at most 500. Default: none (every monitor is shown).",
				Validators:  []validator.Set{setvalidator.SizeAtMost(500)},
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"type": schema.StringAttribute{Required: true, Description: "Monitor type: `http` (website), `dns`, `icmp`, `smtp`, `ssh`, `tcp`, `ftp` or `imap_pop`.",
						Validators: []validator.String{stringvalidator.OneOf(client.MonitorKinds...)}},
					"id": schema.Int64Attribute{Required: true, Description: "Numeric monitor id.",
						Validators: []validator.Int64{int64validator.AtLeast(1)}},
				}},
			},
			"created_at": schema.StringAttribute{Computed: true, PlanModifiers: keep},
			"updated_at": schema.StringAttribute{Computed: true},
		},
	}
}

func (r *statusPageResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		r.client = c
	}
}

func (r *statusPageResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, cfg statusPageResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := map[string]any{
		"customerId":               plan.CustomerID.ValueInt64(),
		"name":                     plan.Name.ValueString(),
		"visibility":               plan.Visibility.ValueString(),
		"isPublished":              plan.IsPublished.ValueBool(),
		"subscriptionsEnabled":     plan.SubscriptionsEnabled.ValueBool(),
		"subscriptionDelayMinutes": plan.SubscriptionDelayMinutes.ValueInt64(),
		"includeCustomerMonitors":  plan.IncludeCustomerMonitors.ValueBool(),
		"hiddenMonitors":           hiddenMonitorsToAPI(ctx, plan.HiddenMonitors),
	}
	wantSlug := !plan.Slug.IsUnknown() && !plan.Slug.IsNull()
	if wantSlug {
		body["slug"] = plan.Slug.ValueString()
	}
	if !plan.Description.IsNull() {
		body["description"] = plan.Description.ValueString()
	}
	if !cfg.AccessPasswordWO.IsNull() {
		body["accessPassword"] = cfg.AccessPasswordWO.ValueString()
	}
	id, slug, err := r.client.CreateStatusPage(ctx, body)
	if err != nil {
		resp.Diagnostics.AddError("Creating status page failed", err.Error())
		return
	}
	// The API answers a taken slug with a numeric suffix instead of an error.
	if wantSlug && slug != plan.Slug.ValueString() {
		delErr := r.client.DeleteStatusPage(ctx, id)
		resp.Diagnostics.AddError("Status page slug is taken",
			fmt.Sprintf("slug %q is already in use (the API offered %q). Choose another slug or leave it unset. Cleanup of page %s: %v",
				plan.Slug.ValueString(), slug, id, delErr))
		return
	}
	resp.Diagnostics.Append(r.readInto(ctx, id, &plan, &resp.State, "Creating status page failed")...)
}

// readInto reads page id into m (keeping the resource-only attributes) and saves it in st.
func (r *statusPageResource) readInto(ctx context.Context, id string, m *statusPageResourceModel, st *tfsdk.State, what string) (diags diag.Diagnostics) {
	p, err := r.client.GetStatusPage(ctx, id)
	if err != nil {
		diags.AddError(what, err.Error())
		return
	}
	m.fromAPI(p)
	m.AccessPasswordWO = types.StringNull()
	return st.Set(ctx, m)
}

func (r *statusPageResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state statusPageResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	p, err := r.client.GetStatusPage(ctx, state.ID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading status page failed", err.Error())
		return
	}
	state.fromAPI(p)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *statusPageResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state, cfg statusPageResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	patch := map[string]any{}
	if !plan.CustomerID.Equal(state.CustomerID) {
		patch["customerId"] = plan.CustomerID.ValueInt64()
	}
	if !plan.Name.Equal(state.Name) {
		patch["name"] = plan.Name.ValueString()
	}
	if !plan.Slug.IsUnknown() && !plan.Slug.Equal(state.Slug) {
		patch["slug"] = plan.Slug.ValueString()
	}
	if !plan.Description.Equal(state.Description) {
		patch["description"] = plan.Description.ValueStringPointer() // nil clears it
	}
	if !plan.Visibility.Equal(state.Visibility) {
		patch["visibility"] = plan.Visibility.ValueString()
	}
	if !plan.IsPublished.Equal(state.IsPublished) {
		patch["isPublished"] = plan.IsPublished.ValueBool()
	}
	if !plan.SubscriptionsEnabled.Equal(state.SubscriptionsEnabled) {
		patch["subscriptionsEnabled"] = plan.SubscriptionsEnabled.ValueBool()
	}
	if !plan.SubscriptionDelayMinutes.Equal(state.SubscriptionDelayMinutes) {
		patch["subscriptionDelayMinutes"] = plan.SubscriptionDelayMinutes.ValueInt64()
	}
	if !plan.IncludeCustomerMonitors.Equal(state.IncludeCustomerMonitors) {
		patch["includeCustomerMonitors"] = plan.IncludeCustomerMonitors.ValueBool()
	}
	if !plan.HiddenMonitors.Equal(state.HiddenMonitors) {
		patch["hiddenMonitors"] = hiddenMonitorsToAPI(ctx, plan.HiddenMonitors)
	}
	if !cfg.AccessPasswordWO.IsNull() && !plan.AccessPasswordWOVersion.Equal(state.AccessPasswordWOVersion) {
		patch["accessPassword"] = cfg.AccessPasswordWO.ValueString()
	}
	id := state.ID.ValueString()
	if len(patch) > 0 { // the API refuses an empty PATCH
		if err := r.client.UpdateStatusPage(ctx, id, patch); err != nil {
			resp.Diagnostics.AddError("Updating status page failed", err.Error())
			return
		}
	}
	resp.Diagnostics.Append(r.readInto(ctx, id, &plan, &resp.State, "Updating status page failed")...)
}

func (r *statusPageResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state statusPageResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if state.ID.ValueString() == "" { // nothing was created
		return
	}
	if err := r.client.DeleteStatusPage(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Deleting status page failed", err.Error())
	}
}

func (r *statusPageResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
