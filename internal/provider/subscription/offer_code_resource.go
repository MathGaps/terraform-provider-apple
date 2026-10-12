// Copyright (c) AO Studio
// SPDX-License-Identifier: MPL-2.0

package subscription

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/AhmedOsman00/terraform-provider-apple/internal/apple"
	"github.com/AhmedOsman00/terraform-provider-apple/internal/apple/models"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var (
	_ resource.Resource                   = &subscriptionOfferCodeResource{}
	_ resource.ResourceWithConfigure      = &subscriptionOfferCodeResource{}
	_ resource.ResourceWithImportState    = &subscriptionOfferCodeResource{}
	_ resource.ResourceWithValidateConfig = &subscriptionOfferCodeResource{}
)

func NewSubscriptionOfferCodeResource() resource.Resource {
	return &subscriptionOfferCodeResource{}
}

type subscriptionOfferCodeResource struct {
	client *apple.Client
}

// offerCodePermanenceNote is the warning every offer code resource carries,
// because nothing in this family can be deleted.
const offerCodePermanenceNote = "~> **Nothing here can be deleted.** App Store Connect API 4.5.1 publishes no " +
	"`DELETE` for an offer code, a custom code or a batch of one-time-use codes. `terraform destroy` " +
	"**deactivates** the record (`active = false`), warns, and drops it from state; the record and its codes " +
	"stay in App Store Connect forever. App Store Connect's help says a deactivated offer \"can't be " +
	"reactivated later\" and that every code under it is invalidated, so deactivating — by destroy, by a " +
	"replacement, or by setting `active = false` — is final for the codes customers already hold."

// Metadata returns the resource type name.
func (r *subscriptionOfferCodeResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_subscription_offer_code"
}

// Schema defines the schema for the resource.
func (r *subscriptionOfferCodeResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a subscription offer code: a discount or free period on one auto-renewable " +
			"subscription, redeemed with a code. It is the App Store's nearest thing to a coupon, and like a " +
			"coupon it targets one plan — one subscription, so one tier and one billing period.\n\n" +
			"The offer carries no codes itself. Customers redeem it through an " +
			"`apple_subscription_offer_code_custom_code` (one shared string, such as `SCHOOL2026`) or an " +
			"`apple_subscription_offer_code_one_time_use_code` (a batch of single-use codes Apple " +
			"generates).\n\n" +
			"~> **Every attribute but `active` is fixed at creation.** Apple's update request carries " +
			"`active` alone, so any other change — the name, who is eligible, the duration, a price — " +
			"replaces the offer: a new one is created and the old one is deactivated, **invalidating every " +
			"code already handed out for it**. Treat a running offer as immutable; to change the terms of a " +
			"campaign, add a new offer beside it.\n\n" +
			offerCodePermanenceNote + "\n\n" +
			"App Store Connect allows at most 10 active offers per subscription at a time.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The unique Apple-generated identifier for the offer code.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"subscription_id": schema.StringAttribute{
				MarkdownDescription: "The ID of the `apple_subscription` the offer discounts. Cannot be changed " +
					"after creation.",
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The offer's reference name in App Store Connect. Customers do not see it. " +
					"Cannot be changed after creation.",
				Required:   true,
				Validators: []validator.String{stringvalidator.UTF8LengthAtLeast(1)},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"customer_eligibilities": schema.SetAttribute{
				MarkdownDescription: "Who may redeem the offer: any of `NEW` (never subscribed), `EXISTING` " +
					"(currently subscribed) and `EXPIRED` (subscribed before and lapsed). Cannot be changed " +
					"after creation.",
				Required:    true,
				ElementType: types.StringType,
				Validators: []validator.Set{
					setvalidator.SizeAtLeast(1),
					setvalidator.ValueStringsAre(CustomerEligibilityValidator),
				},
				PlanModifiers: []planmodifier.Set{
					setplanmodifier.RequiresReplace(),
				},
			},
			"offer_eligibility": schema.StringAttribute{
				MarkdownDescription: "How the offer combines with the subscription's introductory offer: " +
					"`STACK_WITH_INTRO_OFFERS` or `REPLACE_INTRO_OFFERS`. Cannot be changed after creation.",
				Required:   true,
				Validators: []validator.String{OfferCodeEligibilityValidator},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"duration": schema.StringAttribute{
				MarkdownDescription: "The length of one offer period: `THREE_DAYS`, `ONE_WEEK`, `TWO_WEEKS`, " +
					"`ONE_MONTH`, `TWO_MONTHS`, `THREE_MONTHS`, `SIX_MONTHS` or `ONE_YEAR`. Cannot be " +
					"changed after creation.",
				Required:   true,
				Validators: []validator.String{OfferCodeDurationValidator},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"offer_mode": schema.StringAttribute{
				MarkdownDescription: "How the customer pays during the offer: `PAY_AS_YOU_GO` (a discounted " +
					"price each period), `PAY_UP_FRONT` (one discounted price for the whole offer) or " +
					"`FREE_TRIAL` (nothing). Cannot be changed after creation.",
				Required:   true,
				Validators: []validator.String{OfferCodeModeValidator},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"number_of_periods": schema.Int64Attribute{
				MarkdownDescription: "How many `duration` periods the offer lasts. Cannot be changed after " +
					"creation.",
				Required:   true,
				Validators: []validator.Int64{int64validator.AtLeast(1)},
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"auto_renew_enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether the subscription renews automatically once the offer ends. " +
					"Optional; Apple decides when it is unset, and reports its answer. Cannot be changed " +
					"after creation.",
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.Bool{
					// UseStateForUnknown first: RequiresReplace compares the planned value
					// with state, and an omitted computed value is unknown until this fills it.
					boolplanmodifier.UseStateForUnknown(),
					boolplanmodifier.RequiresReplace(),
				},
			},
			"target_subscription_plan_type": schema.StringAttribute{
				MarkdownDescription: "Which plan of the subscription the offer applies to: `MONTHLY` (the " +
					"recurring price) or `UPFRONT` (a pre-paid plan). Optional; Apple decides when it is " +
					"unset. Cannot be changed after creation.",
				Optional:   true,
				Computed:   true,
				Validators: []validator.String{PlanTypeValidator},
				PlanModifiers: []planmodifier.String{
					// UseStateForUnknown first: RequiresReplace compares the planned value
					// with state, and an omitted computed value is unknown until this fills it.
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
			},
			"prices": schema.SetNestedAttribute{
				MarkdownDescription: "The offer's price in each territory it is offered in, one element per " +
					"territory. Required by Apple's create request. A set, because the order carries no " +
					"meaning. Cannot be changed after creation — Apple publishes no way to add, change or " +
					"remove an offer's price — so a change replaces the offer.\n\n" +
					"A price is never a number: `price_point_id` is one of Apple's price points for **this " +
					"subscription**, read through `apple_subscription_price_points` or " +
					"`apple_subscription_price_point_equalizations`.",
				Required: true,
				Validators: []validator.Set{
					setvalidator.SizeAtLeast(1),
				},
				PlanModifiers: []planmodifier.Set{
					setplanmodifier.RequiresReplace(),
				},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"territory_id": schema.StringAttribute{
							MarkdownDescription: "The three-letter Apple territory code, for example `USA`, " +
								"`GBR` or `AUS`.",
							Required:   true,
							Validators: []validator.String{TerritoryValidator},
						},
						"price_point_id": schema.StringAttribute{
							MarkdownDescription: "The ID of the Apple price point the customer pays during " +
								"the offer in this territory. Required for `PAY_AS_YOU_GO` and " +
								"`PAY_UP_FRONT`; omit it for a `FREE_TRIAL`, which has no price — Apple's " +
								"schema makes it optional for that reason.",
							Optional: true,
						},
					},
				},
			},
			"active": schema.BoolAttribute{
				MarkdownDescription: "Whether customers can redeem the offer. Defaults to `true`. Setting it to " +
					"`false` ends the campaign **in place** and keeps the record in the configuration — and " +
					"is **final**: App Store Connect cannot reactivate an offer, and every code under it is " +
					"invalidated. Setting it back to `true` therefore plans a **new** offer rather than an " +
					"update.",
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(true),
				PlanModifiers: []planmodifier.Bool{
					reactivationRequiresReplace(),
				},
			},
			"total_number_of_codes": schema.Int64Attribute{
				MarkdownDescription: "How many codes Apple reports for the offer across both environments.",
				Computed:            true,
			},
			"production_code_count": schema.Int64Attribute{
				MarkdownDescription: "How many of those codes redeem in production.",
				Computed:            true,
			},
			"sandbox_code_count": schema.Int64Attribute{
				MarkdownDescription: "How many of those codes redeem in the sandbox.",
				Computed:            true,
			},
		},
	}
}

// ValidateConfig rejects a price set Apple would refuse or misread: a
// territory priced twice, or a paid offer with a territory that names no
// price point.
func (r *subscriptionOfferCodeResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config subscriptionOfferCodeModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if config.Prices.IsNull() || config.Prices.IsUnknown() || config.OfferMode.IsUnknown() {
		return
	}

	var prices []offerCodePriceModel
	resp.Diagnostics.Append(config.Prices.ElementsAs(ctx, &prices, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	for _, problem := range checkOfferCodePrices(config.OfferMode.ValueString(), prices) {
		resp.Diagnostics.AddAttributeError(path.Root("prices"), "Invalid Offer Code Prices", problem)
	}
}

// Create creates the offer and its prices in one request.
func (r *subscriptionOfferCodeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	tflog.Info(ctx, "Creating subscription offer code resource")

	var plan subscriptionOfferCodeModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	subscriptionID := plan.SubscriptionID.ValueString()
	ctx = tflog.SetField(ctx, "subscription_id", subscriptionID)

	var eligibilities []string
	resp.Diagnostics.Append(plan.CustomerEligibilities.ElementsAs(ctx, &eligibilities, false)...)
	var prices []offerCodePriceModel
	resp.Diagnostics.Append(plan.Prices.ElementsAs(ctx, &prices, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Sorted so the same configuration always sends the same body.
	sort.Strings(eligibilities)
	inputs := make([]apple.SubscriptionOfferCodePriceInput, 0, len(prices))
	for _, price := range prices {
		inputs = append(inputs, apple.SubscriptionOfferCodePriceInput{
			TerritoryID:  price.TerritoryID.ValueString(),
			PricePointID: price.PricePointID.ValueString(),
		})
	}
	sort.Slice(inputs, func(i, j int) bool { return inputs[i].TerritoryID < inputs[j].TerritoryID })

	attributes := models.SubscriptionOfferCodeCreateAttributes{
		Name:                       plan.Name.ValueString(),
		CustomerEligibilities:      eligibilities,
		OfferEligibility:           plan.OfferEligibility.ValueString(),
		Duration:                   plan.Duration.ValueString(),
		OfferMode:                  plan.OfferMode.ValueString(),
		NumberOfPeriods:            int(plan.NumberOfPeriods.ValueInt64()),
		AutoRenewEnabled:           boolOrNil(plan.AutoRenewEnabled),
		TargetSubscriptionPlanType: stringOrNil(plan.TargetSubscriptionPlanType),
	}

	offer, err := r.client.CreateSubscriptionOfferCode(subscriptionID, attributes, inputs, nil)
	if err != nil {
		errMsg := err.Error()
		switch {
		case strings.Contains(errMsg, "not found") || strings.Contains(errMsg, "404"):
			resp.Diagnostics.AddError(
				"Subscription or Price Point Not Found",
				fmt.Sprintf("Could not create offer code '%s' on subscription '%s': %s\n\n"+
					"A price point ID is valid only for the subscription it was read from. Read it from "+
					"apple_subscription_price_points or apple_subscription_price_point_equalizations "+
					"scoped to subscription '%s'.", plan.Name.ValueString(), subscriptionID, errMsg, subscriptionID),
			)
		default:
			resp.Diagnostics.AddError(
				"Error Creating Subscription Offer Code",
				fmt.Sprintf("Could not create offer code '%s' on subscription '%s': %s\n\n"+
					"App Store Connect allows at most 10 active offers per subscription, and prices an offer "+
					"only in territories the subscription is available in (apple_subscription_availability). "+
					"A paid offer needs a price point in every territory; a FREE_TRIAL needs none.",
					plan.Name.ValueString(), subscriptionID, errMsg),
			)
		}
		tflog.Error(ctx, "Failed to create subscription offer code", map[string]interface{}{"error": errMsg})
		return
	}

	plan.ID = types.StringValue(offer.ID)
	applyOfferCodeComputed(&plan, offer)

	// State is written before deactivating, as Apple holds it: active. An offer
	// left out of state is one Terraform can never see again, and Apple will not
	// delete it. Whatever happens next, the state this returns carries the
	// planned active.
	configuredActive := plan.Active
	plan.Active = types.BoolValue(true)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !configuredActive.ValueBool() {
		updated, err := r.client.SetSubscriptionOfferCodeActive(offer.ID, false, nil)
		if err != nil {
			// A warning, not an error: Terraform taints an object whose create
			// errs and plans its replacement, which here would create a second
			// offer Apple can never delete. State keeps the planned value, as
			// core requires of a create; the next refresh reads Apple's
			// active = true, and the next plan deactivates it in place.
			plan.Active = configuredActive
			resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
			resp.Diagnostics.AddWarning(
				"Subscription Offer Code Created Active",
				fmt.Sprintf("Offer code '%s' was created, but deactivating it as configured failed: %s\n\n"+
					"It is still active at Apple. The next plan shows it, and the next apply deactivates it.",
					offer.ID, err.Error()),
			)
			return
		}
		plan.Active = configuredActive
		applyOfferCodeComputed(&plan, updated)
		resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
	}

	tflog.Info(ctx, "Created subscription offer code", map[string]interface{}{"offer_code_id": offer.ID})
}

// Read refreshes the offer from Apple.
//
// The prices are read only when state holds none -- after an import. Apple
// publishes no way to change an offer's prices after creation, so a refresh
// could only ever confirm them, at the cost of a second request every plan.
func (r *subscriptionOfferCodeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	tflog.Debug(ctx, "Reading subscription offer code resource")

	var state subscriptionOfferCodeModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	offerCodeID := state.ID.ValueString()
	ctx = tflog.SetField(ctx, "offer_code_id", offerCodeID)

	offer, err := r.client.GetSubscriptionOfferCode(offerCodeID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "404") {
			tflog.Info(ctx, "Subscription offer code not found, removing from state")
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error Reading Subscription Offer Code",
			fmt.Sprintf("Could not read offer code '%s': %s", offerCodeID, err.Error()),
		)
		return
	}

	a := offer.Attributes
	if a.Name != "" {
		state.Name = types.StringValue(a.Name)
	}
	if len(a.CustomerEligibilities) > 0 {
		eligibilities, diags := types.SetValueFrom(ctx, types.StringType, a.CustomerEligibilities)
		resp.Diagnostics.Append(diags...)
		state.CustomerEligibilities = eligibilities
	}
	if a.OfferEligibility != "" {
		state.OfferEligibility = types.StringValue(a.OfferEligibility)
	}
	if a.Duration != "" {
		state.Duration = types.StringValue(a.Duration)
	}
	if a.OfferMode != "" {
		state.OfferMode = types.StringValue(a.OfferMode)
	}
	if a.NumberOfPeriods != nil {
		state.NumberOfPeriods = types.Int64Value(int64(*a.NumberOfPeriods))
	}
	if a.AutoRenewEnabled != nil {
		state.AutoRenewEnabled = types.BoolValue(*a.AutoRenewEnabled)
	}
	if a.TargetSubscriptionPlanType != nil {
		state.TargetSubscriptionPlanType = types.StringValue(*a.TargetSubscriptionPlanType)
	}
	if a.Active != nil {
		state.Active = types.BoolValue(*a.Active)
	}
	state.TotalNumberOfCodes = offerCodeInt64(a.TotalNumberOfCodes)
	state.ProductionCodeCount = offerCodeInt64(a.ProductionCodeCount)
	state.SandboxCodeCount = offerCodeInt64(a.SandboxCodeCount)

	if offer.Relationships != nil && offer.Relationships.Subscription != nil &&
		offer.Relationships.Subscription.Data.ID != "" {
		state.SubscriptionID = types.StringValue(offer.Relationships.Subscription.Data.ID)
	}

	if state.Prices.IsNull() || state.Prices.IsUnknown() {
		prices, err := r.client.GetSubscriptionOfferCodePrices(offerCodeID)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error Reading Subscription Offer Code Prices",
				fmt.Sprintf("Could not read the prices of offer code '%s': %s", offerCodeID, err.Error()),
			)
			return
		}
		set, diags := offerCodePricesToSet(ctx, offerCodePricesFromAPI(prices))
		resp.Diagnostics.Append(diags...)
		state.Prices = set
	}
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update changes "active", the one member of Apple's update request. Every
// other attribute forces replacement, and a reactivation does too (see
// reactivationRequiresReplace), so in practice this only ever deactivates.
func (r *subscriptionOfferCodeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	tflog.Info(ctx, "Updating subscription offer code resource")

	var plan subscriptionOfferCodeModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	offerCodeID := plan.ID.ValueString()
	offer, err := r.client.SetSubscriptionOfferCodeActive(offerCodeID, plan.Active.ValueBool(), nil)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Updating Subscription Offer Code",
			fmt.Sprintf("Could not set active=%t on offer code '%s': %s",
				plan.Active.ValueBool(), offerCodeID, err.Error()),
		)
		return
	}

	applyOfferCodeComputed(&plan, offer)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// Delete deactivates the offer and drops it from state.
//
// Apple publishes no DELETE for an offer code. Erroring would leave the
// resource impossible to destroy, so it is deactivated instead -- the shape
// apple_device uses -- and a 404 is treated as already gone.
func (r *subscriptionOfferCodeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state subscriptionOfferCodeModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	offerCodeID := state.ID.ValueString()
	ctx = tflog.SetField(ctx, "offer_code_id", offerCodeID)

	if state.Active.IsNull() || state.Active.ValueBool() {
		if _, err := r.client.SetSubscriptionOfferCodeActive(offerCodeID, false, nil); err != nil {
			if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "404") {
				tflog.Warn(ctx, "Offer code not found while deactivating, treating as already removed")
				return
			}
			resp.Diagnostics.AddError(
				"Unable to Deactivate Subscription Offer Code",
				fmt.Sprintf("Apple's API cannot delete an offer code, so Terraform deactivates it instead. "+
					"Deactivating offer code '%s' failed: %s", offerCodeID, err.Error()),
			)
			return
		}
	}

	resp.Diagnostics.AddWarning(
		"Subscription Offer Code Deactivated Rather Than Deleted",
		fmt.Sprintf("The App Store Connect API cannot delete an offer code. Offer code '%s' (%s) has been "+
			"deactivated and removed from Terraform state, but it remains in App Store Connect, and it cannot "+
			"be reactivated. Every code issued for it no longer redeems.",
			state.Name.ValueString(), offerCodeID),
	)
}

// Configure adds the provider configured client to the resource.
func (r *subscriptionOfferCodeResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// ImportState imports an offer code by Apple's ID.
//
// A bare ID is enough: GET /v1/subscriptionOfferCodes/{id} accepts
// include=subscription, so the parent can be read back -- unlike a
// subscription group's app. Read then fills in the rest, prices included.
func (r *subscriptionOfferCodeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	offerCodeID := strings.TrimSpace(req.ID)
	tflog.Info(ctx, "Importing subscription offer code resource", map[string]interface{}{"import_id": offerCodeID})

	offer, err := r.client.GetSubscriptionOfferCode(offerCodeID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Importing Subscription Offer Code",
			fmt.Sprintf("Could not read offer code '%s': %s\n\nThe import ID is Apple's offer code ID; "+
				"apple_subscription_offer_codes lists a subscription's offers with their IDs.",
				offerCodeID, err.Error()),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), offer.ID)...)
	if offer.Relationships != nil && offer.Relationships.Subscription != nil {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("subscription_id"),
			offer.Relationships.Subscription.Data.ID)...)
	}
}

// applyOfferCodeComputed fills what Apple decides from a create or update
// response. An Optional+Computed attribute the configuration left unset is
// adopted -- or set null, never left unknown, which the framework rejects.
// "active" is kept as planned: it is what was just sent.
func applyOfferCodeComputed(plan *subscriptionOfferCodeModel, offer *models.SubscriptionOfferCode) {
	a := offer.Attributes

	if plan.AutoRenewEnabled.IsUnknown() {
		plan.AutoRenewEnabled = offerCodeBool(a.AutoRenewEnabled)
	}
	if plan.TargetSubscriptionPlanType.IsUnknown() {
		plan.TargetSubscriptionPlanType = stringOrNull(a.TargetSubscriptionPlanType)
	}

	plan.TotalNumberOfCodes = offerCodeInt64(a.TotalNumberOfCodes)
	plan.ProductionCodeCount = offerCodeInt64(a.ProductionCodeCount)
	plan.SandboxCodeCount = offerCodeInt64(a.SandboxCodeCount)
}
