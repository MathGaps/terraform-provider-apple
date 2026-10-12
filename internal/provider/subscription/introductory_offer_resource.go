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
	"github.com/hashicorp/terraform-plugin-framework-validators/mapvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var (
	_ resource.Resource                   = &subscriptionIntroductoryOfferResource{}
	_ resource.ResourceWithConfigure      = &subscriptionIntroductoryOfferResource{}
	_ resource.ResourceWithImportState    = &subscriptionIntroductoryOfferResource{}
	_ resource.ResourceWithModifyPlan     = &subscriptionIntroductoryOfferResource{}
	_ resource.ResourceWithValidateConfig = &subscriptionIntroductoryOfferResource{}
)

func NewSubscriptionIntroductoryOfferResource() resource.Resource {
	return &subscriptionIntroductoryOfferResource{}
}

type subscriptionIntroductoryOfferResource struct {
	client *apple.Client
}

// Metadata returns the resource type name.
func (r *subscriptionIntroductoryOfferResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_subscription_introductory_offer"
}

// Schema defines the schema for the resource.
func (r *subscriptionIntroductoryOfferResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages an introductory offer on an auto-renewable subscription — a free trial, or a " +
			"discounted first period — across a set of territories.\n\n" +
			"Apple keeps **one record per territory**: the \"one-week free trial everywhere\" App Store " +
			"Connect's UI shows as one offer is 175 records with identical terms. This resource describes " +
			"the offer once and owns one record in each territory it names, reported through `offer_ids`. " +
			"The records are created in a single `PATCH /v1/subscriptions/{id}` carrying them inline, so " +
			"Apple commits them all or none, and read back in one listing.\n\n" +
			"An offer is either a **free trial**, listing its territories in `territory_ids`, or **paid** " +
			"(`PAY_AS_YOU_GO`, `PAY_UP_FRONT`), keying the discounted price point of each territory in " +
			"`price_point_ids` — the map `apple_subscription_price_point_equalizations` produces.\n\n" +
			"Only `end_date` and the set of territories change in place. Apple's update request carries the " +
			"end date alone, so every other attribute forces replacement, and a territory whose price " +
			"point changes is deleted and created again.\n\n" +
			"~> **A territory holds one introductory offer at a time.** Several of these resources can share " +
			"a subscription — a trial in some territories and a paid offer in others, or a later offer " +
			"scheduled after an earlier one ends — but not with overlapping dates in the same territory.\n\n" +
			"~> **The subscription has to be sellable in a territory** before an offer can be made there: " +
			"declare `depends_on` on its `apple_subscription_availability` and price, since nothing in an " +
			"offer references them.\n\n" +
			"Destroying the resource deletes its record in every territory.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The ID of the subscription the offer belongs to. There is no single " +
					"record for the offer at Apple — `offer_ids` holds the one per territory — so this " +
					"mirrors `subscription_id`.",
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"subscription_id": schema.StringAttribute{
				MarkdownDescription: "The ID of the `apple_subscription` the offer applies to. Cannot be " +
					"changed after creation.",
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"offer_mode": schema.StringAttribute{
				MarkdownDescription: "What the customer pays during the offer: `FREE_TRIAL` (nothing), " +
					"`PAY_AS_YOU_GO` (a discounted price every period, for `number_of_periods` periods) or " +
					"`PAY_UP_FRONT` (one discounted price for the whole `duration`). A free trial takes " +
					"`territory_ids`; the two paid modes take `price_point_ids`. Cannot be changed after " +
					"creation.",
				Required:   true,
				Validators: []validator.String{OfferModeValidator},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"duration": schema.StringAttribute{
				MarkdownDescription: "How long one period of the offer lasts: `THREE_DAYS`, `ONE_WEEK`, " +
					"`TWO_WEEKS`, `ONE_MONTH`, `TWO_MONTHS`, `THREE_MONTHS`, `SIX_MONTHS` or `ONE_YEAR`. " +
					"Cannot be changed after creation.",
				Required:   true,
				Validators: []validator.String{OfferDurationValidator},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"number_of_periods": schema.Int64Attribute{
				MarkdownDescription: "How many `duration` periods the offer runs for. Defaults to `1`, " +
					"which is what a free trial and a pay-up-front offer use; a pay-as-you-go offer of " +
					"three discounted months is `duration = \"ONE_MONTH\"` with `number_of_periods = 3`. " +
					"Cannot be changed after creation.",
				Optional:   true,
				Computed:   true,
				Default:    int64default.StaticInt64(1),
				Validators: []validator.Int64{int64validator.AtLeast(1)},
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"start_date": schema.StringAttribute{
				MarkdownDescription: "The date the offer becomes available to new subscribers, as a plain " +
					"date in `YYYY-MM-DD` form — not a timestamp. Omit it for an offer available " +
					"immediately. Cannot be changed after creation.",
				Optional:   true,
				Validators: []validator.String{OfferDateValidator},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"end_date": schema.StringAttribute{
				MarkdownDescription: "The last date the offer is available to new subscribers, as a plain " +
					"date in `YYYY-MM-DD` form. Omit it for an offer with no end. The one attribute Apple " +
					"lets change in place; removing it removes the end date at Apple. Only a configured " +
					"end date is compared with Apple's on refresh.",
				Optional:   true,
				Validators: []validator.String{OfferDateValidator},
			},
			"target_subscription_plan_type": schema.StringAttribute{
				MarkdownDescription: "Which plan of the subscription the offer applies to: `MONTHLY` for " +
					"the recurring plan or `UPFRONT` for a pre-paid one. Omit it for a subscription with " +
					"a single plan. Cannot be changed after creation.",
				Optional:   true,
				Validators: []validator.String{PlanTypeValidator},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"territory_ids": schema.SetAttribute{
				MarkdownDescription: "The three-letter Apple territory codes a **free trial** is offered " +
					"in, for example `[\"USA\", \"GBR\"]`, or `data.apple_territories.all.ids` for " +
					"everywhere. Required when `offer_mode` is `FREE_TRIAL` and rejected otherwise. " +
					"Adding or removing a territory creates or deletes that territory's record in place.",
				Optional:    true,
				ElementType: types.StringType,
				Validators: []validator.Set{
					setvalidator.SizeAtLeast(1),
					setvalidator.ValueStringsAre(TerritoryValidator),
				},
			},
			"price_point_ids": schema.MapAttribute{
				MarkdownDescription: "For a **paid** offer, the discounted price point in each territory " +
					"it is offered in, keyed by three-letter territory code — the shape " +
					"`apple_subscription_price_point_equalizations.price_point_ids` produces. Required " +
					"when `offer_mode` is `PAY_AS_YOU_GO` or `PAY_UP_FRONT` and rejected for a free " +
					"trial. Price points come from the subscription's own catalogue " +
					"(`apple_subscription_price_points`) and cannot be reused on another subscription. " +
					"Adding or removing a territory creates or deletes its record in place; changing a " +
					"territory's price point deletes its record and creates another.",
				Optional:    true,
				ElementType: types.StringType,
				Validators: []validator.Map{
					mapvalidator.SizeAtLeast(1),
					mapvalidator.KeysAre(TerritoryValidator),
				},
			},
			"offer_ids": schema.MapAttribute{
				MarkdownDescription: "The Apple record behind the offer in each territory, keyed by " +
					"territory code. Any one of them, with the subscription ID, is an import ID.",
				Computed:    true,
				ElementType: types.StringType,
				PlanModifiers: []planmodifier.Map{
					mapplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

// ValidateConfig enforces the free-trial and paid shapes.
//
// A free trial has no price, and a paid offer's price is the offer: one with
// no price point has nothing to charge. Rejecting the wrong attribute at plan
// time names the reason, where Apple's refusal would name a relationship.
func (r *subscriptionIntroductoryOfferResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config subscriptionIntroductoryOfferModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !config.TerritoryIDs.IsNull() && !config.PricePointIDs.IsNull() {
		resp.Diagnostics.AddAttributeError(
			path.Root("price_point_ids"),
			"Conflicting Territory Attributes",
			"Set either territory_ids (a free trial) or price_point_ids (a paid offer), not both: "+
				"the territories of a paid offer are the keys of price_point_ids.",
		)
		return
	}

	if config.OfferMode.IsUnknown() || config.OfferMode.IsNull() {
		return
	}

	mode := config.OfferMode.ValueString()
	if mode == string(models.SubscriptionOfferModeFreeTrial) {
		if !config.PricePointIDs.IsNull() {
			resp.Diagnostics.AddAttributeError(
				path.Root("price_point_ids"),
				"Free Trial Takes No Price",
				"A FREE_TRIAL offer has no price, so it names its territories in territory_ids. "+
					"price_point_ids is for the PAY_AS_YOU_GO and PAY_UP_FRONT modes.",
			)
		} else if config.TerritoryIDs.IsNull() {
			resp.Diagnostics.AddAttributeError(
				path.Root("territory_ids"),
				"Missing Territories",
				"A FREE_TRIAL offer needs territory_ids: Apple keeps one record per territory, and an "+
					"offer in no territory is offered nowhere.",
			)
		}
		return
	}

	if !config.TerritoryIDs.IsNull() {
		resp.Diagnostics.AddAttributeError(
			path.Root("territory_ids"),
			"Paid Offer Needs Price Points",
			fmt.Sprintf("A %s offer charges a discounted price, so it names its territories as the keys "+
				"of price_point_ids, each mapped to the price point it costs there. territory_ids is for "+
				"FREE_TRIAL alone.", mode),
		)
	} else if config.PricePointIDs.IsNull() {
		resp.Diagnostics.AddAttributeError(
			path.Root("price_point_ids"),
			"Missing Price Points",
			fmt.Sprintf("A %s offer needs price_point_ids: the discounted price point in each territory "+
				"it is offered in. apple_subscription_price_point_equalizations produces the map from "+
				"one base price point.", mode),
		)
	}
}

// ModifyPlan marks offer_ids unknown when the set of territories changes.
//
// offer_ids carries the state forward otherwise, so an end date change plans
// no new records. A territory added, removed or repriced is a record created
// or deleted, and Apple issues the IDs.
func (r *subscriptionIntroductoryOfferResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}

	var plan, state subscriptionIntroductoryOfferModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if plan.TerritoryIDs.Equal(state.TerritoryIDs) && plan.PricePointIDs.Equal(state.PricePointIDs) {
		return
	}

	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("offer_ids"), types.MapUnknown(types.StringType))...)
}

// Create creates the offer in every territory in one request.
func (r *subscriptionIntroductoryOfferResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	tflog.Info(ctx, "Creating subscription introductory offer resource")

	var plan subscriptionIntroductoryOfferModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	subscriptionID := plan.SubscriptionID.ValueString()
	ctx = tflog.SetField(ctx, "subscription_id", subscriptionID)

	territories, diags := offerTerritories(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	unlock := lockSubscriptionIntroductoryOffers(subscriptionID)
	defer unlock()

	offerIDs := map[string]string{}
	r.createOffers(ctx, plan, territories, sortedKeys(territories), offerIDs, &resp.Diagnostics)
	if len(offerIDs) == 0 {
		// Apple refused the request whole, so there is nothing to record.
		return
	}

	// Whatever Apple created is recorded, even alongside an error: a record
	// left out of state is one the next apply collides with, because a
	// territory holds one introductory offer at a time. Terraform marks a
	// create that reported an error as tainted, so the next apply deletes
	// these and starts again.
	created := make(map[string]string, len(offerIDs))
	for territory := range offerIDs {
		created[territory] = territories[territory]
	}

	plan.ID = types.StringValue(subscriptionID)
	resp.Diagnostics.Append(setOfferTerritories(ctx, &plan, created, offerIDs)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// Read refreshes the Terraform state with the latest data.
//
// One listing covers every territory. A record that has gone takes its
// territory out of state, so the next plan creates it again; the end date is
// compared only when one is configured -- see driftedEndDate. Nothing else is
// refreshed: every other attribute is fixed for the life of a record, and the
// ones Apple might report unasked (a start date, a plan type) are
// RequiresReplace, where adopting a value the configuration does not set would
// plan a replacement on every run.
func (r *subscriptionIntroductoryOfferResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	tflog.Debug(ctx, "Reading subscription introductory offer resource")

	var state subscriptionIntroductoryOfferModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	subscriptionID := state.SubscriptionID.ValueString()
	ctx = tflog.SetField(ctx, "subscription_id", subscriptionID)

	territories, diags := offerTerritories(ctx, state)
	resp.Diagnostics.Append(diags...)
	offerIDs, diags := stateOfferIDs(ctx, state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	reported, err := r.client.GetSubscriptionIntroductoryOffers(subscriptionID, sortedKeys(offerIDs))
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "404") {
			tflog.Info(ctx, "Subscription not found, removing introductory offer from state")
			resp.State.RemoveResource(ctx)
			return
		}

		resp.Diagnostics.AddError(
			"Error Reading Subscription Introductory Offer",
			fmt.Sprintf("Could not read the introductory offers of subscription '%s': %s", subscriptionID, err.Error()),
		)
		return
	}

	surviving := survivingOffers(offerIDs, reported)
	if len(surviving) == 0 {
		tflog.Info(ctx, "No introductory offer record remains in any territory, removing from state")
		resp.State.RemoveResource(ctx)
		return
	}

	keptTerritories := make(map[string]string, len(surviving))
	keptOfferIDs := make(map[string]string, len(surviving))
	for territory, offer := range surviving {
		keptTerritories[territory] = territories[territory]
		keptOfferIDs[territory] = offer.ID
	}
	if len(surviving) < len(offerIDs) {
		tflog.Info(ctx, "Some territories' introductory offer records are gone", map[string]interface{}{
			"remaining": len(surviving), "in_state": len(offerIDs),
		})
	}

	if !state.EndDate.IsNull() {
		if endDate, drifted := driftedEndDate(stringOrNil(state.EndDate), surviving); drifted {
			state.EndDate = stringOrNull(endDate)
		}
	}

	resp.Diagnostics.Append(setOfferTerritories(ctx, &state, keptTerritories, keptOfferIDs)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update moves the records from the territories in state to those in the plan,
// and applies a changed end date to the records that stay.
//
// Deletions go first, so a territory whose price point changed has no
// overlapping offer when it is created again. After any failure the state
// written is what Apple now holds, so the next plan picks up from there.
func (r *subscriptionIntroductoryOfferResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	tflog.Info(ctx, "Updating subscription introductory offer resource")

	var plan, state subscriptionIntroductoryOfferModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	subscriptionID := plan.SubscriptionID.ValueString()
	ctx = tflog.SetField(ctx, "subscription_id", subscriptionID)

	stateTerritories, diags := offerTerritories(ctx, state)
	resp.Diagnostics.Append(diags...)
	offerIDs, diags := stateOfferIDs(ctx, state)
	resp.Diagnostics.Append(diags...)
	planTerritories, diags := offerTerritories(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	unlock := lockSubscriptionIntroductoryOffers(subscriptionID)
	defer unlock()

	toDelete, toCreate, kept := offerChanges(stateTerritories, offerIDs, planTerritories)
	tflog.Debug(ctx, "Planned introductory offer changes", map[string]interface{}{
		"delete": len(toDelete), "create": len(toCreate), "keep": len(kept),
	})

	// current tracks what Apple holds as each step lands.
	current := make(map[string]string, len(offerIDs))
	for territory, offerID := range offerIDs {
		current[territory] = offerID
	}
	endDate := state.EndDate

	r.deleteOffers(ctx, toDelete, current, &resp.Diagnostics)

	if !resp.Diagnostics.HasError() && !plan.EndDate.Equal(state.EndDate) {
		if r.updateEndDates(ctx, plan.EndDate, kept, current, &resp.Diagnostics) {
			endDate = plan.EndDate
		}
	}

	if !resp.Diagnostics.HasError() {
		r.createOffers(ctx, plan, planTerritories, toCreate, current, &resp.Diagnostics)
	}

	// A record still carrying the ID state had is the old one, at the old
	// price point -- a repriced territory whose delete failed is one of
	// these. Anything else was created from the plan.
	territories := make(map[string]string, len(current))
	for territory, offerID := range current {
		if offerID == offerIDs[territory] {
			territories[territory] = stateTerritories[territory]
		} else {
			territories[territory] = planTerritories[territory]
		}
	}

	plan.ID = types.StringValue(subscriptionID)
	plan.EndDate = endDate
	resp.Diagnostics.Append(setOfferTerritories(ctx, &plan, territories, current)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// Delete deletes the offer's record in every territory.
func (r *subscriptionIntroductoryOfferResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	tflog.Info(ctx, "Deleting subscription introductory offer resource")

	var state subscriptionIntroductoryOfferModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	subscriptionID := state.SubscriptionID.ValueString()
	ctx = tflog.SetField(ctx, "subscription_id", subscriptionID)

	territories, diags := offerTerritories(ctx, state)
	resp.Diagnostics.Append(diags...)
	offerIDs, diags := stateOfferIDs(ctx, state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	unlock := lockSubscriptionIntroductoryOffers(subscriptionID)
	defer unlock()

	remaining := make(map[string]string, len(offerIDs))
	for territory, offerID := range offerIDs {
		remaining[territory] = offerID
	}
	r.deleteOffers(ctx, offerIDs, remaining, &resp.Diagnostics)

	if resp.Diagnostics.HasError() {
		// Keep the records that could not be deleted, so destroy can be run
		// again rather than forgetting offers that are still live.
		remainingTerritories := make(map[string]string, len(remaining))
		for territory := range remaining {
			remainingTerritories[territory] = territories[territory]
		}
		resp.Diagnostics.Append(setOfferTerritories(ctx, &state, remainingTerritories, remaining)...)
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
		return
	}

	tflog.Info(ctx, "Subscription introductory offer deleted in every territory")
}

// Configure adds the provider configured client to the resource.
func (r *subscriptionIntroductoryOfferResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// ImportState adopts an offer made elsewhere, typically in App Store Connect.
//
// The import ID is "<subscription_id>/<offer_id>", where the offer ID is any
// one territory's record: Apple publishes no GET for a single offer, so the
// subscription is what makes it findable. Every record on the subscription
// with the same terms is adopted with it, because that is what one offer
// across many territories looks like at Apple -- the records App Store
// Connect's UI creates for one offer differ only in territory and price.
func (r *subscriptionIntroductoryOfferResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	tflog.Info(ctx, "Importing subscription introductory offer resource", map[string]interface{}{"import_id": req.ID})

	subscriptionID, offerID, found := strings.Cut(req.ID, "/")
	if !found || subscriptionID == "" || offerID == "" {
		resp.Diagnostics.AddError(
			"Invalid Import ID",
			fmt.Sprintf("Introductory offer import ID '%s' is not in the expected form.\n\n"+
				"Use \"<subscription_id>/<offer_id>\", where the offer ID is the record of any one "+
				"territory — the apple_subscription_introductory_offers data source lists them.\n\n"+
				"The subscription ID is required because Apple publishes no GET for a single "+
				"introductory offer; offers are reachable only through their subscription.", req.ID),
		)
		return
	}

	offers, err := r.client.GetSubscriptionIntroductoryOffers(subscriptionID, nil)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Importing Subscription Introductory Offer",
			fmt.Sprintf("Could not read the introductory offers of subscription '%s': %s", subscriptionID, err.Error()),
		)
		return
	}

	anchor, group, found := groupOfferTerritories(offers, offerID)
	if !found {
		resp.Diagnostics.AddError(
			"Introductory Offer Not Found",
			fmt.Sprintf("Subscription '%s' has no introductory offer with ID '%s'. It holds %d introductory "+
				"offer record(s); the apple_subscription_introductory_offers data source lists them.",
				subscriptionID, offerID, len(offers)),
		)
		return
	}
	if len(group) == 0 {
		resp.Diagnostics.AddError(
			"Introductory Offer Has No Territory",
			fmt.Sprintf("Apple reports no territory for introductory offer '%s', so it cannot be "+
				"described by this resource, which names its territories.", offerID),
		)
		return
	}

	territories := make(map[string]string, len(group))
	offerIDs := make(map[string]string, len(group))
	for territory, offer := range group {
		territories[territory] = offerPricePointID(offer)
		offerIDs[territory] = offer.ID
	}

	state := subscriptionIntroductoryOfferModel{
		ID:                         types.StringValue(subscriptionID),
		SubscriptionID:             types.StringValue(subscriptionID),
		OfferMode:                  types.StringValue(string(anchor.Attributes.OfferMode)),
		Duration:                   types.StringValue(string(anchor.Attributes.Duration)),
		NumberOfPeriods:            types.Int64Value(int64(anchor.Attributes.NumberOfPeriods)),
		StartDate:                  stringOrNull(anchor.Attributes.StartDate),
		EndDate:                    stringOrNull(anchor.Attributes.EndDate),
		TargetSubscriptionPlanType: types.StringNull(),
		TerritoryIDs:               types.SetNull(types.StringType),
		PricePointIDs:              types.MapNull(types.StringType),
	}
	if planType := anchor.Attributes.TargetSubscriptionPlanType; planType != nil {
		state.TargetSubscriptionPlanType = types.StringValue(string(*planType))
	}

	resp.Diagnostics.Append(setOfferTerritories(ctx, &state, territories, offerIDs)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// createOffers creates the offer in the named territories and records each new
// record in offerIDs.
func (r *subscriptionIntroductoryOfferResource) createOffers(
	ctx context.Context,
	plan subscriptionIntroductoryOfferModel,
	territories map[string]string,
	toCreate []string,
	offerIDs map[string]string,
	diags *diag.Diagnostics,
) {
	if len(toCreate) == 0 {
		return
	}

	subscriptionID := plan.SubscriptionID.ValueString()

	attrs := models.SubscriptionIntroductoryOfferAttributes{
		StartDate:       stringOrNil(plan.StartDate),
		EndDate:         stringOrNil(plan.EndDate),
		Duration:        models.SubscriptionOfferDuration(plan.Duration.ValueString()),
		OfferMode:       models.SubscriptionOfferMode(plan.OfferMode.ValueString()),
		NumberOfPeriods: int(plan.NumberOfPeriods.ValueInt64()),
	}
	if planType := stringOrNil(plan.TargetSubscriptionPlanType); planType != nil {
		t := models.SubscriptionPlanType(*planType)
		attrs.TargetSubscriptionPlanType = &t
	}

	inputs := make([]apple.SubscriptionIntroductoryOfferTerritory, 0, len(toCreate))
	for _, territory := range toCreate {
		inputs = append(inputs, apple.SubscriptionIntroductoryOfferTerritory{
			TerritoryID:  territory,
			PricePointID: territories[territory],
		})
	}

	tflog.Debug(ctx, "Creating introductory offers", map[string]interface{}{"territory_count": len(inputs)})

	created, err := r.client.CreateSubscriptionIntroductoryOffers(subscriptionID, attrs, inputs, nil)
	if err != nil {
		errMsg := err.Error()
		switch {
		case strings.Contains(errMsg, "were accepted, but reading them back failed"):
			diags.AddError(
				"Error Reading Created Introductory Offers",
				fmt.Sprintf("Apple accepted the introductory offer for %d territory(ies) on subscription "+
					"'%s', but reading the new records back failed: %s\n\nThe offers exist in App Store "+
					"Connect and are not in Terraform state. Import them as \"%s/<offer_id>\" with any one "+
					"territory's record, which the apple_subscription_introductory_offers data source lists.",
					len(inputs), subscriptionID, errMsg, subscriptionID),
			)
		case strings.Contains(errMsg, "not found") || strings.Contains(errMsg, "404"):
			diags.AddError(
				"Subscription or Price Point Not Found",
				fmt.Sprintf("Could not create the introductory offer on subscription '%s': %s\n\n"+
					"A price point ID is only valid for the subscription it was read from — it encodes the "+
					"subscription, the territory and the tier together. Read it from "+
					"apple_subscription_price_points scoped to subscription '%s', or from "+
					"apple_subscription_price_point_equalizations on a price point of that subscription.",
					subscriptionID, errMsg, subscriptionID),
			)
		default:
			diags.AddError(
				"Error Creating Subscription Introductory Offer",
				fmt.Sprintf("Could not create the introductory offer in %d territory(ies) on subscription "+
					"'%s': %s\n\nApple commits the request whole or not at all, so nothing was created. "+
					"A territory holds one introductory offer at a time, so an offer whose dates overlap "+
					"another one's in the same territory is refused — including one made in App Store "+
					"Connect, which the apple_subscription_introductory_offers data source lists. The "+
					"subscription also has to be available in every territory named, through "+
					"apple_subscription_availability, and priced.", len(inputs), subscriptionID, errMsg),
			)
		}
		tflog.Error(ctx, "Failed to create introductory offers", map[string]interface{}{"error": errMsg})
		return
	}

	var missing []string
	for _, territory := range toCreate {
		offer, ok := created[territory]
		if !ok {
			missing = append(missing, territory)
			continue
		}
		offerIDs[territory] = offer.ID
	}

	if len(missing) > 0 {
		diags.AddError(
			"Introductory Offer Missing After Create",
			fmt.Sprintf("Apple accepted the introductory offer on subscription '%s' but reports no new "+
				"record in %s. The records it did create are in state, and this resource is marked for "+
				"replacement so the next apply starts again.", subscriptionID, strings.Join(missing, ", ")),
		)
	}
}

// deleteOffers deletes the given records, removing each from current as it
// goes. A record Apple no longer has counts as deleted.
func (r *subscriptionIntroductoryOfferResource) deleteOffers(ctx context.Context, toDelete map[string]string, current map[string]string, diags *diag.Diagnostics) {
	territories := sortedKeys(toDelete)

	var failures []string
	for _, territory := range territories {
		offerID := toDelete[territory]

		err := r.client.DeleteSubscriptionIntroductoryOffer(offerID, nil)
		if err != nil && !strings.Contains(err.Error(), "not found") && !strings.Contains(err.Error(), "404") {
			failures = append(failures, fmt.Sprintf("%s (%s): %s", territory, offerID, err.Error()))
			continue
		}

		delete(current, territory)
	}

	if len(failures) > 0 {
		sort.Strings(failures)
		diags.AddError(
			"Error Deleting Subscription Introductory Offer",
			fmt.Sprintf("Apple refused to delete the introductory offer in %d of %d territory(ies):\n\n%s\n\n"+
				"The records that were deleted are gone from state; the ones listed remain in it and in App "+
				"Store Connect. If Apple will not delete an offer that is already running, set end_date to "+
				"end it instead.", len(failures), len(territories), strings.Join(failures, "\n")),
		)
		tflog.Error(ctx, "Failed to delete introductory offers", map[string]interface{}{"failures": len(failures)})
	}
}

// updateEndDates applies a new end date to the records that stay, reporting
// whether every one of them took it.
func (r *subscriptionIntroductoryOfferResource) updateEndDates(ctx context.Context, endDate types.String, territories []string, current map[string]string, diags *diag.Diagnostics) bool {
	var failures []string
	for _, territory := range territories {
		offerID := current[territory]
		if _, err := r.client.UpdateSubscriptionIntroductoryOfferEndDate(offerID, stringOrNil(endDate), nil); err != nil {
			failures = append(failures, fmt.Sprintf("%s (%s): %s", territory, offerID, err.Error()))
		}
	}

	if len(failures) > 0 {
		diags.AddError(
			"Error Updating Introductory Offer End Date",
			fmt.Sprintf("Apple refused the new end date in %d of %d territory(ies):\n\n%s\n\n"+
				"State keeps the previous end date, so the next apply sends the new one to every territory "+
				"again.", len(failures), len(territories), strings.Join(failures, "\n")),
		)
		tflog.Error(ctx, "Failed to update introductory offer end dates", map[string]interface{}{"failures": len(failures)})
		return false
	}

	return true
}
