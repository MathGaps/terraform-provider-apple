// Copyright (c) AO Studio
// SPDX-License-Identifier: MPL-2.0

package beta

import (
	"context"
	"fmt"
	"strings"

	"github.com/AhmedOsman00/terraform-provider-apple/internal/apple"
	"github.com/AhmedOsman00/terraform-provider-apple/internal/apple/models"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var (
	_ resource.Resource                   = &betaGroupResource{}
	_ resource.ResourceWithConfigure      = &betaGroupResource{}
	_ resource.ResourceWithImportState    = &betaGroupResource{}
	_ resource.ResourceWithValidateConfig = &betaGroupResource{}
)

func NewBetaGroupResource() resource.Resource {
	return &betaGroupResource{}
}

type betaGroupResource struct {
	client *apple.Client
}

// Metadata returns the resource type name.
func (r *betaGroupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_beta_group"
}

// Schema defines the schema for the resource.
func (r *betaGroupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a TestFlight tester group — the thing `fastlane pilot` creates and " +
			"this provider previously had no answer for.\n\n" +
			"A group is a named set of testers that builds are distributed to. `is_internal_group` " +
			"decides which of two quite different things it is, and it cannot be changed afterwards:\n\n" +
			"- **Internal** (`is_internal_group = true`) — members are users of your App Store Connect " +
			"team, capped at 100, and a build reaches them as soon as it finishes processing. No beta " +
			"review, no public link.\n" +
			"- **External** (`is_internal_group = false`, the default) — members are arbitrary email " +
			"addresses, up to 10,000 per app, and a build cannot reach them until Apple's beta review " +
			"has approved one. Only an external group can be opened to a public link.\n\n" +
			"Because the two are not the same thing, the public-link attributes are rejected on an " +
			"internal group rather than quietly ignored.\n\n" +
			"~> **A group created here starts empty.** Put testers in it with `apple_beta_tester`, one " +
			"resource per person. Assigning individual builds to a group is still not modelled — set " +
			"`has_access_to_all_builds` to let it see every build automatically.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The unique Apple-generated identifier for the group.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"app_id": schema.StringAttribute{
				MarkdownDescription: "The Apple ID of the app this group tests. Read it from the " +
					"`apple_apps` data source. A group cannot move between apps, so changing this " +
					"replaces it.",
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The group's name, unique within the app — `QA`, `Design review`, " +
					"`Public beta`. Testers of an external group see it on the TestFlight invitation. " +
					"Updatable in place.",
				Required:   true,
				Validators: GetBetaGroupNameValidator(),
			},
			"is_internal_group": schema.BoolAttribute{
				MarkdownDescription: "Whether the group draws its members from your App Store Connect " +
					"team (`true`) or from arbitrary email addresses (`false`, the default). Absent from " +
					"Apple's update request — a group does not change kind, so changing this replaces it.\n\n" +
					"An internal group is capped at 100 team members and receives a build the moment it " +
					"finishes processing. An external group holds up to 10,000 testers per app and cannot " +
					"receive a build until beta review has approved one.",
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
					boolplanmodifier.RequiresReplace(),
				},
			},
			"has_access_to_all_builds": schema.BoolAttribute{
				MarkdownDescription: "Whether the group automatically sees every build of the app rather " +
					"than only the builds assigned to it. Absent from Apple's update request, so changing " +
					"it replaces the group.\n\n" +
					"Since this provider does not assign builds to groups, a group without this sees " +
					"nothing until a build is assigned to it by hand or by another tool — its testers " +
					"included.",
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
					boolplanmodifier.RequiresReplace(),
				},
			},
			"public_link_enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether to issue a public TestFlight link anyone can join the group " +
					"through. **External groups only** — Apple rejects it on an internal group. Turning it " +
					"off invalidates the link that was issued.",
				Optional: true,
				Computed: true,
			},
			"public_link_limit_enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether the public link stops accepting testers once " +
					"`public_link_limit` is reached. Without it the link is uncapped up to the app's own " +
					"10,000-tester ceiling.",
				Optional: true,
				Computed: true,
			},
			"public_link_limit": schema.Int64Attribute{
				MarkdownDescription: "How many testers the public link accepts before it closes. Requires " +
					"`public_link_limit_enabled`. A limit of zero is not a closed link — Apple rejects it; " +
					"close a link with `public_link_enabled = false`.",
				Optional:   true,
				Computed:   true,
				Validators: GetPublicLinkLimitValidator(),
			},
			"feedback_enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether testers can send screenshot and crash feedback from the " +
					"TestFlight app.",
				Optional: true,
				Computed: true,
			},
			"ios_builds_available_for_apple_silicon_mac": schema.BoolAttribute{
				MarkdownDescription: "Whether this group's testers can install the iOS build on an Apple " +
					"silicon Mac.",
				Optional: true,
				Computed: true,
			},
			"ios_builds_available_for_apple_vision": schema.BoolAttribute{
				MarkdownDescription: "Whether this group's testers can install the iOS build on Apple " +
					"Vision Pro.",
				Optional: true,
				Computed: true,
			},
			"public_link": schema.StringAttribute{
				MarkdownDescription: "The public TestFlight URL Apple issued, null unless " +
					"`public_link_enabled` is true. Apple's, never the provider's.",
				Computed: true,
			},
			"public_link_id": schema.StringAttribute{
				MarkdownDescription: "The identifier embedded in `public_link`.",
				Computed:            true,
			},
			"created_date": schema.StringAttribute{
				MarkdownDescription: "When Apple created the group.",
				Computed:            true,
			},
		},
	}
}

// Create a new resource.
func (r *betaGroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	tflog.Info(ctx, "Creating beta group resource")

	var plan betaGroupModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	appID := plan.AppID.ValueString()
	name := plan.Name.ValueString()
	ctx = tflog.SetField(ctx, "app_id", appID)
	ctx = tflog.SetField(ctx, "beta_group_name", name)

	group, err := r.client.CreateBetaGroup(appID, models.BetaGroupCreateAttributes{
		Name:                                 name,
		IsInternalGroup:                      boolOrNil(plan.IsInternalGroup),
		HasAccessToAllBuilds:                 boolOrNil(plan.HasAccessToAllBuilds),
		PublicLinkEnabled:                    boolOrNil(plan.PublicLinkEnabled),
		PublicLinkLimitEnabled:               boolOrNil(plan.PublicLinkLimitEnabled),
		PublicLinkLimit:                      int64OrNil(plan.PublicLinkLimit),
		FeedbackEnabled:                      boolOrNil(plan.FeedbackEnabled),
		IOSBuildsAvailableForAppleSiliconMac: boolOrNil(plan.IOSBuildsAvailableForAppleSiliconMac),
		IOSBuildsAvailableForAppleVision:     boolOrNil(plan.IOSBuildsAvailableForAppleVision),
	}, nil)
	if err != nil {
		errMsg := err.Error()
		switch {
		case strings.Contains(errMsg, "already exist") || strings.Contains(errMsg, "ENTITY_ERROR.ATTRIBUTE.INVALID.DUPLICATE"):
			resp.Diagnostics.AddError(
				"Beta Group Already Exists",
				fmt.Sprintf("Apple refused to create beta group '%s' in app '%s': %s\n\n"+
					"Group names are unique within an app. Import the existing group instead:\n\n"+
					"  terraform import <address> %s/%s", name, appID, errMsg, appID, name),
			)
		case strings.Contains(errMsg, "not found") || strings.Contains(errMsg, "404"):
			resp.Diagnostics.AddError(
				"App Not Found",
				fmt.Sprintf("Could not create a beta group in app '%s': %s\n\n"+
					"Apple's API cannot create an app record — the app must already exist in App Store "+
					"Connect.", appID, errMsg),
			)
		default:
			resp.Diagnostics.AddError(
				"Error Creating Beta Group",
				fmt.Sprintf("Could not create beta group '%s' in app '%s': %s", name, appID, errMsg),
			)
		}
		tflog.Error(ctx, "Failed to create beta group", map[string]interface{}{"error": errMsg})

		return
	}

	applyBetaGroup(&plan, group)

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

// Read refreshes the Terraform state with the latest data.
func (r *betaGroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	tflog.Debug(ctx, "Reading beta group resource")

	var state betaGroupModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	groupID := state.ID.ValueString()
	ctx = tflog.SetField(ctx, "beta_group_id", groupID)

	group, err := r.client.GetBetaGroup(groupID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "404") {
			tflog.Info(ctx, "Beta group not found, removing from state")
			resp.State.RemoveResource(ctx)

			return
		}

		resp.Diagnostics.AddError(
			"Error Reading Beta Group",
			fmt.Sprintf("Could not read beta group '%s': %s", groupID, err.Error()),
		)

		return
	}

	applyBetaGroup(&state, group)

	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
}

// Update patches the group in place.
func (r *betaGroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	tflog.Info(ctx, "Updating beta group resource")

	var plan betaGroupModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state betaGroupModel
	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	groupID := state.ID.ValueString()
	ctx = tflog.SetField(ctx, "beta_group_id", groupID)

	group, err := r.client.UpdateBetaGroup(groupID, models.BetaGroupUpdateAttributes{
		Name:                                 stringOrNil(plan.Name),
		PublicLinkEnabled:                    boolOrNil(plan.PublicLinkEnabled),
		PublicLinkLimitEnabled:               boolOrNil(plan.PublicLinkLimitEnabled),
		PublicLinkLimit:                      int64OrNil(plan.PublicLinkLimit),
		FeedbackEnabled:                      boolOrNil(plan.FeedbackEnabled),
		IOSBuildsAvailableForAppleSiliconMac: boolOrNil(plan.IOSBuildsAvailableForAppleSiliconMac),
		IOSBuildsAvailableForAppleVision:     boolOrNil(plan.IOSBuildsAvailableForAppleVision),
	}, nil)
	if err != nil {
		errMsg := err.Error()
		switch {
		case strings.Contains(errMsg, "already exist") || strings.Contains(errMsg, "DUPLICATE"):
			resp.Diagnostics.AddError(
				"Beta Group Name Already Taken",
				fmt.Sprintf("Apple refused to rename beta group '%s' to '%s': %s\n\n"+
					"Group names are unique within an app.", groupID, plan.Name.ValueString(), errMsg),
			)
		case strings.Contains(errMsg, "publicLink") || strings.Contains(errMsg, "INTERNAL_GROUP"):
			resp.Diagnostics.AddError(
				"Public Link Not Available",
				fmt.Sprintf("Apple refused the change to beta group '%s': %s\n\n"+
					"Only an external group can have a public link. An internal group draws its members "+
					"from your App Store Connect team, and a group's kind is fixed at creation.",
					plan.Name.ValueString(), errMsg),
			)
		default:
			resp.Diagnostics.AddError(
				"Error Updating Beta Group",
				fmt.Sprintf("Could not update beta group '%s': %s", groupID, errMsg),
			)
		}

		return
	}

	applyBetaGroup(&plan, group)

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

// Delete removes the group.
//
// Deleting a group takes away its testers' access to the app's builds; the
// testers themselves stay in the app's tester list and in any other group they
// belong to, so this is not the irreversible act it can look like.
func (r *betaGroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state betaGroupModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	groupID := state.ID.ValueString()
	ctx = tflog.SetField(ctx, "beta_group_id", groupID)

	if err := r.client.DeleteBetaGroup(groupID, nil); err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "not found") || strings.Contains(errMsg, "404") {
			tflog.Info(ctx, "Beta group already gone")
		} else {
			resp.Diagnostics.AddError(
				"Error Deleting Beta Group",
				fmt.Sprintf("Could not delete beta group '%s': %s", state.Name.ValueString(), errMsg),
			)

			return
		}
	}

	tflog.Info(ctx, "Beta group deleted")
}

// ValidateConfig rejects the public-link attributes on an internal group.
//
// Apple refuses them there, and the refusal names the attribute rather than the
// reason. An internal group's members are App Store Connect users, so there is
// nothing a public link could let anyone do.
func (r *betaGroupResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config betaGroupModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if config.IsInternalGroup.ValueBool() {
		for attribute, set := range map[string]bool{
			"public_link_enabled":       !config.PublicLinkEnabled.IsNull(),
			"public_link_limit_enabled": !config.PublicLinkLimitEnabled.IsNull(),
			"public_link_limit":         !config.PublicLinkLimit.IsNull(),
		} {
			if !set {
				continue
			}

			resp.Diagnostics.AddAttributeError(
				path.Root(attribute),
				"Public Link On An Internal Group",
				fmt.Sprintf("%s applies only to an external group, and Apple rejects it on an internal "+
					"one. An internal group's members are users of your App Store Connect team, who are "+
					"added by account rather than by link.\n\nRemove %s, or set is_internal_group = false "+
					"to make this an external group.", attribute, attribute),
			)
		}
	}

	if !config.PublicLinkLimit.IsNull() && config.PublicLinkLimitEnabled.IsNull() {
		resp.Diagnostics.AddAttributeError(
			path.Root("public_link_limit"),
			"Public Link Limit Without The Limit Enabled",
			"public_link_limit is the number the public link closes at, and Apple ignores it unless "+
				"public_link_limit_enabled is true. Set public_link_limit_enabled = true as well, or "+
				"remove public_link_limit.",
		)
	}
}

// Configure adds the provider configured client to the resource.
func (r *betaGroupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// ImportState imports a beta group.
//
// Two forms are accepted: a bare Apple group ID, and the composite
// "<app_id>/<name>". Unlike apple_subscription_group, the bare form works --
// GET /v1/betaGroups/{id} accepts include=app, so the owning app comes back
// with the record and app_id can be written from it.
func (r *betaGroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	tflog.Info(ctx, "Importing beta group resource", map[string]interface{}{"import_id": req.ID})

	var (
		group *models.BetaGroup
		err   error
	)

	parts := strings.SplitN(req.ID, "/", 2)
	switch len(parts) {
	case 1:
		group, err = r.client.GetBetaGroup(req.ID)
	case 2:
		group, err = r.client.GetBetaGroupByName(parts[0], parts[1])
	default:
		resp.Diagnostics.AddError(
			"Invalid Import ID",
			fmt.Sprintf("Could not parse import ID '%s'. Accepted forms are a bare Apple group ID and "+
				"'<app_id>/<group_name>'.", req.ID),
		)

		return
	}

	if err != nil {
		resp.Diagnostics.AddError(
			"Error Importing Beta Group",
			fmt.Sprintf("Could not read beta group '%s': %s", req.ID, err.Error()),
		)

		return
	}

	state := betaGroupModel{}
	if len(parts) == 2 {
		state.AppID = types.StringValue(parts[0])
	}

	applyBetaGroup(&state, group)

	if state.AppID.IsNull() {
		resp.Diagnostics.AddError(
			"Incomplete Beta Group Import",
			fmt.Sprintf("Apple did not report which app beta group '%s' belongs to, so app_id cannot be "+
				"written. The client requests include=app, so this is unexpected — import with "+
				"'<app_id>/<group_name>' instead, and please report it.", req.ID),
		)

		return
	}

	diags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
}

// applyBetaGroup copies Apple's view of the record into the model.
//
// The optional-and-computed attributes go through the adopt* helpers rather
// than being assigned outright: an attribute Apple does not report back must
// not blank a value the plan holds, or the apply is rejected as an inconsistent
// result -- and it must not stay unknown either.
func applyBetaGroup(model *betaGroupModel, group *models.BetaGroup) {
	model.ID = types.StringValue(group.ID)

	if name := group.Attributes.Name; name != nil {
		model.Name = types.StringValue(*name)
	}

	model.IsInternalGroup = adoptBool(model.IsInternalGroup, group.Attributes.IsInternalGroup)
	model.HasAccessToAllBuilds = adoptBool(model.HasAccessToAllBuilds, group.Attributes.HasAccessToAllBuilds)
	model.PublicLinkEnabled = adoptBool(model.PublicLinkEnabled, group.Attributes.PublicLinkEnabled)
	model.PublicLinkLimitEnabled = adoptBool(model.PublicLinkLimitEnabled, group.Attributes.PublicLinkLimitEnabled)
	model.PublicLinkLimit = adoptInt64(model.PublicLinkLimit, group.Attributes.PublicLinkLimit)
	model.FeedbackEnabled = adoptBool(model.FeedbackEnabled, group.Attributes.FeedbackEnabled)
	model.IOSBuildsAvailableForAppleSiliconMac = adoptBool(
		model.IOSBuildsAvailableForAppleSiliconMac, group.Attributes.IOSBuildsAvailableForAppleSiliconMac)
	model.IOSBuildsAvailableForAppleVision = adoptBool(
		model.IOSBuildsAvailableForAppleVision, group.Attributes.IOSBuildsAvailableForAppleVision)

	model.PublicLink = stringOrNull(group.Attributes.PublicLink)
	model.PublicLinkID = stringOrNull(group.Attributes.PublicLinkID)
	model.CreatedDate = stringOrNull(group.Attributes.CreatedDate)

	if group.Relationships != nil {
		if appID := relationshipID(group.Relationships.App); !appID.IsNull() {
			model.AppID = appID
		}
	}
}

// adoptBool takes Apple's value when it reported one, resolves an unknown to
// null when it did not, and otherwise leaves the planned value alone.
func adoptBool(current types.Bool, reported *bool) types.Bool {
	switch {
	case reported != nil:
		return types.BoolValue(*reported)
	case current.IsUnknown():
		return types.BoolNull()
	default:
		return current
	}
}

// adoptInt64 is adoptBool for an integer attribute.
func adoptInt64(current types.Int64, reported *int64) types.Int64 {
	switch {
	case reported != nil:
		return types.Int64Value(*reported)
	case current.IsUnknown():
		return types.Int64Null()
	default:
		return current
	}
}
