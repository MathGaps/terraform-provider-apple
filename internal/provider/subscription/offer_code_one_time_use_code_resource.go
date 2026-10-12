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
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var (
	_ resource.Resource                   = &subscriptionOfferCodeOneTimeUseCodeResource{}
	_ resource.ResourceWithConfigure      = &subscriptionOfferCodeOneTimeUseCodeResource{}
	_ resource.ResourceWithImportState    = &subscriptionOfferCodeOneTimeUseCodeResource{}
	_ resource.ResourceWithValidateConfig = &subscriptionOfferCodeOneTimeUseCodeResource{}
)

func NewSubscriptionOfferCodeOneTimeUseCodeResource() resource.Resource {
	return &subscriptionOfferCodeOneTimeUseCodeResource{}
}

type subscriptionOfferCodeOneTimeUseCodeResource struct {
	client *apple.Client
}

// Metadata returns the resource type name.
func (r *subscriptionOfferCodeOneTimeUseCodeResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_subscription_offer_code_one_time_use_code"
}

// Schema defines the schema for the resource.
func (r *subscriptionOfferCodeOneTimeUseCodeResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a batch of one-time-use codes for an `apple_subscription_offer_code`: Apple " +
			"generates `number_of_codes` distinct codes, each redeemable once, for a campaign to hand out one " +
			"per person.\n\n" +
			"**The codes themselves are not attributes of this resource.** They are secrets — anyone holding " +
			"one gets the offer — and a batch runs to thousands, so they would sit in this resource's state in " +
			"plain text for every configuration that creates one, whether it distributes them or not. Read " +
			"them through the `apple_subscription_offer_code_one_time_use_code_values` data source, in the " +
			"configuration that actually hands them out, or download them from App Store Connect. Apple says " +
			"codes can take up to an hour to become redeemable.\n\n" +
			"Every attribute except `active` is fixed at creation, so a change generates a **new** batch and " +
			"deactivates this one, invalidating every code already handed out.\n\n" +
			offerCodePermanenceNote,

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The unique Apple-generated identifier for the batch.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"offer_code_id": schema.StringAttribute{
				MarkdownDescription: "The ID of the `apple_subscription_offer_code` the codes redeem. Cannot be " +
					"changed after creation.",
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"number_of_codes": schema.Int64Attribute{
				MarkdownDescription: fmt.Sprintf("How many codes to generate: %d to %d for `PRODUCTION`, %d to "+
					"%d for `SANDBOX`. For more, create several batches for the same offer. Cannot be "+
					"changed after creation.",
					minProductionOneTimeUseCodes, maxProductionOneTimeUseCodes,
					minSandboxOneTimeUseCodes, maxSandboxOneTimeUseCodes),
				Required: true,
				Validators: []validator.Int64{
					int64validator.Between(minSandboxOneTimeUseCodes, maxProductionOneTimeUseCodes),
				},
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"expiration_date": schema.StringAttribute{
				MarkdownDescription: "The last day the codes redeem, as a plain date in `YYYY-MM-DD` form — at " +
					"most six months after the batch is created; codes expire at 12:00 a.m. Pacific Time on " +
					"that date. Required. Cannot be changed after creation.",
				Required:   true,
				Validators: []validator.String{ExpirationDateValidator},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"environment": schema.StringAttribute{
				MarkdownDescription: "Where the codes redeem: `PRODUCTION` (the App Store, the default) or " +
					"`SANDBOX` (test accounts only). Cannot be changed after creation.",
				Optional:   true,
				Computed:   true,
				Default:    stringdefault.StaticString(models.OfferCodeEnvironmentProduction),
				Validators: []validator.String{OfferCodeEnvironmentValidator},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"active": schema.BoolAttribute{
				MarkdownDescription: "Whether the codes redeem. Defaults to `true`. Setting it to `false` " +
					"expires every unredeemed code in the batch at once, in place — customers who already " +
					"redeemed one keep it — and is **final**: setting it back to `true` plans a new batch.",
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(true),
				PlanModifiers: []planmodifier.Bool{
					reactivationRequiresReplace(),
				},
			},
			"created_date": schema.StringAttribute{
				MarkdownDescription: "When Apple created the batch.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

// ValidateConfig applies the batch size range for the chosen environment --
// the schema validator can only hold the union of the two -- and the
// six-month ceiling on the expiration date.
func (r *subscriptionOfferCodeOneTimeUseCodeResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config subscriptionOfferCodeOneTimeUseCodeModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !config.NumberOfCodes.IsNull() && !config.NumberOfCodes.IsUnknown() && !config.Environment.IsUnknown() {
		environment := config.Environment.ValueString()
		if environment == "" {
			environment = models.OfferCodeEnvironmentProduction
		}
		if err := checkOneTimeUseCodeCount(environment, config.NumberOfCodes.ValueInt64()); err != nil {
			resp.Diagnostics.AddAttributeError(path.Root("number_of_codes"), "Invalid Number of Codes", err.Error())
		}
	}

	if !config.ExpirationDate.IsNull() && !config.ExpirationDate.IsUnknown() {
		if err := checkExpirationWithinSixMonths(config.ExpirationDate.ValueString(), time.Now()); err != nil {
			resp.Diagnostics.AddAttributeError(path.Root("expiration_date"), "Invalid Expiration Date", err.Error())
		}
	}
}

// Create asks Apple to generate the batch.
func (r *subscriptionOfferCodeOneTimeUseCodeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	tflog.Info(ctx, "Creating subscription offer code one-time-use code resource")

	var plan subscriptionOfferCodeOneTimeUseCodeModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	offerCodeID := plan.OfferCodeID.ValueString()
	ctx = tflog.SetField(ctx, "offer_code_id", offerCodeID)

	batch, err := r.client.CreateSubscriptionOfferCodeOneTimeUseCode(offerCodeID, models.SubscriptionOfferCodeOneTimeUseCodeCreateAttrs{
		NumberOfCodes:  int(plan.NumberOfCodes.ValueInt64()),
		ExpirationDate: plan.ExpirationDate.ValueString(),
		Environment:    stringOrNil(plan.Environment),
	}, nil)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "not found") || strings.Contains(errMsg, "404") {
			resp.Diagnostics.AddError(
				"Subscription Offer Code Not Found",
				fmt.Sprintf("Could not generate codes: offer code '%s' does not exist: %s", offerCodeID, errMsg),
			)
		} else {
			resp.Diagnostics.AddError(
				"Error Generating One-Time-Use Codes",
				fmt.Sprintf("Could not generate %d %s codes for offer code '%s': %s\n\n"+
					"An app's offer codes share a quarterly quota across every subscription, a batch expires at "+
					"most six months after it is created, and a deactivated offer accepts no new codes.",
					plan.NumberOfCodes.ValueInt64(), plan.Environment.ValueString(), offerCodeID, errMsg),
			)
		}
		tflog.Error(ctx, "Failed to create one-time-use codes", map[string]interface{}{"error": errMsg})
		return
	}

	plan.ID = types.StringValue(batch.ID)
	plan.CreatedDate = stringOrNull(batch.Attributes.CreatedDate)

	// State first, as Apple holds it -- active -- for the reason the offer
	// code's Create gives.
	configuredActive := plan.Active
	plan.Active = types.BoolValue(true)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
	if resp.Diagnostics.HasError() || configuredActive.ValueBool() {
		return
	}

	if _, err := r.client.SetSubscriptionOfferCodeOneTimeUseCodeActive(batch.ID, false, nil); err != nil {
		// A warning, for the reason the offer code's Create gives: an error
		// would taint this and create another that Apple can never delete.
		// State keeps the planned value; the next refresh reads Apple's.
		plan.Active = configuredActive
		resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
		resp.Diagnostics.AddWarning(
			"One-Time-Use Codes Created Active",
			fmt.Sprintf("Batch '%s' was created, but deactivating it as configured failed: %s\n\n"+
				"It is still active at Apple. The next plan shows it, and the next apply deactivates it.",
				batch.ID, err.Error()),
		)
		return
	}
	plan.Active = configuredActive
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// Read refreshes the batch from Apple.
func (r *subscriptionOfferCodeOneTimeUseCodeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state subscriptionOfferCodeOneTimeUseCodeModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	batch, err := r.client.GetSubscriptionOfferCodeOneTimeUseCode(state.ID.ValueString())
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "404") {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error Reading One-Time-Use Codes",
			fmt.Sprintf("Could not read batch '%s': %s", state.ID.ValueString(), err.Error()),
		)
		return
	}

	applyOneTimeUseCode(&state, batch)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update changes "active", the one member of Apple's update request.
func (r *subscriptionOfferCodeOneTimeUseCodeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan subscriptionOfferCodeOneTimeUseCodeModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if _, err := r.client.SetSubscriptionOfferCodeOneTimeUseCodeActive(plan.ID.ValueString(), plan.Active.ValueBool(), nil); err != nil {
		resp.Diagnostics.AddError(
			"Error Updating One-Time-Use Codes",
			fmt.Sprintf("Could not set active=%t on batch '%s': %s",
				plan.Active.ValueBool(), plan.ID.ValueString(), err.Error()),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// Delete deactivates the batch and drops it from state; Apple publishes no
// DELETE.
func (r *subscriptionOfferCodeOneTimeUseCodeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state subscriptionOfferCodeOneTimeUseCodeModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := state.ID.ValueString()
	if state.Active.IsNull() || state.Active.ValueBool() {
		if _, err := r.client.SetSubscriptionOfferCodeOneTimeUseCodeActive(id, false, nil); err != nil {
			if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "404") {
				tflog.Warn(ctx, "One-time-use code batch not found while deactivating, treating as already removed")
				return
			}
			resp.Diagnostics.AddError(
				"Unable to Deactivate One-Time-Use Codes",
				fmt.Sprintf("Apple's API cannot delete a batch of one-time-use codes, so Terraform deactivates "+
					"it instead. Deactivating batch '%s' failed: %s", id, err.Error()),
			)
			return
		}
	}

	resp.Diagnostics.AddWarning(
		"One-Time-Use Codes Deactivated Rather Than Deleted",
		fmt.Sprintf("The App Store Connect API cannot delete a batch of one-time-use codes. Batch '%s' has been "+
			"deactivated and removed from Terraform state, but it remains in App Store Connect and cannot be "+
			"reactivated. Its unredeemed codes no longer redeem; customers who already redeemed one are "+
			"unaffected.", id),
	)
}

// Configure adds the provider configured client to the resource.
func (r *subscriptionOfferCodeOneTimeUseCodeResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// ImportState imports a batch by Apple's ID. The offer is read back through
// include=offerCode, so no composite form is needed.
func (r *subscriptionOfferCodeOneTimeUseCodeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id := strings.TrimSpace(req.ID)

	batch, err := r.client.GetSubscriptionOfferCodeOneTimeUseCode(id)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Importing One-Time-Use Codes",
			fmt.Sprintf("Could not read batch '%s': %s\n\nThe import ID is Apple's one-time-use code batch ID.",
				id, err.Error()),
		)
		return
	}

	state := subscriptionOfferCodeOneTimeUseCodeModel{
		ID:             types.StringValue(batch.ID),
		OfferCodeID:    types.StringNull(),
		NumberOfCodes:  types.Int64Null(),
		ExpirationDate: types.StringNull(),
		Environment:    types.StringNull(),
		Active:         types.BoolNull(),
		CreatedDate:    types.StringNull(),
	}
	applyOneTimeUseCode(&state, batch)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// applyOneTimeUseCode refreshes state from Apple's record.
func applyOneTimeUseCode(state *subscriptionOfferCodeOneTimeUseCodeModel, batch *models.SubscriptionOfferCodeOneTimeUseCode) {
	a := batch.Attributes

	if a.NumberOfCodes != nil {
		state.NumberOfCodes = types.Int64Value(int64(*a.NumberOfCodes))
	}
	if a.ExpirationDate != nil {
		state.ExpirationDate = types.StringValue(*a.ExpirationDate)
	}
	if a.Environment != nil {
		state.Environment = types.StringValue(*a.Environment)
	}
	if a.Active != nil {
		state.Active = types.BoolValue(*a.Active)
	}
	if a.CreatedDate != nil {
		state.CreatedDate = types.StringValue(*a.CreatedDate)
	}
	if batch.Relationships != nil && batch.Relationships.OfferCode != nil && batch.Relationships.OfferCode.Data.ID != "" {
		state.OfferCodeID = types.StringValue(batch.Relationships.OfferCode.Data.ID)
	}
}
