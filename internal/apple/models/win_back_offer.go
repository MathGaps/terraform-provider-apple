// Copyright (c) AO Studio
// SPDX-License-Identifier: MPL-2.0

package models

import "encoding/json"

// --- Win-back offers ---

// Win-back offer enumerations.
//
// Duration and offer mode are Apple's shared SubscriptionOfferDuration and
// SubscriptionOfferMode (subscription_offer_enums.go); priority and
// promotionIntent are inline enums on WinBackOffer itself, kept as plain
// strings validated against these lists.
var (
	// ValidWinBackOfferPriorities is WinBackOffer.attributes.priority.
	ValidWinBackOfferPriorities = []string{"HIGH", "NORMAL"}

	// ValidWinBackOfferPromotionIntents is WinBackOffer.attributes.promotionIntent.
	ValidWinBackOfferPromotionIntents = []string{"NOT_PROMOTED", "USE_AUTO_GENERATED_ASSETS"}
)

// IntegerRange is Apple's IntegerRange schema: an inclusive minimum and
// maximum. A win-back offer uses it for how long ago a customer must have
// lapsed to be eligible.
type IntegerRange struct {
	Minimum *int `json:"minimum,omitempty"`
	Maximum *int `json:"maximum,omitempty"`
}

// WinBackOffer is an offer App Store shows to customers whose subscription has
// lapsed.
//
// Unlike every other record under a subscription it has **no subscription
// relationship** on the read side: WinBackOffer's relationships carry only
// "prices", and GET /v1/winBackOffers/{id} accepts include=prices and nothing
// else. The owning subscription therefore cannot be read back from an offer.
type WinBackOffer struct {
	Type          string                     `json:"type"`
	ID            string                     `json:"id"`
	Attributes    WinBackOfferAttributes     `json:"attributes"`
	Relationships *WinBackOfferRelationships `json:"relationships,omitempty"`
	Links         *ResourceLinks             `json:"links,omitempty"`
}

// WinBackOfferAttributes is what Apple reports for an offer, and what the
// create request sends: the two share every member, and every member the
// create request requires is a plain value here.
type WinBackOfferAttributes struct {
	ReferenceName                                       string                    `json:"referenceName"`
	OfferID                                             string                    `json:"offerId"`
	Duration                                            SubscriptionOfferDuration `json:"duration"`
	OfferMode                                           SubscriptionOfferMode     `json:"offerMode"`
	PeriodCount                                         int                       `json:"periodCount"`
	CustomerEligibilityPaidSubscriptionDurationInMonths int                       `json:"customerEligibilityPaidSubscriptionDurationInMonths"`
	CustomerEligibilityTimeSinceLastSubscribedInMonths  *IntegerRange             `json:"customerEligibilityTimeSinceLastSubscribedInMonths,omitempty"`
	CustomerEligibilityWaitBetweenOffersInMonths        *int                      `json:"customerEligibilityWaitBetweenOffersInMonths,omitempty"`
	StartDate                                           string                    `json:"startDate"`
	EndDate                                             *string                   `json:"endDate,omitempty"`
	Priority                                            string                    `json:"priority"`
	PromotionIntent                                     *string                   `json:"promotionIntent,omitempty"`
	TargetSubscriptionPlanType                          *string                   `json:"targetSubscriptionPlanType,omitempty"`
}

type WinBackOfferRelationships struct {
	Prices *ResourceIdentifiers `json:"prices,omitempty"`
}

// WinBackOfferCreateRequest is the whole create document.
//
// It is not wrapped in Request[T] because, like the price schedules, it carries
// a top-level "included" member beside "data": the prices do not exist yet, so
// each travels inline under a placeholder ID that the "prices" relationship
// references.
type WinBackOfferCreateRequest struct {
	Data     WinBackOfferCreateData          `json:"data"`
	Included []WinBackOfferPriceInlineCreate `json:"included,omitempty"`
}

type WinBackOfferCreateData struct {
	Type          string                          `json:"type"`
	Attributes    WinBackOfferAttributes          `json:"attributes"`
	Relationships WinBackOfferCreateRelationships `json:"relationships"`
}

// WinBackOfferCreateRelationships names the subscription and the placeholder
// prices. Both are required by Apple's WinBackOfferCreateRequest.
type WinBackOfferCreateRelationships struct {
	Subscription ResourceIdentifier  `json:"subscription"`
	Prices       ResourceIdentifiers `json:"prices"`
}

// WinBackOfferPriceInlineCreate is one price inside the "included" array.
//
// **Apple's OpenAPI schema for this type (4.5.1) declares only "type" and
// "id"** -- no attributes and no relationships, which would leave a price with
// no price point. Apple's own "Create a Win-Back Offer" example request sends a
// "subscriptionPricePoint" relationship on each inline price and nothing else,
// and WinBackOfferPrice on the read side carries exactly "territory" and
// "subscriptionPricePoint". This follows the example: the price point is
// always sent, the territory only when the caller names one, since a price
// point ID already encodes its territory. Unlike SubscriptionPriceInlineCreate
// there is no relationship repeating the parent.
type WinBackOfferPriceInlineCreate struct {
	Type          string                               `json:"type"`
	ID            string                               `json:"id"`
	Relationships WinBackOfferPriceInlineRelationships `json:"relationships"`
}

type WinBackOfferPriceInlineRelationships struct {
	SubscriptionPricePoint ResourceIdentifier  `json:"subscriptionPricePoint"`
	Territory              *ResourceIdentifier `json:"territory,omitempty"`
}

// WinBackOfferPrice is one price of an existing offer, as listed under
// /v1/winBackOffers/{id}/prices. It has no attributes: the price is entirely
// the price point it references.
type WinBackOfferPrice struct {
	Type          string                          `json:"type"`
	ID            string                          `json:"id"`
	Relationships *WinBackOfferPriceRelationships `json:"relationships,omitempty"`
}

type WinBackOfferPriceRelationships struct {
	Territory              *ResourceIdentifier `json:"territory,omitempty"`
	SubscriptionPricePoint *ResourceIdentifier `json:"subscriptionPricePoint,omitempty"`
}

// WinBackOfferUpdateRequest is the PATCH document. It has no relationships and
// no "included": Apple's WinBackOfferUpdateRequest cannot change the prices,
// nor the reference name, offer ID, duration, mode, period count or plan type.
type WinBackOfferUpdateRequest struct {
	Type       string                       `json:"type"`
	ID         string                       `json:"id"`
	Attributes WinBackOfferUpdateAttributes `json:"attributes"`
}

// WinBackOfferUpdateAttributes holds the seven members Apple's update request
// accepts.
//
// A nil pointer is omitted, which leaves Apple's value alone. That cannot
// express removing an end date or a wait between offers -- omitting the member
// keeps the old value -- so ClearEndDate and ClearWaitBetweenOffers send an
// explicit null instead, which the spec marks both members as accepting.
// MarshalJSON assembles the member set, since omitempty cannot tell "unset"
// from "set to null".
type WinBackOfferUpdateAttributes struct {
	CustomerEligibilityPaidSubscriptionDurationInMonths *int
	CustomerEligibilityTimeSinceLastSubscribedInMonths  *IntegerRange
	CustomerEligibilityWaitBetweenOffersInMonths        *int
	ClearWaitBetweenOffers                              bool
	StartDate                                           *string
	EndDate                                             *string
	ClearEndDate                                        bool
	Priority                                            *string
	PromotionIntent                                     *string
}

// MarshalJSON writes only the members that are set, and an explicit null for
// a member being cleared.
func (a WinBackOfferUpdateAttributes) MarshalJSON() ([]byte, error) {
	members := map[string]any{}

	if a.CustomerEligibilityPaidSubscriptionDurationInMonths != nil {
		members["customerEligibilityPaidSubscriptionDurationInMonths"] = *a.CustomerEligibilityPaidSubscriptionDurationInMonths
	}
	if a.CustomerEligibilityTimeSinceLastSubscribedInMonths != nil {
		members["customerEligibilityTimeSinceLastSubscribedInMonths"] = a.CustomerEligibilityTimeSinceLastSubscribedInMonths
	}
	if a.CustomerEligibilityWaitBetweenOffersInMonths != nil {
		members["customerEligibilityWaitBetweenOffersInMonths"] = *a.CustomerEligibilityWaitBetweenOffersInMonths
	} else if a.ClearWaitBetweenOffers {
		members["customerEligibilityWaitBetweenOffersInMonths"] = nil
	}
	if a.StartDate != nil {
		members["startDate"] = *a.StartDate
	}
	if a.EndDate != nil {
		members["endDate"] = *a.EndDate
	} else if a.ClearEndDate {
		members["endDate"] = nil
	}
	if a.Priority != nil {
		members["priority"] = *a.Priority
	}
	if a.PromotionIntent != nil {
		members["promotionIntent"] = *a.PromotionIntent
	}

	return json.Marshal(members)
}
