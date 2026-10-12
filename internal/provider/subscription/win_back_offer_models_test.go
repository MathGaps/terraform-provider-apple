// Copyright (c) AO Studio
// SPDX-License-Identifier: MPL-2.0

package subscription

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/AhmedOsman00/terraform-provider-apple/internal/apple/models"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func winBackPrice(pointID, territory string) models.WinBackOfferPrice {
	return models.WinBackOfferPrice{
		Type: "winBackOfferPrices",
		ID:   "price-" + pointID,
		Relationships: &models.WinBackOfferPriceRelationships{
			Territory:              &models.ResourceIdentifier{Data: models.ResourceData{Type: "territories", ID: territory}},
			SubscriptionPricePoint: &models.ResourceIdentifier{Data: models.ResourceData{Type: "subscriptionPricePoints", ID: pointID}},
		},
	}
}

// TestReconcileWinBackOfferPrices covers the refresh of a RequiresReplace set,
// where adopting the wrong thing plans the destruction of a live offer.
func TestReconcileWinBackOfferPrices(t *testing.T) {
	reported := []models.WinBackOfferPrice{
		winBackPrice("point-usa", "USA"),
		winBackPrice("point-gbr", "GBR"),
		winBackPrice("point-can", "CAN"),
	}

	t.Run("keeps a configured price exactly as written", func(t *testing.T) {
		// territory_id was left out of configuration; Apple reports one
		// anyway, and adopting it would be a diff on a RequiresReplace set.
		state := []winBackOfferPriceModel{{PricePointID: types.StringValue("point-usa"), TerritoryID: types.StringNull()}}

		got := reconcileWinBackOfferPrices(state, reported)

		if len(got) != 1 {
			t.Fatalf("got %d prices, want 1 -- a price Apple derived must not be adopted over configuration: %+v", len(got), got)
		}
		if !got[0].TerritoryID.IsNull() {
			t.Errorf("territory_id = %s, want null as configured", got[0].TerritoryID)
		}
	})

	t.Run("drops a configured price Apple no longer reports", func(t *testing.T) {
		state := []winBackOfferPriceModel{
			{PricePointID: types.StringValue("point-usa"), TerritoryID: types.StringValue("USA")},
			{PricePointID: types.StringValue("point-deu"), TerritoryID: types.StringValue("DEU")},
		}

		got := reconcileWinBackOfferPrices(state, reported)

		if len(got) != 1 || got[0].PricePointID.ValueString() != "point-usa" {
			t.Errorf("got %+v, want only point-usa", got)
		}
	})

	t.Run("adopts every price on import", func(t *testing.T) {
		got := reconcileWinBackOfferPrices(nil, reported)

		if len(got) != 3 {
			t.Fatalf("got %d prices, want 3", len(got))
		}
		if got[1].PricePointID.ValueString() != "point-gbr" || got[1].TerritoryID.ValueString() != "GBR" {
			t.Errorf("second adopted price = %+v, want point-gbr in GBR", got[1])
		}
	})

	t.Run("skips a price reported without its price point", func(t *testing.T) {
		got := reconcileWinBackOfferPrices(nil, []models.WinBackOfferPrice{{Type: "winBackOfferPrices", ID: "bare"}})

		if len(got) != 0 {
			t.Errorf("got %+v, want nothing -- a price without a price point cannot be written back", got)
		}
	})
}

func winBackOfferTestModel() winBackOfferModel {
	return winBackOfferModel{
		PaidSubscriptionDuration: types.Int64Value(6),
		TimeSinceLastSubscribed:  monthRangeToTerraform(&models.IntegerRange{Minimum: intPointer(2), Maximum: intPointer(24)}),
		WaitBetweenOffers:        types.Int64Value(2),
		StartDate:                types.StringValue("2027-07-01"),
		EndDate:                  types.StringValue("2027-07-31"),
		Priority:                 types.StringValue("HIGH"),
		PromotionIntent:          types.StringValue("NOT_PROMOTED"),
	}
}

func intPointer(i int) *int { return &i }

// TestWinBackOfferUpdateSendsOnlyWhatChanged checks the PATCH body built from
// plan and state: unchanged members are left out, and removed ones become
// explicit nulls rather than omissions Apple would read as "keep".
func TestWinBackOfferUpdateSendsOnlyWhatChanged(t *testing.T) {
	ctx := context.Background()
	state := winBackOfferTestModel()

	plan := winBackOfferTestModel()
	plan.Priority = types.StringValue("NORMAL")
	plan.EndDate = types.StringNull()
	plan.WaitBetweenOffers = types.Int64Null()
	plan.TimeSinceLastSubscribed = monthRangeToTerraform(&models.IntegerRange{Minimum: intPointer(3), Maximum: intPointer(12)})

	var diags diag.Diagnostics
	update, changed := winBackOfferUpdate(ctx, plan, state, &diags)
	if diags.HasError() {
		t.Fatalf("diagnostics: %+v", diags)
	}
	if !changed {
		t.Fatal("changed = false for a plan that changes four members")
	}

	body, err := json.Marshal(update)
	if err != nil {
		t.Fatalf("marshal: %s", err)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal: %s", err)
	}

	want := map[string]any{
		"priority": "NORMAL",
		"endDate":  nil,
		"customerEligibilityWaitBetweenOffersInMonths":       nil,
		"customerEligibilityTimeSinceLastSubscribedInMonths": map[string]any{"minimum": float64(3), "maximum": float64(12)},
	}
	if len(got) != len(want) {
		t.Errorf("body = %s, want exactly %v", body, want)
	}
	for name, value := range want {
		gotValue, present := got[name]
		if !present {
			t.Errorf("%s missing from %s", name, body)
			continue
		}
		if string(mustJSON(t, gotValue)) != string(mustJSON(t, value)) {
			t.Errorf("%s = %v, want %v", name, gotValue, value)
		}
	}
}

// TestWinBackOfferUpdateLeavesPromotionIntentAloneWhenOmitted: promotion_intent
// is Optional+Computed, so a configuration that stops naming it keeps Apple's
// value rather than clearing it.
func TestWinBackOfferUpdateLeavesPromotionIntentAloneWhenOmitted(t *testing.T) {
	state := winBackOfferTestModel()
	plan := winBackOfferTestModel()
	plan.PromotionIntent = types.StringUnknown()

	var diags diag.Diagnostics
	update, changed := winBackOfferUpdate(context.Background(), plan, state, &diags)
	if changed || update.PromotionIntent != nil {
		t.Errorf("changed = %v, promotionIntent = %v; want nothing sent", changed, update.PromotionIntent)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %s", err)
	}
	return b
}

// TestValidateWinBackOfferConfig covers the two rules that span attributes.
func TestValidateWinBackOfferConfig(t *testing.T) {
	window := func(minimum, maximum int64) *monthRangeModel {
		return &monthRangeModel{Minimum: types.Int64Value(minimum), Maximum: types.Int64Value(maximum)}
	}

	tests := []struct {
		name     string
		window   *monthRangeModel
		start    types.String
		end      types.String
		problems int
	}{
		{"valid", window(2, 24), types.StringValue("2027-07-01"), types.StringValue("2027-07-31"), 0},
		{"no end date", window(1, 1), types.StringValue("2027-07-01"), types.StringNull(), 0},
		{"inverted window", window(12, 3), types.StringValue("2027-07-01"), types.StringNull(), 1},
		{"end before start", window(2, 24), types.StringValue("2027-07-31"), types.StringValue("2027-07-01"), 1},
		{"end on start", window(2, 24), types.StringValue("2027-07-01"), types.StringValue("2027-07-01"), 1},
		{"one day apart", window(2, 24), types.StringValue("2027-07-01"), types.StringValue("2027-07-02"), 1},
		{"two days apart, three counted inclusively", window(2, 24), types.StringValue("2027-07-01"),
			types.StringValue("2027-07-03"), 0},
		{"no thirteenth month", window(2, 24), types.StringValue("2027-13-01"), types.StringNull(), 1},
		{"no thirtieth of February", window(2, 24), types.StringValue("2027-07-01"),
			types.StringValue("2027-02-30"), 1},
		{"unknown dates are Apple's to check", window(2, 24), types.StringUnknown(), types.StringValue("2027-07-01"), 0},
		{"unknown window bound", &monthRangeModel{Minimum: types.Int64Unknown(), Maximum: types.Int64Value(1)},
			types.StringValue("2027-07-01"), types.StringNull(), 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validateWinBackOfferConfig(tt.window, tt.start, tt.end); len(got) != tt.problems {
				t.Errorf("got %d problems %q, want %d", len(got), got, tt.problems)
			}
		})
	}
}

// TestApplyWinBackOfferAttributesMapsOmissionsToNull: an optional member Apple
// omits must be null in state, not zero or "", or a configuration that left it
// out would show a diff.
func TestApplyWinBackOfferAttributesMapsOmissionsToNull(t *testing.T) {
	var model winBackOfferModel
	applyWinBackOfferAttributes(&model, &models.WinBackOffer{
		ID: "1",
		Attributes: models.WinBackOfferAttributes{
			ReferenceName: "r", OfferID: "o", Duration: "ONE_MONTH", OfferMode: "FREE_TRIAL", PeriodCount: 1,
			CustomerEligibilityPaidSubscriptionDurationInMonths: 6,
			StartDate: "2027-07-01", Priority: "HIGH",
		},
	})

	for name, isNull := range map[string]bool{
		"customer_eligibility_wait_between_offers_in_months": model.WaitBetweenOffers.IsNull(),
		"end_date":                      model.EndDate.IsNull(),
		"promotion_intent":              model.PromotionIntent.IsNull(),
		"target_subscription_plan_type": model.TargetSubscriptionPlanType.IsNull(),
		"customer_eligibility_time_since_last_subscribed_in_months": model.TimeSinceLastSubscribed.IsNull(),
	} {
		if !isNull {
			t.Errorf("%s is not null for a member Apple omitted", name)
		}
	}
	if model.ID.ValueString() != "1" || model.PeriodCount.ValueInt64() != 1 {
		t.Errorf("model = %+v", model)
	}
}

// TestTargetPlanTypeOmittedDoesNotReplace runs the schema's own plan modifiers
// for target_subscription_plan_type on an update whose configuration leaves it
// out. It is Optional+Computed+RequiresReplace, so the omitted value plans as
// unknown, and if RequiresReplace saw that before UseStateForUnknown resolved
// it, every in-place update -- a new end date, a priority change -- would
// instead destroy the live offer.
func TestTargetPlanTypeOmittedDoesNotReplace(t *testing.T) {
	ctx := context.Background()

	resp := &resource.SchemaResponse{}
	NewWinBackOfferResource().Schema(ctx, resource.SchemaRequest{}, resp)
	attribute, ok := resp.Schema.Attributes["target_subscription_plan_type"].(schema.StringAttribute)
	if !ok {
		t.Fatal("target_subscription_plan_type is not a string attribute")
	}

	present := tftypes.NewValue(tftypes.String, "present")
	req := planmodifier.StringRequest{
		ConfigValue: types.StringNull(),
		StateValue:  types.StringNull(),
		State:       tfsdk.State{Raw: present},
		Plan:        tfsdk.Plan{Raw: present},
	}
	planned := planmodifier.StringResponse{PlanValue: types.StringUnknown()}

	for _, modifier := range attribute.PlanModifiers {
		req.PlanValue = planned.PlanValue
		step := planmodifier.StringResponse{PlanValue: planned.PlanValue}
		modifier.PlanModifyString(ctx, req, &step)
		planned.PlanValue = step.PlanValue
		planned.RequiresReplace = planned.RequiresReplace || step.RequiresReplace
	}

	if planned.RequiresReplace {
		t.Error("an omitted target_subscription_plan_type forces replacement of an unchanged offer")
	}
	if !planned.PlanValue.IsNull() {
		t.Errorf("planned value = %s, want the null held in state", planned.PlanValue)
	}
}
