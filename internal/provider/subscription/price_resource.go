// Copyright (c) AO Studio
// SPDX-License-Identifier: MPL-2.0

package subscription

import (
	"context"
	"fmt"
	"strings"

	"github.com/AhmedOsman00/terraform-provider-apple/internal/apple"
	"github.com/AhmedOsman00/terraform-provider-apple/internal/apple/models"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var (
	_ resource.Resource                = &subscriptionPriceResource{}
	_ resource.ResourceWithConfigure   = &subscriptionPriceResource{}
	_ resource.ResourceWithImportState = &subscriptionPriceResource{}
)

func NewSubscriptionPriceResource() resource.Resource {
	return &subscriptionPriceResource{}
}

type subscriptionPriceResource struct {
	client *apple.Client
}

// Metadata returns the resource type name.
func (r *subscriptionPriceResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_subscription_price"
}

// Schema defines the schema for the resource.
func (r *subscriptionPriceResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Schedules a price for an auto-renewable subscription in one territory.\n\n" +
			"A price is never expressed as a number. Apple publishes a catalogue of price points, each " +
			"fixing a customer price and the developer proceeds for one territory, and a price references " +
			"one of them — look one up with the `apple_subscription_price_points` data source. A " +
			"subscription stays in `MISSING_METADATA` until it has at least one price.\n\n" +
			"Every attribute forces replacement, because Apple publishes no `PATCH` for " +
			"`subscriptionPrices`: changing a price means creating a new record and deleting the old one, " +
			"which is exactly what Terraform does here.\n\n" +
			"~> **Use `apple_subscription_price_schedule` to price more than a handful of territories.** " +
			"This resource is one `POST /v1/subscriptionPrices` per territory, so a fan-out across the " +
			"App Store's 175 storefronts is 175 resources and 175 API calls — plus a refresh that lists " +
			"the subscription's whole price collection once per resource, because Apple publishes no " +
			"`GET` for a single price. The schedule resource sends the same set in one request. Do not " +
			"point both at the same subscription: every write of the schedule replaces the subscription's " +
			"manual price set.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The unique Apple-generated identifier for the scheduled price.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"subscription_id": schema.StringAttribute{
				MarkdownDescription: "The ID of the `apple_subscription` this price applies to.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"price_point_id": schema.StringAttribute{
				MarkdownDescription: "The ID of the Apple price point this price references. Obtain it from " +
					"the `apple_subscription_price_points` data source — the ID encodes the subscription, " +
					"the territory and the price tier together, so a price point from one subscription " +
					"cannot be used on another.",
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"territory_id": schema.StringAttribute{
				MarkdownDescription: "The three-letter Apple territory code this price applies to, for example " +
					"`USA`, `GBR` or `EGY`. Optional: a price point already belongs to a territory, and " +
					"Apple infers it. Set this only when deliberately equalizing prices across territories.\n\n" +
					"Computed when it is not configured, because Apple reports the territory of every price " +
					"whether or not one was sent. Leaving it merely optional made `Read` write Apple's value " +
					"into state against a configuration that held none, and since the attribute forces " +
					"replacement, the next plan destroyed the price to remove it.",
				Optional:   true,
				Computed:   true,
				Validators: []validator.String{TerritoryValidator},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
			},
			"start_date": schema.StringAttribute{
				MarkdownDescription: "The date the price takes effect, as a plain date in `YYYY-MM-DD` form — " +
					"not a timestamp. Omit it for a price that takes effect immediately.",
				Optional:   true,
				Validators: []validator.String{StartDateValidator},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"preserve_current_price": schema.BoolAttribute{
				MarkdownDescription: "Whether existing subscribers keep the price they signed up at rather " +
					"than moving to this one. Applies only when this price replaces an earlier one.",
				Optional: true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.RequiresReplace(),
				},
			},
			"plan_type": schema.StringAttribute{
				MarkdownDescription: "Which plan the price applies to: `MONTHLY` for the recurring price, or " +
					"`UPFRONT` for a pre-paid plan where the customer pays the whole term in advance.",
				Optional:   true,
				Computed:   true,
				Validators: []validator.String{PlanTypeValidator},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
			},
			"preserved": schema.BoolAttribute{
				MarkdownDescription: "Whether Apple actually held existing subscribers at their previous price. " +
					"This is the read side of `preserve_current_price` and is computed by Apple.",
				Computed: true,
			},
		},
	}
}

// territorySuffix names the territory in an error when one was configured.
// A price usually derives its territory from the price point, so the attribute
// is absent more often than not and an empty clause reads better than "<null>".
func territorySuffix(territoryID types.String) string {
	if territoryID.IsNull() || territoryID.ValueString() == "" {
		return ""
	}
	return fmt.Sprintf(" in territory '%s'", territoryID.ValueString())
}

// Create a new resource.
func (r *subscriptionPriceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	tflog.Info(ctx, "Creating subscription price resource")

	var plan subscriptionPriceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx = tflog.SetField(ctx, "subscription_id", plan.SubscriptionID.ValueString())
	ctx = tflog.SetField(ctx, "price_point_id", plan.PricePointID.ValueString())

	attributes := &models.SubscriptionPriceCreateAttributes{
		StartDate:            stringOrNil(plan.StartDate),
		PreserveCurrentPrice: boolOrNil(plan.PreserveCurrentPrice),
	}
	if planType := stringOrNil(plan.PlanType); planType != nil {
		t := models.SubscriptionPlanType(*planType)
		attributes.PlanType = &t
	}

	price, err := r.client.CreateSubscriptionPrice(
		plan.SubscriptionID.ValueString(),
		plan.PricePointID.ValueString(),
		stringOrNil(plan.TerritoryID),
		attributes,
		nil,
	)
	if err != nil {
		errMsg := err.Error()
		switch {
		case strings.Contains(errMsg, "not found") || strings.Contains(errMsg, "404"):
			resp.Diagnostics.AddError(
				"Subscription or Price Point Not Found",
				fmt.Sprintf("Could not create the price: %s\n\n"+
					"A price point ID is only valid for the subscription it was read from — it encodes "+
					"the subscription, the territory and the tier together. Read it from the "+
					"apple_subscription_price_points data source scoped to subscription '%s'.",
					err.Error(), plan.SubscriptionID.ValueString()),
			)
		case strings.Contains(errMsg, "processing the pricing information"):
			// Apple answers a rejected price with "An error occurred while
			// processing the pricing information" and nothing else -- no
			// mention of which part of the request it objected to. The usual
			// cause is the one that message is least likely to suggest: the
			// subscription has no availability record, so there is no territory
			// it can be priced in. Apple requires availability before pricing
			// and reports its absence only here.
			resp.Diagnostics.AddError(
				"Error Creating Subscription Price",
				fmt.Sprintf("Could not create price for subscription '%s' from price point '%s'%s: %s\n\n"+
					"Apple rejects a price with this message when the subscription has no availability "+
					"record, which it requires before any pricing. Add an apple_subscription_availability "+
					"for subscription '%s' listing the territory this price point belongs to, and make "+
					"this resource depend on it.\n\n"+
					"The same message also covers a price point that is not valid for this subscription, "+
					"and a second price for a territory that already has one at the same start date.",
					plan.SubscriptionID.ValueString(),
					plan.PricePointID.ValueString(),
					territorySuffix(plan.TerritoryID),
					err.Error(),
					plan.SubscriptionID.ValueString()),
			)
		default:
			// Name the price point and territory even when the message is
			// unfamiliar: they are the only inputs, and Apple's pricing errors
			// are rarely diagnosable without them.
			resp.Diagnostics.AddError(
				"Error Creating Subscription Price",
				fmt.Sprintf("Could not create price for subscription '%s' from price point '%s'%s: %s",
					plan.SubscriptionID.ValueString(),
					plan.PricePointID.ValueString(),
					territorySuffix(plan.TerritoryID),
					err.Error()),
			)
		}
		tflog.Error(ctx, "Failed to create subscription price", map[string]interface{}{"error": err.Error()})
		return
	}

	plan.ID = types.StringValue(price.ID)
	applyPriceAttributes(&plan, price)

	// Apple omits the territory relationship from the create response. Only a
	// read carries one, because the read asks for include=territory and a POST
	// takes no include parameter -- so a configuration that leaves territory_id
	// to be computed still held an unknown after apply, which the framework
	// rejects as an invalid result. Re-read the price to resolve it.
	if plan.TerritoryID.IsUnknown() {
		created, readErr := r.client.GetSubscriptionPrice(plan.SubscriptionID.ValueString(), price.ID)
		if readErr != nil {
			tflog.Warn(ctx, "Could not re-read the created price to resolve its territory", map[string]interface{}{
				"error": readErr.Error(),
			})
		} else {
			applyPriceAttributes(&plan, created)
		}

		// A territory Apple will not report is null, never unknown: an unknown
		// left in state fails the apply outright, while a null is filled in by
		// the next refresh without proposing a change -- territory_id is
		// computed, so a configuration that omits it accepts whatever Apple
		// reports.
		if plan.TerritoryID.IsUnknown() {
			plan.TerritoryID = types.StringNull()
		}
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

// Read refreshes the Terraform state with the latest data.
//
// Apple publishes no GET for a single subscriptionPrice, so this lists the
// subscription's price schedule and scans it -- the same shape the bundle ID
// capability resource is forced into, and the reason subscription_id has to be
// known before a price can be read at all.
func (r *subscriptionPriceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	tflog.Debug(ctx, "Reading subscription price resource")

	var state subscriptionPriceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx = tflog.SetField(ctx, "price_id", state.ID.ValueString())

	price, err := r.client.GetSubscriptionPrice(state.SubscriptionID.ValueString(), state.ID.ValueString())
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "404") {
			tflog.Info(ctx, "Subscription price not found, removing from state")
			resp.State.RemoveResource(ctx)
			return
		}

		resp.Diagnostics.AddError(
			"Error Reading Subscription Price",
			fmt.Sprintf("Could not read price '%s' on subscription '%s': %s",
				state.ID.ValueString(), state.SubscriptionID.ValueString(), err.Error()),
		)
		return
	}

	applyPriceAttributes(&state, price)

	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
}

// Update exists only to satisfy the resource interface.
//
// Every configurable attribute is RequiresReplace because Apple publishes no
// PATCH /v1/subscriptionPrices, so the framework should never route a change
// here. Reaching it means a plan modifier was dropped.
func (r *subscriptionPriceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan subscriptionPriceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.AddError(
		"Subscription Prices Cannot Be Updated",
		"Apple publishes no PATCH endpoint for subscription prices: a price change is a new price "+
			"record and the deletion of the old one. Every attribute of this resource forces "+
			"replacement, so Terraform should not have reached Update. Please report this as a "+
			"provider bug.",
	)
}

// Delete deletes the resource and removes the Terraform state on success.
func (r *subscriptionPriceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	tflog.Info(ctx, "Deleting subscription price resource")

	var state subscriptionPriceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx = tflog.SetField(ctx, "price_id", state.ID.ValueString())

	err := r.client.DeleteSubscriptionPrice(state.ID.ValueString(), nil)
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "404") {
			tflog.Info(ctx, "Subscription price already deleted")
			return
		}

		// Apple refuses to delete a price that is already in effect:
		//
		//   409 Cannot delete Subscription Price with id <id>.
		//       Only future price changes can be deleted.
		//
		// A live price is not a record that can be withdrawn -- it is what the
		// subscription currently costs, and the only way past it is to schedule
		// another price that supersedes it. Warn and let Terraform drop it from
		// state, the way apple_device does for a deletion Apple cannot perform
		// either. Deleting the subscription removes the price with it.
		if strings.Contains(err.Error(), "Only future price changes can be deleted") {
			resp.Diagnostics.AddWarning(
				"Subscription Price Removed From State Rather Than Deleted",
				fmt.Sprintf("Price '%s' is the price subscription '%s' currently charges, and Apple deletes "+
					"only price changes scheduled for the future. It has been removed from Terraform state "+
					"and still applies in App Store Connect.\n\nTo change what the subscription costs, add "+
					"another apple_subscription_price rather than destroying this one. Deleting the "+
					"subscription removes its prices along with it.",
					state.ID.ValueString(), state.SubscriptionID.ValueString()),
			)
			tflog.Info(ctx, "Subscription price is currently in effect; removed from state only")
			return
		}

		resp.Diagnostics.AddError(
			"Error Deleting Subscription Price",
			fmt.Sprintf("Could not delete price '%s': %s", state.ID.ValueString(), err.Error()),
		)
		return
	}

	tflog.Info(ctx, "Subscription price deleted successfully")
}

// Configure adds the provider configured client to the resource.
func (r *subscriptionPriceResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// ImportState imports an existing price.
//
// The composite "<subscription_id>/<price_id>" form is the only one accepted:
// Apple has no endpoint that reads a price by ID, so the parent subscription is
// what makes the price findable at all.
func (r *subscriptionPriceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	tflog.Info(ctx, "Importing subscription price resource", map[string]interface{}{"import_id": req.ID})

	subscriptionID, priceID, found := strings.Cut(req.ID, "/")
	if !found || subscriptionID == "" || priceID == "" {
		resp.Diagnostics.AddError(
			"Invalid Import ID",
			fmt.Sprintf("Subscription price import ID '%s' is not in the expected form.\n\n"+
				"Use \"<subscription_id>/<price_id>\".\n\n"+
				"The subscription ID is required because Apple publishes no GET for a single "+
				"subscription price — a price is only reachable through the collection hanging off "+
				"its subscription.", req.ID),
		)
		return
	}

	price, err := r.client.GetSubscriptionPrice(subscriptionID, priceID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Importing Subscription Price",
			fmt.Sprintf("Could not import price '%s' on subscription '%s': %s",
				priceID, subscriptionID, err.Error()),
		)
		return
	}

	state := subscriptionPriceModel{
		ID:             types.StringValue(price.ID),
		SubscriptionID: types.StringValue(subscriptionID),
	}
	applyPriceAttributes(&state, price)

	// preserve_current_price is an instruction, not a property: Apple reports
	// only the outcome, through "preserved". Leave the input null on import so
	// it matches a configuration that does not set it.
	state.PreserveCurrentPrice = types.BoolNull()

	diags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
}

// applyPriceAttributes copies Apple's reported attributes into a model.
func applyPriceAttributes(model *subscriptionPriceModel, price *models.SubscriptionPrice) {
	model.StartDate = stringOrNull(price.Attributes.StartDate)

	if price.Attributes.PlanType != nil {
		model.PlanType = types.StringValue(string(*price.Attributes.PlanType))
	} else {
		model.PlanType = types.StringNull()
	}

	if price.Attributes.Preserved != nil {
		model.Preserved = types.BoolValue(*price.Attributes.Preserved)
	} else {
		model.Preserved = types.BoolNull()
	}

	if price.Relationships != nil {
		if id := territoryID(price.Relationships.Territory); !id.IsNull() {
			model.TerritoryID = id
		}
		if price.Relationships.SubscriptionPricePoint != nil {
			if pointID := price.Relationships.SubscriptionPricePoint.Data.ID; pointID != "" {
				model.PricePointID = types.StringValue(pointID)
			}
		}
	}
}
