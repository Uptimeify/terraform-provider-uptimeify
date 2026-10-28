// Package provider implements the Uptimeify Terraform/OpenTofu provider.
package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptimeify/terraform-provider-uptimeify/internal/client"
)

// DefaultEndpoint is the production base URL (OpenAPI `servers`, docs-site BASE_URL).
const DefaultEndpoint = "https://uptimeify.io"

type uptimeifyProvider struct{ version string }

type providerModel struct {
	Endpoint types.String `tfsdk:"endpoint"`
	APIToken types.String `tfsdk:"api_token"`
}

func New(version string) func() provider.Provider {
	return func() provider.Provider { return &uptimeifyProvider{version: version} }
}

func (p *uptimeifyProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "uptimeify"
	resp.Version = p.version
}

func (p *uptimeifyProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manage Uptimeify resources through the Uptimeify REST API.",
		Attributes: map[string]schema.Attribute{
			"endpoint": schema.StringAttribute{
				Optional:    true,
				Description: "API base URL. Falls back to `UPTIMEIFY_ENDPOINT`, then `" + DefaultEndpoint + "`.",
			},
			"api_token": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "Organization API token (`wsm_...`) or customer token. Falls back to `UPTIMEIFY_API_TOKEN`.",
			},
		},
	}
}

func (p *uptimeifyProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var cfg providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	endpoint := firstNonEmpty(cfg.Endpoint.ValueString(), os.Getenv("UPTIMEIFY_ENDPOINT"), DefaultEndpoint)
	token := firstNonEmpty(cfg.APIToken.ValueString(), os.Getenv("UPTIMEIFY_API_TOKEN"))
	if token == "" {
		resp.Diagnostics.AddError("Missing API token",
			"Set api_token in the provider block or the UPTIMEIFY_API_TOKEN environment variable.")
		return
	}
	c := client.New(endpoint, token, p.version)
	resp.ResourceData = c
	resp.DataSourceData = c
}

func (p *uptimeifyProvider) Resources(context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewTagResource,
		NewMaintenanceWindowResource,
		NewMonitorTagResource,
		NewDNSMonitorResource,
		NewTCPMonitorResource,
		NewICMPMonitorResource,
		NewSMTPMonitorResource,
		NewSSHMonitorResource,
		NewFTPMonitorResource,
		NewIMAPPopMonitorResource,
		NewWebsiteResource,
		NewIMTeamResource,
		NewIMTeamMemberResource,
		NewIMEscalationTierResource,
		NewIMScheduleResource,
		NewIMScheduleOverrideResource,
		NewIMTeamOverrideResource,
		NewImAlertSourceResource,
		NewImRoutingRuleResource,
		NewImOrgSettingsResource,
		NewImChannelResource,
		NewImOutboundResource,
		NewStatusPageResource,
		NewStatusPageDesignResource,
		NewStatusPageDomainResource,
		NewStatusPageDomainActivationResource,
		NewStatusPageComponentResource,
		NewStatusPageComponentGroupResource,
		NewImStatuspageRuleResource,
		NewOrgUserResource,
		NewOrgSettingsResource,
		NewWhitelabelDomainResource,
		NewReportResource,
		NewSMTPSettingsResource,
		NewBrandingResource,
		NewCustomerResource,
		NewCustomerIPResource,
		NewCustomerDomainResource,
		NewNotificationChannelResource,
		NewEscalationConfigResource,
		NewCustomFieldResource,
		NewSSOOIDCResource,
		NewSSODomainVerificationResource,
	}
}

func (p *uptimeifyProvider) DataSources(context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewTagDataSource,
		NewMaintenanceWindowDataSource,
		NewMonitoringLocationsDataSource,
		NewCustomerDataSource,
		NewDNSMonitorDataSource,
		NewTCPMonitorDataSource,
		NewICMPMonitorDataSource,
		NewSMTPMonitorDataSource,
		NewSSHMonitorDataSource,
		NewFTPMonitorDataSource,
		NewIMAPPopMonitorDataSource,
		NewWebsiteDataSource,
		NewIMTeamDataSource,
		NewUserDataSource,
		NewImAlertSourceDataSource,
		NewImRoutingRuleDataSource,
		NewImChannelDataSource,
		NewImOutboundDataSource,
		NewImOnCallDataSource,
		NewStatusPageDataSource,
		NewImStatuspageRuleDataSource,
		NewOrganizationDataSource,
		NewPackageConfigsDataSource,
		NewNotificationChannelDataSource,
		NewCustomFieldDataSource,
	}
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}
