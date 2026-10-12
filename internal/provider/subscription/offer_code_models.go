// Copyright (c) AO Studio
// SPDX-License-Identifier: MPL-2.0

package subscription

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"time"

	"github.com/AhmedOsman00/terraform-provider-apple/internal/apple/models"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
)

// subscriptionOfferCodeModel maps the apple_subscription_offer_code schema.
//
// customer_eligibilities and prices are types.Set rather than Go slices: a
// configuration builds prices with a for expression over an equalization data
// source, and the whole set is unknown at plan time whenever that data source
// reads a subscription created in the same apply. A Go slice cannot hold an
// unknown value.
type subscriptionOfferCodeModel struct {
	ID                         types.String `tfsdk:"id"`
	SubscriptionID             types.String `tfsdk:"subscription_id"`
	Name                       types.String `tfsdk:"name"`
	CustomerEligibilities      types.Set    `tfsdk:"customer_eligibilities"`
	OfferEligibility           types.String `tfsdk:"offer_eligibility"`
	Duration                   types.String `tfsdk:"duration"`
	OfferMode                  types.String `tfsdk:"offer_mode"`
	NumberOfPeriods            types.Int64  `tfsdk:"number_of_periods"`
	AutoRenewEnabled           types.Bool   `tfsdk:"auto_renew_enabled"`
	TargetSubscriptionPlanType types.String `tfsdk:"target_subscription_plan_type"`
	Prices                     types.Set    `tfsdk:"prices"`
	Active                     types.Bool   `tfsdk:"active"`
	TotalNumberOfCodes         types.Int64  `tfsdk:"total_number_of_codes"`
	ProductionCodeCount        types.Int64  `tfsdk:"production_code_count"`
	SandboxCodeCount           types.Int64  `tfsdk:"sandbox_code_count"`
}

// offerCodePriceModel is one territory's price inside an offer code.
//
// Like the price schedule's nested price it carries no computed attribute: a
// computed member makes a set element unknown at plan time, and Terraform can
// then no longer match it against the element in state.
type offerCodePriceModel struct {
	TerritoryID  types.String `tfsdk:"territory_id"`
	PricePointID types.String `tfsdk:"price_point_id"`
}

// offerCodePriceAttrTypes is the object type of one element of prices.
var offerCodePriceAttrTypes = map[string]attr.Type{
	"territory_id":   types.StringType,
	"price_point_id": types.StringType,
}

// subscriptionOfferCodeCustomCodeModel maps apple_subscription_offer_code_custom_code.
type subscriptionOfferCodeCustomCodeModel struct {
	ID             types.String `tfsdk:"id"`
	OfferCodeID    types.String `tfsdk:"offer_code_id"`
	CustomCode     types.String `tfsdk:"custom_code"`
	NumberOfCodes  types.Int64  `tfsdk:"number_of_codes"`
	ExpirationDate types.String `tfsdk:"expiration_date"`
	Active         types.Bool   `tfsdk:"active"`
	CreatedDate    types.String `tfsdk:"created_date"`
}

// subscriptionOfferCodeOneTimeUseCodeModel maps
// apple_subscription_offer_code_one_time_use_code. The generated codes are
// deliberately absent -- see the values data source.
type subscriptionOfferCodeOneTimeUseCodeModel struct {
	ID             types.String `tfsdk:"id"`
	OfferCodeID    types.String `tfsdk:"offer_code_id"`
	NumberOfCodes  types.Int64  `tfsdk:"number_of_codes"`
	ExpirationDate types.String `tfsdk:"expiration_date"`
	Environment    types.String `tfsdk:"environment"`
	Active         types.Bool   `tfsdk:"active"`
	CreatedDate    types.String `tfsdk:"created_date"`
}

// subscriptionOfferCodeValuesDataSourceModel maps the opt-in read of a batch's
// generated codes.
type subscriptionOfferCodeValuesDataSourceModel struct {
	ID               types.String `tfsdk:"id"`
	OneTimeUseCodeID types.String `tfsdk:"one_time_use_code_id"`
	CSV              types.String `tfsdk:"csv"`
}

// subscriptionOfferCodeSummaryModel is one offer in the listing data source.
// It carries no prices: reading them is one more request per offer.
type subscriptionOfferCodeSummaryModel struct {
	ID                         types.String   `tfsdk:"id"`
	Name                       types.String   `tfsdk:"name"`
	CustomerEligibilities      []types.String `tfsdk:"customer_eligibilities"`
	OfferEligibility           types.String   `tfsdk:"offer_eligibility"`
	Duration                   types.String   `tfsdk:"duration"`
	OfferMode                  types.String   `tfsdk:"offer_mode"`
	NumberOfPeriods            types.Int64    `tfsdk:"number_of_periods"`
	AutoRenewEnabled           types.Bool     `tfsdk:"auto_renew_enabled"`
	TargetSubscriptionPlanType types.String   `tfsdk:"target_subscription_plan_type"`
	Active                     types.Bool     `tfsdk:"active"`
	TotalNumberOfCodes         types.Int64    `tfsdk:"total_number_of_codes"`
	ProductionCodeCount        types.Int64    `tfsdk:"production_code_count"`
	SandboxCodeCount           types.Int64    `tfsdk:"sandbox_code_count"`
}

// subscriptionOfferCodesDataSourceModel maps apple_subscription_offer_codes.
type subscriptionOfferCodesDataSourceModel struct {
	SubscriptionID types.String                        `tfsdk:"subscription_id"`
	NamePattern    types.String                        `tfsdk:"name_pattern"`
	Active         types.Bool                          `tfsdk:"active"`
	OfferCodes     []subscriptionOfferCodeSummaryModel `tfsdk:"offer_codes"`
	TotalCount     types.Int64                         `tfsdk:"total_count"`
	FilteredCount  types.Int64                         `tfsdk:"filtered_count"`
}

// Offer code limits, from App Store Connect Help ("Set up offer codes"). The
// OpenAPI spec states none of them -- every one of these fields is a bare
// integer or string there -- so these are Apple's documented product rules,
// and Apple remains the final word at apply.
const (
	// A custom code's redemption limit: "You can choose a limit of up to
	// 25,000 at a time" (App Store Connect Help).
	maxCustomCodeRedemptions = 25000

	// One-time-use batch sizes: "a minimum of 500 and a maximum of 25,000"
	// for production codes, "a minimum of 10 and a maximum of 10,000" for
	// sandbox codes.
	minProductionOneTimeUseCodes = 500
	maxProductionOneTimeUseCodes = 25000
	minSandboxOneTimeUseCodes    = 10
	maxSandboxOneTimeUseCodes    = 10000

	// "Enter a custom code up to 64 characters and without any special
	// characters" (App Store Connect Help).
	maxCustomCodeLength = 64
)

// Offer code validators.
var (
	CustomerEligibilityValidator  = stringvalidator.OneOf(models.ValidSubscriptionCustomerEligibilities...)
	OfferCodeEligibilityValidator = stringvalidator.OneOf(models.ValidSubscriptionOfferCodeEligibilities...)
	OfferCodeDurationValidator    = stringvalidator.OneOf(models.ValidSubscriptionOfferDurations...)
	OfferCodeModeValidator        = stringvalidator.OneOf(models.ValidSubscriptionOfferModes...)
	OfferCodeEnvironmentValidator = stringvalidator.OneOf(models.ValidOfferCodeEnvironments...)

	// CustomCodeValidator enforces "without any special characters": letters
	// and digits only.
	CustomCodeValidator = stringvalidator.RegexMatches(
		regexp.MustCompile(`^[A-Za-z0-9]+$`),
		"Custom code must contain only letters and digits -- App Store Connect accepts no special characters",
	)

	// ExpirationDateValidator validates the plain date Apple expects:
	// "2026-12-31", never a timestamp. The spec types it format: date.
	ExpirationDateValidator = stringvalidator.RegexMatches(
		regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`),
		"Expiration date must be a plain date in YYYY-MM-DD form, not a timestamp",
	)
)

// checkExpirationWithinSixMonths enforces "valid for a maximum of six months",
// which App Store Connect Help states for one-time-use codes and for custom
// codes that carry an expiration date.
//
// Only the upper bound is checked. A lower bound -- "not in the past" -- would
// start failing every plan of a configuration the day its campaign ended, for
// a record that already exists and is never sent again; the upper bound only
// gets easier to satisfy as time passes. One day of slack covers month-end
// arithmetic, so the check never refuses a date Apple would accept.
func checkExpirationWithinSixMonths(date string, now time.Time) error {
	expiration, err := time.Parse("2006-01-02", date)
	if err != nil {
		return fmt.Errorf("%q is not a valid date: %w", date, err)
	}

	limit := now.UTC().Truncate(24*time.Hour).AddDate(0, 6, 1)
	if expiration.After(limit) {
		return fmt.Errorf("%s is more than six months away; App Store Connect accepts an expiration at most six "+
			"months after the codes are created (latest accepted today: %s)", date, limit.Format("2006-01-02"))
	}

	return nil
}

// checkOneTimeUseCodeCount applies the batch size range for an environment.
func checkOneTimeUseCodeCount(environment string, count int64) error {
	lo, hi := int64(minProductionOneTimeUseCodes), int64(maxProductionOneTimeUseCodes)
	if environment == models.OfferCodeEnvironmentSandbox {
		lo, hi = minSandboxOneTimeUseCodes, maxSandboxOneTimeUseCodes
	}

	if count < lo || count > hi {
		return fmt.Errorf("a %s batch holds between %d and %d codes, got %d; for more, create several batches "+
			"for the same offer", environment, lo, hi, count)
	}

	return nil
}

// checkOfferCodePrices reports what is wrong with a configured price set: a
// territory named twice, or a paid offer with a territory that has no price
// point. Prices that are still unknown are skipped.
func checkOfferCodePrices(offerMode string, prices []offerCodePriceModel) []string {
	var problems []string
	seen := map[string]bool{}

	for _, price := range prices {
		if price.TerritoryID.IsUnknown() || price.TerritoryID.IsNull() {
			continue
		}
		territory := price.TerritoryID.ValueString()

		if seen[territory] {
			problems = append(problems, fmt.Sprintf("territory %s is priced more than once; an offer has one "+
				"price per territory", territory))
		}
		seen[territory] = true

		if offerMode != "" && offerMode != string(models.SubscriptionOfferModeFreeTrial) &&
			price.PricePointID.IsNull() {
			problems = append(problems, fmt.Sprintf("territory %s has no price_point_id; a %s offer charges "+
				"a price, which is one of Apple's price points for the subscription", territory, offerMode))
		}
	}

	sort.Strings(problems)
	return problems
}

// offerCodePricesFromAPI converts Apple's prices to the set's elements, in a
// stable order. A price with no territory linkage is skipped -- it cannot be
// told apart from any other -- and a missing price point is a null, which is
// what a FREE_TRIAL reports.
func offerCodePricesFromAPI(prices []models.SubscriptionOfferCodePrice) []offerCodePriceModel {
	converted := make([]offerCodePriceModel, 0, len(prices))

	for _, price := range prices {
		if price.Relationships == nil || price.Relationships.Territory == nil ||
			price.Relationships.Territory.Data.ID == "" {
			continue
		}

		model := offerCodePriceModel{
			TerritoryID:  types.StringValue(price.Relationships.Territory.Data.ID),
			PricePointID: types.StringNull(),
		}
		if point := price.Relationships.SubscriptionPricePoint; point != nil && point.Data.ID != "" {
			model.PricePointID = types.StringValue(point.Data.ID)
		}

		converted = append(converted, model)
	}

	sort.Slice(converted, func(i, j int) bool {
		return converted[i].TerritoryID.ValueString() < converted[j].TerritoryID.ValueString()
	})

	return converted
}

// offerCodePricesToSet builds the Terraform set for prices.
func offerCodePricesToSet(ctx context.Context, prices []offerCodePriceModel) (types.Set, diag.Diagnostics) {
	return types.SetValueFrom(ctx, types.ObjectType{AttrTypes: offerCodePriceAttrTypes}, prices)
}

// offerCodeInt64 converts an optional API integer to Terraform.
func offerCodeInt64(v *int) types.Int64 {
	if v == nil {
		return types.Int64Null()
	}
	return types.Int64Value(int64(*v))
}

// offerCodeBool converts an optional API boolean to Terraform.
func offerCodeBool(v *bool) types.Bool {
	if v == nil {
		return types.BoolNull()
	}
	return types.BoolValue(*v)
}

// reactivationRequiresReplace is the plan modifier on every offer code
// record's "active".
//
// Apple's update request accepts "active": true, but App Store Connect's help
// says a deactivated offer "can't be reactivated later", and every code under
// it is invalidated. Turning one back on is therefore not an update: the only
// honest plan for false -> true is a new record. true -> false stays in place,
// which is how a campaign is ended without dropping it from the configuration.
func reactivationRequiresReplace() planmodifier.Bool {
	return boolplanmodifier.RequiresReplaceIf(
		func(_ context.Context, req planmodifier.BoolRequest, resp *boolplanmodifier.RequiresReplaceIfFuncResponse) {
			resp.RequiresReplace = isReactivation(req.StateValue, req.PlanValue)
		},
		"A deactivated record cannot be reactivated, so turning it back on creates a new one.",
		"A deactivated record cannot be reactivated, so turning it back on creates a new one.",
	)
}

// isReactivation reports whether a plan turns an inactive record back on.
func isReactivation(state, plan basetypes.BoolValue) bool {
	if state.IsNull() || state.IsUnknown() || plan.IsNull() || plan.IsUnknown() {
		return false
	}
	return !state.ValueBool() && plan.ValueBool()
}
