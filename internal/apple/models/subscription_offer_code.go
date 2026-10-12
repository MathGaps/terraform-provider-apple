// Copyright (c) AO Studio
// SPDX-License-Identifier: MPL-2.0

package models

// --- BEGIN copy of the shared offer enums from feat/subscription-promotional-offer ---
// Identical to internal/apple/models/subscription_promotional_offer.go on that
// branch, which lands first; delete this block when rebasing onto it.

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

// --- END copy of the shared offer enums ---

// --- Subscription offer codes ---
//
// An offer code is the App Store's nearest thing to a coupon: a discount or a
// free period on one subscription, redeemed by a code a customer types into
// the App Store or that an app presents through StoreKit. Three Apple records
// make one up -- the offer itself (subscriptionOfferCodes), and two kinds of
// code that redeem it: a custom code, one string many people share
// (subscriptionOfferCodeCustomCodes), and a batch of one-time-use codes Apple
// generates (subscriptionOfferCodeOneTimeUseCodes).
//
// None of the three has a DELETE in App Store Connect API 4.5.1. Each has a
// PATCH whose only member is "active", and App Store Connect's help says a
// deactivated offer "can't be reactivated later". Everything created here
// stays at Apple for good.

// ValidSubscriptionCustomerEligibilities lists who an offer code may be
// redeemed by: customers who never subscribed, current subscribers, and
// lapsed ones (Apple's SubscriptionCustomerEligibility).
var ValidSubscriptionCustomerEligibilities = []string{"NEW", "EXISTING", "EXPIRED"}

// ValidSubscriptionOfferCodeEligibilities lists how an offer code combines
// with an introductory offer (Apple's SubscriptionOfferEligibility).
var ValidSubscriptionOfferCodeEligibilities = []string{"STACK_WITH_INTRO_OFFERS", "REPLACE_INTRO_OFFERS"}

// ValidOfferCodeEnvironments lists where a batch of one-time-use codes
// redeems (Apple's OfferCodeEnvironment).
var ValidOfferCodeEnvironments = []string{"PRODUCTION", "SANDBOX"}

// Offer code environments.
const (
	OfferCodeEnvironmentProduction = "PRODUCTION"
	OfferCodeEnvironmentSandbox    = "SANDBOX"
)

// SubscriptionOfferCode is the offer: what the discount is, who may redeem it
// and on which subscription. It carries no codes itself.
type SubscriptionOfferCode struct {
	Type          string                              `json:"type"`
	ID            string                              `json:"id"`
	Attributes    SubscriptionOfferCodeAttributes     `json:"attributes"`
	Relationships *SubscriptionOfferCodeRelationships `json:"relationships,omitempty"`
	Links         *ResourceLinks                      `json:"links,omitempty"`
}

// SubscriptionOfferCodeAttributes is what Apple reports. The three counts and
// "active" are Apple's; everything else is what the create request sent.
type SubscriptionOfferCodeAttributes struct {
	Name                       string   `json:"name,omitempty"`
	CustomerEligibilities      []string `json:"customerEligibilities,omitempty"`
	OfferEligibility           string   `json:"offerEligibility,omitempty"`
	Duration                   string   `json:"duration,omitempty"`
	OfferMode                  string   `json:"offerMode,omitempty"`
	NumberOfPeriods            *int     `json:"numberOfPeriods,omitempty"`
	TotalNumberOfCodes         *int     `json:"totalNumberOfCodes,omitempty"`
	ProductionCodeCount        *int     `json:"productionCodeCount,omitempty"`
	SandboxCodeCount           *int     `json:"sandboxCodeCount,omitempty"`
	Active                     *bool    `json:"active,omitempty"`
	AutoRenewEnabled           *bool    `json:"autoRenewEnabled,omitempty"`
	TargetSubscriptionPlanType *string  `json:"targetSubscriptionPlanType,omitempty"`
}

// SubscriptionOfferCodeRelationships holds the subscription linkage, which
// Apple populates only when the request asks for include=subscription.
type SubscriptionOfferCodeRelationships struct {
	Subscription *ResourceIdentifier `json:"subscription,omitempty"`
}

// SubscriptionOfferCodeCreateRequest is the whole create document: the offer
// in "data" and its prices inline in "included", because the prices do not
// exist yet and Apple publishes no endpoint to create one on its own.
type SubscriptionOfferCodeCreateRequest struct {
	Data     SubscriptionOfferCodeCreateData          `json:"data"`
	Included []SubscriptionOfferCodePriceInlineCreate `json:"included,omitempty"`
}

type SubscriptionOfferCodeCreateData struct {
	Type          string                                   `json:"type"`
	Attributes    SubscriptionOfferCodeCreateAttributes    `json:"attributes"`
	Relationships SubscriptionOfferCodeCreateRelationships `json:"relationships"`
}

// SubscriptionOfferCodeCreateAttributes are the attributes Apple accepts at
// creation, and the only time it accepts them: the update request carries
// "active" alone. name, customerEligibilities, offerEligibility, duration,
// offerMode and numberOfPeriods are required; the other two are nullable.
type SubscriptionOfferCodeCreateAttributes struct {
	Name                       string   `json:"name"`
	CustomerEligibilities      []string `json:"customerEligibilities"`
	OfferEligibility           string   `json:"offerEligibility"`
	Duration                   string   `json:"duration"`
	OfferMode                  string   `json:"offerMode"`
	NumberOfPeriods            int      `json:"numberOfPeriods"`
	AutoRenewEnabled           *bool    `json:"autoRenewEnabled,omitempty"`
	TargetSubscriptionPlanType *string  `json:"targetSubscriptionPlanType,omitempty"`
}

// SubscriptionOfferCodeCreateRelationships names the subscription and the
// placeholder IDs of the inline prices. Both are required.
type SubscriptionOfferCodeCreateRelationships struct {
	Subscription ResourceIdentifier  `json:"subscription"`
	Prices       ResourceIdentifiers `json:"prices"`
}

// SubscriptionOfferCodePriceInlineCreate is one price inside "included".
//
// Unlike SubscriptionPriceInlineCreate it has no attributes member and no
// relationship back to its parent -- Apple's schema carries only the territory
// and the price point, and marks neither required.
type SubscriptionOfferCodePriceInlineCreate struct {
	Type          string                                          `json:"type"`
	ID            string                                          `json:"id"`
	Relationships SubscriptionOfferCodePriceInlineCreateRelations `json:"relationships"`
}

// SubscriptionOfferCodePriceInlineCreateRelations names the territory and,
// for a paid offer, the price point. A FREE_TRIAL has no price to point at,
// so the price point is omitted rather than sent empty.
type SubscriptionOfferCodePriceInlineCreateRelations struct {
	Territory              ResourceIdentifier  `json:"territory"`
	SubscriptionPricePoint *ResourceIdentifier `json:"subscriptionPricePoint,omitempty"`
}

// SubscriptionOfferCodeActiveUpdateRequest is the update request shared by
// all three offer code records: Apple's update request for each carries
// "active" and nothing else.
type SubscriptionOfferCodeActiveUpdateRequest struct {
	Type       string                          `json:"type"`
	ID         string                          `json:"id"`
	Attributes SubscriptionOfferCodeActiveOnly `json:"attributes"`
}

// SubscriptionOfferCodeActiveOnly always sends "active", since false is the
// value that matters and omitempty would drop it.
type SubscriptionOfferCodeActiveOnly struct {
	Active bool `json:"active"`
}

// SubscriptionOfferCodePrice is one territory's price for an offer.
type SubscriptionOfferCodePrice struct {
	Type          string                                   `json:"type"`
	ID            string                                   `json:"id"`
	Relationships *SubscriptionOfferCodePriceRelationships `json:"relationships,omitempty"`
}

type SubscriptionOfferCodePriceRelationships struct {
	Territory              *ResourceIdentifier `json:"territory,omitempty"`
	SubscriptionPricePoint *ResourceIdentifier `json:"subscriptionPricePoint,omitempty"`
}

// --- Custom codes ---

// SubscriptionOfferCodeCustomCode is one shared code string -- "SCHOOL2026" --
// with a cap on how many times it may be redeemed.
type SubscriptionOfferCodeCustomCode struct {
	Type          string                                       `json:"type"`
	ID            string                                       `json:"id"`
	Attributes    SubscriptionOfferCodeCustomCodeAttributes    `json:"attributes"`
	Relationships *SubscriptionOfferCodeChildCodeRelationships `json:"relationships,omitempty"`
	Links         *ResourceLinks                               `json:"links,omitempty"`
}

type SubscriptionOfferCodeCustomCodeAttributes struct {
	CustomCode     string  `json:"customCode,omitempty"`
	NumberOfCodes  *int    `json:"numberOfCodes,omitempty"`
	CreatedDate    *string `json:"createdDate,omitempty"`
	ExpirationDate *string `json:"expirationDate,omitempty"`
	Active         *bool   `json:"active,omitempty"`
}

// SubscriptionOfferCodeChildCodeRelationships holds the offer linkage of a
// custom code or a one-time-use batch, populated with include=offerCode.
type SubscriptionOfferCodeChildCodeRelationships struct {
	OfferCode *ResourceIdentifier `json:"offerCode,omitempty"`
}

type SubscriptionOfferCodeCustomCodeCreateRequest struct {
	Type          string                                     `json:"type"`
	Attributes    SubscriptionOfferCodeCustomCodeCreateAttrs `json:"attributes"`
	Relationships SubscriptionOfferCodeChildCreateRelations  `json:"relationships"`
}

// SubscriptionOfferCodeCustomCodeCreateAttrs: customCode and numberOfCodes are
// required; expirationDate is a nullable plain date, and a code without one
// does not expire.
type SubscriptionOfferCodeCustomCodeCreateAttrs struct {
	CustomCode     string  `json:"customCode"`
	NumberOfCodes  int     `json:"numberOfCodes"`
	ExpirationDate *string `json:"expirationDate,omitempty"`
}

// SubscriptionOfferCodeChildCreateRelations names the offer a custom code or a
// one-time-use batch redeems.
type SubscriptionOfferCodeChildCreateRelations struct {
	OfferCode ResourceIdentifier `json:"offerCode"`
}

// --- One-time-use codes ---

// SubscriptionOfferCodeOneTimeUseCode is a batch of codes Apple generates,
// each redeemable once. The codes themselves are not attributes: they are a
// CSV document under /v1/subscriptionOfferCodeOneTimeUseCodes/{id}/values.
type SubscriptionOfferCodeOneTimeUseCode struct {
	Type          string                                        `json:"type"`
	ID            string                                        `json:"id"`
	Attributes    SubscriptionOfferCodeOneTimeUseCodeAttributes `json:"attributes"`
	Relationships *SubscriptionOfferCodeChildCodeRelationships  `json:"relationships,omitempty"`
	Links         *ResourceLinks                                `json:"links,omitempty"`
}

type SubscriptionOfferCodeOneTimeUseCodeAttributes struct {
	NumberOfCodes  *int    `json:"numberOfCodes,omitempty"`
	CreatedDate    *string `json:"createdDate,omitempty"`
	ExpirationDate *string `json:"expirationDate,omitempty"`
	Active         *bool   `json:"active,omitempty"`
	Environment    *string `json:"environment,omitempty"`
}

type SubscriptionOfferCodeOneTimeUseCodeCreateRequest struct {
	Type          string                                         `json:"type"`
	Attributes    SubscriptionOfferCodeOneTimeUseCodeCreateAttrs `json:"attributes"`
	Relationships SubscriptionOfferCodeChildCreateRelations      `json:"relationships"`
}

// SubscriptionOfferCodeOneTimeUseCodeCreateAttrs: numberOfCodes and
// expirationDate are required here, unlike on a custom code; environment is
// nullable.
type SubscriptionOfferCodeOneTimeUseCodeCreateAttrs struct {
	NumberOfCodes  int     `json:"numberOfCodes"`
	ExpirationDate string  `json:"expirationDate"`
	Environment    *string `json:"environment,omitempty"`
}
