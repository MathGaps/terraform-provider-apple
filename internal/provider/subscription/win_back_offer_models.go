// Copyright (c) AO Studio
// SPDX-License-Identifier: MPL-2.0

package subscription

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"time"

	"github.com/AhmedOsman00/terraform-provider-apple/internal/apple"
	"github.com/AhmedOsman00/terraform-provider-apple/internal/apple/models"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

// winBackOfferModel maps the apple_subscription_win_back_offer schema.
//
// The eligibility window and the prices are framework values rather than Go
// structs and slices: either can come from a variable or a for expression, and
// is then unknown at plan time, which a Go type cannot hold.
type winBackOfferModel struct {
	ID                         types.String `tfsdk:"id"`
	SubscriptionID             types.String `tfsdk:"subscription_id"`
	ReferenceName              types.String `tfsdk:"reference_name"`
	OfferID                    types.String `tfsdk:"offer_id"`
	Duration                   types.String `tfsdk:"duration"`
	OfferMode                  types.String `tfsdk:"offer_mode"`
	PeriodCount                types.Int64  `tfsdk:"period_count"`
	PaidSubscriptionDuration   types.Int64  `tfsdk:"customer_eligibility_paid_subscription_duration_in_months"`
	TimeSinceLastSubscribed    types.Object `tfsdk:"customer_eligibility_time_since_last_subscribed_in_months"`
	WaitBetweenOffers          types.Int64  `tfsdk:"customer_eligibility_wait_between_offers_in_months"`
	StartDate                  types.String `tfsdk:"start_date"`
	EndDate                    types.String `tfsdk:"end_date"`
	Priority                   types.String `tfsdk:"priority"`
	PromotionIntent            types.String `tfsdk:"promotion_intent"`
	TargetSubscriptionPlanType types.String `tfsdk:"target_subscription_plan_type"`
	Prices                     types.Set    `tfsdk:"prices"`
}

// winBackOfferPriceModel is one element of prices.
type winBackOfferPriceModel struct {
	PricePointID types.String `tfsdk:"price_point_id"`
	TerritoryID  types.String `tfsdk:"territory_id"`
}

// monthRangeModel is Apple's IntegerRange, in months.
type monthRangeModel struct {
	Minimum types.Int64 `tfsdk:"minimum"`
	Maximum types.Int64 `tfsdk:"maximum"`
}

// winBackOfferListingModel is one offer as the data source reports it. It is a
// computed value, never unknown, so plain Go types are safe here.
type winBackOfferListingModel struct {
	ID                         types.String     `tfsdk:"id"`
	ReferenceName              types.String     `tfsdk:"reference_name"`
	OfferID                    types.String     `tfsdk:"offer_id"`
	Duration                   types.String     `tfsdk:"duration"`
	OfferMode                  types.String     `tfsdk:"offer_mode"`
	PeriodCount                types.Int64      `tfsdk:"period_count"`
	PaidSubscriptionDuration   types.Int64      `tfsdk:"customer_eligibility_paid_subscription_duration_in_months"`
	TimeSinceLastSubscribed    *monthRangeModel `tfsdk:"customer_eligibility_time_since_last_subscribed_in_months"`
	WaitBetweenOffers          types.Int64      `tfsdk:"customer_eligibility_wait_between_offers_in_months"`
	StartDate                  types.String     `tfsdk:"start_date"`
	EndDate                    types.String     `tfsdk:"end_date"`
	Priority                   types.String     `tfsdk:"priority"`
	PromotionIntent            types.String     `tfsdk:"promotion_intent"`
	TargetSubscriptionPlanType types.String     `tfsdk:"target_subscription_plan_type"`
}

// winBackOffersDataSourceModel maps the apple_subscription_win_back_offers
// data source.
type winBackOffersDataSourceModel struct {
	SubscriptionID types.String               `tfsdk:"subscription_id"`
	OfferID        types.String               `tfsdk:"offer_id"`
	WinBackOffers  []winBackOfferListingModel `tfsdk:"win_back_offers"`
	TotalCount     types.Int64                `tfsdk:"total_count"`
}

var (
	monthRangeAttrTypes = map[string]attr.Type{
		"minimum": types.Int64Type,
		"maximum": types.Int64Type,
	}

	winBackOfferPriceAttrTypes = map[string]attr.Type{
		"price_point_id": types.StringType,
		"territory_id":   types.StringType,
	}
)

// Win-back offer validators.
//
// The integer limits come from App Store Connect Help ("Set up win-back
// offers"), not from the OpenAPI spec, which types every eligibility member as
// a bare integer: minimum paid duration "between 1 and 24 months, or 3, 4, and
// 5 years"; time since last subscribed "a minimum value of 1 month and a
// maximum value of 24 months ... Both a minimum and maximum value are
// required"; wait between offers "between 2 and 24 months".
var (
	WinBackOfferDurationValidator        = stringvalidator.OneOf(models.ValidSubscriptionOfferDurations...)
	WinBackOfferModeValidator            = stringvalidator.OneOf(models.ValidSubscriptionOfferModes...)
	WinBackOfferPriorityValidator        = stringvalidator.OneOf(models.ValidWinBackOfferPriorities...)
	WinBackOfferPromotionIntentValidator = stringvalidator.OneOf(models.ValidWinBackOfferPromotionIntents...)

	// WinBackOfferDateValidator validates the plain dates Apple's startDate and
	// endDate are (format: date, not date-time).
	WinBackOfferDateValidator = stringvalidator.RegexMatches(
		regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`),
		"must be a plain date in YYYY-MM-DD form, not a timestamp",
	)

	// WinBackOfferPaidDurationValidator: 1-24 months, or 3, 4 or 5 years.
	WinBackOfferPaidDurationValidator = int64validator.OneOf(
		1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 36, 48, 60,
	)

	// WinBackOfferLapsedMonthsValidator bounds either end of the time since
	// the customer last subscribed.
	WinBackOfferLapsedMonthsValidator = int64validator.Between(1, 24)

	// WinBackOfferWaitValidator bounds the wait between redemptions.
	WinBackOfferWaitValidator = int64validator.Between(2, 24)
)

// GetWinBackOfferStringValidators returns the validators for the reference
// name and the offer ID. Apple publishes no length or character rule for
// either, only that both are required and unique, so this only refuses an
// empty string rather than guess a rule Apple might not apply.
func GetWinBackOfferStringValidators() []validator.String {
	return []validator.String{stringvalidator.LengthAtLeast(1)}
}

// int64OrNull maps an optional API integer to Terraform.
func int64OrNull(i *int) types.Int64 {
	if i == nil {
		return types.Int64Null()
	}
	return types.Int64Value(int64(*i))
}

// monthRangeToTerraform converts Apple's IntegerRange to the object attribute.
func monthRangeToTerraform(r *models.IntegerRange) types.Object {
	if r == nil {
		return types.ObjectNull(monthRangeAttrTypes)
	}
	return types.ObjectValueMust(monthRangeAttrTypes, map[string]attr.Value{
		"minimum": int64OrNull(r.Minimum),
		"maximum": int64OrNull(r.Maximum),
	})
}

// monthRangeToAPI converts the object attribute to Apple's IntegerRange.
func monthRangeToAPI(ctx context.Context, o types.Object, diags *diag.Diagnostics) *models.IntegerRange {
	if o.IsNull() || o.IsUnknown() {
		return nil
	}
	var r monthRangeModel
	diags.Append(o.As(ctx, &r, basetypes.ObjectAsOptions{})...)
	return &models.IntegerRange{Minimum: intOrNil(r.Minimum), Maximum: intOrNil(r.Maximum)}
}

// winBackOfferPricesToAPI converts the prices set into the client's input,
// sorted by price point so the same configuration always produces the same
// request body.
func winBackOfferPricesToAPI(ctx context.Context, set types.Set, diags *diag.Diagnostics) []apple.WinBackOfferPriceInput {
	if set.IsNull() || set.IsUnknown() {
		return nil
	}
	var prices []winBackOfferPriceModel
	diags.Append(set.ElementsAs(ctx, &prices, false)...)

	inputs := make([]apple.WinBackOfferPriceInput, 0, len(prices))
	for _, price := range prices {
		inputs = append(inputs, apple.WinBackOfferPriceInput{
			PricePointID: price.PricePointID.ValueString(),
			TerritoryID:  price.TerritoryID.ValueString(),
		})
	}
	sort.Slice(inputs, func(i, j int) bool { return inputs[i].PricePointID < inputs[j].PricePointID })
	return inputs
}

// winBackOfferPricesToTerraform builds the prices set.
func winBackOfferPricesToTerraform(prices []winBackOfferPriceModel) types.Set {
	elems := make([]attr.Value, 0, len(prices))
	for _, price := range prices {
		elems = append(elems, types.ObjectValueMust(winBackOfferPriceAttrTypes, map[string]attr.Value{
			"price_point_id": price.PricePointID,
			"territory_id":   price.TerritoryID,
		}))
	}
	return types.SetValueMust(types.ObjectType{AttrTypes: winBackOfferPriceAttrTypes}, elems)
}

// reconcileWinBackOfferPrices matches the prices Apple reports against what
// state holds.
//
// A price state knows about is kept exactly as written while Apple still
// reports its price point -- territory_id included, which Apple reports on
// every price whether or not one was sent, the same trap
// reconcileSchedulePrices avoids. A price state knows about that Apple no
// longer reports is dropped, so its loss shows as drift.
//
// A price Apple reports that state does not know about is adopted only when
// state holds no prices at all, which is an import. Otherwise it is ignored:
// prices are RequiresReplace because Apple's update request cannot change
// them, so adopting a price nobody configured would plan the destruction of a
// live offer -- and App Store Connect cannot add one after creation either, so
// such a price can only be one Apple derived itself.
func reconcileWinBackOfferPrices(state []winBackOfferPriceModel, reported []models.WinBackOfferPrice) []winBackOfferPriceModel {
	type reportedPrice struct{ territory types.String }

	byPricePoint := make(map[string]reportedPrice, len(reported))
	order := make([]string, 0, len(reported))
	for _, price := range reported {
		if price.Relationships == nil || price.Relationships.SubscriptionPricePoint == nil {
			continue
		}
		pointID := price.Relationships.SubscriptionPricePoint.Data.ID
		if pointID == "" {
			continue
		}
		if _, seen := byPricePoint[pointID]; !seen {
			order = append(order, pointID)
		}
		byPricePoint[pointID] = reportedPrice{territory: territoryID(price.Relationships.Territory)}
	}

	reconciled := make([]winBackOfferPriceModel, 0, len(byPricePoint))
	if len(state) > 0 {
		for _, price := range state {
			if _, ok := byPricePoint[price.PricePointID.ValueString()]; ok {
				reconciled = append(reconciled, price)
			}
		}
		return reconciled
	}

	for _, pointID := range order {
		reconciled = append(reconciled, winBackOfferPriceModel{
			PricePointID: types.StringValue(pointID),
			TerritoryID:  byPricePoint[pointID].territory,
		})
	}
	return reconciled
}

// applyWinBackOfferAttributes copies what Apple reports into the resource
// model. Every attribute is Apple's to report; none is an instruction Apple
// does not echo back, so there is nothing to preserve from configuration.
func applyWinBackOfferAttributes(model *winBackOfferModel, offer *models.WinBackOffer) {
	a := offer.Attributes
	model.ID = types.StringValue(offer.ID)
	model.ReferenceName = types.StringValue(a.ReferenceName)
	model.OfferID = types.StringValue(a.OfferID)
	model.Duration = types.StringValue(string(a.Duration))
	model.OfferMode = types.StringValue(string(a.OfferMode))
	model.PeriodCount = types.Int64Value(int64(a.PeriodCount))
	model.PaidSubscriptionDuration = types.Int64Value(int64(a.CustomerEligibilityPaidSubscriptionDurationInMonths))
	model.TimeSinceLastSubscribed = monthRangeToTerraform(a.CustomerEligibilityTimeSinceLastSubscribedInMonths)
	model.WaitBetweenOffers = int64OrNull(a.CustomerEligibilityWaitBetweenOffersInMonths)
	model.StartDate = types.StringValue(a.StartDate)
	model.EndDate = stringOrNull(a.EndDate)
	model.Priority = types.StringValue(a.Priority)
	model.PromotionIntent = stringOrNull(a.PromotionIntent)
	model.TargetSubscriptionPlanType = stringOrNull(a.TargetSubscriptionPlanType)
}

// winBackOfferListing converts one offer for the data source.
func winBackOfferListing(offer models.WinBackOffer) winBackOfferListingModel {
	var resourceModel winBackOfferModel
	applyWinBackOfferAttributes(&resourceModel, &offer)

	listing := winBackOfferListingModel{
		ID:                         resourceModel.ID,
		ReferenceName:              resourceModel.ReferenceName,
		OfferID:                    resourceModel.OfferID,
		Duration:                   resourceModel.Duration,
		OfferMode:                  resourceModel.OfferMode,
		PeriodCount:                resourceModel.PeriodCount,
		PaidSubscriptionDuration:   resourceModel.PaidSubscriptionDuration,
		WaitBetweenOffers:          resourceModel.WaitBetweenOffers,
		StartDate:                  resourceModel.StartDate,
		EndDate:                    resourceModel.EndDate,
		Priority:                   resourceModel.Priority,
		PromotionIntent:            resourceModel.PromotionIntent,
		TargetSubscriptionPlanType: resourceModel.TargetSubscriptionPlanType,
	}
	if r := offer.Attributes.CustomerEligibilityTimeSinceLastSubscribedInMonths; r != nil {
		listing.TimeSinceLastSubscribed = &monthRangeModel{Minimum: int64OrNull(r.Minimum), Maximum: int64OrNull(r.Maximum)}
	}
	return listing
}

// winBackOfferUpdate builds the PATCH body from the difference between plan
// and state.
//
// Only what changed is sent. Apple accepts some of these members only while an
// offer has not gone live, so resending an unchanged start date alongside a
// new end date could fail a change Apple would otherwise accept. A member
// removed from configuration is sent as an explicit null: omitting it would
// leave Apple's value in place. promotion_intent is Optional+Computed and so
// is never removed, only changed.
func winBackOfferUpdate(ctx context.Context, plan, state winBackOfferModel, diags *diag.Diagnostics) (models.WinBackOfferUpdateAttributes, bool) {
	var update models.WinBackOfferUpdateAttributes
	changed := false

	if !plan.PaidSubscriptionDuration.Equal(state.PaidSubscriptionDuration) {
		update.CustomerEligibilityPaidSubscriptionDurationInMonths = intOrNil(plan.PaidSubscriptionDuration)
		changed = true
	}
	if !plan.TimeSinceLastSubscribed.Equal(state.TimeSinceLastSubscribed) {
		update.CustomerEligibilityTimeSinceLastSubscribedInMonths = monthRangeToAPI(ctx, plan.TimeSinceLastSubscribed, diags)
		changed = true
	}
	if !plan.WaitBetweenOffers.Equal(state.WaitBetweenOffers) {
		update.CustomerEligibilityWaitBetweenOffersInMonths = intOrNil(plan.WaitBetweenOffers)
		update.ClearWaitBetweenOffers = plan.WaitBetweenOffers.IsNull()
		changed = true
	}
	if !plan.StartDate.Equal(state.StartDate) {
		update.StartDate = stringOrNil(plan.StartDate)
		changed = true
	}
	if !plan.EndDate.Equal(state.EndDate) {
		update.EndDate = stringOrNil(plan.EndDate)
		update.ClearEndDate = plan.EndDate.IsNull()
		changed = true
	}
	if !plan.Priority.Equal(state.Priority) {
		update.Priority = stringOrNil(plan.Priority)
		changed = true
	}
	if !plan.PromotionIntent.Equal(state.PromotionIntent) && !plan.PromotionIntent.IsNull() && !plan.PromotionIntent.IsUnknown() {
		update.PromotionIntent = stringOrNil(plan.PromotionIntent)
		changed = true
	}

	return update, changed
}

// validateWinBackOfferConfig checks the rules that span two attributes. Each
// value is checked only once it is known; an unknown one is checked at apply
// by Apple.
func validateWinBackOfferConfig(window *monthRangeModel, startDate, endDate types.String) []string {
	var problems []string

	if window != nil && !window.Minimum.IsNull() && !window.Minimum.IsUnknown() &&
		!window.Maximum.IsNull() && !window.Maximum.IsUnknown() &&
		window.Minimum.ValueInt64() > window.Maximum.ValueInt64() {
		problems = append(problems, fmt.Sprintf(
			"customer_eligibility_time_since_last_subscribed_in_months has minimum %d above maximum %d: the "+
				"range is the months since the customer's subscription lapsed, inclusive at both ends.",
			window.Minimum.ValueInt64(), window.Maximum.ValueInt64()))
	}

	// The validator holds both to the YYYY-MM-DD form; only parsing says whether
	// the form names a day that exists.
	start, startOK := winBackOfferDate("start_date", startDate, &problems)
	end, endOK := winBackOfferDate("end_date", endDate, &problems)

	// Apple requires a win-back offer to run for at least three days and does
	// not say whether it counts both ends. An end two days after the start is
	// three days counted inclusively, so anything shorter fails under either
	// reading; that is all this refuses, and Apple decides the rest.
	if startOK && endOK && end.Before(start.AddDate(0, 0, 2)) {
		problems = append(problems, fmt.Sprintf(
			"end_date %s is less than two days after start_date %s. Apple requires a win-back offer to run "+
				"for at least three days.", endDate.ValueString(), startDate.ValueString()))
	}

	return problems
}

// winBackOfferDate parses a known plain date, adding a problem when it names no
// real day (2027-02-30, 2027-13-01). A null or unknown value is not checked.
func winBackOfferDate(attribute string, value types.String, problems *[]string) (time.Time, bool) {
	if value.IsNull() || value.IsUnknown() {
		return time.Time{}, false
	}

	date, err := time.Parse(time.DateOnly, value.ValueString())
	if err != nil {
		*problems = append(*problems, fmt.Sprintf("%s %s is not a calendar date.", attribute, value.ValueString()))
		return time.Time{}, false
	}

	return date, true
}

// winBackOfferStartTooSoon reports a start date that is not after today, which
// Apple refuses: a win-back offer starts tomorrow at the earliest. Apple does
// not say in which time zone, so today is taken where the calendar is furthest
// behind (UTC-12): a date that is not after today there is not after today
// anywhere. A date that is not a calendar date is ValidateConfig's to report.
func winBackOfferStartTooSoon(startDate string, now time.Time) string {
	start, err := time.Parse(time.DateOnly, startDate)
	if err != nil {
		return ""
	}

	latest := now.In(time.FixedZone("UTC-12", -12*60*60))
	today := time.Date(latest.Year(), latest.Month(), latest.Day(), 0, 0, 0, 0, time.UTC)
	if start.After(today) {
		return ""
	}

	return fmt.Sprintf("start_date %s is not after today (%s). Apple starts a win-back offer tomorrow at the "+
		"earliest.", startDate, today.Format(time.DateOnly))
}
