// Copyright (c) AO Studio
// SPDX-License-Identifier: MPL-2.0

package models

// Subscription offer duration enumeration.
//
// Apple's SubscriptionOfferDuration, shared by every subscription offer kind.
// It is a wider set than SubscriptionPeriod: an offer can last three days or
// two weeks, which no subscription period can.
type SubscriptionOfferDuration string

const (
	SubscriptionOfferDurationThreeDays   SubscriptionOfferDuration = "THREE_DAYS"
	SubscriptionOfferDurationOneWeek     SubscriptionOfferDuration = "ONE_WEEK"
	SubscriptionOfferDurationTwoWeeks    SubscriptionOfferDuration = "TWO_WEEKS"
	SubscriptionOfferDurationOneMonth    SubscriptionOfferDuration = "ONE_MONTH"
	SubscriptionOfferDurationTwoMonths   SubscriptionOfferDuration = "TWO_MONTHS"
	SubscriptionOfferDurationThreeMonths SubscriptionOfferDuration = "THREE_MONTHS"
	SubscriptionOfferDurationSixMonths   SubscriptionOfferDuration = "SIX_MONTHS"
	SubscriptionOfferDurationOneYear     SubscriptionOfferDuration = "ONE_YEAR"
)

// ValidSubscriptionOfferDurations lists the accepted offer durations.
var ValidSubscriptionOfferDurations = []string{
	string(SubscriptionOfferDurationThreeDays),
	string(SubscriptionOfferDurationOneWeek),
	string(SubscriptionOfferDurationTwoWeeks),
	string(SubscriptionOfferDurationOneMonth),
	string(SubscriptionOfferDurationTwoMonths),
	string(SubscriptionOfferDurationThreeMonths),
	string(SubscriptionOfferDurationSixMonths),
	string(SubscriptionOfferDurationOneYear),
}

// Subscription offer mode enumeration.
//
// PAY_AS_YOU_GO discounts each of numberOfPeriods billing periods,
// PAY_UP_FRONT charges one discounted price for the whole duration, and
// FREE_TRIAL charges nothing for it.
type SubscriptionOfferMode string

const (
	SubscriptionOfferModePayAsYouGo SubscriptionOfferMode = "PAY_AS_YOU_GO"
	SubscriptionOfferModePayUpFront SubscriptionOfferMode = "PAY_UP_FRONT"
	SubscriptionOfferModeFreeTrial  SubscriptionOfferMode = "FREE_TRIAL"
)

// ValidSubscriptionOfferModes lists the accepted offer modes.
var ValidSubscriptionOfferModes = []string{
	string(SubscriptionOfferModePayAsYouGo),
	string(SubscriptionOfferModePayUpFront),
	string(SubscriptionOfferModeFreeTrial),
}

// --- Subscription promotional offers ---

// SubscriptionPromotionalOffer resource model.
//
// A promotional offer is a discount an app presents to a customer who has (or
// had) the subscription. The app asks the App Store for it by offerCode, with a
// signature its server computes, so offerCode is the value the app's code and
// this record have to agree on.
type SubscriptionPromotionalOffer struct {
	Type          string                                     `json:"type"`
	ID            string                                     `json:"id"`
	Attributes    SubscriptionPromotionalOfferAttributes     `json:"attributes"`
	Relationships *SubscriptionPromotionalOfferRelationships `json:"relationships,omitempty"`
	Links         *ResourceLinks                             `json:"links,omitempty"`
}

// SubscriptionPromotionalOfferAttributes covers what is sent at creation and
// what is returned. None of these can be changed afterwards: Apple's update
// request carries the prices relationship and nothing else.
type SubscriptionPromotionalOfferAttributes struct {
	Duration                   *SubscriptionOfferDuration `json:"duration,omitempty"`
	Name                       string                     `json:"name,omitempty"`
	NumberOfPeriods            *int                       `json:"numberOfPeriods,omitempty"`
	OfferCode                  string                     `json:"offerCode,omitempty"`
	OfferMode                  *SubscriptionOfferMode     `json:"offerMode,omitempty"`
	TargetSubscriptionPlanType *SubscriptionPlanType      `json:"targetSubscriptionPlanType,omitempty"`
}

// SubscriptionPromotionalOfferRelationships holds the subscription linkage.
//
// Apple populates "data" only when the request asks for it with
// include=subscription; otherwise the relationship carries links alone. The
// prices relationship is not modelled here: GET accepts include=prices but caps
// it at limit[prices]=50, so the prices are read from their own paginated
// collection instead.
type SubscriptionPromotionalOfferRelationships struct {
	Subscription *ResourceIdentifier `json:"subscription,omitempty"`
}

// SubscriptionPromotionalOfferCreateRequest is the whole create document.
//
// It is a top-level type rather than a Request[T] because JSON:API's
// "included" member sits beside "data": the prices do not exist yet, so each
// travels inline under a placeholder ID ("${price0}") that the prices
// relationship references, the shape SubscriptionPricesUpdateRequest uses.
type SubscriptionPromotionalOfferCreateRequest struct {
	Data     SubscriptionPromotionalOfferCreateData          `json:"data"`
	Included []SubscriptionPromotionalOfferPriceInlineCreate `json:"included,omitempty"`
}

type SubscriptionPromotionalOfferCreateData struct {
	Type          string                                          `json:"type"`
	Attributes    SubscriptionPromotionalOfferCreateAttributes    `json:"attributes"`
	Relationships SubscriptionPromotionalOfferCreateRelationships `json:"relationships"`
}

// SubscriptionPromotionalOfferCreateAttributes are the attributes accepted at
// creation. Apple requires all but targetSubscriptionPlanType, which is
// nullable and omitted when unset.
type SubscriptionPromotionalOfferCreateAttributes struct {
	Duration                   SubscriptionOfferDuration `json:"duration"`
	Name                       string                    `json:"name"`
	NumberOfPeriods            int                       `json:"numberOfPeriods"`
	OfferCode                  string                    `json:"offerCode"`
	OfferMode                  SubscriptionOfferMode     `json:"offerMode"`
	TargetSubscriptionPlanType *SubscriptionPlanType     `json:"targetSubscriptionPlanType,omitempty"`
}

// SubscriptionPromotionalOfferCreateRelationships names the subscription and
// the prices. Apple requires both, prices included even for a free trial.
type SubscriptionPromotionalOfferCreateRelationships struct {
	Subscription ResourceIdentifier  `json:"subscription"`
	Prices       ResourceIdentifiers `json:"prices"`
}

// SubscriptionPromotionalOfferUpdateRequest is the whole update document.
//
// Apple's SubscriptionPromotionalOfferUpdateRequest has no attributes member
// at all -- only the prices relationship and the inline prices it references --
// so every attribute of the offer is fixed at creation.
type SubscriptionPromotionalOfferUpdateRequest struct {
	Data     SubscriptionPromotionalOfferUpdateData          `json:"data"`
	Included []SubscriptionPromotionalOfferPriceInlineCreate `json:"included,omitempty"`
}

type SubscriptionPromotionalOfferUpdateData struct {
	Type          string                                          `json:"type"`
	ID            string                                          `json:"id"`
	Relationships SubscriptionPromotionalOfferUpdateRelationships `json:"relationships"`
}

type SubscriptionPromotionalOfferUpdateRelationships struct {
	Prices ResourceIdentifiers `json:"prices"`
}

// --- Subscription promotional offer prices ---

// SubscriptionPromotionalOfferPrice is one territory's price for an offer.
//
// It has no attributes: the price is the price point it references, in the
// territory it names. Both linkages are populated only when the read asks for
// include=territory,subscriptionPricePoint.
type SubscriptionPromotionalOfferPrice struct {
	Type          string                                          `json:"type"`
	ID            string                                          `json:"id"`
	Relationships *SubscriptionPromotionalOfferPriceRelationships `json:"relationships,omitempty"`
	Links         *ResourceLinks                                  `json:"links,omitempty"`
}

type SubscriptionPromotionalOfferPriceRelationships struct {
	Territory              *ResourceIdentifier `json:"territory,omitempty"`
	SubscriptionPricePoint *ResourceIdentifier `json:"subscriptionPricePoint,omitempty"`
}

// SubscriptionPromotionalOfferPriceInlineCreate is one price inside the
// "included" array of a create or an update.
//
// Unlike SubscriptionPriceInlineCreate it does not repeat the parent: Apple's
// schema names only the territory and the price point. Both are optional in
// the schema; the price point is omitted for a territory that carries no
// price, which is what a free trial is.
type SubscriptionPromotionalOfferPriceInlineCreate struct {
	Type          string                                               `json:"type"`
	ID            string                                               `json:"id"`
	Relationships SubscriptionPromotionalOfferPriceInlineRelationships `json:"relationships"`
}

type SubscriptionPromotionalOfferPriceInlineRelationships struct {
	Territory              *ResourceIdentifier `json:"territory,omitempty"`
	SubscriptionPricePoint *ResourceIdentifier `json:"subscriptionPricePoint,omitempty"`
}
