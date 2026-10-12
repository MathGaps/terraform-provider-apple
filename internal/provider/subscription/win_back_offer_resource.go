// Copyright (c) AO Studio
// SPDX-License-Identifier: MPL-2.0

package subscription

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/AhmedOsman00/terraform-provider-apple/internal/apple"
	"github.com/AhmedOsman00/terraform-provider-apple/internal/apple/models"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var (
	_ resource.Resource                   = &winBackOfferResource{}
	_ resource.ResourceWithConfigure      = &winBackOfferResource{}
	_ resource.ResourceWithImportState    = &winBackOfferResource{}
	_ resource.ResourceWithValidateConfig = &winBackOfferResource{}
	_ resource.ResourceWithModifyPlan     = &winBackOfferResource{}
)

func NewWinBackOfferResource() resource.Resource {
	return &winBackOfferResource{}
}

type winBackOfferResource struct {
	client *apple.Client
}

// Metadata returns the resource type name.
func (r *winBackOfferResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_subscription_win_back_offer"
}

// Schema defines the schema for the resource.
func (r *winBackOfferResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	immutable := []planmodifier.String{stringplanmodifier.RequiresReplace()}

	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a win-back offer: a discounted or free period the App Store offers to " +
			"customers whose subscription has lapsed, shown on their Manage Subscriptions page, on the " +
			"win-back offer sheet inside the app, and — if promoted — on the App Store itself.\n\n" +
			"An offer belongs to one subscription, not to the group: a customer is eligible only for offers " +
			"on the product they most recently lapsed from within each group, so reaching lapsed subscribers " +
			"of several plans takes one offer per plan.\n\n" +
			"~> **The subscription has to have been approved by App Review first.** Apple refuses to create " +
			"a win-back offer for one that has never been approved.\n\n" +
			"~> **The prices cannot change after creation.** Apple's update request carries no prices, so " +
			"every change to `prices` destroys the offer and creates a new one — and the new one cannot " +
			"reuse the old `offer_id` (see below). The same is true of `reference_name`, `offer_id`, " +
			"`duration`, `offer_mode`, `period_count` and `target_subscription_plan_type`. The eligibility " +
			"rules, the dates, `priority` and `promotion_intent` change in place, though Apple refuses some " +
			"of those changes once the offer is live.\n\n" +
			"Apple allows at most 350 win-back offers per subscription, and at most 5 running at once per " +
			"storefront.\n\n" +
			"`terraform destroy` deletes the offer, which Apple allows at any time; a customer holding a " +
			"link to it is told the offer is no longer available.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Apple's ID for the win-back offer.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"subscription_id": schema.StringAttribute{
				MarkdownDescription: "The ID of the `apple_subscription` the offer wins customers back to. " +
					"Changing it forces a new offer. Apple never reports it back — an offer has no " +
					"subscription relationship — so it is kept as configured.",
				Required:      true,
				PlanModifiers: immutable,
			},
			"reference_name": schema.StringAttribute{
				MarkdownDescription: "The offer's name inside App Store Connect; customers never see it. Must " +
					"not have been used by another offer of any kind. Changing it forces a new offer.",
				Required:      true,
				Validators:    GetWinBackOfferStringValidators(),
				PlanModifiers: immutable,
			},
			"offer_id": schema.StringAttribute{
				MarkdownDescription: "The identifier StoreKit transactions and Sales and Trends reports carry " +
					"for this offer. It must not have been used by another offer in the same subscription " +
					"group — Apple's documentation adds promotional offers, offer codes and introductory " +
					"offers to that — and **treat it as reserved once used**: Apple says it must not have " +
					"*already been used*, not that it must not be in use, so a replacement needs a new one. " +
					"Changing it forces a new offer.",
				Required:      true,
				Validators:    GetWinBackOfferStringValidators(),
				PlanModifiers: immutable,
			},
			"duration": schema.StringAttribute{
				MarkdownDescription: "The length of one offer period: `THREE_DAYS`, `ONE_WEEK`, `TWO_WEEKS`, " +
					"`ONE_MONTH`, `TWO_MONTHS`, `THREE_MONTHS`, `SIX_MONTHS` or `ONE_YEAR`. Changing it forces " +
					"a new offer.",
				Required:      true,
				Validators:    []validator.String{WinBackOfferDurationValidator},
				PlanModifiers: immutable,
			},
			"offer_mode": schema.StringAttribute{
				MarkdownDescription: "How the customer pays: `PAY_AS_YOU_GO` (the offer price each period " +
					"for `period_count` periods), `PAY_UP_FRONT` (the offer price once, for the whole " +
					"duration) or `FREE_TRIAL`. Changing it forces a new offer.",
				Required:      true,
				Validators:    []validator.String{WinBackOfferModeValidator},
				PlanModifiers: immutable,
			},
			"period_count": schema.Int64Attribute{
				MarkdownDescription: "How many `duration` periods the offer lasts. Changing it forces a new offer.",
				Required:            true,
				Validators:          []validator.Int64{int64validator.AtLeast(1)},
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.RequiresReplace()},
			},
			"customer_eligibility_paid_subscription_duration_in_months": schema.Int64Attribute{
				MarkdownDescription: "How long a customer must have paid for the subscription before it " +
					"lapsed to be eligible, in months: any value from `1` to `24`, or `36`, `48` or `60`.",
				Required:   true,
				Validators: []validator.Int64{WinBackOfferPaidDurationValidator},
			},
			"customer_eligibility_time_since_last_subscribed_in_months": schema.SingleNestedAttribute{
				MarkdownDescription: "How long ago the customer's subscription must have lapsed, in months, " +
					"inclusive at both ends. `{ minimum = 5, maximum = 11 }` shows the offer only to customers " +
					"lapsed for five to eleven months.",
				Required: true,
				Attributes: map[string]schema.Attribute{
					"minimum": schema.Int64Attribute{
						MarkdownDescription: "The fewest months since the subscription lapsed, from `1` to `24`.",
						Required:            true,
						Validators:          []validator.Int64{WinBackOfferLapsedMonthsValidator},
					},
					"maximum": schema.Int64Attribute{
						MarkdownDescription: "The most months since the subscription lapsed, from `1` to `24`, " +
							"and at least `minimum`.",
						Required:   true,
						Validators: []validator.Int64{WinBackOfferLapsedMonthsValidator},
					},
				},
			},
			"customer_eligibility_wait_between_offers_in_months": schema.Int64Attribute{
				MarkdownDescription: "How long a customer must wait after this offer ends before they can " +
					"redeem it again, in months, from `2` to `24`. Omit it for no wait.",
				Optional:   true,
				Validators: []validator.Int64{WinBackOfferWaitValidator},
			},
			"start_date": schema.StringAttribute{
				MarkdownDescription: "The first day the offer is available, as a plain `YYYY-MM-DD` date. Apple " +
					"accepts no earlier than tomorrow, and the exact start time varies by region.",
				Required:   true,
				Validators: []validator.String{WinBackOfferDateValidator},
			},
			"end_date": schema.StringAttribute{
				MarkdownDescription: "The last day the offer is available, as a plain `YYYY-MM-DD` date. Apple " +
					"requires an offer to run for at least three days. Omit it for an offer with no end date.",
				Optional:   true,
				Validators: []validator.String{WinBackOfferDateValidator},
			},
			"priority": schema.StringAttribute{
				MarkdownDescription: "How much this offer matters to you, which Apple weighs when choosing " +
					"between overlapping offers and when ranking it on the App Store: `HIGH` or `NORMAL`.",
				Required:   true,
				Validators: []validator.String{WinBackOfferPriorityValidator},
			},
			"promotion_intent": schema.StringAttribute{
				MarkdownDescription: "Whether to promote the offer on the App Store — on the product page and in " +
					"editorial and personalized placements: `USE_AUTO_GENERATED_ASSETS` or `NOT_PROMOTED`. " +
					"Promotion needs an approved subscription image, and replaces any in-app purchase " +
					"promotions in the same storefront. An unpromoted offer still appears on the Manage " +
					"Subscriptions page and in the app. Apple's own value is kept when this is omitted.",
				Optional:   true,
				Computed:   true,
				Validators: []validator.String{WinBackOfferPromotionIntentValidator},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"target_subscription_plan_type": schema.StringAttribute{
				MarkdownDescription: "Which payment plan the offer applies to, for a subscription sold both " +
					"ways: `MONTHLY` or `UPFRONT`. Changing it forces a new offer.",
				Optional:   true,
				Computed:   true,
				Validators: []validator.String{PlanTypeValidator},
				// UseStateForUnknown has to run first: an omitted value plans
				// as unknown, and RequiresReplace would read unknown-versus-
				// state as a change and replace a live offer.
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
			},
			"prices": schema.SetNestedAttribute{
				MarkdownDescription: "The offer price in each storefront it runs in, one per territory. " +
					"**Changing this destroys the offer and creates a new one**: Apple's update request has no " +
					"prices, and App Store Connect itself does not allow a price change after creation.\n\n" +
					"A price is never a number: it references an entry in the subscription's price point " +
					"catalogue, read through `apple_subscription_price_points`, and " +
					"`apple_subscription_price_point_equalizations` fans one price out to every territory. " +
					"For a `FREE_TRIAL` the price still names the storefront the trial runs in.",
				Required: true,
				Validators: []validator.Set{
					setvalidator.SizeAtLeast(1),
				},
				PlanModifiers: []planmodifier.Set{
					setplanmodifier.RequiresReplace(),
				},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"price_point_id": schema.StringAttribute{
							MarkdownDescription: "The ID of the subscription price point this price references. " +
								"It encodes the subscription and the territory, so it is only valid on the " +
								"subscription it was read from.",
							Required: true,
						},
						"territory_id": schema.StringAttribute{
							MarkdownDescription: "The three-letter Apple territory code this price applies to, " +
								"such as `USA` or `GBR`. Optional: the price point already encodes it, and " +
								"Apple's own example request does not send it.",
							Optional:   true,
							Validators: []validator.String{TerritoryValidator},
						},
					},
				},
			},
		},
	}
}

// ValidateConfig checks what spans two attributes: the eligibility window's
// ends, and the dates' order. Apple would refuse both at apply, with messages
// that do not say which bound was wrong.
func (r *winBackOfferResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config winBackOfferModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var window *monthRangeModel
	if !config.TimeSinceLastSubscribed.IsNull() && !config.TimeSinceLastSubscribed.IsUnknown() {
		window = &monthRangeModel{}
		resp.Diagnostics.Append(config.TimeSinceLastSubscribed.As(ctx, window, basetypes.ObjectAsOptions{})...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	for _, problem := range validateWinBackOfferConfig(window, config.StartDate, config.EndDate) {
		resp.Diagnostics.AddError("Invalid Win-Back Offer", problem)
	}
}

// ModifyPlan refuses a start date Apple would refuse -- one earlier than
// tomorrow -- when the plan sets it: on create, or when it changes. It is not
// in ValidateConfig because it depends on today: an offer's start date passes,
// and checking it on every plan would break every plan of a running offer.
func (r *winBackOfferResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}

	var plan winBackOfferModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() || plan.StartDate.IsNull() || plan.StartDate.IsUnknown() {
		return
	}

	if !req.State.Raw.IsNull() {
		var state winBackOfferModel
		resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
		if resp.Diagnostics.HasError() || plan.StartDate.Equal(state.StartDate) {
			return
		}
	}

	if problem := winBackOfferStartTooSoon(plan.StartDate.ValueString(), time.Now()); problem != "" {
		resp.Diagnostics.AddAttributeError(path.Root("start_date"), "Invalid Win-Back Offer", problem)
	}
}

// Create a new resource.
func (r *winBackOfferResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	tflog.Info(ctx, "Creating subscription win-back offer resource")

	var plan winBackOfferModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	subscriptionID := plan.SubscriptionID.ValueString()
	offerID := plan.OfferID.ValueString()
	ctx = tflog.SetField(ctx, "subscription_id", subscriptionID)
	ctx = tflog.SetField(ctx, "offer_id", offerID)

	attributes := models.WinBackOfferAttributes{
		ReferenceName: plan.ReferenceName.ValueString(),
		OfferID:       offerID,
		Duration:      models.SubscriptionOfferDuration(plan.Duration.ValueString()),
		OfferMode:     models.SubscriptionOfferMode(plan.OfferMode.ValueString()),
		PeriodCount:   int(plan.PeriodCount.ValueInt64()),
		CustomerEligibilityPaidSubscriptionDurationInMonths: int(plan.PaidSubscriptionDuration.ValueInt64()),
		CustomerEligibilityTimeSinceLastSubscribedInMonths:  monthRangeToAPI(ctx, plan.TimeSinceLastSubscribed, &resp.Diagnostics),
		CustomerEligibilityWaitBetweenOffersInMonths:        intOrNil(plan.WaitBetweenOffers),
		StartDate:                  plan.StartDate.ValueString(),
		EndDate:                    stringOrNil(plan.EndDate),
		Priority:                   plan.Priority.ValueString(),
		PromotionIntent:            stringOrNil(plan.PromotionIntent),
		TargetSubscriptionPlanType: stringOrNil(plan.TargetSubscriptionPlanType),
	}
	prices := winBackOfferPricesToAPI(ctx, plan.Prices, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Creating win-back offer with Apple API", map[string]interface{}{"price_count": len(prices)})

	offer, err := r.client.CreateWinBackOffer(subscriptionID, attributes, prices, nil)
	if err != nil {
		errMsg := err.Error()
		lower := strings.ToLower(errMsg)
		switch {
		case strings.Contains(lower, "already") || strings.Contains(lower, "unique") || strings.Contains(lower, "duplicate"):
			resp.Diagnostics.AddError(
				"Win-Back Offer Identifier Already Used",
				fmt.Sprintf("Could not create win-back offer '%s': %s\n\n"+
					"Apple requires an offer_id that has not already been used by another offer in the same "+
					"subscription group, and a reference_name and offer_id not used by any promotional offer, "+
					"offer code or introductory offer. An identifier stays used after its offer is deleted, so "+
					"replacing an offer needs a new offer_id.", offerID, errMsg),
			)
		case strings.Contains(lower, "not found") || strings.Contains(errMsg, "404"):
			resp.Diagnostics.AddError(
				"Subscription or Price Point Not Found",
				fmt.Sprintf("Could not create win-back offer '%s' on subscription '%s': %s\n\n"+
					"A price point ID is only valid on the subscription it was read from — it encodes the "+
					"subscription and the territory. Read it from apple_subscription_price_points or "+
					"apple_subscription_price_point_equalizations scoped to subscription '%s'.",
					offerID, subscriptionID, errMsg, subscriptionID),
			)
		default:
			resp.Diagnostics.AddError(
				"Error Creating Win-Back Offer",
				fmt.Sprintf("Could not create win-back offer '%s' on subscription '%s': %s\n\n"+
					"Apple creates win-back offers only for a subscription App Review has approved, with a "+
					"start date no earlier than tomorrow and a run of at least three days. It allows 350 "+
					"offers per subscription and 5 running at once per storefront, and refuses a price in a "+
					"storefront the subscription is not available in.",
					offerID, subscriptionID, errMsg),
			)
		}
		tflog.Error(ctx, "Failed to create win-back offer", map[string]interface{}{"error": errMsg})
		return
	}

	// The prices stay exactly as planned: the create response lists them as
	// opaque linkages only, and they can never change afterwards.
	applyWinBackOfferAttributes(&plan, offer)

	tflog.Info(ctx, "Win-back offer created successfully", map[string]interface{}{"win_back_offer_id": offer.ID})

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// Read refreshes the Terraform state with the latest data.
//
// Two requests: the offer, then its prices, which Apple lists as a separate
// collection. subscription_id is never refreshed -- Apple cannot report it.
func (r *winBackOfferResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	tflog.Debug(ctx, "Reading subscription win-back offer resource")

	var state winBackOfferModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := state.ID.ValueString()
	ctx = tflog.SetField(ctx, "win_back_offer_id", id)

	offer, err := r.client.GetWinBackOffer(id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "404") {
			tflog.Info(ctx, "Win-back offer not found, removing from state")
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error Reading Win-Back Offer",
			fmt.Sprintf("Could not read win-back offer '%s': %s", id, err.Error()),
		)
		return
	}

	reported, err := r.client.GetWinBackOfferPrices(id)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading Win-Back Offer Prices",
			fmt.Sprintf("Could not read the prices of win-back offer '%s': %s", id, err.Error()),
		)
		return
	}

	var statePrices []winBackOfferPriceModel
	if !state.Prices.IsNull() && !state.Prices.IsUnknown() {
		resp.Diagnostics.Append(state.Prices.ElementsAs(ctx, &statePrices, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	applyWinBackOfferAttributes(&state, offer)
	state.Prices = winBackOfferPricesToTerraform(reconcileWinBackOfferPrices(statePrices, reported))

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update changes the eligibility rules, dates, priority or promotion in place.
// Everything else is RequiresReplace, so nothing else can reach here.
func (r *winBackOfferResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	tflog.Info(ctx, "Updating subscription win-back offer resource")

	var plan, state winBackOfferModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := state.ID.ValueString()
	ctx = tflog.SetField(ctx, "win_back_offer_id", id)

	update, changed := winBackOfferUpdate(ctx, plan, state, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	if changed {
		offer, err := r.client.UpdateWinBackOffer(id, update, nil)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error Updating Win-Back Offer",
				fmt.Sprintf("Could not update win-back offer '%s': %s\n\n"+
					"Apple lets some details of a win-back offer change only before it goes live on the App "+
					"Store. If this offer is live, the change may have to wait for a new offer — which needs a "+
					"new offer_id.", id, err.Error()),
			)
			tflog.Error(ctx, "Failed to update win-back offer", map[string]interface{}{"error": err.Error()})
			return
		}
		applyWinBackOfferAttributes(&plan, offer)
	} else {
		plan.ID = state.ID
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// Delete deletes the offer. Apple allows it whether or not the offer is live.
func (r *winBackOfferResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	tflog.Info(ctx, "Deleting subscription win-back offer resource")

	var state winBackOfferModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := state.ID.ValueString()
	ctx = tflog.SetField(ctx, "win_back_offer_id", id)

	if err := r.client.DeleteWinBackOffer(id, nil); err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "404") {
			tflog.Info(ctx, "Win-back offer already deleted")
			return
		}
		resp.Diagnostics.AddError(
			"Error Deleting Win-Back Offer",
			fmt.Sprintf("Could not delete win-back offer '%s': %s", id, err.Error()),
		)
		return
	}

	tflog.Info(ctx, "Win-back offer deleted successfully")
}

// Configure adds the provider configured client to the resource.
func (r *winBackOfferResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// ImportState imports an offer as <subscription_id>/<offer>.
//
// The subscription is part of the ID because Apple cannot report it: a
// WinBackOffer has no subscription relationship and its GET includes only
// prices. The second half is either Apple's ID or the offer_id, matched while
// listing the subscription's offers -- which is also the ownership check, the
// only one the API offers. Read then fills in the rest, adopting every price
// because state holds none.
func (r *winBackOfferResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	tflog.Info(ctx, "Importing subscription win-back offer resource", map[string]interface{}{"import_id": req.ID})

	subscriptionID, wanted, ok := strings.Cut(req.ID, "/")
	if !ok || subscriptionID == "" || wanted == "" {
		resp.Diagnostics.AddError(
			"Invalid Win-Back Offer Import ID",
			fmt.Sprintf("Import ID '%s' is not of the form <subscription_id>/<win_back_offer>.\n\n"+
				"The subscription has to be named because Apple never reports which subscription an offer "+
				"belongs to. The second part is either Apple's win-back offer ID or the offer_id.", req.ID),
		)
		return
	}

	offers, err := r.client.GetWinBackOffers(subscriptionID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Importing Win-Back Offer",
			fmt.Sprintf("Could not list the win-back offers of subscription '%s': %s", subscriptionID, err.Error()),
		)
		return
	}

	var found *models.WinBackOffer
	for i := range offers {
		if offers[i].ID == wanted || offers[i].Attributes.OfferID == wanted {
			found = &offers[i]
			break
		}
	}
	if found == nil {
		resp.Diagnostics.AddError(
			"Win-Back Offer Not Found",
			fmt.Sprintf("Subscription '%s' has no win-back offer whose ID or offer_id is '%s' (it has %d).",
				subscriptionID, wanted, len(offers)),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), found.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("subscription_id"), subscriptionID)...)
}
