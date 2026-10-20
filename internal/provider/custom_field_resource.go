package provider

import (
	"context"
	"regexp"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptimeify/terraform-provider-uptimeify/internal/client"
)

var (
	_ resource.ResourceWithConfigure   = &customFieldResource{}
	_ resource.ResourceWithImportState = &customFieldResource{}
)

type customFieldResource struct{ client *client.Client }

// customFieldModel is shared by the uptimeify_custom_field resource and data source.
type customFieldModel struct {
	ID             types.String `tfsdk:"id"`
	OrganizationID types.Int64  `tfsdk:"organization_id"`
	Name           types.String `tfsdk:"name"`
	FieldKey       types.String `tfsdk:"field_key"`
	FieldType      types.String `tfsdk:"field_type"`
	IsRequired     types.Bool   `tfsdk:"is_required"`
	DisplayOrder   types.Int64  `tfsdk:"display_order"`
	Options        types.List   `tfsdk:"options"`
	Placeholder    types.String `tfsdk:"placeholder"`
	HelpText       types.String `tfsdk:"help_text"`
	ShowInTable    types.Bool   `tfsdk:"show_in_table"`
	CreatedAt      types.String `tfsdk:"created_at"`
	UpdatedAt      types.String `tfsdk:"updated_at"`
}

func (m *customFieldModel) fromAPI(ctx context.Context, f *client.CustomField) diag.Diagnostics {
	m.ID = types.StringValue(strconv.FormatInt(f.ID, 10))
	m.OrganizationID = types.Int64Value(f.OrganizationID)
	m.Name = types.StringValue(f.Name)
	m.FieldKey = types.StringValue(f.FieldKey)
	m.FieldType = types.StringValue(f.FieldType)
	m.IsRequired = types.BoolValue(f.IsRequired)
	m.DisplayOrder = types.Int64Value(f.DisplayOrder)
	opts := f.Options
	if opts == nil {
		opts = []string{}
	}
	var d diag.Diagnostics
	m.Options, d = types.ListValueFrom(ctx, types.StringType, opts)
	m.Placeholder = nonEmpty(f.Placeholder)
	m.HelpText = nonEmpty(f.HelpText)
	m.ShowInTable = types.BoolValue(f.ShowInTable)
	m.CreatedAt = types.StringValue(f.CreatedAt)
	m.UpdatedAt = types.StringValue(f.UpdatedAt)
	return d
}

func NewCustomFieldResource() resource.Resource { return &customFieldResource{} }

func (r *customFieldResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_custom_field"
}

func (r *customFieldResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	keep := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	resp.Schema = schema.Schema{
		Description: "A custom field of the customer master data (organization token with write access). The API never " +
			"deletes a field: destroying it deactivates it, and creating a field whose `field_key` belongs to a " +
			"deactivated one reactivates that field (same id) with the configured values, so customer values stored " +
			"under the key come back. An active field with the key answers 409; import it instead. A field " +
			"deactivated outside Terraform counts as gone. Import with the numeric id.",
		Attributes: map[string]schema.Attribute{
			"id":              schema.StringAttribute{Computed: true, PlanModifiers: keep, Description: "Numeric field id."},
			"organization_id": schema.Int64Attribute{Computed: true, PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()}},
			"name":            schema.StringAttribute{Required: true, Validators: []validator.String{trimmedText, stringvalidator.LengthAtMost(100)}},
			"field_key": schema.StringAttribute{Optional: true, Computed: true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown(), stringplanmodifier.RequiresReplace()},
				Description:   "Key in the customer's `custom_fields`: lower-case letters, digits, single underscores. Unset = derived from `name`.",
				Validators: []validator.String{stringvalidator.LengthAtMost(100),
					stringvalidator.RegexMatches(regexp.MustCompile(`^[a-z0-9]+(_[a-z0-9]+)*$`), "must be lower-case letters and digits separated by single underscores")}},
			"field_type": schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString("text"),
				Description: "`" + strings.Join(client.CustomFieldTypes, "`, `") + "`. Default `text`.",
				Validators:  []validator.String{stringvalidator.OneOf(client.CustomFieldTypes...)}},
			"is_required": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false)},
			"display_order": schema.Int64Attribute{Optional: true, Computed: true, Default: int64default.StaticInt64(0),
				Validators: []validator.Int64{int64validator.Between(0, 10000)}},
			"options": schema.ListAttribute{Optional: true, Computed: true, ElementType: types.StringType,
				Default:     listdefault.StaticValue(types.ListValueMust(types.StringType, nil)),
				Description: "Choices of a `select` field, at most 50 of up to 100 characters.",
				Validators: []validator.List{listvalidator.SizeAtMost(50),
					listvalidator.ValueStringsAre(trimmedText, stringvalidator.LengthAtMost(100))}},
			"placeholder":   schema.StringAttribute{Optional: true, Validators: []validator.String{trimmedText, stringvalidator.LengthAtMost(200)}},
			"help_text":     schema.StringAttribute{Optional: true, Validators: []validator.String{trimmedText, stringvalidator.LengthAtMost(500)}},
			"show_in_table": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true), Description: "Show as a column in the customer list."},
			"created_at":    schema.StringAttribute{Computed: true, PlanModifiers: keep},
			"updated_at":    schema.StringAttribute{Computed: true},
		},
	}
}

func (r *customFieldResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		r.client = c
	}
}

func (r *customFieldResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan customFieldModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var opts []string
	resp.Diagnostics.Append(plan.Options.ElementsAs(ctx, &opts, false)...)
	body := map[string]any{
		"name": plan.Name.ValueString(), "fieldType": plan.FieldType.ValueString(), "isRequired": plan.IsRequired.ValueBool(),
		"displayOrder": plan.DisplayOrder.ValueInt64(), "options": opts, "showInTable": plan.ShowInTable.ValueBool(),
		"placeholder": strOrNil(plan.Placeholder), "helpText": strOrNil(plan.HelpText),
	}
	if !plan.FieldKey.IsUnknown() {
		body["fieldKey"] = plan.FieldKey.ValueString()
	}
	f, err := r.client.CreateCustomField(ctx, body)
	if err != nil {
		resp.Diagnostics.AddError("Creating custom field failed", err.Error())
		return
	}
	resp.Diagnostics.Append(plan.fromAPI(ctx, f)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *customFieldResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state customFieldModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	f, err := r.client.GetCustomField(ctx, state.ID.ValueString())
	if client.IsNotFound(err) || err == nil && !f.IsActive {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading custom field failed", err.Error())
		return
	}
	resp.Diagnostics.Append(state.fromAPI(ctx, f)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *customFieldResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state customFieldModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	patch := map[string]any{}
	if !plan.Name.Equal(state.Name) {
		patch["name"] = plan.Name.ValueString()
	}
	if !plan.FieldType.Equal(state.FieldType) {
		patch["fieldType"] = plan.FieldType.ValueString()
	}
	if !plan.IsRequired.Equal(state.IsRequired) {
		patch["isRequired"] = plan.IsRequired.ValueBool()
	}
	if !plan.DisplayOrder.Equal(state.DisplayOrder) {
		patch["displayOrder"] = plan.DisplayOrder.ValueInt64()
	}
	if !plan.Options.Equal(state.Options) {
		var opts []string
		resp.Diagnostics.Append(plan.Options.ElementsAs(ctx, &opts, false)...)
		if opts == nil {
			opts = []string{}
		}
		patch["options"] = opts
	}
	if !plan.Placeholder.Equal(state.Placeholder) {
		patch["placeholder"] = strOrNil(plan.Placeholder)
	}
	if !plan.HelpText.Equal(state.HelpText) {
		patch["helpText"] = strOrNil(plan.HelpText)
	}
	if !plan.ShowInTable.Equal(state.ShowInTable) {
		patch["showInTable"] = plan.ShowInTable.ValueBool()
	}
	f, err := r.client.UpdateCustomField(ctx, state.ID.ValueString(), patch)
	if err != nil {
		resp.Diagnostics.AddError("Updating custom field failed", err.Error())
		return
	}
	resp.Diagnostics.Append(plan.fromAPI(ctx, f)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete deactivates the field (the API has no hard delete).
func (r *customFieldResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state customFieldModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || state.ID.ValueString() == "" {
		return
	}
	if err := r.client.DeleteCustomField(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Deactivating custom field failed", err.Error())
	}
}

func (r *customFieldResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
