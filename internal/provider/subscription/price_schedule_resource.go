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

	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var (
	_ resource.Resource                = &subscriptionPriceScheduleResource{}
	_ resource.ResourceWithConfigure   = &subscriptionPriceScheduleResource{}
	_ resource.ResourceWithImportState = &subscriptionPriceScheduleResource{}
)

func NewSubscriptionPriceScheduleResource() resource.Resource {
	return &subscriptionPriceScheduleResource{}
}

type subscriptionPriceScheduleResource struct {
	client *apple.Client
}

// Metadata returns the resource type name.
func (r *subscriptionPriceScheduleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_subscription_price_schedule"
}

// Schema defines the schema for the resource.
func (r *subscriptionPriceScheduleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Sets every price of an auto-renewable subscription in one request.\n\n" +
			"This is the bulk counterpart to `apple_subscription_price`. That resource maps onto " +
			"`POST /v1/subscriptionPrices`, which takes one price point in one territory, so pricing a " +
			"subscription in all 175 App Store storefronts through it means 175 resources and 175 API " +
			"calls on apply — and a refresh that lists the subscription's whole price collection once per " +
			"resource, because Apple publishes no `GET` for a single price. This resource sends the same " +
			"set as a single `PATCH /v1/subscriptions/{id}` carrying the prices inline, and reads them " +
			"back in one listing. Once the subscription is approved Apple accepts a price only as a " +
			"change, so the resource then sends one `POST /v1/subscriptionPrices` for each territory " +
			"whose price point differs from the one in effect.\n\n" +
			"Pair it with `apple_subscription_price_point_equalizations`, which maps a base price point " +
			"to the equivalent point in every other territory: the two together are what App Store " +
			"Connect's own price matrix does.\n\n" +
			"A price is never expressed as a number. Apple publishes a catalogue of price points, each " +
			"fixing a customer price and the developer proceeds for one territory, and a price references " +
			"one of them. Price point IDs are scoped to the subscription they were read from.\n\n" +
			"~> **An `apple_subscription_availability` has to exist first.** Apple rejects pricing for a " +
			"subscription with no availability record, and answers with a message that mentions neither " +
			"availability nor a territory. Nothing in a price references one, so declare the ordering " +
			"with `depends_on`.\n\n" +
			"~> **Apple refuses price writes while the subscription is in review** " +
			"(`WAITING_FOR_REVIEW`, `IN_REVIEW`, `PENDING_BINARY_APPROVAL`). In `MISSING_METADATA`, " +
			"`READY_TO_SUBMIT` and `DEVELOPER_ACTION_NEEDED` the prices are set as initial prices; in " +
			"any other state (for example `APPROVED`) a changed price is a new price record that takes " +
			"effect on `start_date`, or as soon as Apple allows when it is unset. Once a price change " +
			"is in effect it cannot be reverted.\n\n" +
			"~> **This resource cannot be destroyed.** A subscription that has been priced stays priced, " +
			"and Apple deletes only price changes scheduled for the future — a live price is what the " +
			"subscription currently costs. `terraform destroy` drops the resource from state and warns; " +
			"the prices remain in App Store Connect. Deleting the subscription removes them with it.\n\n" +
			"Do not manage the same subscription with both this resource and `apple_subscription_price`: " +
			"each write here replaces the subscription's manual price set, so a price only the other " +
			"resource knows about would be removed behind its back.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The ID of the subscription these prices belong to. A subscription " +
					"has no separate price schedule record at Apple — unlike an in-app purchase — so " +
					"this mirrors `subscription_id`.",
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"subscription_id": schema.StringAttribute{
				MarkdownDescription: "The ID of the `apple_subscription` these prices apply to. Cannot be " +
					"changed after creation.",
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"prices": schema.SetNestedAttribute{
				MarkdownDescription: "The complete set of prices the subscription should have. A set, " +
					"because Apple returns them in no particular order and their order carries no " +
					"meaning. At least one is required — a subscription stays in `MISSING_METADATA` " +
					"until it has a price.\n\n" +
					"While the subscription still accepts metadata every write replaces the whole set, so " +
					"a price removed from the configuration is removed at Apple. On an approved " +
					"subscription only the territories named here are written, and a price already in " +
					"effect cannot be withdrawn.",
				Required: true,
				Validators: []validator.Set{
					setvalidator.SizeAtLeast(1),
				},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"price_point_id": schema.StringAttribute{
							MarkdownDescription: "The ID of the Apple price point this price references. " +
								"Obtain it from the `apple_subscription_price_points` data source, or " +
								"from `apple_subscription_price_point_equalizations` for the fan-out " +
								"across territories — the ID encodes the subscription, the territory and " +
								"the price tier together, so a price point from one subscription cannot " +
								"be used on another.",
							Required: true,
						},
						"territory_id": schema.StringAttribute{
							MarkdownDescription: "The three-letter Apple territory code this price applies " +
								"to, for example `USA`, `GBR` or `EGY`. Optional: a price point already " +
								"encodes its territory and Apple infers it. Setting it makes an " +
								"equalized fan-out self-describing, which is the case it exists for.",
							Optional:   true,
							Validators: []validator.String{TerritoryValidator},
						},
						"start_date": schema.StringAttribute{
							MarkdownDescription: "The date the price takes effect, as a plain date in " +
								"`YYYY-MM-DD` form — not a timestamp. Unset is 3 days from today (UTC) " +
								"for a price change on an approved subscription: Apple refuses an undated one " +
								"as an initial price, and accepts a change only one to two days ahead or more. " +
								"Apple ignores dates for an initial price, which is what is written until the " +
								"subscription is approved.",
							Optional:   true,
							Validators: []validator.String{StartDateValidator},
						},
						"preserve_current_price": schema.BoolAttribute{
							MarkdownDescription: "Whether existing subscribers keep the price they signed " +
								"up at rather than moving to this one. Applies when this price " +
								"replaces an earlier one, which is how an approved subscription is " +
								"changed.",
							Optional: true,
						},
						"plan_type": schema.StringAttribute{
							MarkdownDescription: "Which plan the price applies to. Unset is the ordinary price, which Apple " +
								"reports as `UPFRONT`. Set `MONTHLY` only for a monthly-payment plan on a " +
								"longer commitment.",
							Optional:   true,
							Validators: []validator.String{PlanTypeValidator},
						},
					},
				},
			},
		},
	}
}

// Create a new resource.
func (r *subscriptionPriceScheduleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	tflog.Info(ctx, "Creating subscription price schedule resource")

	var plan subscriptionPriceScheduleModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.write(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

// Read refreshes the Terraform state with the latest data.
//
// One listing of the subscription's price collection covers every price, where
// a configuration built from apple_subscription_price resources pays for that
// listing once per territory. Prices are matched against state by price point;
// the dates and flags on a price already in state are left as configured -- see
// reconcileSchedulePrices.
func (r *subscriptionPriceScheduleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	tflog.Debug(ctx, "Reading subscription price schedule resource")

	var state subscriptionPriceScheduleModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	subscriptionID := state.SubscriptionID.ValueString()
	ctx = tflog.SetField(ctx, "subscription_id", subscriptionID)

	prices, err := r.client.GetSubscriptionPrices(subscriptionID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "404") {
			tflog.Info(ctx, "Subscription not found, removing price schedule from state")
			resp.State.RemoveResource(ctx)
			return
		}

		resp.Diagnostics.AddError(
			"Error Reading Subscription Price Schedule",
			fmt.Sprintf("Could not read the prices of subscription '%s': %s", subscriptionID, err.Error()),
		)
		return
	}

	// A subscription with no prices at all is one that has been un-priced
	// outside Terraform, which is the only way this resource can cease to
	// exist: there is no record of its own to delete.
	if len(prices) == 0 {
		tflog.Info(ctx, "Subscription has no prices, removing price schedule from state")
		resp.State.RemoveResource(ctx)
		return
	}

	state.ID = types.StringValue(subscriptionID)
	state.Prices = reconcileSchedulePrices(state.Prices, prices, today())

	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
}

// Update replaces the price set.
//
// Apple's PATCH names the complete set of manual prices the subscription should
// have afterwards, which is exactly the semantics Terraform wants for an
// in-place update -- so this is a genuine update rather than a replacement.
func (r *subscriptionPriceScheduleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	tflog.Info(ctx, "Updating subscription price schedule resource")

	var plan subscriptionPriceScheduleModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.write(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

// Delete drops the schedule from state.
//
// There is no record to delete: the prices are the subscription's, and Apple
// deletes only price changes scheduled for the future. Warn and let Terraform
// forget them, the way apple_device does for a deletion Apple cannot perform
// either.
func (r *subscriptionPriceScheduleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state subscriptionPriceScheduleModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.AddWarning(
		"Subscription Prices Not Deleted",
		fmt.Sprintf("A subscription that has been priced stays priced: Apple deletes only price changes "+
			"scheduled for the future, and a price already in effect is what subscription '%s' currently "+
			"costs. Terraform has removed the resource from state only, and the prices still apply in "+
			"App Store Connect.\n\nTo stop selling the subscription, narrow its "+
			"apple_subscription_availability or remove it from sale in App Store Connect. Deleting the "+
			"subscription removes its prices along with it.",
			state.SubscriptionID.ValueString()),
	)

	tflog.Info(ctx, "Subscription price schedule removed from state", map[string]interface{}{
		"subscription_id": state.SubscriptionID.ValueString(),
	})
}

// Configure adds the provider configured client to the resource.
func (r *subscriptionPriceScheduleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// ImportState imports the prices of a subscription.
//
// The import ID is the subscription ID: the prices are reachable only through
// the collection hanging off it, and this resource has no record of its own.
func (r *subscriptionPriceScheduleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	tflog.Info(ctx, "Importing subscription price schedule resource", map[string]interface{}{"import_id": req.ID})

	prices, err := r.client.GetSubscriptionPrices(req.ID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Importing Subscription Price Schedule",
			fmt.Sprintf("Could not read the prices of subscription '%s': %s\n\n"+
				"The import ID is the subscription ID — a subscription has no separate price schedule "+
				"record, and its prices are only reachable through the collection hanging off it.",
				req.ID, err.Error()),
		)
		return
	}

	if len(prices) == 0 {
		resp.Diagnostics.AddError(
			"Subscription Has No Prices",
			fmt.Sprintf("Subscription '%s' has no prices to import, so there is nothing for this "+
				"resource to adopt. Apply the configuration instead of importing it.", req.ID),
		)
		return
	}

	state := subscriptionPriceScheduleModel{
		ID:             types.StringValue(req.ID),
		SubscriptionID: types.StringValue(req.ID),
		Prices:         reconcileSchedulePrices(nil, prices, today()),
	}

	diags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
}

// defaultLeadDays is how far ahead an approved subscription's price change
// starts when start_date is unset. Apple reads a POST without a startDate as an
// initial price and refuses it once the subscription is approved (409), so a
// change is always dated. Apple's help says the soonest date it accepts is
// "generally 1 to 2 days in advance", by time zone; the storefront furthest
// ahead of UTC (+14h) is a calendar day ahead of the UTC date, so UTC today
// plus 3 days is two days ahead there and more everywhere else.
const defaultLeadDays = 3

// now is the clock; tests replace it.
var now = time.Now

// defaultStartDate is the date an undated change on an approved subscription is sent with.
func defaultStartDate() string {
	return now().UTC().AddDate(0, 0, defaultLeadDays).Format("2006-01-02")
}

// today is the date the pending-price rule in reconcileSchedulePrices compares
// start dates against; tests pass their own.
func today() string {
	return now().UTC().Format("2006-01-02")
}

// acceptsInitialPrice reports whether the subscription state is one in which Apple takes
// a price as an initial price, through the replace-all PATCH. In every other state a
// price is a change, and a change is a new record.
func acceptsInitialPrice(state string) bool {
	switch models.SubscriptionState(state) {
	case models.SubscriptionStateMissingMetadata,
		models.SubscriptionStateReadyToSubmit,
		models.SubscriptionStateDeveloperActionNeeded:
		return true
	}
	// A missing state keeps the behaviour from before the state was read.
	return state == ""
}

// inReview reports the states in which Apple refuses price writes altogether.
func inReview(state string) bool {
	switch models.SubscriptionState(state) {
	case models.SubscriptionStateWaitingForReview,
		models.SubscriptionStateInReview,
		models.SubscriptionStatePendingBinaryApproval:
		return true
	}
	return false
}

// write puts the price set at Apple the way the subscription's state allows.
//
// While the subscription accepts metadata, one PATCH replaces the whole set as
// initial prices. Apple rejects that request for any other state with 409
// "Initial price cannot be created again after subscription is approved": a
// price there is a change, made by POST /v1/subscriptionPrices, one territory
// at a time.
func (r *subscriptionPriceScheduleResource) write(ctx context.Context, plan *subscriptionPriceScheduleModel, diags *diag.Diagnostics) {
	subscriptionID := plan.SubscriptionID.ValueString()
	ctx = tflog.SetField(ctx, "subscription_id", subscriptionID)

	prices := make([]apple.SubscriptionManualPrice, 0, len(plan.Prices))
	for _, price := range plan.Prices {
		manual := apple.SubscriptionManualPrice{
			PricePointID:         price.PricePointID.ValueString(),
			TerritoryID:          price.TerritoryID.ValueString(),
			StartDate:            stringOrNil(price.StartDate),
			PreserveCurrentPrice: boolOrNil(price.PreserveCurrentPrice),
		}
		if planType := stringOrNil(price.PlanType); planType != nil {
			t := models.SubscriptionPlanType(*planType)
			manual.PlanType = &t
		}

		prices = append(prices, manual)
	}

	sub, err := r.client.GetSubscription(subscriptionID)
	if err != nil {
		diags.AddError(
			"Error Reading Subscription Before Writing Its Prices",
			fmt.Sprintf("Could not read subscription '%s' to learn its state: %s", subscriptionID, err.Error()),
		)
		return
	}
	state := ""
	if sub.Attributes.State != nil {
		state = string(*sub.Attributes.State)
	}
	ctx = tflog.SetField(ctx, "subscription_state", state)

	posting := !acceptsInitialPrice(state)
	count := len(prices)

	tflog.Debug(ctx, "Writing subscription price schedule", map[string]interface{}{"price_count": len(prices), "post": posting})

	if posting {
		err = r.postChangedPrices(subscriptionID, prices, &count)
	} else {
		_, err = r.client.SetSubscriptionPrices(subscriptionID, prices, nil)
	}
	if err != nil {
		errMsg := err.Error()
		switch {
		case strings.Contains(errMsg, "not found") || strings.Contains(errMsg, "404"):
			diags.AddError(
				"Subscription or Price Point Not Found",
				fmt.Sprintf("Could not write the prices of subscription '%s': %s\n\n"+
					"A price point ID is only valid for the subscription it was read from — it encodes "+
					"the subscription, the territory and the tier together. Read it from the "+
					"apple_subscription_price_points data source scoped to subscription '%s', or from "+
					"apple_subscription_price_point_equalizations on a price point of that subscription.",
					subscriptionID, err.Error(), subscriptionID),
			)
		case strings.Contains(errMsg, "Initial price cannot be created again"):
			diags.AddError(
				"Error Writing Subscription Price Schedule",
				fmt.Sprintf("Could not write %d price(s) for subscription '%s' (state %s): %s\n\n"+
					"Apple refuses an initial price once a subscription is approved, and reads a price "+
					"without a start date as one. The provider sends POST /v1/subscriptionPrices with a "+
					"start date for state %s, so this is a bug in the provider: please report it.", count, subscriptionID, state, err.Error(), state),
			)
		case strings.Contains(errMsg, "processing the pricing information"):
			// Apple answers a rejected price with "An error occurred while
			// processing the pricing information" and nothing else -- no mention
			// of which part of the request it objected to. The usual cause is
			// the one that message is least likely to suggest: the subscription
			// has no availability record, so there is no territory it can be
			// priced in. Apple requires availability before pricing and reports
			// its absence only here.
			diags.AddError(
				"Error Writing Subscription Price Schedule",
				fmt.Sprintf("Could not write %d price(s) for subscription '%s': %s\n\n"+
					"Apple rejects pricing with this message when the subscription has no availability "+
					"record, which it requires before any pricing. Add an apple_subscription_availability "+
					"for subscription '%s' listing every territory these prices cover, and make this "+
					"resource depend on it.\n\n"+
					"The same message also covers a price point that is not valid for this subscription, "+
					"and two prices for the same territory at the same start date.",
					count, subscriptionID, err.Error(), subscriptionID),
			)
		default:
			hint := fmt.Sprintf("The subscription is in state %s. On a state outside MISSING_METADATA, "+
				"READY_TO_SUBMIT and DEVELOPER_ACTION_NEEDED a price change is made by POST "+
				"/v1/subscriptionPrices, one request per territory, each with a start date (start_date, or "+
				"%d days from today when unset); this is the request that failed.", state, defaultLeadDays)
			if state == "" {
				hint = "Apple reported no state for the subscription."
			}
			if inReview(state) {
				hint += " Apple refuses price writes while the subscription is in review."
			}
			diags.AddError(
				"Error Writing Subscription Price Schedule",
				fmt.Sprintf("Could not write %d price(s) for subscription '%s': %s\n\n%s",
					count, subscriptionID, err.Error(), hint),
			)
		}
		tflog.Error(ctx, "Failed to write subscription price schedule", map[string]interface{}{"error": err.Error()})
		return
	}

	plan.ID = types.StringValue(subscriptionID)
}

// postChangedPrices creates one price record for each planned territory whose
// price point is not the one in effect. count is set to how many were needed.
func (r *subscriptionPriceScheduleResource) postChangedPrices(subscriptionID string, plan []apple.SubscriptionManualPrice, count *int) error {
	reported, err := r.client.GetSubscriptionPrices(subscriptionID)
	if err != nil {
		return fmt.Errorf("could not list the current prices: %w", err)
	}

	effective, pointTerritory := effectivePrices(reported)
	posts, err := pricesToPost(plan, effective, pointTerritory)
	if err != nil {
		return err
	}
	*count = len(posts)

	start := defaultStartDate()
	for _, price := range posts {
		if price.StartDate == nil {
			price.StartDate = &start
		}
		var territory *string
		if price.TerritoryID != "" {
			territory = &price.TerritoryID
		}
		attributes := &models.SubscriptionPriceCreateAttributes{
			StartDate:            price.StartDate,
			PreserveCurrentPrice: price.PreserveCurrentPrice,
			PlanType:             price.PlanType,
		}
		if _, err := r.client.CreateSubscriptionPrice(subscriptionID, price.PricePointID, territory, attributes, nil); err != nil {
			return err
		}
	}
	return nil
}

// pricesToPost decides which planned prices are changes. effective maps a
// slot (priceKey: territory and plan) to the price point in force (or scheduled) there; pointTerritory
// maps a price point to its territory, for a plan entry that names no
// territory_id. A territory in effect but absent from the plan is not touched.
func pricesToPost(plan []apple.SubscriptionManualPrice, effective, pointTerritory map[string]string) ([]apple.SubscriptionManualPrice, error) {
	var posts []apple.SubscriptionManualPrice
	for _, price := range plan {
		territory := price.TerritoryID
		if territory == "" {
			territory = pointTerritory[price.PricePointID]
		}
		if territory == "" {
			return nil, fmt.Errorf("cannot tell which territory price point '%s' belongs to: set territory_id on that price", price.PricePointID)
		}
		if effective[priceKey(territory, price.PlanType)] == price.PricePointID {
			continue
		}
		price.TerritoryID = territory
		posts = append(posts, price)
	}
	return posts, nil
}

// effectiveRecords reduces Apple's listing to one record per territory: the
// preserved records are dropped and the one with the greatest start date is
// kept (a null start date is in force since before any dated record). The
// result is the current price, or the pending change when one is scheduled.
func effectiveRecords(reported []models.SubscriptionPrice) map[string]models.SubscriptionPrice {
	effective := make(map[string]models.SubscriptionPrice)
	for _, price := range reported {
		if price.Attributes.Preserved != nil && *price.Attributes.Preserved {
			continue
		}
		key := recordKey(price)
		if key == "" {
			continue
		}
		if kept, ok := effective[key]; !ok || startsAfter(price, kept) {
			effective[key] = price
		}
	}
	return effective
}

// priceKey names the slot a price fills: a territory and a plan. An unset
// plan is UPFRONT, on both sides: Apple reports planType UPFRONT on the
// ordinary price of a subscription whose price never named one (read live from
// prod's pro_monthly, 2026-11-12).
func priceKey(territory string, plan *models.SubscriptionPlanType) string {
	p := models.SubscriptionPlanType("UPFRONT")
	if plan != nil && *plan != "" {
		p = *plan
	}
	return territory + "/" + string(p)
}

// recordKey is the slot a record prices; the territory falls back to its price
// point when Apple did not report one.
func recordKey(price models.SubscriptionPrice) string {
	if price.Relationships == nil {
		return ""
	}
	if t := price.Relationships.Territory; t != nil && t.Data.ID != "" {
		return priceKey(t.Data.ID, price.Attributes.PlanType)
	}
	if p := price.Relationships.SubscriptionPricePoint; p != nil {
		return priceKey(p.Data.ID, price.Attributes.PlanType)
	}
	return ""
}

func pointID(price models.SubscriptionPrice) string {
	if price.Relationships == nil || price.Relationships.SubscriptionPricePoint == nil {
		return ""
	}
	return price.Relationships.SubscriptionPricePoint.Data.ID
}

// startsAfter reports whether a starts strictly later than b; a null start
// date is earlier than every date.
func startsAfter(a, b models.SubscriptionPrice) bool {
	as, bs := "", ""
	if a.Attributes.StartDate != nil {
		as = *a.Attributes.StartDate
	}
	if b.Attributes.StartDate != nil {
		bs = *b.Attributes.StartDate
	}
	return as > bs
}

// effectivePrices returns the effective price point per territory, and the
// territory of every price point the listing mentions.
func effectivePrices(reported []models.SubscriptionPrice) (effective, pointTerritory map[string]string) {
	effective = make(map[string]string)
	pointTerritory = make(map[string]string)
	for _, price := range reported {
		if id := pointID(price); id != "" && price.Relationships != nil &&
			price.Relationships.Territory != nil && price.Relationships.Territory.Data.ID != "" {
			pointTerritory[id] = price.Relationships.Territory.Data.ID
		}
	}
	for key, price := range effectiveRecords(reported) {
		effective[key] = pointID(price)
	}
	return effective, pointTerritory
}

// reconcileSchedulePrices matches Apple's prices against what state holds.
//
// Apple lists every price record of a subscription, including those that ended
// and those kept for subscribers who stayed (preserved). Only the effective
// record of each territory is matched -- see effectiveRecords -- so a state
// price whose price point is no longer effective is dropped, and shows as a
// diff only if the configuration still wants it.
//
// A price state already knows about is kept exactly as configured. Apple
// reports the territory of every price whether or not one was sent, and reports
// planType on prices that never named one, so adopting what it returns would
// show as a permanent diff against a configuration that set neither -- the same
// reason territory_id had to become Optional+Computed on
// apple_subscription_price, solved here by not refreshing rather than by
// widening the attribute, because a computed attribute inside a set makes the
// element unknown at plan time.
//
// An effective price Apple reports that state does not know about is added with
// Apple's own values, so a price added in App Store Connect surfaces as drift
// rather than silently disappearing. One that has not started yet (its start
// date is after today) is adopted without that date and with
// preserve_current_price set when a record of its territory that it replaces is
// preserved, which is the shape of the configuration that would schedule it.
func reconcileSchedulePrices(state []subscriptionSchedulePriceModel, reported []models.SubscriptionPrice, today string) []subscriptionSchedulePriceModel {
	byPricePoint := make(map[string]models.SubscriptionPrice)
	for _, price := range effectiveRecords(reported) {
		if id := pointID(price); id != "" {
			byPricePoint[id] = price
		}
	}

	reconciled := make([]subscriptionSchedulePriceModel, 0, len(byPricePoint))
	for _, price := range state {
		id := price.PricePointID.ValueString()
		if _, ok := byPricePoint[id]; !ok {
			continue
		}
		reconciled = append(reconciled, price)
		delete(byPricePoint, id)
	}

	for _, price := range byPricePoint {
		adopted := subscriptionSchedulePriceModel{
			PricePointID: types.StringValue(pointID(price)),
			StartDate:    stringOrNull(price.Attributes.StartDate),
			// preserveCurrentPrice is an instruction, not a property: Apple
			// reports only its outcome, through "preserved" on the record that
			// was kept. Leave the input null so it matches a configuration
			// that does not set it, except for a pending price below.
			PreserveCurrentPrice: types.BoolNull(),
		}

		if price.Attributes.StartDate != nil && *price.Attributes.StartDate > today {
			adopted.StartDate = types.StringNull()
			if supersedesPreserved(reported, price) {
				adopted.PreserveCurrentPrice = types.BoolValue(true)
			}
		}

		if price.Relationships != nil {
			adopted.TerritoryID = territoryID(price.Relationships.Territory)
		}
		if price.Attributes.PlanType != nil {
			adopted.PlanType = types.StringValue(string(*price.Attributes.PlanType))
		}

		reconciled = append(reconciled, adopted)
	}

	return reconciled
}

// supersedesPreserved reports whether another record of the pending price's
// territory is preserved, i.e. subscribers were held on what it replaces.
func supersedesPreserved(reported []models.SubscriptionPrice, pending models.SubscriptionPrice) bool {
	key := recordKey(pending)
	for _, other := range reported {
		if other.ID == pending.ID || recordKey(other) != key {
			continue
		}
		if other.Attributes.Preserved != nil && *other.Attributes.Preserved {
			return true
		}
	}
	return false
}
