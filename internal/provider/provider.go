// Copyright (c) AO Studio
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"os"
	"regexp"
	"strings"

	"github.com/AhmedOsman00/terraform-provider-apple/internal/apple"
	"github.com/AhmedOsman00/terraform-provider-apple/internal/provider/app"
	"github.com/AhmedOsman00/terraform-provider-apple/internal/provider/beta"
	"github.com/AhmedOsman00/terraform-provider-apple/internal/provider/bundle"
	"github.com/AhmedOsman00/terraform-provider-apple/internal/provider/certificate"
	"github.com/AhmedOsman00/terraform-provider-apple/internal/provider/device"
	"github.com/AhmedOsman00/terraform-provider-apple/internal/provider/inapppurchase"
	"github.com/AhmedOsman00/terraform-provider-apple/internal/provider/merchant"
	"github.com/AhmedOsman00/terraform-provider-apple/internal/provider/passtypeid"
	"github.com/AhmedOsman00/terraform-provider-apple/internal/provider/profile"
	"github.com/AhmedOsman00/terraform-provider-apple/internal/provider/subscription"
	"github.com/AhmedOsman00/terraform-provider-apple/internal/provider/territory"
	"github.com/AhmedOsman00/terraform-provider-apple/internal/provider/user"

	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ provider.Provider = &appleProvider{}
)

// New is a helper function to simplify provider server and testing implementation.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &appleProvider{
			version: version,
		}
	}
}

type appleProvider struct {
	// version is set to the provider version on release, "dev" when the
	// provider is built and ran locally, and "test" when running acceptance
	// testing.
	version string
}

// Metadata returns the provider type name.
func (p *appleProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "apple"
	resp.Version = p.version
}

// Schema defines the provider-level schema for configuration data.
func (p *appleProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The Apple provider is used to manage resources in Apple App Store Connect.\n\n" +
			"Use the provider to manage Bundle IDs, certificates, profiles, and other App Store Connect resources. " +
			"You must configure the provider with the proper credentials before you can use it.\n\n" +
			"## Authentication\n\n" +
			"The provider requires App Store Connect API credentials:\n\n" +
			"- **Issuer ID**: Your App Store Connect API issuer ID (UUID format)\n" +
			"- **API Key**: Your 10-character alphanumeric API key ID\n" +
			"- **Private Key**: The contents of your `.p8` private key file\n\n" +
			"These can be provided via provider configuration or environment variables.",

		Attributes: map[string]schema.Attribute{
			"issuer_id": schema.StringAttribute{
				MarkdownDescription: "The App Store Connect API issuer ID. This is a UUID that identifies your organization. " +
					"Can also be set via the `APPLE_APP_STORE_CONNECT_ISSUER_ID` environment variable. " +
					"Find this in App Store Connect under Users and Access > Keys.",
				Optional:  true,
				Sensitive: true,
				Validators: []validator.String{
					stringvalidator.RegexMatches(
						regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`),
						"Issuer ID must be a valid UUID format (e.g., 12345678-1234-1234-1234-123456789012)",
					),
				},
			},
			"api_key": schema.StringAttribute{
				MarkdownDescription: "The App Store Connect API key ID. This is a 10-character alphanumeric string. " +
					"Can also be set via the `APPLE_APP_STORE_CONNECT_API_KEY` environment variable. " +
					"Generate this in App Store Connect under Users and Access > Keys.",
				Optional:  true,
				Sensitive: true,
				Validators: []validator.String{
					stringvalidator.RegexMatches(
						regexp.MustCompile(`^[A-Z0-9]{10}$`),
						"API Key must be a 10-character alphanumeric string (e.g., ABC123DEF4)",
					),
				},
			},
			"private_key": schema.StringAttribute{
				MarkdownDescription: "The contents of your App Store Connect API private key (.p8 file). " +
					"Can also be set via the `APPLE_APP_STORE_CONNECT_PRIVATE_KEY` environment variable. " +
					"This should be the full PEM-encoded private key including headers and footers.",
				Optional:  true,
				Sensitive: true,
				Validators: []validator.String{
					stringvalidator.RegexMatches(
						regexp.MustCompile(`-----BEGIN PRIVATE KEY-----[\s\S]*-----END PRIVATE KEY-----`),
						"Private Key must be a valid PEM-encoded private key",
					),
				},
			},
			"scope": schema.ListAttribute{
				MarkdownDescription: "Restricts the generated JWT to specific App Store Connect operations, each written as " +
					"`<METHOD> <path>` (for example `GET /v1/apps`). **Leave this unset for most use cases** — an " +
					"unscoped token can perform every operation the API key itself is permitted. Do not configure it as " +
					"an empty list: Apple rejects a token carrying an empty scope claim with a `400` error titled " +
					"\"JSON processing failed\". Refer to Apple's documentation for specific scope requirements.",
				Optional:    true,
				ElementType: types.StringType,
				Validators: []validator.List{
					listvalidator.SizeAtMost(10), // Reasonable limit
				},
			},
		},
	}
}

type appleProviderModel struct {
	IssuerID   types.String `tfsdk:"issuer_id"`
	APIKey     types.String `tfsdk:"api_key"`
	PrivateKey types.String `tfsdk:"private_key"`
	Scope      types.List   `tfsdk:"scope"`
}

func (p *appleProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	tflog.Info(ctx, "Configuring Apple App Store Connect provider")

	// Retrieve provider data from configuration
	var config appleProviderModel
	diags := req.Config.Get(ctx, &config)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Check for unknown configuration values
	if config.IssuerID.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("issuer_id"),
			"Unknown Apple Issuer ID",
			"The provider cannot create the Apple client as there is an unknown configuration value for the Apple API issuer ID. "+
				"Either target apply the source of the value first, set the value statically in the configuration, or use the APPLE_APP_STORE_CONNECT_ISSUER_ID environment variable.",
		)
	}

	if config.PrivateKey.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("private_key"),
			"Unknown Apple Private Key",
			"The provider cannot create the Apple client as there is an unknown configuration value for the Apple API private key. "+
				"Either target apply the source of the value first, set the value statically in the configuration, or use the APPLE_APP_STORE_CONNECT_PRIVATE_KEY environment variable.",
		)
	}

	if config.APIKey.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("api_key"),
			"Unknown Apple API Key",
			"The provider cannot create the Apple client as there is an unknown configuration value for the Apple API key. "+
				"Either target apply the source of the value first, set the value statically in the configuration, or use the APPLE_APP_STORE_CONNECT_API_KEY environment variable.",
		)
	}

	// Process scope configuration
	var scope []string
	if !config.Scope.IsUnknown() && !config.Scope.IsNull() {
		diags := config.Scope.ElementsAs(ctx, &scope, false)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
	} else {
		scope = []string{} // Default to empty scope
	}

	if resp.Diagnostics.HasError() {
		return
	}

	// Get configuration values, with environment variables as fallback
	issuerID := os.Getenv("APPLE_APP_STORE_CONNECT_ISSUER_ID")
	apiKey := os.Getenv("APPLE_APP_STORE_CONNECT_API_KEY")
	privateKey := os.Getenv("APPLE_APP_STORE_CONNECT_PRIVATE_KEY")

	// Override with explicit configuration if provided
	if !config.IssuerID.IsNull() {
		issuerID = config.IssuerID.ValueString()
	}

	if !config.APIKey.IsNull() {
		apiKey = config.APIKey.ValueString()
	}

	if !config.PrivateKey.IsNull() {
		privateKey = config.PrivateKey.ValueString()
	}

	// Validate that all required configurations are present
	if strings.TrimSpace(issuerID) == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("issuer_id"),
			"Missing Apple Issuer ID",
			"The provider cannot create the Apple API client as the issuer ID is missing. "+
				"Set the issuer_id attribute in the provider configuration or use the APPLE_APP_STORE_CONNECT_ISSUER_ID environment variable.",
		)
	}

	if strings.TrimSpace(privateKey) == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("private_key"),
			"Missing Apple Private Key",
			"The provider cannot create the Apple API client as the private key is missing. "+
				"Set the private_key attribute in the provider configuration or use the APPLE_APP_STORE_CONNECT_PRIVATE_KEY environment variable.",
		)
	}

	if strings.TrimSpace(apiKey) == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("api_key"),
			"Missing Apple API Key",
			"The provider cannot create the Apple API client as the API key is missing. "+
				"Set the api_key attribute in the provider configuration or use the APPLE_APP_STORE_CONNECT_API_KEY environment variable.",
		)
	}

	if resp.Diagnostics.HasError() {
		return
	}

	// Add structured logging context (avoiding sensitive data)
	ctx = tflog.SetField(ctx, "issuer_id_set", issuerID != "")
	ctx = tflog.SetField(ctx, "api_key_set", apiKey != "")
	ctx = tflog.SetField(ctx, "private_key_set", privateKey != "")
	ctx = tflog.SetField(ctx, "scope_count", len(scope))

	tflog.Debug(ctx, "Creating Apple API client")

	// Create Apple API client
	client, err := apple.NewClient(issuerID, apiKey, privateKey, scope)
	if err != nil {
		// Provide helpful error context based on the error message
		var errorDetail string
		errStr := err.Error()

		if strings.Contains(errStr, "private key") {
			errorDetail = "The private key appears to be invalid. Ensure it's a valid PEM-encoded PKCS#8 private key from your App Store Connect API key."
		} else if strings.Contains(errStr, "issuer") {
			errorDetail = "The issuer ID appears to be invalid. Ensure it matches the issuer ID from your App Store Connect API key."
		} else if strings.Contains(errStr, "key") {
			errorDetail = "The API key appears to be invalid. Ensure it's the correct 10-character key ID from App Store Connect."
		} else {
			errorDetail = "Please verify your credentials are correct and that your API key has the necessary permissions."
		}

		resp.Diagnostics.AddError(
			"Unable to Create Apple API Client",
			"An unexpected error occurred when creating the Apple API client. "+errorDetail+"\n\n"+
				"Apple Client Error: "+err.Error(),
		)
		return
	}

	tflog.Info(ctx, "Apple API client configured successfully")

	// Make the Apple client available during DataSource and Resource Configure methods
	resp.DataSourceData = client
	resp.ResourceData = client
}

// DataSources defines the data sources implemented in the provider.
func (p *appleProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		bundle.NewBundleIDsDataSource,
		bundle.NewBundleIDCapabilitiesDataSource,
		certificate.NewCertificatesDataSource,
		device.NewDevicesDataSource,
		merchant.NewMerchantIDsDataSource,
		passtypeid.NewPassTypeIDsDataSource,
		profile.NewProfilesDataSource,
		app.NewAppsDataSource,
		app.NewAppCategoriesDataSource,
		app.NewAppPricePointsDataSource,
		app.NewAppStoreVersionsDataSource,
		subscription.NewSubscriptionGroupsDataSource,
		subscription.NewSubscriptionsDataSource,
		subscription.NewSubscriptionPricePointsDataSource,
		subscription.NewSubscriptionPricePointEqualizationsDataSource,
		subscription.NewSubscriptionPromotionalOffersDataSource,
		subscription.NewSubscriptionIntroductoryOffersDataSource,
		inapppurchase.NewInAppPurchasesDataSource,
		inapppurchase.NewInAppPurchasePricePointsDataSource,
		territory.NewTerritoriesDataSource,
		user.NewUsersDataSource,
	}
}

// Resources defines the resources implemented in the provider.
func (p *appleProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		bundle.NewBundleIDResource,
		bundle.NewBundleIDCapabilityResource,
		certificate.NewCertificateResource,
		device.NewDeviceResource,
		merchant.NewMerchantIDResource,
		passtypeid.NewPassTypeIDResource,
		profile.NewProfileResource,
		app.NewAppSettingsResource,
		app.NewAppInfoResource,
		app.NewAppInfoLocalizationResource,
		app.NewAgeRatingDeclarationResource,
		app.NewAppStoreVersionResource,
		app.NewAppStoreVersionLocalizationResource,
		app.NewAppStoreReviewDetailResource,
		app.NewAppPriceScheduleResource,
		app.NewAppAvailabilityResource,
		beta.NewBetaGroupResource,
		beta.NewBetaTesterResource,
		beta.NewBetaAppLocalizationResource,
		beta.NewBetaBuildLocalizationResource,
		beta.NewBetaAppReviewDetailResource,
		subscription.NewSubscriptionGroupResource,
		subscription.NewSubscriptionGroupLocalizationResource,
		subscription.NewSubscriptionResource,
		subscription.NewSubscriptionLocalizationResource,
		subscription.NewSubscriptionPriceResource,
		subscription.NewSubscriptionPriceScheduleResource,
		subscription.NewSubscriptionAvailabilityResource,
		subscription.NewSubscriptionPromotionalOfferResource,
		subscription.NewSubscriptionIntroductoryOfferResource,
		inapppurchase.NewInAppPurchaseResource,
		inapppurchase.NewInAppPurchaseLocalizationResource,
		inapppurchase.NewInAppPurchasePriceScheduleResource,
		inapppurchase.NewInAppPurchaseAvailabilityResource,
		user.NewUserResource,
		user.NewUserInvitationResource,
	}
}
