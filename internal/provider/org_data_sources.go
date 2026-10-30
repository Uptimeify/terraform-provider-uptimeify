package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptimeify/terraform-provider-uptimeify/internal/client"
)

// Both data sources store the API structs directly (tfsdk tags in internal/client/org_users.go).

var (
	_ datasource.DataSourceWithConfigure = &organizationDataSource{}
	_ datasource.DataSourceWithConfigure = &packageConfigsDataSource{}
)

// computedAttrs builds computed attributes from name -> kind ("s", "b", "i", "ls" = list of strings).
func computedAttrs(kinds map[string]string) map[string]schema.Attribute {
	out := map[string]schema.Attribute{}
	for name, kind := range kinds {
		switch kind {
		case "s":
			out[name] = schema.StringAttribute{Computed: true}
		case "b":
			out[name] = schema.BoolAttribute{Computed: true}
		case "i":
			out[name] = schema.Int64Attribute{Computed: true}
		case "ls":
			out[name] = schema.ListAttribute{Computed: true, ElementType: types.StringType}
		}
	}
	return out
}

type organizationDataSource struct{ client *client.Client }

func NewOrganizationDataSource() datasource.DataSource { return &organizationDataSource{} }

func (d *organizationDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_organization"
}

func (d *organizationDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	attrs := computedAttrs(map[string]string{
		"id": "i", "public_id": "s", "name": "s", "status": "s", "deletion_due_at": "s", "email_language": "s",
		"billing_email": "s", "billing_cycle": "s", "payment_method": "s",
		"require_mfa_admins": "b", "require_mfa_editors": "b", "require_mfa_readonly": "b", "require_mfa_customers": "b",
		"company_name": "s", "street": "s", "postal_code": "s", "city": "s", "country": "s", "vat_id": "s",
		"quota_websites_limit": "i", "quota_ips_limit": "i", "quota_sms_included": "i", "quota_monthly_price_cents": "i",
		"quota_extra_sms_price_cents": "i", "quota_auto_upgrade_enabled": "b", "sms_overage_safety_cap_cents_monthly": "i",
	})
	attrs["default_notification_channels"] = schema.SingleNestedAttribute{Computed: true,
		Attributes: computedAttrs(map[string]string{"email": "b", "sms": "b", "webhook": "b", "integrations": "b"})}
	attrs["default_notification_targets"] = schema.SingleNestedAttribute{Computed: true,
		Attributes: computedAttrs(map[string]string{"email": "s", "sms": "s", "webhook": "s", "integrations": "s"})}
	resp.Schema = schema.Schema{
		Description: "The token's own organization (`GET /api/organization`). Billing, address, quota and MFA fields are " +
			"only filled for admin tokens.",
		Attributes: attrs,
	}
}

func (d *organizationDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		d.client = c
	}
}

func (d *organizationDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	o, err := d.client.GetOrganization(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Reading organization failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, o)...)
}

type packageConfigsDataSource struct{ client *client.Client }

type packageConfigsModel struct {
	PackageConfigs []client.PackageConfig `tfsdk:"package_configs"`
}

func NewPackageConfigsDataSource() datasource.DataSource { return &packageConfigsDataSource{} }

func (d *packageConfigsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_package_configs"
}

func (d *packageConfigsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "The organization's package configurations (`GET /api/package-configs`): limits, checks and alert " +
			"defaults per package type, e.g. to look up the `package_type` of a customer.",
		Attributes: map[string]schema.Attribute{
			"package_configs": schema.ListNestedAttribute{Computed: true, NestedObject: schema.NestedAttributeObject{
				Attributes: computedAttrs(map[string]string{
					"id": "i", "package_type": "s", "display_name": "s", "max_urls": "i", "max_status_pages": "i", "allowed_check_country_codes": "ls",
					"data_retention_months": "i", "check_interval_minutes": "i", "check_locations": "i",
					"notification_delay_minutes": "i", "reminder_delay_minutes": "i", "alert_consecutive_checks": "i",
					"alert_location_threshold": "s", "alert_location_threshold_count": "i", "alert_reminder_interval": "i",
					"dnsbl_reminder_interval": "i", "ssl_expiry_reminder_interval": "i",
					"enable_ssl_check": "b", "enable_https_check": "b", "enable_status_check": "b", "enable_size_check": "b",
					"enable_response_time_check": "b", "enable_keyword_check": "b", "enable_email_alerts": "b",
					"enable_sms_alerts": "b", "enable_webhook_alerts": "b", "enable_integration_alerts": "b",
					"enable_post_request_escalation": "b", "enable_maintenance_windows": "b", "enable_pdf_reports": "b",
					"monthly_reports_default": "b", "allow_self_service": "b", "max_self_service_urls": "i",
					"flapping_enabled": "b", "flapping_incident_count": "i", "flapping_window_minutes": "i",
					"flapping_recovery_clean_cycles": "i", "alert_repeat_interval_minutes": "i", "notes": "s",
					"used_by_customer_count": "i",
				}),
			}},
		},
	}
}

func (d *packageConfigsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		d.client = c
	}
}

func (d *packageConfigsDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	pcs, err := d.client.ListPackageConfigs(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Listing package configurations failed", err.Error())
		return
	}
	if pcs == nil {
		pcs = []client.PackageConfig{}
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &packageConfigsModel{PackageConfigs: pcs})...)
}
