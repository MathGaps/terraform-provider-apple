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
	_ datasource.DataSource              = &subscriptionPromotionalOffersDataSource{}
	_ datasource.DataSourceWithConfigure = &subscriptionPromotionalOffersDataSource{}
)

func NewSubscriptionPromotionalOffersDataSource() datasource.DataSource {
	return &subscriptionPromotionalOffersDataSource{}
}

type subscriptionPromotionalOffersDataSource struct {
	client *apple.Client
}

// Metadata returns the data source type name.
func (d *subscriptionPromotionalOffersDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_subscription_promotional_offers"
}

// Schema defines the schema for the data source.
func (d *subscriptionPromotionalOffersDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists the promotional offers of an auto-renewable subscription.\n\n" +
			"An offer made in App Store Connect is known by its offer code, which is what the app holds; " +
			"this is how a configuration finds its Apple ID, either to import it into " +
			"`apple_subscription_promotional_offer` or to hand the codes to whatever signs offers on the " +
			"app's server. Apple publishes no top-level collection of offers, so the listing is scoped to " +
			"one subscription. Prices are not listed: they are a separate collection per offer.",

		Attributes: map[string]schema.Attribute{
			"subscription_id": schema.StringAttribute{
				MarkdownDescription: "The ID of the subscription whose promotional offers to list.",
				Required:            true,
			},
			"offer_code": schema.StringAttribute{
				MarkdownDescription: "Return only the offer with exactly this offer code. Applied in memory: " +
					"Apple publishes no offer code filter.",
				Optional:   true,
				Validators: GetOfferCodeValidator(),
			},
			"promotional_offers": schema.ListNestedAttribute{
				MarkdownDescription: "The promotional offers matching the filter.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							MarkdownDescription: "The Apple-generated identifier of the offer.",
							Computed:            true,
						},
						"name": schema.StringAttribute{
							MarkdownDescription: "The offer's reference name, internal to App Store Connect.",
							Computed:            true,
						},
						"offer_code": schema.StringAttribute{
							MarkdownDescription: "The identifier the app passes to the App Store to request the offer.",
							Computed:            true,
						},
						"duration": schema.StringAttribute{
							MarkdownDescription: "The length of one offer period: `THREE_DAYS`, `ONE_WEEK`, " +
								"`TWO_WEEKS`, `ONE_MONTH`, `TWO_MONTHS`, `THREE_MONTHS`, `SIX_MONTHS` or `ONE_YEAR`.",
							Computed: true,
						},
						"offer_mode": schema.StringAttribute{
							MarkdownDescription: "How the customer pays during the offer: `PAY_AS_YOU_GO`, " +
								"`PAY_UP_FRONT` or `FREE_TRIAL`.",
							Computed: true,
						},
						"number_of_periods": schema.Int64Attribute{
							MarkdownDescription: "How many `duration` periods the offer lasts.",
							Computed:            true,
						},
						"target_subscription_plan_type": schema.StringAttribute{
							MarkdownDescription: "Which plan the offer applies to, `MONTHLY` or `UPFRONT`, when " +
								"Apple reports one.",
							Computed: true,
						},
					},
				},
			},
			"total_count": schema.Int64Attribute{
				MarkdownDescription: "Number of offers the subscription has, before the filter.",
				Computed:            true,
			},
			"filtered_count": schema.Int64Attribute{
				MarkdownDescription: "Number of offers remaining after the filter.",
				Computed:            true,
			},
		},
	}
}

// Read lists and filters the offers.
func (d *subscriptionPromotionalOffersDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	tflog.Info(ctx, "Reading subscription promotional offers data source")

	var config subscriptionPromotionalOffersDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	subscriptionID := config.SubscriptionID.ValueString()
	ctx = tflog.SetField(ctx, "subscription_id", subscriptionID)

	offers, err := d.client.GetSubscriptionPromotionalOffers(subscriptionID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Read Subscription Promotional Offers",
			fmt.Sprintf("Could not list the promotional offers of subscription '%s': %s", subscriptionID, err.Error()),
		)
		return
	}

	offerCode := config.OfferCode.ValueString()
	summaries := make([]subscriptionPromotionalOfferSummaryModel, 0, len(offers))
	for _, offer := range offers {
		if offerCode != "" && offer.Attributes.OfferCode != offerCode {
			continue
		}
		summaries = append(summaries, promotionalOfferSummary(offer))
	}

	config.PromotionalOffers = summaries
	config.TotalCount = types.Int64Value(int64(len(offers)))
	config.FilteredCount = types.Int64Value(int64(len(summaries)))

	tflog.Info(ctx, "Subscription promotional offers data source read completed", map[string]interface{}{
		"total_count":    len(offers),
		"filtered_count": len(summaries),
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

// Configure adds the provider configured client to the data source.
func (d *subscriptionPromotionalOffersDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
