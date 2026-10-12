// Copyright (c) AO Studio
// SPDX-License-Identifier: MPL-2.0

package subscription

import (
	"context"
	"fmt"

	"github.com/AhmedOsman00/terraform-provider-apple/internal/apple"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var (
	_ datasource.DataSource              = &subscriptionOfferCodesDataSource{}
	_ datasource.DataSourceWithConfigure = &subscriptionOfferCodesDataSource{}
)

func NewSubscriptionOfferCodesDataSource() datasource.DataSource {
	return &subscriptionOfferCodesDataSource{}
}

type subscriptionOfferCodesDataSource struct {
	client *apple.Client
}

// Metadata returns the data source type name.
func (d *subscriptionOfferCodesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_subscription_offer_codes"
}

// Schema defines the schema for the data source.
func (d *subscriptionOfferCodesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists the offer codes of an auto-renewable subscription, including deactivated " +
			"ones — Apple keeps every offer forever.\n\n" +
			"It exists for the offers made by hand in App Store Connect: their IDs appear nowhere in the UI, " +
			"and the ID is what `terraform import apple_subscription_offer_code` takes. `subscription_id` is " +
			"required because Apple publishes no top-level offer code collection. Prices are not listed — " +
			"reading them is one request per offer; the resource reads them on import.",

		Attributes: map[string]schema.Attribute{
			"subscription_id": schema.StringAttribute{
				MarkdownDescription: "The ID of the subscription whose offer codes to list.",
				Required:            true,
			},
			"name_pattern": schema.StringAttribute{
				MarkdownDescription: "Regular expression matched against each offer's reference name. Matches " +
					"as a substring unless anchored with `^` and `$`.",
				Optional:   true,
				Validators: GetPatternValidator(),
			},
			"active": schema.BoolAttribute{
				MarkdownDescription: "Return only active offers (`true`) or only deactivated ones (`false`). " +
					"Unset returns both.",
				Optional: true,
			},
			"offer_codes": schema.ListNestedAttribute{
				MarkdownDescription: "The offer codes matching the filters, sorted by name.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							MarkdownDescription: "The unique Apple-generated identifier for the offer code.",
							Computed:            true,
						},
						"name": schema.StringAttribute{
							MarkdownDescription: "The offer's reference name.",
							Computed:            true,
						},
						"customer_eligibilities": schema.ListAttribute{
							MarkdownDescription: "Who may redeem it: `NEW`, `EXISTING` and/or `EXPIRED`.",
							Computed:            true,
							ElementType:         types.StringType,
						},
						"offer_eligibility": schema.StringAttribute{
							MarkdownDescription: "`STACK_WITH_INTRO_OFFERS` or `REPLACE_INTRO_OFFERS`.",
							Computed:            true,
						},
						"duration": schema.StringAttribute{
							MarkdownDescription: "The length of one offer period.",
							Computed:            true,
						},
						"offer_mode": schema.StringAttribute{
							MarkdownDescription: "`PAY_AS_YOU_GO`, `PAY_UP_FRONT` or `FREE_TRIAL`.",
							Computed:            true,
						},
						"number_of_periods": schema.Int64Attribute{
							MarkdownDescription: "How many periods the offer lasts.",
							Computed:            true,
						},
						"auto_renew_enabled": schema.BoolAttribute{
							MarkdownDescription: "Whether the subscription renews once the offer ends.",
							Computed:            true,
						},
						"target_subscription_plan_type": schema.StringAttribute{
							MarkdownDescription: "`MONTHLY` or `UPFRONT`, when Apple reports one.",
							Computed:            true,
						},
						"active": schema.BoolAttribute{
							MarkdownDescription: "Whether the offer can still be redeemed.",
							Computed:            true,
						},
						"total_number_of_codes": schema.Int64Attribute{
							MarkdownDescription: "How many codes Apple reports for the offer.",
							Computed:            true,
						},
						"production_code_count": schema.Int64Attribute{
							MarkdownDescription: "How many of them redeem in production.",
							Computed:            true,
						},
						"sandbox_code_count": schema.Int64Attribute{
							MarkdownDescription: "How many of them redeem in the sandbox.",
							Computed:            true,
						},
					},
				},
			},
			"total_count": schema.Int64Attribute{
				MarkdownDescription: "Total number of offer codes on the subscription before filtering.",
				Computed:            true,
			},
			"filtered_count": schema.Int64Attribute{
				MarkdownDescription: "Number of offer codes remaining after filtering.",
				Computed:            true,
			},
		},
	}
}

// Read lists and filters the offer codes.
func (d *subscriptionOfferCodesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config subscriptionOfferCodesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	subscriptionID := config.SubscriptionID.ValueString()
	ctx = tflog.SetField(ctx, "subscription_id", subscriptionID)

	offers, err := d.client.GetSubscriptionOfferCodes(subscriptionID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Read Subscription Offer Codes",
			fmt.Sprintf("Could not list the offer codes of subscription '%s': %s", subscriptionID, err.Error()),
		)
		return
	}

	filters := OfferCodeFilters{NamePattern: config.NamePattern.ValueString()}
	if !config.Active.IsNull() && !config.Active.IsUnknown() {
		active := config.Active.ValueBool()
		filters.Active = &active
	}

	filtered, err := FilterOfferCodes(offers, filters)
	if err != nil {
		resp.Diagnostics.AddError("Invalid Offer Code Filter", err.Error())
		return
	}

	config.OfferCodes = make([]subscriptionOfferCodeSummaryModel, 0, len(filtered))
	for _, offer := range filtered {
		a := offer.Attributes
		eligibilities := make([]types.String, 0, len(a.CustomerEligibilities))
		for _, e := range a.CustomerEligibilities {
			eligibilities = append(eligibilities, types.StringValue(e))
		}

		config.OfferCodes = append(config.OfferCodes, subscriptionOfferCodeSummaryModel{
			ID:                         types.StringValue(offer.ID),
			Name:                       types.StringValue(a.Name),
			CustomerEligibilities:      eligibilities,
			OfferEligibility:           types.StringValue(a.OfferEligibility),
			Duration:                   types.StringValue(a.Duration),
			OfferMode:                  types.StringValue(a.OfferMode),
			NumberOfPeriods:            offerCodeInt64(a.NumberOfPeriods),
			AutoRenewEnabled:           offerCodeBool(a.AutoRenewEnabled),
			TargetSubscriptionPlanType: stringOrNull(a.TargetSubscriptionPlanType),
			Active:                     offerCodeBool(a.Active),
			TotalNumberOfCodes:         offerCodeInt64(a.TotalNumberOfCodes),
			ProductionCodeCount:        offerCodeInt64(a.ProductionCodeCount),
			SandboxCodeCount:           offerCodeInt64(a.SandboxCodeCount),
		})
	}

	config.TotalCount = types.Int64Value(int64(len(offers)))
	config.FilteredCount = types.Int64Value(int64(len(filtered)))

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

// Configure adds the provider configured client to the data source.
func (d *subscriptionOfferCodesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*apple.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *apple.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	d.client = client
}
