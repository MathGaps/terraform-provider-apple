// Copyright (c) AO Studio
// SPDX-License-Identifier: MPL-2.0

package user

import (
	"context"
	"fmt"
	"strings"

	"github.com/AhmedOsman00/terraform-provider-apple/internal/apple"
	"github.com/AhmedOsman00/terraform-provider-apple/internal/apple/models"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var (
	_ resource.Resource                   = &userInvitationResource{}
	_ resource.ResourceWithConfigure      = &userInvitationResource{}
	_ resource.ResourceWithImportState    = &userInvitationResource{}
	_ resource.ResourceWithValidateConfig = &userInvitationResource{}
)

func NewUserInvitationResource() resource.Resource {
	return &userInvitationResource{}
}

type userInvitationResource struct {
	client *apple.Client
}

// Metadata returns the resource type name.
func (r *userInvitationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user_invitation"
}

// Schema defines the schema for the resource.
func (r *userInvitationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Invites somebody to join your App Store Connect team, with the roles and app " +
			"visibility they will have when they accept. This is the only way to add a member: Apple " +
			"publishes no endpoint that creates one directly.\n\n" +
			"!> **Creating this sends a real invitation email.** Point it only at addresses you are " +
			"entitled to invite.\n\n" +
			"~> **An accepted invitation stops existing.** Apple destroys the record the moment the " +
			"person joins and issues a team member in its place, with a different identifier. The " +
			"resource notices: `accepted` turns true, `user_id` names the member, and the invitation " +
			"stays in state rather than being planned for recreation — recreating it would be an error, " +
			"since Apple refuses to invite somebody who is already on the team. Hand the member over to " +
			"`apple_user` to manage them from then on.\n\n" +
			"An invitation **lapses after 72 hours**. One that expires or is cancelled outside Terraform " +
			"is gone from state on the next refresh and sent again on the next apply, which is usually " +
			"what you want.\n\n" +
			"Apple publishes no update for an invitation, so changing any of these answers cancels this " +
			"one and sends another — harmless while it is pending, and an error once it has been " +
			"accepted. Change the member's roles with `apple_user` instead.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The unique Apple-generated identifier for the invitation. It names " +
					"the invitation rather than the person: once accepted, the member's own identifier " +
					"is reported as `user_id`.",
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"email": schema.StringAttribute{
				MarkdownDescription: "The address Apple sends the invitation to, and the Apple Account the " +
					"person will sign in with. Apple has no way to change it, so a different address is " +
					"a different invitation and this replaces the resource.",
				Required:   true,
				Validators: []validator.String{EmailValidator},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"first_name": schema.StringAttribute{
				MarkdownDescription: "The person's first name, shown in the invitation email and in the " +
					"team list until their Apple Account supplies its own. Required by Apple here and " +
					"absent from every other request, which is why `apple_user` reports it as computed.",
				Required:   true,
				Validators: GetNameValidator(),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"last_name": schema.StringAttribute{
				MarkdownDescription: "The person's last name. See `first_name`.",
				Required:            true,
				Validators:          GetNameValidator(),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"roles": schema.SetAttribute{
				MarkdownDescription: "The roles the person will hold once they accept. At least one is " +
					"required. Valid values: `ADMIN`, `FINANCE`, `SALES`, `MARKETING`, `APP_MANAGER`, " +
					"`DEVELOPER`, `ACCESS_TO_REPORTS`, `CUSTOMER_SUPPORT`, `CREATE_APPS`, " +
					"`CLOUD_MANAGED_DEVELOPER_ID`, `CLOUD_MANAGED_APP_DISTRIBUTION`, " +
					"`GENERATE_INDIVIDUAL_KEYS`. `ACCOUNT_HOLDER` cannot be granted and is rejected.\n\n" +
					"Apple has no update for an invitation, so a changed set replaces it.",
				Required:    true,
				ElementType: types.StringType,
				Validators:  GetRolesValidator(),
				PlanModifiers: []planmodifier.Set{
					setplanmodifier.RequiresReplace(),
				},
			},
			"all_apps_visible": schema.BoolAttribute{
				MarkdownDescription: "Whether the person will see every app on the team. Set it to `false` " +
					"and name the apps in `visible_apps` to restrict them.",
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
					boolplanmodifier.RequiresReplace(),
				},
			},
			"visible_apps": schema.SetAttribute{
				MarkdownDescription: "The Apple IDs of the apps the person will see, used when " +
					"`all_apps_visible` is `false`. Read back only when the configuration sets it, for " +
					"the reason given on `apple_user`.",
				Optional:    true,
				ElementType: types.StringType,
				PlanModifiers: []planmodifier.Set{
					setplanmodifier.RequiresReplace(),
				},
			},
			"provisioning_allowed": schema.BoolAttribute{
				MarkdownDescription: "Whether the person may create signing certificates and provisioning " +
					"profiles once they join. Defaults to `false`.",
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.RequiresReplace(),
				},
			},
			"expiration_date": schema.StringAttribute{
				MarkdownDescription: "When the invitation lapses, as Apple reports it (ISO 8601). " +
					"Invitations are good for 72 hours; after that the person has to be invited again.",
				Computed: true,
			},
			"accepted": schema.BoolAttribute{
				MarkdownDescription: "Whether the person has joined the team. Apple destroys an invitation " +
					"when it is accepted, so this is how the resource tells that from an invitation that " +
					"was cancelled or allowed to lapse — those leave state instead.",
				Computed: true,
			},
			"user_id": schema.StringAttribute{
				MarkdownDescription: "The Apple identifier of the team member created when the invitation " +
					"was accepted, and null while it is still pending. Pass it nowhere: `apple_user` " +
					"adopts a member by the address they sign in with.",
				Computed: true,
			},
		},
	}
}

// ValidateConfig rejects the combinations Apple refuses, at plan time.
func (r *userInvitationResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config userInvitationModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !config.AllAppsVisible.IsNull() && !config.AllAppsVisible.IsUnknown() && config.AllAppsVisible.ValueBool() &&
		!config.VisibleApps.IsNull() && !config.VisibleApps.IsUnknown() && len(config.VisibleApps.Elements()) > 0 {
		resp.Diagnostics.AddError(
			"Conflicting App Visibility",
			"all_apps_visible is true and visible_apps names apps. Somebody who will see every app has "+
				"no list of apps, and Apple rejects a request carrying both.\n\n"+
				"Set all_apps_visible = false to restrict them to visible_apps, or drop visible_apps.",
		)
	}

	validateAssignableRoles(ctx, config.Roles, &resp.Diagnostics)
}

// Create sends the invitation.
func (r *userInvitationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	tflog.Info(ctx, "Creating user invitation resource")

	var plan userInvitationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	email := plan.Email.ValueString()
	ctx = tflog.SetField(ctx, "email", email)

	roles := rolesToAPI(ctx, plan.Roles, &resp.Diagnostics)
	visibleApps := appIDsToAPI(ctx, plan.VisibleApps, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	attributes := models.UserInvitationCreateAttributes{
		Email:               email,
		FirstName:           plan.FirstName.ValueString(),
		LastName:            plan.LastName.ValueString(),
		Roles:               roles,
		AllAppsVisible:      boolOrNil(plan.AllAppsVisible),
		ProvisioningAllowed: boolOrNil(plan.ProvisioningAllowed),
	}

	// A named list of apps is only meaningful for somebody who is not seeing
	// everything, and Apple rejects a request that says both -- see the same
	// case in the user resource's write.
	if visibleApps != nil && attributes.AllAppsVisible == nil {
		allAppsVisible := false
		attributes.AllAppsVisible = &allAppsVisible
		plan.AllAppsVisible = types.BoolValue(false)
	}

	invitation, err := r.client.CreateUserInvitation(attributes, visibleApps, nil)
	if err != nil {
		r.addCreateError(&resp.Diagnostics, email, err)
		tflog.Error(ctx, "Failed to create user invitation", map[string]interface{}{"error": err.Error()})

		return
	}

	plan.ID = types.StringValue(invitation.ID)
	plan.ExpirationDate = stringOrNull(invitation.Attributes.ExpirationDate)
	plan.AllAppsVisible = adoptBool(plan.AllAppsVisible, invitation.Attributes.AllAppsVisible)
	plan.ProvisioningAllowed = adoptBool(plan.ProvisioningAllowed, invitation.Attributes.ProvisioningAllowed)
	plan.Accepted = types.BoolValue(false)
	plan.UserID = types.StringNull()

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// Read refreshes the Terraform state with the latest data.
//
// An invitation that can no longer be read has either been accepted, cancelled
// or allowed to lapse, and Apple's 404 cannot say which -- so the team's member
// list is asked before the resource is dropped. A person who is now a member is
// the successful outcome, and reporting it as a deletion would have Terraform
// invite them again on the next apply, which Apple refuses.
func (r *userInvitationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	tflog.Debug(ctx, "Reading user invitation resource")

	var state userInvitationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	invitationID := state.ID.ValueString()
	email := state.Email.ValueString()
	ctx = tflog.SetField(ctx, "user_invitation_id", invitationID)

	invitation, err := r.client.GetUserInvitation(invitationID)
	if err != nil {
		if !isNotFound(err) {
			resp.Diagnostics.AddError(
				"Error Reading User Invitation",
				fmt.Sprintf("Could not read the invitation sent to '%s': %s", email, err.Error()),
			)

			return
		}

		member, memberErr := r.client.GetUserByUsername(email)
		if memberErr == nil {
			tflog.Info(ctx, "User invitation was accepted", map[string]interface{}{"user_id": member.ID})

			// Nothing else is refreshed from the member. The invitation's
			// attributes all force replacement, and a role changed since through
			// apple_user would otherwise plan one -- cancelling an invitation that
			// is not there and re-inviting somebody Apple says is already on the
			// team. What the invitation asked for is history now, so state keeps
			// it as history.
			state.Accepted = types.BoolValue(true)
			state.UserID = types.StringValue(member.ID)

			resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)

			return
		}

		if !isNotFound(memberErr) {
			resp.Diagnostics.AddError(
				"Error Reading Users",
				fmt.Sprintf("The invitation sent to '%s' is no longer pending, and the team's member "+
					"list could not be read to find out whether it was accepted: %s",
					email, memberErr.Error()),
			)

			return
		}

		tflog.Info(ctx, "User invitation is no longer pending and was not accepted, removing from state")
		resp.State.RemoveResource(ctx)

		return
	}

	state.Email = stringOrNull(invitation.Attributes.Email)
	state.FirstName = stringOrNull(invitation.Attributes.FirstName)
	state.LastName = stringOrNull(invitation.Attributes.LastName)
	state.Roles = rolesToTerraform(ctx, invitation.Attributes.Roles, &resp.Diagnostics)
	state.AllAppsVisible = boolOrNull(invitation.Attributes.AllAppsVisible)
	state.ProvisioningAllowed = boolOrNull(invitation.Attributes.ProvisioningAllowed)
	state.ExpirationDate = stringOrNull(invitation.Attributes.ExpirationDate)
	state.Accepted = types.BoolValue(false)
	state.UserID = types.StringNull()

	if !state.VisibleApps.IsNull() {
		apps, appsErr := r.client.GetUserInvitationVisibleApps(invitationID)
		if appsErr != nil {
			resp.Diagnostics.AddError(
				"Error Reading Visible Apps",
				fmt.Sprintf("Could not read the apps the invitation to '%s' grants sight of: %s",
					email, appsErr.Error()),
			)

			return
		}

		state.VisibleApps = appIDsToTerraform(ctx, apps, &resp.Diagnostics)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is unreachable: every configurable attribute forces replacement,
// because Apple publishes no PATCH /v1/userInvitations.
func (r *userInvitationResource) Update(ctx context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	tflog.Error(ctx, "Update called on a User Invitation, which Apple does not support")

	resp.Diagnostics.AddError(
		"User Invitation Update Not Supported",
		"App Store Connect cannot change a pending invitation, so this resource cancels it and sends "+
			"another instead. Reaching this method means an attribute changed without forcing "+
			"replacement, which is a bug in the provider: please report it.",
	)
}

// Delete cancels the invitation.
//
// An invitation that has already been accepted cannot be cancelled: the person
// is a member of the team, and taking their access away is a different request
// against a different record. Doing it here would turn removing a stale
// invitation from a configuration into removing a colleague, so it warns and
// drops state instead and names apple_user.
func (r *userInvitationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state userInvitationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	invitationID := state.ID.ValueString()
	email := state.Email.ValueString()
	ctx = tflog.SetField(ctx, "user_invitation_id", invitationID)

	if state.Accepted.ValueBool() {
		r.warnAlreadyAccepted(&resp.Diagnostics, email)

		return
	}

	if err := r.client.DeleteUserInvitation(invitationID, nil); err != nil {
		if !isNotFound(err) {
			resp.Diagnostics.AddError(
				"Error Cancelling User Invitation",
				fmt.Sprintf("Could not cancel the invitation sent to '%s': %s", email, err.Error()),
			)

			return
		}

		// The invitation went away between the last refresh and now. Accepted is
		// the one outcome that matters here, because it leaves somebody on the
		// team that this resource is no longer tracking.
		if _, memberErr := r.client.GetUserByUsername(email); memberErr == nil {
			r.warnAlreadyAccepted(&resp.Diagnostics, email)

			return
		}

		tflog.Info(ctx, "User invitation was already cancelled or had expired")

		return
	}

	tflog.Info(ctx, "User invitation cancelled")
}

// Configure adds the provider configured client to the resource.
func (r *userInvitationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// ImportState imports one pending invitation by Apple's identifier or by the
// address it was sent to, told apart by the "@".
//
// Only a pending invitation can be imported. An accepted one no longer exists
// at Apple, and the member it produced is imported as an apple_user.
func (r *userInvitationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	tflog.Info(ctx, "Importing user invitation resource", map[string]interface{}{"import_id": req.ID})

	var (
		invitation *models.UserInvitation
		err        error
	)

	if strings.Contains(req.ID, "@") {
		invitation, err = r.client.GetUserInvitationByEmail(req.ID)
	} else {
		invitation, err = r.client.GetUserInvitation(req.ID)
	}

	if err != nil {
		resp.Diagnostics.AddError(
			"Error Importing User Invitation",
			fmt.Sprintf("Could not read the invitation '%s': %s\n\n"+
				"The accepted import IDs are the Apple invitation ID and the address it was sent to. "+
				"Only a pending invitation can be imported — one that has been accepted no longer "+
				"exists, and the member it created is imported as an apple_user instead.",
				req.ID, err.Error()),
		)

		return
	}

	state := userInvitationModel{
		ID:                  types.StringValue(invitation.ID),
		Email:               stringOrNull(invitation.Attributes.Email),
		FirstName:           stringOrNull(invitation.Attributes.FirstName),
		LastName:            stringOrNull(invitation.Attributes.LastName),
		Roles:               rolesToTerraform(ctx, invitation.Attributes.Roles, &resp.Diagnostics),
		AllAppsVisible:      boolOrNull(invitation.Attributes.AllAppsVisible),
		ProvisioningAllowed: boolOrNull(invitation.Attributes.ProvisioningAllowed),
		ExpirationDate:      stringOrNull(invitation.Attributes.ExpirationDate),
		Accepted:            types.BoolValue(false),
		UserID:              types.StringNull(),
		VisibleApps:         types.SetNull(types.StringType),
	}

	if invitation.Attributes.AllAppsVisible != nil && !*invitation.Attributes.AllAppsVisible {
		apps, appsErr := r.client.GetUserInvitationVisibleApps(invitation.ID)
		if appsErr != nil {
			resp.Diagnostics.AddError(
				"Error Importing Visible Apps",
				fmt.Sprintf("Could not read the apps the invitation '%s' grants sight of: %s",
					req.ID, appsErr.Error()),
			)

			return
		}

		state.VisibleApps = appIDsToTerraform(ctx, apps, &resp.Diagnostics)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// warnAlreadyAccepted reports that there was no invitation left to cancel
// because the person took it up.
func (r *userInvitationResource) warnAlreadyAccepted(diags *diag.Diagnostics, email string) {
	diags.AddWarning(
		"User Invitation Already Accepted",
		fmt.Sprintf("'%s' accepted the invitation and is a member of this App Store Connect team, so "+
			"there was nothing to cancel and the resource has been dropped from state.\n\n"+
			"They still have the access the invitation granted. To take it away, manage them with "+
			"apple_user and destroy that, or remove them under Users and Access.", email),
	)
}

// addCreateError maps Apple's refusals onto diagnostics that name the cause.
func (r *userInvitationResource) addCreateError(diags *diag.Diagnostics, email string, err error) {
	errMsg := err.Error()
	lower := strings.ToLower(errMsg)

	switch {
	case strings.Contains(errMsg, "ENTITY_ERROR.ATTRIBUTE.INVALID.DUPLICATE") ||
		strings.Contains(lower, "already exist") || strings.Contains(lower, "already a member") ||
		strings.Contains(lower, "already been invited") || strings.Contains(errMsg, "DUPLICATE"):
		diags.AddError(
			"Already Invited Or Already A Member",
			fmt.Sprintf("Apple refused to invite '%s': %s\n\n"+
				"Either an invitation to that address is still pending, or the person is already on "+
				"the team. Import whichever it is rather than sending another:\n\n"+
				"  terraform import apple_user_invitation.<name> %s\n"+
				"  terraform import apple_user.<name> %s", email, errMsg, email, email),
		)
	case strings.Contains(errMsg, "FORBIDDEN") || strings.Contains(errMsg, "403"):
		diags.AddError(
			"Not Permitted To Invite Users",
			fmt.Sprintf("Apple refused to invite '%s': %s\n\n"+
				"Inviting a team member needs an App Store Connect API key with the Admin role.",
				email, errMsg),
		)
	case strings.Contains(errMsg, "not found") || strings.Contains(errMsg, "404"):
		diags.AddError(
			"App Not Found",
			fmt.Sprintf("Apple could not find something the invitation to '%s' named: %s\n\n"+
				"An Apple ID in visible_apps names an app this team does not hold.", email, errMsg),
		)
	default:
		diags.AddError(
			"Error Inviting User",
			fmt.Sprintf("Could not invite '%s': %s", email, errMsg),
		)
	}
}
