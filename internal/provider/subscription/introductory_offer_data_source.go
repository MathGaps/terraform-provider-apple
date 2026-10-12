// Copyright (c) AO Studio
// SPDX-License-Identifier: MPL-2.0

package subscription

import (
	"context"
	"fmt"
	"sort"

	"github.com/AhmedOsman00/terraform-provider-apple/internal/apple"
	"github.com/AhmedOsman00/terraform-provider-apple/internal/apple/models"

	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var (
	_ datasource.DataSource              = &subscriptionIntroductoryOffersDataSource{}
	_ datasource.DataSourceWithConfigure = &subscriptionIntroductoryOffersDataSource{}
)

func NewSubscriptionIntroductoryOffersDataSource() datasource.DataSource {
	return &subscriptionIntroductoryOffersDataSource{}
}

type subscriptionIntroductoryOffersDataSource struct {
	client *apple.Client
}

// Metadata returns the data source type name.
func (d *subscriptionIntroductoryOffersDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_subscription_introductory_offers"
}

// Schema defines the schema for the data source.
func (d *subscriptionIntroductoryOffersDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists the introductory offer records of an auto-renewable subscription — one " +
			"per territory, the way Apple keeps them, whoever made them.\n\n" +
			"This is how an offer made in App Store Connect is found: its record IDs are what " +
			"`apple_subscription_introductory_offer` imports from (`<subscription_id>/<offer_id>`), and " +
			"Apple publishes no `GET` for a single offer, so the subscription's collection is the only " +
			"place they appear. It is also how to see what already occupies a territory — a territory " +
			"holds one introductory offer at a time, and an overlapping one is refused.",

		Attributes: map[string]schema.Attribute{
			"subscription_id": schema.StringAttribute{
				MarkdownDescription: "The ID of the subscription whose introductory offers to list.",
				Required:            true,
			},
			"territories": schema.SetAttribute{
				MarkdownDescription: "Three-letter Apple territory codes to restrict the listing to, for " +
					"example `[\"USA\", \"GBR\"]`. Applied by Apple, not in memory.",
				Optional:    true,
				ElementType: types.StringType,
				Validators: []validator.Set{
					setvalidator.ValueStringsAre(TerritoryValidator),
				},
			},
			"offer_mode": schema.StringAttribute{
				MarkdownDescription: "Return only records of this mode: `FREE_TRIAL`, `PAY_AS_YOU_GO` or " +
					"`PAY_UP_FRONT`. Applied in memory.",
				Optional:   true,
				Validators: []validator.String{OfferModeValidator},
			},
			"introductory_offers": schema.ListNestedAttribute{
				MarkdownDescription: "The matching records, ordered by territory and then start date.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							MarkdownDescription: "The Apple ID of the record.",
							Computed:            true,
						},
						"territory_id": schema.StringAttribute{
							MarkdownDescription: "The three-letter Apple territory code the record " +
								"applies in. Null when Apple reports none.",
							Computed: true,
						},
						"price_point_id": schema.StringAttribute{
							MarkdownDescription: "The price point a paid offer charges in this " +
								"territory. Null for a free trial.",
							Computed: true,
						},
						"offer_mode": schema.StringAttribute{
							MarkdownDescription: "`FREE_TRIAL`, `PAY_AS_YOU_GO` or `PAY_UP_FRONT`.",
							Computed:            true,
						},
						"duration": schema.StringAttribute{
							MarkdownDescription: "How long one period of the offer lasts, for example `ONE_WEEK`.",
							Computed:            true,
						},
						"number_of_periods": schema.Int64Attribute{
							MarkdownDescription: "How many periods the offer runs for.",
							Computed:            true,
						},
						"start_date": schema.StringAttribute{
							MarkdownDescription: "The date the offer becomes available, or null for one " +
								"available from creation.",
							Computed: true,
						},
						"end_date": schema.StringAttribute{
							MarkdownDescription: "The last date the offer is available, or null for one " +
								"with no end.",
							Computed: true,
						},
						"target_subscription_plan_type": schema.StringAttribute{
							MarkdownDescription: "`MONTHLY` or `UPFRONT` when the offer targets one plan " +
								"of the subscription; null otherwise.",
							Computed: true,
						},
					},
				},
			},
			"total_count": schema.Int64Attribute{
				MarkdownDescription: "Number of records Apple returned before the in-memory `offer_mode` filter.",
				Computed:            true,
			},
		},
	}
}

// Read fetches and filters the offers.
func (d *subscriptionIntroductoryOffersDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	tflog.Info(ctx, "Reading subscription introductory offers data source")

	var config subscriptionIntroductoryOffersDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	subscriptionID := config.SubscriptionID.ValueString()
	ctx = tflog.SetField(ctx, "subscription_id", subscriptionID)

	var territories []string
	if !config.Territories.IsNull() && !config.Territories.IsUnknown() {
		resp.Diagnostics.Append(config.Territories.ElementsAs(ctx, &territories, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		sort.Strings(territories)
	}

	offers, err := d.client.GetSubscriptionIntroductoryOffers(subscriptionID, territories)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Read Subscription Introductory Offers",
			fmt.Sprintf("Could not retrieve the introductory offers of subscription '%s': %s", subscriptionID, err.Error()),
		)
		return
	}

	filtered := FilterIntroductoryOffers(offers, config.OfferMode.ValueString())

	offerModels := make([]subscriptionIntroductoryOfferListModel, len(filtered))
	for i, offer := range filtered {
		offerModels[i] = introductoryOfferListModel(offer)
	}

	config.IntroductoryOffers = offerModels
	config.TotalCount = types.Int64Value(int64(len(offers)))

	tflog.Info(ctx, "Subscription introductory offers data source read completed", map[string]interface{}{
		"total_count":    len(offers),
		"returned_count": len(offerModels),
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

// Configure adds the provider configured client to the data source.
func (d *subscriptionIntroductoryOffersDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

// FilterIntroductoryOffers keeps the records of one offer mode (all of them
// when mode is empty) and orders them by territory, then start date, then ID,
// so the listing is the same on every read whatever order Apple answered in.
func FilterIntroductoryOffers(offers []models.SubscriptionIntroductoryOffer, mode string) []models.SubscriptionIntroductoryOffer {
	filtered := make([]models.SubscriptionIntroductoryOffer, 0, len(offers))
	for _, offer := range offers {
		if mode != "" && string(offer.Attributes.OfferMode) != mode {
			continue
		}
		filtered = append(filtered, offer)
	}

	sort.SliceStable(filtered, func(i, j int) bool {
		a, b := filtered[i], filtered[j]
		if ta, tb := offerTerritoryID(a), offerTerritoryID(b); ta != tb {
			return ta < tb
		}
		if sa, sb := dateOrEmpty(a.Attributes.StartDate), dateOrEmpty(b.Attributes.StartDate); sa != sb {
			return sa < sb
		}
		return a.ID < b.ID
	})

	return filtered
}

// introductoryOfferListModel converts one record for the data source.
func introductoryOfferListModel(offer models.SubscriptionIntroductoryOffer) subscriptionIntroductoryOfferListModel {
	model := subscriptionIntroductoryOfferListModel{
		ID:                         types.StringValue(offer.ID),
		TerritoryID:                types.StringNull(),
		PricePointID:               types.StringNull(),
		OfferMode:                  types.StringValue(string(offer.Attributes.OfferMode)),
		Duration:                   types.StringValue(string(offer.Attributes.Duration)),
		NumberOfPeriods:            types.Int64Value(int64(offer.Attributes.NumberOfPeriods)),
		StartDate:                  stringOrNull(offer.Attributes.StartDate),
		EndDate:                    stringOrNull(offer.Attributes.EndDate),
		TargetSubscriptionPlanType: types.StringNull(),
	}

	if territory := offerTerritoryID(offer); territory != "" {
		model.TerritoryID = types.StringValue(territory)
	}
	if pricePoint := offerPricePointID(offer); pricePoint != "" {
		model.PricePointID = types.StringValue(pricePoint)
	}
	if planType := offer.Attributes.TargetSubscriptionPlanType; planType != nil {
		model.TargetSubscriptionPlanType = types.StringValue(string(*planType))
	}

	return model
}

func dateOrEmpty(date *string) string {
	if date == nil {
		return ""
	}
	return *date
}
