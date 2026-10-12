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
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var (
	_ resource.Resource                   = &subscriptionOfferCodeCustomCodeResource{}
	_ resource.ResourceWithConfigure      = &subscriptionOfferCodeCustomCodeResource{}
	_ resource.ResourceWithImportState    = &subscriptionOfferCodeCustomCodeResource{}
	_ resource.ResourceWithValidateConfig = &subscriptionOfferCodeCustomCodeResource{}
)

func NewSubscriptionOfferCodeCustomCodeResource() resource.Resource {
	return &subscriptionOfferCodeCustomCodeResource{}
}

type subscriptionOfferCodeCustomCodeResource struct {
	client *apple.Client
}

// Metadata returns the resource type name.
func (r *subscriptionOfferCodeCustomCodeResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_subscription_offer_code_custom_code"
}

// Schema defines the schema for the resource.
func (r *subscriptionOfferCodeCustomCodeResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a custom code for an `apple_subscription_offer_code`: one string, such as " +
			"`SCHOOL2026`, that many customers redeem, up to `number_of_codes` times in all. It is the shape for " +
			"a code printed in a newsletter or read out at a school; for codes handed to one person each, use " +
			"`apple_subscription_offer_code_one_time_use_code`.\n\n" +
			"~> **A code string is held for as long as it is active.** App Store Connect refuses a custom code " +
			"that another active offer of the same app already uses, and allows it again once that one is " +
			"deactivated. Because a replacement deactivates the old record before creating the new one, " +
			"changing `number_of_codes` or `expiration_date` re-issues the same string — but every attribute " +
			"except `active` is fixed at creation, so any change replaces the code, and redemptions already " +
			"counted against the old record do not carry over.\n\n" +
			offerCodePermanenceNote,

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The unique Apple-generated identifier for the custom code.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"offer_code_id": schema.StringAttribute{
				MarkdownDescription: "The ID of the `apple_subscription_offer_code` this code redeems. Cannot be " +
					"changed after creation.",
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"custom_code": schema.StringAttribute{
				MarkdownDescription: fmt.Sprintf("The code customers type, up to %d letters and digits — App "+
					"Store Connect accepts no special characters. Cannot be changed after creation.",
					maxCustomCodeLength),
				Required: true,
				Validators: []validator.String{
					CustomCodeValidator,
					stringvalidator.LengthBetween(1, maxCustomCodeLength),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"number_of_codes": schema.Int64Attribute{
				MarkdownDescription: fmt.Sprintf("How many times the code can be redeemed in all, from 1 to "+
					"%d. For more, create another custom code with the same string once this one is "+
					"deactivated. Cannot be changed after creation.", maxCustomCodeRedemptions),
				Required:   true,
				Validators: []validator.Int64{int64validator.Between(1, maxCustomCodeRedemptions)},
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"expiration_date": schema.StringAttribute{
				MarkdownDescription: "The last day the code redeems, as a plain date in `YYYY-MM-DD` form — at " +
					"most six months after it is created; codes expire at 12:00 a.m. Pacific Time on that " +
					"date. Optional: a custom code without one does not expire. Cannot be changed after " +
					"creation.",
				Optional:   true,
				Validators: []validator.String{ExpirationDateValidator},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"active": schema.BoolAttribute{
				MarkdownDescription: "Whether the code redeems. Defaults to `true`. Setting it to `false` ends " +
					"it in place, releases the string for another offer of the app, and is **final**: a " +
					"deactivated code cannot be turned back on, so setting it back to `true` plans a new " +
					"custom code.",
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(true),
				PlanModifiers: []planmodifier.Bool{
					reactivationRequiresReplace(),
				},
			},
			"created_date": schema.StringAttribute{
				MarkdownDescription: "When Apple created the code.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

// ValidateConfig enforces App Store Connect's six-month ceiling on an
// expiration date. It is checked here rather than by a schema validator
// because it depends on today's date; see checkExpirationWithinSixMonths.
func (r *subscriptionOfferCodeCustomCodeResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config subscriptionOfferCodeCustomCodeModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if config.ExpirationDate.IsNull() || config.ExpirationDate.IsUnknown() {
		return
	}
	if err := checkExpirationWithinSixMonths(config.ExpirationDate.ValueString(), time.Now()); err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("expiration_date"), "Invalid Expiration Date", err.Error())
	}
}

// Create creates the custom code.
func (r *subscriptionOfferCodeCustomCodeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	tflog.Info(ctx, "Creating subscription offer code custom code resource")

	var plan subscriptionOfferCodeCustomCodeModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	offerCodeID := plan.OfferCodeID.ValueString()
	ctx = tflog.SetField(ctx, "offer_code_id", offerCodeID)

	code, err := r.client.CreateSubscriptionOfferCodeCustomCode(offerCodeID, models.SubscriptionOfferCodeCustomCodeCreateAttrs{
		CustomCode:     plan.CustomCode.ValueString(),
		NumberOfCodes:  int(plan.NumberOfCodes.ValueInt64()),
		ExpirationDate: stringOrNil(plan.ExpirationDate),
	}, nil)
	if err != nil {
		errMsg := err.Error()
		lower := strings.ToLower(errMsg)
		switch {
		case strings.Contains(lower, "already") || strings.Contains(lower, "duplicate") ||
			strings.Contains(lower, "in use") || strings.Contains(lower, "unique"):
			resp.Diagnostics.AddError(
				"Custom Code Already in Use",
				fmt.Sprintf("Could not create custom code '%s' for offer code '%s': %s\n\n"+
					"App Store Connect refuses a custom code that another active offer of the same app "+
					"already uses. Deactivate the code that holds it — set active = false on the resource "+
					"that manages it, or deactivate it in App Store Connect — or choose another string. "+
					"It also refuses any code once the offer itself has been deactivated.",
					plan.CustomCode.ValueString(), offerCodeID, errMsg),
			)
		case strings.Contains(errMsg, "not found") || strings.Contains(errMsg, "404"):
			resp.Diagnostics.AddError(
				"Subscription Offer Code Not Found",
				fmt.Sprintf("Could not create custom code '%s': offer code '%s' does not exist: %s",
					plan.CustomCode.ValueString(), offerCodeID, errMsg),
			)
		default:
			resp.Diagnostics.AddError(
				"Error Creating Custom Code",
				fmt.Sprintf("Could not create custom code '%s' for offer code '%s': %s\n\n"+
					"An expiration date can be at most six months away, and an app's offer codes share a "+
					"quarterly quota across every subscription.",
					plan.CustomCode.ValueString(), offerCodeID, errMsg),
			)
		}
		tflog.Error(ctx, "Failed to create custom code", map[string]interface{}{"error": errMsg})
		return
	}

	plan.ID = types.StringValue(code.ID)
	plan.CreatedDate = stringOrNull(code.Attributes.CreatedDate)

	// State first, as Apple holds it -- active -- for the reason the offer
	// code's Create gives.
	configuredActive := plan.Active
	plan.Active = types.BoolValue(true)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
	if resp.Diagnostics.HasError() || configuredActive.ValueBool() {
		return
	}

	if _, err := r.client.SetSubscriptionOfferCodeCustomCodeActive(code.ID, false, nil); err != nil {
		// A warning, for the reason the offer code's Create gives: an error
		// would taint this and create another that Apple can never delete.
		// State keeps the planned value; the next refresh reads Apple's.
		plan.Active = configuredActive
		resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
		resp.Diagnostics.AddWarning(
			"Custom Code Created Active",
			fmt.Sprintf("Custom code '%s' was created, but deactivating it as configured failed: %s\n\n"+
				"It is still active at Apple. The next plan shows it, and the next apply deactivates it.",
				code.ID, err.Error()),
		)
		return
	}
	plan.Active = configuredActive
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// Read refreshes the custom code from Apple.
func (r *subscriptionOfferCodeCustomCodeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state subscriptionOfferCodeCustomCodeModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	code, err := r.client.GetSubscriptionOfferCodeCustomCode(state.ID.ValueString())
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "404") {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error Reading Custom Code",
			fmt.Sprintf("Could not read custom code '%s': %s", state.ID.ValueString(), err.Error()),
		)
		return
	}

	applyCustomCode(&state, code)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update changes "active", the one member of Apple's update request.
func (r *subscriptionOfferCodeCustomCodeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan subscriptionOfferCodeCustomCodeModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if _, err := r.client.SetSubscriptionOfferCodeCustomCodeActive(plan.ID.ValueString(), plan.Active.ValueBool(), nil); err != nil {
		resp.Diagnostics.AddError(
			"Error Updating Custom Code",
			fmt.Sprintf("Could not set active=%t on custom code '%s': %s",
				plan.Active.ValueBool(), plan.ID.ValueString(), err.Error()),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// Delete deactivates the code and drops it from state; Apple publishes no
// DELETE. Deactivating also releases the string for another offer of the app.
func (r *subscriptionOfferCodeCustomCodeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state subscriptionOfferCodeCustomCodeModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := state.ID.ValueString()
	if state.Active.IsNull() || state.Active.ValueBool() {
		if _, err := r.client.SetSubscriptionOfferCodeCustomCodeActive(id, false, nil); err != nil {
			if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "404") {
				tflog.Warn(ctx, "Custom code not found while deactivating, treating as already removed")
				return
			}
			resp.Diagnostics.AddError(
				"Unable to Deactivate Custom Code",
				fmt.Sprintf("Apple's API cannot delete a custom code, so Terraform deactivates it instead. "+
					"Deactivating custom code '%s' failed: %s", id, err.Error()),
			)
			return
		}
	}

	resp.Diagnostics.AddWarning(
		"Custom Code Deactivated Rather Than Deleted",
		fmt.Sprintf("The App Store Connect API cannot delete a custom code. Custom code '%s' (%s) has been "+
			"deactivated and removed from Terraform state, but it remains in App Store Connect and cannot be "+
			"reactivated. The string itself is free for another offer of the app.",
			state.CustomCode.ValueString(), id),
	)
}

// Configure adds the provider configured client to the resource.
func (r *subscriptionOfferCodeCustomCodeResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// ImportState imports a custom code by Apple's ID. The offer is read back
// through include=offerCode, so no composite form is needed.
func (r *subscriptionOfferCodeCustomCodeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id := strings.TrimSpace(req.ID)

	code, err := r.client.GetSubscriptionOfferCodeCustomCode(id)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Importing Custom Code",
			fmt.Sprintf("Could not read custom code '%s': %s\n\nThe import ID is Apple's custom code ID.", id, err.Error()),
		)
		return
	}

	state := subscriptionOfferCodeCustomCodeModel{
		ID:             types.StringValue(code.ID),
		OfferCodeID:    types.StringNull(),
		CustomCode:     types.StringNull(),
		NumberOfCodes:  types.Int64Null(),
		ExpirationDate: types.StringNull(),
		Active:         types.BoolNull(),
		CreatedDate:    types.StringNull(),
	}
	applyCustomCode(&state, code)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// applyCustomCode refreshes state from Apple's record.
//
// The code string is kept as configured when Apple reports it differing only
// in case. The spec does not say whether Apple normalises the case of a
// custom code, and custom_code forces replacement: adopting an upper-cased
// "SCHOOL2026" over a configured "school2026" would plan a new code -- and a
// deactivated old one -- on every run.
func applyCustomCode(state *subscriptionOfferCodeCustomCodeModel, code *models.SubscriptionOfferCodeCustomCode) {
	a := code.Attributes

	if a.CustomCode != "" && !strings.EqualFold(a.CustomCode, state.CustomCode.ValueString()) {
		state.CustomCode = types.StringValue(a.CustomCode)
	}
	if a.NumberOfCodes != nil {
		state.NumberOfCodes = types.Int64Value(int64(*a.NumberOfCodes))
	}
	if a.ExpirationDate != nil {
		state.ExpirationDate = types.StringValue(*a.ExpirationDate)
	}
	if a.Active != nil {
		state.Active = types.BoolValue(*a.Active)
	}
	if a.CreatedDate != nil {
		state.CreatedDate = types.StringValue(*a.CreatedDate)
	}
	if code.Relationships != nil && code.Relationships.OfferCode != nil && code.Relationships.OfferCode.Data.ID != "" {
		state.OfferCodeID = types.StringValue(code.Relationships.OfferCode.Data.ID)
	}
}
