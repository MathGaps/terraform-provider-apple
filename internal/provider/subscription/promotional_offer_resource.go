// Copyright (c) AO Studio
// SPDX-License-Identifier: MPL-2.0

package subscription

import (
	"context"
	"fmt"
	"strings"

	"github.com/AhmedOsman00/terraform-provider-apple/internal/apple"
	"github.com/AhmedOsman00/terraform-provider-apple/internal/apple/models"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var (
	_ resource.Resource                   = &subscriptionPromotionalOfferResource{}
	_ resource.ResourceWithConfigure      = &subscriptionPromotionalOfferResource{}
	_ resource.ResourceWithImportState    = &subscriptionPromotionalOfferResource{}
	_ resource.ResourceWithValidateConfig = &subscriptionPromotionalOfferResource{}
)

func NewSubscriptionPromotionalOfferResource() resource.Resource {
	return &subscriptionPromotionalOfferResource{}
}

type subscriptionPromotionalOfferResource struct {
	client *apple.Client
}

// Metadata returns the resource type name.
func (r *subscriptionPromotionalOfferResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_subscription_promotional_offer"
}

// Schema defines the schema for the resource.
func (r *subscriptionPromotionalOfferResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a promotional offer on an auto-renewable subscription: a discount the app " +
			"presents to a customer who has, or had, the subscription.\n\n" +
			"The App Store never shows a promotional offer on its own. The app asks for it at purchase time " +
			"by `offer_code`, together with a signature its server computes with an In-App Purchase key, so " +
			"`offer_code` is the value the app's code and this resource have to agree on. Who is eligible is " +
			"the app's decision, not Apple's.\n\n" +
			"Apple's update request carries the prices and nothing else, so every other attribute is fixed " +
			"at creation and changing one replaces the offer. The `prices` set changes in place.\n\n" +
			"~> **The subscription has to be available in every territory the offer is priced in.** An " +
			"offer price references the subscription's own price point catalogue and territories, so an " +
			"`apple_subscription_availability` covering them has to exist first. Nothing in the offer " +
			"references it, so declare the ordering with `depends_on`.\n\n" +
			"~> **Treat an offer code as spent once used.** It is the offer's identifier, so expect Apple " +
			"to refuse one already used on the subscription; Apple's API documents neither that rule nor " +
			"whether deleting an offer ever releases its code.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The unique Apple-generated identifier for the promotional offer.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"subscription_id": schema.StringAttribute{
				MarkdownDescription: "The ID of the `apple_subscription` this offer discounts. Cannot be changed " +
					"after creation.",
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The offer's reference name, internal to App Store Connect. Customers never " +
					"see it. Cannot be changed after creation — Apple's update request has no `name` member.",
				Required:   true,
				Validators: []validator.String{stringvalidator.UTF8LengthAtLeast(1)},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"offer_code": schema.StringAttribute{
				MarkdownDescription: "The identifier the app passes to the App Store to request this offer, and " +
					"that its server signs. Letters, digits, dots, underscores and hyphens. Expect Apple to refuse " +
					"a code already used on the subscription; whether deleting the offer releases it is " +
					"undocumented, so treat a code as spent once used. Cannot be changed after creation.",
				Required:   true,
				Validators: GetOfferCodeValidator(),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"duration": schema.StringAttribute{
				MarkdownDescription: "The length of one offer period: `THREE_DAYS`, `ONE_WEEK`, `TWO_WEEKS`, " +
					"`ONE_MONTH`, `TWO_MONTHS`, `THREE_MONTHS`, `SIX_MONTHS` or `ONE_YEAR`. With " +
					"`PAY_AS_YOU_GO` it is the billing period being discounted; with `PAY_UP_FRONT` and " +
					"`FREE_TRIAL` it is the whole offer. Cannot be changed after creation.",
				Required:   true,
				Validators: []validator.String{OfferDurationValidator},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"offer_mode": schema.StringAttribute{
				MarkdownDescription: "How the customer pays during the offer: `PAY_AS_YOU_GO` (a discounted " +
					"price each period), `PAY_UP_FRONT` (one discounted price for the whole offer) or " +
					"`FREE_TRIAL` (nothing). Cannot be changed after creation.",
				Required:   true,
				Validators: []validator.String{OfferModeValidator},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"number_of_periods": schema.Int64Attribute{
				MarkdownDescription: "How many `duration` periods the offer lasts. For `PAY_UP_FRONT` and " +
					"`FREE_TRIAL` this is normally `1`. Cannot be changed after creation.",
				Required:   true,
				Validators: []validator.Int64{int64validator.AtLeast(1)},
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"target_subscription_plan_type": schema.StringAttribute{
				MarkdownDescription: "Which of the subscription's plans the offer applies to: `MONTHLY` or " +
					"`UPFRONT`, matching `plan_type` on the subscription's prices. Optional; when it is left " +
					"unset, whatever Apple reports is kept. Cannot be changed after creation.",
				Optional:   true,
				Computed:   true,
				Validators: []validator.String{PlanTypeValidator},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
			},
			"prices": schema.SetNestedAttribute{
				MarkdownDescription: "The offer's price in each territory it is available in, one entry per " +
					"territory. A set, because Apple returns them in no particular order. At least one is " +
					"required — Apple requires the prices on every create, a free trial included. Changes " +
					"in place: every write replaces the whole set.",
				Required: true,
				Validators: []validator.Set{
					setvalidator.SizeAtLeast(1),
				},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"territory_id": schema.StringAttribute{
							MarkdownDescription: "The three-letter Apple territory code this price applies to, " +
								"for example `USA`, `GBR` or `EGY`.",
							Required:   true,
							Validators: []validator.String{TerritoryValidator},
						},
						"price_point_id": schema.StringAttribute{
							MarkdownDescription: "The ID of the subscription price point the customer pays " +
								"during the offer, from `apple_subscription_price_points` or " +
								"`apple_subscription_price_point_equalizations` for the same subscription. " +
								"Required unless `offer_mode` is `FREE_TRIAL`, where a territory carries no " +
								"price and this is omitted.",
							Optional: true,
						},
					},
				},
			},
		},
	}
}

// ValidateConfig checks what the schema cannot express on its own: a paid
// offer needs a price point in every territory, and an offer has one price per
// territory.
func (r *subscriptionPromotionalOfferResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config subscriptionPromotionalOfferModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if config.Prices.IsNull() || config.Prices.IsUnknown() {
		return
	}

	var prices []promotionalOfferPriceModel
	resp.Diagnostics.Append(config.Prices.ElementsAs(ctx, &prices, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	paid := !config.OfferMode.IsUnknown() && !config.OfferMode.IsNull() &&
		config.OfferMode.ValueString() != string(models.SubscriptionOfferModeFreeTrial)

	seen := make(map[string]bool, len(prices))
	for _, price := range prices {
		if price.TerritoryID.IsUnknown() {
			continue
		}
		territory := price.TerritoryID.ValueString()

		if seen[territory] {
			resp.Diagnostics.AddAttributeError(
				path.Root("prices"),
				"Duplicate Territory",
				fmt.Sprintf("Territory '%s' appears more than once in prices. An offer has one price per "+
					"territory.", territory),
			)
		}
		seen[territory] = true

		if paid && price.PricePointID.IsNull() {
			resp.Diagnostics.AddAttributeError(
				path.Root("prices"),
				"Missing Price Point",
				fmt.Sprintf("The price for territory '%s' has no price_point_id, and an offer_mode of '%s' "+
					"charges the customer something. Only a FREE_TRIAL offer omits the price point.",
					territory, config.OfferMode.ValueString()),
			)
		}
	}
}

// Create a new resource.
func (r *subscriptionPromotionalOfferResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	tflog.Info(ctx, "Creating subscription promotional offer resource")

	var plan subscriptionPromotionalOfferModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	subscriptionID := plan.SubscriptionID.ValueString()
	offerCode := plan.OfferCode.ValueString()
	ctx = tflog.SetField(ctx, "subscription_id", subscriptionID)
	ctx = tflog.SetField(ctx, "offer_code", offerCode)

	prices := r.planPrices(ctx, plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	attributes := models.SubscriptionPromotionalOfferCreateAttributes{
		Duration:        models.SubscriptionOfferDuration(plan.Duration.ValueString()),
		Name:            plan.Name.ValueString(),
		NumberOfPeriods: int(plan.NumberOfPeriods.ValueInt64()),
		OfferCode:       offerCode,
		OfferMode:       models.SubscriptionOfferMode(plan.OfferMode.ValueString()),
	}
	if planType := stringOrNil(plan.TargetSubscriptionPlanType); planType != nil {
		t := models.SubscriptionPlanType(*planType)
		attributes.TargetSubscriptionPlanType = &t
	}

	tflog.Debug(ctx, "Creating promotional offer", map[string]interface{}{"price_count": len(prices)})

	offer, err := r.client.CreateSubscriptionPromotionalOffer(subscriptionID, attributes, prices, nil)
	if err != nil {
		errMsg := err.Error()
		switch {
		case strings.Contains(errMsg, "already exists") || strings.Contains(errMsg, "already being used") ||
			strings.Contains(errMsg, "DUPLICATE") || strings.Contains(errMsg, "unique"):
			resp.Diagnostics.AddError(
				"Offer Code Already Used",
				fmt.Sprintf("Apple refused offer code '%s' on subscription '%s': %s\n\n"+
					"An offer code identifies the offer on its subscription. If an offer with this "+
					"code exists, import it as \"%s/%s\"; if one was deleted, choose a new code — Apple does "+
					"not document whether a deleted offer's code is ever released.",
					offerCode, subscriptionID, errMsg, subscriptionID, offerCode),
			)
		case strings.Contains(errMsg, "not found") || strings.Contains(errMsg, "404"):
			resp.Diagnostics.AddError(
				"Subscription or Price Point Not Found",
				fmt.Sprintf("Could not create promotional offer '%s' on subscription '%s': %s\n\n"+
					"A price point ID is only valid for the subscription it was read from. Read it from "+
					"apple_subscription_price_points or apple_subscription_price_point_equalizations scoped "+
					"to subscription '%s'.", offerCode, subscriptionID, errMsg, subscriptionID),
			)
		default:
			resp.Diagnostics.AddError(
				"Error Creating Subscription Promotional Offer",
				fmt.Sprintf("Could not create promotional offer '%s' on subscription '%s': %s\n\n"+
					"Every territory in prices has to be one the subscription is available in, through an "+
					"apple_subscription_availability this resource depends on, and every price point has to "+
					"belong to this subscription and to the territory it is listed under.",
					offerCode, subscriptionID, errMsg),
			)
		}
		tflog.Error(ctx, "Failed to create subscription promotional offer", map[string]interface{}{"error": errMsg})
		return
	}

	plan.ID = types.StringValue(offer.ID)
	if plan.TargetSubscriptionPlanType.IsUnknown() {
		plan.TargetSubscriptionPlanType = types.StringNull()
		if offer.Attributes.TargetSubscriptionPlanType != nil {
			plan.TargetSubscriptionPlanType = types.StringValue(string(*offer.Attributes.TargetSubscriptionPlanType))
		}
	}

	tflog.Info(ctx, "Subscription promotional offer created", map[string]interface{}{"offer_id": offer.ID})

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// Read refreshes the Terraform state with the latest data.
//
// Two requests: the offer, and its prices from their own collection -- the
// offer's include=prices is capped at 50, fewer than the storefronts an offer
// can be priced in.
func (r *subscriptionPromotionalOfferResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	tflog.Debug(ctx, "Reading subscription promotional offer resource")

	var state subscriptionPromotionalOfferModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	offerID := state.ID.ValueString()
	ctx = tflog.SetField(ctx, "offer_id", offerID)

	offer, err := r.client.GetSubscriptionPromotionalOffer(offerID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "404") {
			tflog.Info(ctx, "Subscription promotional offer not found, removing from state")
			resp.State.RemoveResource(ctx)
			return
		}

		resp.Diagnostics.AddError(
			"Error Reading Subscription Promotional Offer",
			fmt.Sprintf("Could not read promotional offer '%s': %s", offerID, err.Error()),
		)
		return
	}

	applyPromotionalOfferAttributes(&state, offer)

	if state.SubscriptionID.IsNull() || state.SubscriptionID.ValueString() == "" {
		state.SubscriptionID = promotionalOfferSubscriptionID(offer)
	}

	var current []promotionalOfferPriceModel
	if !state.Prices.IsNull() && !state.Prices.IsUnknown() {
		resp.Diagnostics.Append(state.Prices.ElementsAs(ctx, &current, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	prices, diags := r.readPrices(ctx, offerID, current)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	state.Prices = prices

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update replaces the offer's prices, the only thing Apple's update request
// can change. Everything else is RequiresReplace and never reaches here.
func (r *subscriptionPromotionalOfferResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	tflog.Info(ctx, "Updating subscription promotional offer resource")

	var plan subscriptionPromotionalOfferModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	offerID := plan.ID.ValueString()
	ctx = tflog.SetField(ctx, "offer_id", offerID)

	prices := r.planPrices(ctx, plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Replacing promotional offer prices", map[string]interface{}{"price_count": len(prices)})

	if _, err := r.client.SetSubscriptionPromotionalOfferPrices(offerID, prices, nil); err != nil {
		resp.Diagnostics.AddError(
			"Error Updating Subscription Promotional Offer",
			fmt.Sprintf("Could not replace the prices of promotional offer '%s': %s\n\n"+
				"Every territory in prices has to be one the subscription is available in, and every price "+
				"point has to belong to subscription '%s' and to the territory it is listed under.",
				offerID, err.Error(), plan.SubscriptionID.ValueString()),
		)
		tflog.Error(ctx, "Failed to update subscription promotional offer", map[string]interface{}{"error": err.Error()})
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// Delete deletes the offer. Apple publishes DELETE /v1/subscriptionPromotionalOffers/{id},
// so unlike the price schedule this is a real deletion.
func (r *subscriptionPromotionalOfferResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	tflog.Info(ctx, "Deleting subscription promotional offer resource")

	var state subscriptionPromotionalOfferModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	offerID := state.ID.ValueString()
	ctx = tflog.SetField(ctx, "offer_id", offerID)

	if err := r.client.DeleteSubscriptionPromotionalOffer(offerID, nil); err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "404") {
			tflog.Info(ctx, "Subscription promotional offer already deleted")
			return
		}

		resp.Diagnostics.AddError(
			"Error Deleting Subscription Promotional Offer",
			fmt.Sprintf("Could not delete promotional offer '%s' (offer code '%s'): %s",
				offerID, state.OfferCode.ValueString(), err.Error()),
		)
		return
	}

	tflog.Info(ctx, "Subscription promotional offer deleted successfully")
}

// Configure adds the provider configured client to the resource.
func (r *subscriptionPromotionalOfferResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*apple.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *apple.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	r.client = client
}

// ImportState imports an existing promotional offer.
//
// A bare offer ID is accepted because GetSubscriptionPromotionalOffer asks for
// include=subscription, so the parent comes back with it. The composite
// "<subscription_id>/<offer_code>" form is accepted too, for an offer made in
// App Store Connect: the code is the value the app already holds, and the
// opaque ID is not shown anywhere a person would look. Offer codes cannot
// contain "/", so the two forms do not overlap.
func (r *subscriptionPromotionalOfferResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	tflog.Info(ctx, "Importing subscription promotional offer resource", map[string]interface{}{"import_id": req.ID})

	offerID := req.ID
	subscriptionID, offerCode, composite := strings.Cut(req.ID, "/")
	if composite {
		found, err := r.client.GetSubscriptionPromotionalOfferByCode(subscriptionID, offerCode)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error Importing Subscription Promotional Offer",
				fmt.Sprintf("Could not find offer code '%s' on subscription '%s': %s\n\n"+
					"Import ID should be either the offer ID on its own, or \"<subscription_id>/<offer_code>\".",
					offerCode, subscriptionID, err.Error()),
			)
			return
		}
		offerID = found.ID
	}

	offer, err := r.client.GetSubscriptionPromotionalOffer(offerID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Importing Subscription Promotional Offer",
			fmt.Sprintf("Could not import promotional offer '%s': %s\n\n"+
				"Import ID should be either the offer ID on its own, or \"<subscription_id>/<offer_code>\".",
				offerID, err.Error()),
		)
		return
	}

	state := subscriptionPromotionalOfferModel{
		ID:                         types.StringValue(offer.ID),
		SubscriptionID:             promotionalOfferSubscriptionID(offer),
		TargetSubscriptionPlanType: types.StringNull(),
	}
	if composite {
		state.SubscriptionID = types.StringValue(subscriptionID)
	}
	if state.SubscriptionID.IsNull() {
		resp.Diagnostics.AddError(
			"Incomplete Promotional Offer Import",
			fmt.Sprintf("Apple did not report a parent subscription for promotional offer '%s'. Import it as "+
				"\"<subscription_id>/<offer_code>\" instead.", offerID),
		)
		return
	}
	applyPromotionalOfferAttributes(&state, offer)

	prices, diags := r.readPrices(ctx, offer.ID, nil)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	state.Prices = prices

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// planPrices reads the planned price set into client input.
func (r *subscriptionPromotionalOfferResource) planPrices(ctx context.Context, plan subscriptionPromotionalOfferModel, diags *diag.Diagnostics) []apple.SubscriptionPromotionalOfferPrice {
	var prices []promotionalOfferPriceModel
	diags.Append(plan.Prices.ElementsAs(ctx, &prices, false)...)
	return promotionalOfferPricesToAPI(prices)
}

// readPrices reads the offer's prices from Apple and reconciles them against
// the set state holds.
func (r *subscriptionPromotionalOfferResource) readPrices(ctx context.Context, offerID string, current []promotionalOfferPriceModel) (types.Set, diag.Diagnostics) {
	var diags diag.Diagnostics

	reported, err := r.client.GetSubscriptionPromotionalOfferPrices(offerID)
	if err != nil {
		diags.AddError(
			"Error Reading Subscription Promotional Offer Prices",
			fmt.Sprintf("Could not read the prices of promotional offer '%s': %s", offerID, err.Error()),
		)
		return types.SetNull(promotionalOfferPriceObjectType), diags
	}

	reconciled := reconcilePromotionalOfferPrices(current, reported)
	set, setDiags := types.SetValueFrom(ctx, promotionalOfferPriceObjectType, reconciled)
	diags.Append(setDiags...)
	return set, diags
}

// applyPromotionalOfferAttributes copies Apple's reported attributes into a
// model. A value Apple omits leaves the model's value alone, so an attribute
// missing from one response does not read as a change to a RequiresReplace
// attribute.
func applyPromotionalOfferAttributes(model *subscriptionPromotionalOfferModel, offer *models.SubscriptionPromotionalOffer) {
	summary := promotionalOfferSummary(*offer)

	if offer.Attributes.Name != "" {
		model.Name = summary.Name
	}
	if offer.Attributes.OfferCode != "" {
		model.OfferCode = summary.OfferCode
	}
	if !summary.Duration.IsNull() {
		model.Duration = summary.Duration
	}
	if !summary.OfferMode.IsNull() {
		model.OfferMode = summary.OfferMode
	}
	if !summary.NumberOfPeriods.IsNull() {
		model.NumberOfPeriods = summary.NumberOfPeriods
	}
	if !summary.TargetSubscriptionPlanType.IsNull() {
		model.TargetSubscriptionPlanType = summary.TargetSubscriptionPlanType
	} else if model.TargetSubscriptionPlanType.IsUnknown() {
		model.TargetSubscriptionPlanType = types.StringNull()
	}
}

// promotionalOfferSubscriptionID reads the parent out of an offer read with
// include=subscription, returning null when Apple did not populate it.
func promotionalOfferSubscriptionID(offer *models.SubscriptionPromotionalOffer) types.String {
	if offer.Relationships == nil || offer.Relationships.Subscription == nil ||
		offer.Relationships.Subscription.Data.ID == "" {
		return types.StringNull()
	}
	return types.StringValue(offer.Relationships.Subscription.Data.ID)
}
