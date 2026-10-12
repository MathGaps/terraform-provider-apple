// Copyright (c) AO Studio
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// TestOmittedComputedValueNeverForcesReplacement walks every resource's
// top-level Optional+Computed attributes and plans each as Terraform does when
// the configuration omits it and something else about the resource changes:
// the framework marks the value unknown, and the attribute's plan modifiers run
// in order against the value in state. An attribute with a Default is never
// unknown there -- the framework plans the default instead -- so it is skipped.
//
// RequiresReplace compares the planned value with state, and an unknown value
// never equals a known one, so a RequiresReplace that runs before
// UseStateForUnknown replaces the resource over an attribute nobody touched --
// renaming an apple_beta_group without restating is_internal_group planned
// its replacement. Every attribute that keeps state for an omitted value must
// keep it before anything decides on replacement.
func TestOmittedComputedValueNeverForcesReplacement(t *testing.T) {
	ctx := context.Background()
	present := tftypes.NewValue(tftypes.String, "present")

	for _, newResource := range New("test")().Resources(ctx) {
		r := newResource()

		metadata := &resource.MetadataResponse{}
		r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "apple"}, metadata)
		schemaResp := &resource.SchemaResponse{}
		r.Schema(ctx, resource.SchemaRequest{}, schemaResp)

		for name, attribute := range schemaResp.Schema.Attributes {
			address := metadata.TypeName + "." + name

			switch a := attribute.(type) {
			case schema.StringAttribute:
				if !a.Optional || !a.Computed || a.Default != nil {
					continue
				}
				req := planmodifier.StringRequest{
					ConfigValue: types.StringNull(),
					StateValue:  types.StringValue("in state"),
					State:       tfsdk.State{Raw: present},
					Plan:        tfsdk.Plan{Raw: present},
				}
				planned, replace := types.StringUnknown(), false
				for _, modifier := range a.PlanModifiers {
					req.PlanValue = planned
					step := planmodifier.StringResponse{PlanValue: planned}
					modifier.PlanModifyString(ctx, req, &step)
					planned, replace = step.PlanValue, replace || step.RequiresReplace
				}
				if replace {
					t.Errorf("%s: an omitted value forces replacement of a resource whose %s did not change", address, name)
				}

			case schema.BoolAttribute:
				if !a.Optional || !a.Computed || a.Default != nil {
					continue
				}
				req := planmodifier.BoolRequest{
					ConfigValue: types.BoolNull(),
					StateValue:  types.BoolValue(true),
					State:       tfsdk.State{Raw: present},
					Plan:        tfsdk.Plan{Raw: present},
				}
				planned, replace := types.BoolUnknown(), false
				for _, modifier := range a.PlanModifiers {
					req.PlanValue = planned
					step := planmodifier.BoolResponse{PlanValue: planned}
					modifier.PlanModifyBool(ctx, req, &step)
					planned, replace = step.PlanValue, replace || step.RequiresReplace
				}
				if replace {
					t.Errorf("%s: an omitted value forces replacement of a resource whose %s did not change", address, name)
				}

			case schema.Int64Attribute:
				if !a.Optional || !a.Computed || a.Default != nil {
					continue
				}
				req := planmodifier.Int64Request{
					ConfigValue: types.Int64Null(),
					StateValue:  types.Int64Value(1),
					State:       tfsdk.State{Raw: present},
					Plan:        tfsdk.Plan{Raw: present},
				}
				planned, replace := types.Int64Unknown(), false
				for _, modifier := range a.PlanModifiers {
					req.PlanValue = planned
					step := planmodifier.Int64Response{PlanValue: planned}
					modifier.PlanModifyInt64(ctx, req, &step)
					planned, replace = step.PlanValue, replace || step.RequiresReplace
				}
				if replace {
					t.Errorf("%s: an omitted value forces replacement of a resource whose %s did not change", address, name)
				}
			}
		}
	}
}
