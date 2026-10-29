package provider

import (
	"context"
	"regexp"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptimeify/terraform-provider-uptimeify/internal/client"
)

// The audience groups of a page are their own resource, not an attribute of uptimeify_status_page:
// they name components, and components need the page first. On the page the reference would be a
// dependency cycle.

var (
	_ resource.ResourceWithConfigure   = &statusPageAudienceGroupsResource{}
	_ resource.ResourceWithImportState = &statusPageAudienceGroupsResource{}

	// What the API stores unchanged (normalizeSsoDomain: trimmed, lowercase, no trailing dot).
	audienceDomainRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$`)
)

type statusPageAudienceGroupsResource struct{ client *client.Client }

type audienceGroupsModel struct {
	ID           types.String `tfsdk:"id"`
	StatusPageID types.Int64  `tfsdk:"status_page_id"`
	Groups       types.List   `tfsdk:"groups"`
}

var audienceGroupAttrTypes = map[string]attr.Type{
	"name":          types.StringType,
	"email_domains": types.SetType{ElemType: types.StringType},
	"component_ids": types.SetType{ElemType: types.Int64Type},
}

func audienceGroupsFromAPI(groups []client.AudienceGroup) types.List {
	elems := []attr.Value{}
	for _, g := range groups {
		domains := []attr.Value{}
		for _, d := range g.EmailDomains {
			domains = append(domains, types.StringValue(d))
		}
		ids := []attr.Value{}
		for _, id := range g.ComponentIDs {
			ids = append(ids, types.Int64Value(id))
		}
		elems = append(elems, types.ObjectValueMust(audienceGroupAttrTypes, map[string]attr.Value{
			"name":          types.StringValue(g.Name),
			"email_domains": types.SetValueMust(types.StringType, domains),
			"component_ids": types.SetValueMust(types.Int64Type, ids),
		}))
	}
	return types.ListValueMust(types.ObjectType{AttrTypes: audienceGroupAttrTypes}, elems)
}

func audienceGroupsToAPI(ctx context.Context, l types.List) []map[string]any {
	var in []struct {
		Name         types.String `tfsdk:"name"`
		EmailDomains types.Set    `tfsdk:"email_domains"`
		ComponentIDs types.Set    `tfsdk:"component_ids"`
	}
	l.ElementsAs(ctx, &in, false)
	out := []map[string]any{}
	for _, g := range in {
		domains := []string{}
		g.EmailDomains.ElementsAs(ctx, &domains, false)
		ids := []int64{}
		g.ComponentIDs.ElementsAs(ctx, &ids, false)
		out = append(out, map[string]any{"name": g.Name.ValueString(), "emailDomains": domains, "componentIds": ids})
	}
	return out
}

func NewStatusPageAudienceGroupsResource() resource.Resource {
	return &statusPageAudienceGroupsResource{}
}

func (r *statusPageAudienceGroupsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_status_page_audience_groups"
}

func (r *statusPageAudienceGroupsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "The audience groups of one status page (one resource per page): a component listed in a group is shown " +
			"only to visitors signed in with a verified email address of one of the group's domains, and to editors of the " +
			"page; components in no group stay visible to everyone who may view the page. A page with groups has no badge, " +
			"widget, feeds or subscriptions. Destroying the resource removes all groups. Import with the numeric status page id.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
				Description: "Same as `status_page_id`."},
			"status_page_id": schema.Int64Attribute{Required: true, Description: "Numeric status page id. Changing it replaces the resource.",
				PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()},
				Validators:    []validator.Int64{int64validator.AtLeast(1)}},
			"groups": schema.ListNestedAttribute{Required: true, Description: "At most 20 groups, in this order; `[]` = none.",
				Validators: []validator.List{listvalidator.SizeAtMost(20)},
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"name": schema.StringAttribute{Required: true, Description: "1 to 80 characters, no surrounding whitespace.",
						Validators: []validator.String{stringvalidator.LengthBetween(1, 80), statusPageTrimmed}},
					"email_domains": schema.SetAttribute{ElementType: types.StringType, Required: true,
						Description: "1 to 20 domains, lowercase, e.g. `acme.com`; visitors with a verified email of one of them see the components.",
						Validators: []validator.Set{setvalidator.SizeBetween(1, 20), setvalidator.ValueStringsAre(
							stringvalidator.LengthAtMost(253),
							stringvalidator.RegexMatches(audienceDomainRe, "must be a lowercase domain without trailing dot, e.g. acme.com"))}},
					"component_ids": schema.SetAttribute{ElementType: types.Int64Type, Required: true,
						Description: "1 to 100 numeric component ids of this page (`uptimeify_status_page_component.<name>.component_id`).",
						Validators:  []validator.Set{setvalidator.SizeBetween(1, 100), setvalidator.ValueInt64sAre(int64validator.AtLeast(1))}},
				}},
			},
		},
	}
}

func (r *statusPageAudienceGroupsResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		r.client = c
	}
}

// write replaces all groups of the page and reads them back into m.
func (r *statusPageAudienceGroupsResource) write(ctx context.Context, m *audienceGroupsModel, groups []map[string]any) error {
	page := strconv.FormatInt(m.StatusPageID.ValueInt64(), 10)
	if err := r.client.UpdateStatusPage(ctx, page, map[string]any{"audienceGroups": groups}); err != nil {
		return err
	}
	p, err := r.client.GetStatusPage(ctx, page)
	if err != nil {
		return err
	}
	m.ID = types.StringValue(page)
	m.Groups = audienceGroupsFromAPI(p.AudienceGroups)
	return nil
}

func (r *statusPageAudienceGroupsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan audienceGroupsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.write(ctx, &plan, audienceGroupsToAPI(ctx, plan.Groups)); err != nil {
		resp.Diagnostics.AddError("Setting status page audience groups failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *statusPageAudienceGroupsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state audienceGroupsModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	page := strconv.FormatInt(state.StatusPageID.ValueInt64(), 10)
	p, err := r.client.GetStatusPage(ctx, page)
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading status page audience groups failed", err.Error())
		return
	}
	state.ID = types.StringValue(page)
	state.Groups = audienceGroupsFromAPI(p.AudienceGroups)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *statusPageAudienceGroupsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan audienceGroupsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.write(ctx, &plan, audienceGroupsToAPI(ctx, plan.Groups)); err != nil {
		resp.Diagnostics.AddError("Updating status page audience groups failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *statusPageAudienceGroupsResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state audienceGroupsModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.UpdateStatusPage(ctx, strconv.FormatInt(state.StatusPageID.ValueInt64(), 10), map[string]any{"audienceGroups": []any{}})
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Removing status page audience groups failed", err.Error())
	}
}

func (r *statusPageAudienceGroupsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil || id <= 0 {
		resp.Diagnostics.AddError("Invalid import id", "expected the numeric status page id, got "+strconv.Quote(req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("status_page_id"), id)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}
