// Copyright (c) AO Studio
// SPDX-License-Identifier: MPL-2.0

package subscription

import (
	"context"
	"fmt"
	"sort"

	"github.com/AhmedOsman00/terraform-provider-apple/internal/apple"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var (
	_ datasource.DataSource              = &winBackOffersDataSource{}
	_ datasource.DataSourceWithConfigure = &winBackOffersDataSource{}
)

func NewWinBackOffersDataSource() datasource.DataSource {
	return &winBackOffersDataSource{}
}

type winBackOffersDataSource struct {
	client *apple.Client
}

// Metadata returns the data source type name.
func (d *winBackOffersDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_subscription_win_back_offers"
}

// Schema defines the schema for the data source.
func (d *winBackOffersDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists the win-back offers of a subscription.\n\n" +
			"Apple publishes no collection of win-back offers of its own, and an offer never reports which " +
			"subscription it belongs to, so this is the only way to find one made in App Store Connect — " +
			"for reading it, or for the `<subscription_id>/<offer_id>` ID that imports it into an " +
			"`apple_subscription_win_back_offer`. Prices are not listed: they are a separate collection " +
			"per offer, and the resource reads them.",

		Attributes: map[string]schema.Attribute{
			"subscription_id": schema.StringAttribute{
				MarkdownDescription: "The ID of the subscription whose win-back offers to list.",
				Required:            true,
			},
			"offer_id": schema.StringAttribute{
				MarkdownDescription: "Return only the offer with this `offer_id`, matched exactly.",
				Optional:            true,
			},
			"win_back_offers": schema.ListNestedAttribute{
				MarkdownDescription: "The win-back offers, sorted by `offer_id`.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							MarkdownDescription: "Apple's ID for the offer.",
							Computed:            true,
						},
						"reference_name": schema.StringAttribute{
							MarkdownDescription: "The offer's name inside App Store Connect.",
							Computed:            true,
						},
						"offer_id": schema.StringAttribute{
							MarkdownDescription: "The identifier StoreKit transactions and reports carry.",
							Computed:            true,
						},
						"duration": schema.StringAttribute{
							MarkdownDescription: "The length of one offer period, such as `ONE_MONTH`.",
							Computed:            true,
						},
						"offer_mode": schema.StringAttribute{
							MarkdownDescription: "`PAY_AS_YOU_GO`, `PAY_UP_FRONT` or `FREE_TRIAL`.",
							Computed:            true,
						},
						"period_count": schema.Int64Attribute{
							MarkdownDescription: "How many periods the offer lasts.",
							Computed:            true,
						},
						"customer_eligibility_paid_subscription_duration_in_months": schema.Int64Attribute{
							MarkdownDescription: "How long a customer must have paid before lapsing, in months.",
							Computed:            true,
						},
						"customer_eligibility_time_since_last_subscribed_in_months": schema.SingleNestedAttribute{
							MarkdownDescription: "How long ago the customer must have lapsed, in months, inclusive.",
							Computed:            true,
							Attributes: map[string]schema.Attribute{
								"minimum": schema.Int64Attribute{
									MarkdownDescription: "The fewest months since lapsing.",
									Computed:            true,
								},
								"maximum": schema.Int64Attribute{
									MarkdownDescription: "The most months since lapsing.",
									Computed:            true,
								},
							},
						},
						"customer_eligibility_wait_between_offers_in_months": schema.Int64Attribute{
							MarkdownDescription: "How long a customer must wait before redeeming the offer again, " +
								"in months; null for no wait.",
							Computed: true,
						},
						"start_date": schema.StringAttribute{
							MarkdownDescription: "The first day the offer is available (`YYYY-MM-DD`).",
							Computed:            true,
						},
						"end_date": schema.StringAttribute{
							MarkdownDescription: "The last day the offer is available (`YYYY-MM-DD`); null for none.",
							Computed:            true,
						},
						"priority": schema.StringAttribute{
							MarkdownDescription: "`HIGH` or `NORMAL`.",
							Computed:            true,
						},
						"promotion_intent": schema.StringAttribute{
							MarkdownDescription: "`USE_AUTO_GENERATED_ASSETS` or `NOT_PROMOTED`.",
							Computed:            true,
						},
						"target_subscription_plan_type": schema.StringAttribute{
							MarkdownDescription: "`MONTHLY` or `UPFRONT`, for a subscription sold both ways.",
							Computed:            true,
						},
					},
				},
			},
			"total_count": schema.Int64Attribute{
				MarkdownDescription: "Number of win-back offers the subscription has, before `offer_id` narrows them.",
				Computed:            true,
			},
		},
	}
}

// Read lists the subscription's win-back offers.
func (d *winBackOffersDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	tflog.Info(ctx, "Reading subscription win-back offers data source")

	var config winBackOffersDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	subscriptionID := config.SubscriptionID.ValueString()
	ctx = tflog.SetField(ctx, "subscription_id", subscriptionID)

	offers, err := d.client.GetWinBackOffers(subscriptionID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Read Win-Back Offers",
			fmt.Sprintf("Could not list the win-back offers of subscription '%s': %s", subscriptionID, err.Error()),
		)
		return
	}

	listings := make([]winBackOfferListingModel, 0, len(offers))
	for _, offer := range offers {
		if !config.OfferID.IsNull() && offer.Attributes.OfferID != config.OfferID.ValueString() {
			continue
		}
		listings = append(listings, winBackOfferListing(offer))
	}
	sort.Slice(listings, func(i, j int) bool {
		return listings[i].OfferID.ValueString() < listings[j].OfferID.ValueString()
	})

	config.WinBackOffers = listings
	config.TotalCount = types.Int64Value(int64(len(offers)))

	tflog.Info(ctx, "Subscription win-back offers data source read completed", map[string]interface{}{
		"total_count":    len(offers),
		"filtered_count": len(listings),
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

// Configure adds the provider configured client to the data source.
func (d *winBackOffersDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
