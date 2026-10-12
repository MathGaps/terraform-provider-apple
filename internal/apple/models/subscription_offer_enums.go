// Copyright (c) AO Studio
// SPDX-License-Identifier: MPL-2.0

package models

// TEMPORARY COPY -- delete this whole file when rebasing onto a main that
// carries feat/subscription-promotional-offer, which defines these same
// identifiers, verbatim, in subscription_promotional_offer.go. Kept in a file
// of its own so that is a one-file deletion rather than a merge.

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
