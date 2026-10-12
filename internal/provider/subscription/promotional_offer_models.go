// Copyright (c) AO Studio
// SPDX-License-Identifier: MPL-2.0

package subscription

import (
	"regexp"
	"sort"

	"github.com/AhmedOsman00/terraform-provider-apple/internal/apple"
	"github.com/AhmedOsman00/terraform-provider-apple/internal/apple/models"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// subscriptionPromotionalOfferModel maps the promotional offer resource.
//
// Prices is a types.Set rather than a Go slice: a configuration that builds
// the set with a for expression over a data source has an unknown set at plan
// time, and a slice cannot hold that.
type subscriptionPromotionalOfferModel struct {
	ID                         types.String `tfsdk:"id"`
	SubscriptionID             types.String `tfsdk:"subscription_id"`
	Name                       types.String `tfsdk:"name"`
	OfferCode                  types.String `tfsdk:"offer_code"`
	Duration                   types.String `tfsdk:"duration"`
	OfferMode                  types.String `tfsdk:"offer_mode"`
	NumberOfPeriods            types.Int64  `tfsdk:"number_of_periods"`
	TargetSubscriptionPlanType types.String `tfsdk:"target_subscription_plan_type"`
	Prices                     types.Set    `tfsdk:"prices"`
}

// promotionalOfferPriceModel is one territory's price inside the offer.
//
// It carries no id: a computed attribute inside a set makes the element
// unknown at plan time, which leaves Terraform unable to match a planned
// element against the one in state -- the same reason the price schedule's
// nested price has none.
type promotionalOfferPriceModel struct {
	TerritoryID  types.String `tfsdk:"territory_id"`
	PricePointID types.String `tfsdk:"price_point_id"`
}

// promotionalOfferPriceAttrTypes is the object type of one element of prices.
var promotionalOfferPriceAttrTypes = map[string]attr.Type{
	"territory_id":   types.StringType,
	"price_point_id": types.StringType,
}

// promotionalOfferPriceObjectType is the element type of prices.
var promotionalOfferPriceObjectType = types.ObjectType{AttrTypes: promotionalOfferPriceAttrTypes}

// subscriptionPromotionalOfferSummaryModel is one offer in the listing data
// source. It has no prices: those are a second request per offer.
type subscriptionPromotionalOfferSummaryModel struct {
	ID                         types.String `tfsdk:"id"`
	Name                       types.String `tfsdk:"name"`
	OfferCode                  types.String `tfsdk:"offer_code"`
	Duration                   types.String `tfsdk:"duration"`
	OfferMode                  types.String `tfsdk:"offer_mode"`
	NumberOfPeriods            types.Int64  `tfsdk:"number_of_periods"`
	TargetSubscriptionPlanType types.String `tfsdk:"target_subscription_plan_type"`
}

// subscriptionPromotionalOffersDataSourceModel maps the listing data source.
type subscriptionPromotionalOffersDataSourceModel struct {
	// Scope
	SubscriptionID types.String `tfsdk:"subscription_id"`

	// Filter configuration
	OfferCode types.String `tfsdk:"offer_code"`

	// Output
	PromotionalOffers []subscriptionPromotionalOfferSummaryModel `tfsdk:"promotional_offers"`

	// Computed metadata
	TotalCount    types.Int64 `tfsdk:"total_count"`
	FilteredCount types.Int64 `tfsdk:"filtered_count"`
}

// Promotional offer validators.
var (
	// OfferDurationValidator validates a subscription offer duration.
	OfferDurationValidator = stringvalidator.OneOf(models.ValidSubscriptionOfferDurations...)

	// OfferModeValidator validates a subscription offer mode.
	OfferModeValidator = stringvalidator.OneOf(models.ValidSubscriptionOfferModes...)

	// OfferCodeValidator validates a promotional offer code.
	//
	// Apple's schema types offerCode as a bare string. The pattern is the
	// product identifier's, which is what App Store Connect accepts for an
	// identifier, and it keeps "/" out so the composite import ID
	// "<subscription_id>/<offer_code>" is unambiguous.
	OfferCodeValidator = stringvalidator.RegexMatches(
		regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`),
		"Offer code must start with a letter or digit and contain only letters, digits, dots, underscores and hyphens",
	)
)

// GetOfferCodeValidator returns validators for a promotional offer code.
func GetOfferCodeValidator() []validator.String {
	return []validator.String{
		OfferCodeValidator,
		stringvalidator.UTF8LengthAtLeast(1),
	}
}

// promotionalOfferPricesToAPI converts the configured prices to client input,
// sorted by territory so the same set always produces the same request body.
func promotionalOfferPricesToAPI(prices []promotionalOfferPriceModel) []apple.SubscriptionPromotionalOfferPrice {
	converted := make([]apple.SubscriptionPromotionalOfferPrice, 0, len(prices))
	for _, price := range prices {
		converted = append(converted, apple.SubscriptionPromotionalOfferPrice{
			TerritoryID:  price.TerritoryID.ValueString(),
			PricePointID: price.PricePointID.ValueString(),
		})
	}

	sort.Slice(converted, func(i, j int) bool {
		return converted[i].TerritoryID < converted[j].TerritoryID
	})

	return converted
}

// reconcilePromotionalOfferPrices matches Apple's prices against state.
//
// An offer has one price per territory, so the match is by territory. A price
// state already holds is kept as it is, with one exception: when Apple reports
// a different price point for that territory, Apple's is taken, so a price
// changed in App Store Connect surfaces as drift. A price state holds with no
// price point -- a free-trial territory -- is kept as it is whatever Apple
// reports for it, since there is nothing configured to drift from.
//
// A territory Apple reports that state does not know is added with Apple's
// values, and a territory state holds that Apple no longer reports is dropped,
// so both directions of an out-of-band change show up in the next plan. Import
// passes no state and adopts everything.
func reconcilePromotionalOfferPrices(state []promotionalOfferPriceModel, reported []models.SubscriptionPromotionalOfferPrice) []promotionalOfferPriceModel {
	byTerritory := make(map[string]string, len(reported))
	for _, price := range reported {
		if price.Relationships == nil || price.Relationships.Territory == nil {
			continue
		}
		territory := price.Relationships.Territory.Data.ID
		if territory == "" {
			continue
		}
		pointID := ""
		if price.Relationships.SubscriptionPricePoint != nil {
			pointID = price.Relationships.SubscriptionPricePoint.Data.ID
		}
		byTerritory[territory] = pointID
	}

	reconciled := make([]promotionalOfferPriceModel, 0, len(byTerritory))
	for _, price := range state {
		territory := price.TerritoryID.ValueString()
		pointID, ok := byTerritory[territory]
		if !ok {
			continue
		}
		if !price.PricePointID.IsNull() && pointID != "" && pointID != price.PricePointID.ValueString() {
			price.PricePointID = types.StringValue(pointID)
		}
		reconciled = append(reconciled, price)
		delete(byTerritory, territory)
	}

	adopted := make([]string, 0, len(byTerritory))
	for territory := range byTerritory {
		adopted = append(adopted, territory)
	}
	sort.Strings(adopted)

	for _, territory := range adopted {
		price := promotionalOfferPriceModel{
			TerritoryID:  types.StringValue(territory),
			PricePointID: types.StringNull(),
		}
		if pointID := byTerritory[territory]; pointID != "" {
			price.PricePointID = types.StringValue(pointID)
		}
		reconciled = append(reconciled, price)
	}

	return reconciled
}

// promotionalOfferSummary converts one offer for the listing data source.
func promotionalOfferSummary(offer models.SubscriptionPromotionalOffer) subscriptionPromotionalOfferSummaryModel {
	summary := subscriptionPromotionalOfferSummaryModel{
		ID:                         types.StringValue(offer.ID),
		Name:                       types.StringValue(offer.Attributes.Name),
		OfferCode:                  types.StringValue(offer.Attributes.OfferCode),
		Duration:                   types.StringNull(),
		OfferMode:                  types.StringNull(),
		NumberOfPeriods:            types.Int64Null(),
		TargetSubscriptionPlanType: types.StringNull(),
	}
	if offer.Attributes.Duration != nil {
		summary.Duration = types.StringValue(string(*offer.Attributes.Duration))
	}
	if offer.Attributes.OfferMode != nil {
		summary.OfferMode = types.StringValue(string(*offer.Attributes.OfferMode))
	}
	if offer.Attributes.NumberOfPeriods != nil {
		summary.NumberOfPeriods = types.Int64Value(int64(*offer.Attributes.NumberOfPeriods))
	}
	if offer.Attributes.TargetSubscriptionPlanType != nil {
		summary.TargetSubscriptionPlanType = types.StringValue(string(*offer.Attributes.TargetSubscriptionPlanType))
	}
	return summary
}
