// Copyright (c) AO Studio
// SPDX-License-Identifier: MPL-2.0

package models

// --- Subscription introductory offers ---

// SubscriptionIntroductoryOffer is one introductory offer in one territory.
//
// Apple keeps one record per territory: an offer App Store Connect's UI shows
// as "a one-week free trial in 175 countries" is 175 of these, with identical
// attributes and a territory each. There is no GET for a single record -- only
// the collection under /v1/subscriptions/{id}/introductoryOffers.
type SubscriptionIntroductoryOffer struct {
	Type          string                                      `json:"type"`
	ID            string                                      `json:"id"`
	Attributes    SubscriptionIntroductoryOfferAttributes     `json:"attributes"`
	Relationships *SubscriptionIntroductoryOfferRelationships `json:"relationships,omitempty"`
	Links         *ResourceLinks                              `json:"links,omitempty"`
}

// SubscriptionIntroductoryOfferAttributes covers both what is sent at creation
// and what is returned.
//
// startDate and endDate are plain dates (YYYY-MM-DD). duration, offerMode and
// numberOfPeriods are the three members Apple requires on create, so they carry
// no omitempty: a missing one is sent as a visible zero value and refused by
// Apple, rather than silently dropped.
type SubscriptionIntroductoryOfferAttributes struct {
	StartDate                  *string                   `json:"startDate,omitempty"`
	EndDate                    *string                   `json:"endDate,omitempty"`
	Duration                   SubscriptionOfferDuration `json:"duration"`
	OfferMode                  SubscriptionOfferMode     `json:"offerMode"`
	NumberOfPeriods            int                       `json:"numberOfPeriods"`
	TargetSubscriptionPlanType *SubscriptionPlanType     `json:"targetSubscriptionPlanType,omitempty"`
}

// SubscriptionIntroductoryOfferRelationships is populated with data only for
// the relationships a request asks for through include.
type SubscriptionIntroductoryOfferRelationships struct {
	Subscription           *ResourceIdentifier `json:"subscription,omitempty"`
	Territory              *ResourceIdentifier `json:"territory,omitempty"`
	SubscriptionPricePoint *ResourceIdentifier `json:"subscriptionPricePoint,omitempty"`
}

// SubscriptionIntroductoryOffersUpdateRequest creates introductory offers in
// bulk.
//
// It is a PATCH of the subscription, not of the offers: Apple's
// SubscriptionUpdateRequest carries an introductoryOffers relationship and an
// "included" member accepting SubscriptionIntroductoryOfferInlineCreate, the
// same shape SubscriptionPricesUpdateRequest uses for prices. New offers travel
// inline under "${offer0}"-style placeholders, and the relationship lists both
// the placeholders and every offer the subscription already has -- see
// CreateSubscriptionIntroductoryOffers for why the existing ones are repeated.
//
// It is its own type rather than a field on SubscriptionUpdateRequest because
// that one always sends an attributes member, and an empty one is refused.
type SubscriptionIntroductoryOffersUpdateRequest struct {
	Data     SubscriptionIntroductoryOffersUpdateData    `json:"data"`
	Included []SubscriptionIntroductoryOfferInlineCreate `json:"included,omitempty"`
}

type SubscriptionIntroductoryOffersUpdateData struct {
	Type          string                                            `json:"type"`
	ID            string                                            `json:"id"`
	Relationships SubscriptionIntroductoryOffersUpdateRelationships `json:"relationships"`
}

type SubscriptionIntroductoryOffersUpdateRelationships struct {
	IntroductoryOffers ResourceIdentifiers `json:"introductoryOffers"`
}

// SubscriptionIntroductoryOfferInlineCreate is one new offer inside "included".
type SubscriptionIntroductoryOfferInlineCreate struct {
	Type          string                                                 `json:"type"`
	ID            string                                                 `json:"id"`
	Attributes    SubscriptionIntroductoryOfferAttributes                `json:"attributes"`
	Relationships SubscriptionIntroductoryOfferInlineCreateRelationships `json:"relationships"`
}

// SubscriptionIntroductoryOfferInlineCreateRelationships names the subscription
// and the territory, and the price point for a paid offer.
//
// The territory is always sent by this provider even though Apple marks it
// optional: a price point encodes its territory, but a free trial has no price
// point, and what Apple does with a free trial that names no territory is not
// documented.
type SubscriptionIntroductoryOfferInlineCreateRelationships struct {
	Subscription           ResourceIdentifier  `json:"subscription"`
	Territory              *ResourceIdentifier `json:"territory,omitempty"`
	SubscriptionPricePoint *ResourceIdentifier `json:"subscriptionPricePoint,omitempty"`
}

// SubscriptionIntroductoryOfferUpdateRequest is Apple's whole update request:
// endDate is the only member. Everything else about an offer is fixed once it
// exists.
type SubscriptionIntroductoryOfferUpdateRequest struct {
	Type       string                                        `json:"type"`
	ID         string                                        `json:"id"`
	Attributes SubscriptionIntroductoryOfferUpdateAttributes `json:"attributes"`
}

// SubscriptionIntroductoryOfferUpdateAttributes deliberately has no omitempty.
//
// Apple marks endDate nullable, and an explicit null is how an end date comes
// off an offer. Omitting the member would leave the old end date in place, the
// same trap NullableResourceIdentifier exists for on relationships.
type SubscriptionIntroductoryOfferUpdateAttributes struct {
	EndDate *string `json:"endDate"`
}
