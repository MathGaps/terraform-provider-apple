// Copyright (c) AO Studio
// SPDX-License-Identifier: MPL-2.0

package apple

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/AhmedOsman00/terraform-provider-apple/internal/apple/models"
)

// GetSubscriptionIntroductoryOffers lists a subscription's introductory offers,
// optionally narrowed to some territories.
//
// Apple publishes no GET for a single offer -- /v1/subscriptionIntroductoryOffers/{id}
// answers PATCH and DELETE only -- so this collection is the only read there
// is, and the subscription has to be known before an offer can be found.
// include is what makes a returned offer usable: without it Apple reports the
// territory and price point as links alone, and an offer whose territory is
// unknown cannot be matched to configuration. The territory filter is applied
// by Apple.
func (c *Client) GetSubscriptionIntroductoryOffers(subscriptionID string, territories []string) ([]models.SubscriptionIntroductoryOffer, error) {
	params := url.Values{}
	params.Set("include", "territory,subscriptionPricePoint")
	if len(territories) > 0 {
		params.Set("filter[territory]", strings.Join(territories, ","))
	}

	return getAllPagesQuery[models.SubscriptionIntroductoryOffer](
		c,
		fmt.Sprintf("/v1/subscriptions/%s/introductoryOffers", subscriptionID),
		defaultPageSize,
		params,
	)
}

// GetSubscriptionIntroductoryOfferIDs lists the IDs of every introductory offer
// a subscription holds, through the relationship endpoint, which returns
// linkages alone and so is the cheapest complete answer.
func (c *Client) GetSubscriptionIntroductoryOfferIDs(subscriptionID string) ([]string, error) {
	linkages, err := getAllPages[models.ResourceData](
		c,
		fmt.Sprintf("/v1/subscriptions/%s/relationships/introductoryOffers", subscriptionID),
		defaultPageSize,
	)
	if err != nil {
		return nil, err
	}

	ids := make([]string, 0, len(linkages))
	for _, linkage := range linkages {
		ids = append(ids, linkage.ID)
	}

	return ids, nil
}

// SubscriptionIntroductoryOfferTerritory is one territory of an offer being
// created, and the price point it costs there.
//
// It is an input helper rather than a wire type. PricePointID is empty for a
// free trial, which has no price.
type SubscriptionIntroductoryOfferTerritory struct {
	TerritoryID  string
	PricePointID string
}

// CreateSubscriptionIntroductoryOffers creates one introductory offer per
// territory, all sharing attrs, in a single request, and returns the new offers
// keyed by territory.
//
// POST /v1/subscriptionIntroductoryOffers creates one record in one territory,
// so offering a free trial everywhere through it is 175 requests, and an apply
// that fails part-way leaves offers Terraform never recorded -- which the next
// apply then collides with, because a territory holds one introductory offer
// at a time. The bulk form is on the subscription instead, as it is for
// prices: PATCH /v1/subscriptions/{id} with an introductoryOffers relationship
// whose new members travel inline in "included". Apple commits it or refuses
// it whole.
//
// The relationship repeats every offer the subscription already has. Apple's
// specification does not say whether a to-many relationship in a PATCH
// replaces the set or adds to it, and JSON:API, whose shape the request takes,
// says it replaces. Sending only the new offers would therefore risk deleting
// every other offer on the subscription -- including ones another
// apple_subscription_introductory_offer, or App Store Connect's own UI, put
// there. Naming them all is correct under either reading. The cost is a read
// before the write, and a race with any other writer between the two, which is
// why the provider serializes these writes per subscription.
//
// The same read is what identifies the new offers afterwards. Apple's response
// is the subscription, not the offers, so the offers are listed again and the
// ones whose IDs were not there before are the ones this call created.
func (c *Client) CreateSubscriptionIntroductoryOffers(
	subscriptionID string,
	attrs models.SubscriptionIntroductoryOfferAttributes,
	territories []SubscriptionIntroductoryOfferTerritory,
	authToken *string,
) (map[string]models.SubscriptionIntroductoryOffer, error) {
	existingIDs, err := c.GetSubscriptionIntroductoryOfferIDs(subscriptionID)
	if err != nil {
		return nil, fmt.Errorf("listing the subscription's existing introductory offers: %w", err)
	}

	existing := make(map[string]bool, len(existingIDs))
	linkages := make([]models.ResourceData, 0, len(existingIDs)+len(territories))
	for _, id := range existingIDs {
		existing[id] = true
		linkages = append(linkages, models.ResourceData{Type: "subscriptionIntroductoryOffers", ID: id})
	}

	subscriptionRef := models.ResourceIdentifier{
		Data: models.ResourceData{Type: "subscriptions", ID: subscriptionID},
	}

	included := make([]models.SubscriptionIntroductoryOfferInlineCreate, 0, len(territories))
	territoryIDs := make([]string, 0, len(territories))

	for i, territory := range territories {
		placeholderID := fmt.Sprintf("${offer%d}", i)

		linkages = append(linkages, models.ResourceData{Type: "subscriptionIntroductoryOffers", ID: placeholderID})

		relationships := models.SubscriptionIntroductoryOfferInlineCreateRelationships{
			Subscription: subscriptionRef,
			Territory: &models.ResourceIdentifier{
				Data: models.ResourceData{Type: "territories", ID: territory.TerritoryID},
			},
		}
		if territory.PricePointID != "" {
			relationships.SubscriptionPricePoint = &models.ResourceIdentifier{
				Data: models.ResourceData{Type: "subscriptionPricePoints", ID: territory.PricePointID},
			}
		}

		included = append(included, models.SubscriptionIntroductoryOfferInlineCreate{
			Type:          "subscriptionIntroductoryOffers",
			ID:            placeholderID,
			Attributes:    attrs,
			Relationships: relationships,
		})
		territoryIDs = append(territoryIDs, territory.TerritoryID)
	}

	requestData := models.SubscriptionIntroductoryOffersUpdateRequest{
		Data: models.SubscriptionIntroductoryOffersUpdateData{
			Type: "subscriptions",
			ID:   subscriptionID,
			Relationships: models.SubscriptionIntroductoryOffersUpdateRelationships{
				IntroductoryOffers: models.ResourceIdentifiers{Data: linkages},
			},
		},
		Included: included,
	}

	rb, err := json.Marshal(requestData)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("PATCH", fmt.Sprintf("%s/v1/subscriptions/%s", c.HostURL, subscriptionID), strings.NewReader(string(rb)))
	if err != nil {
		return nil, err
	}

	if _, err := c.doRequest(req, authToken); err != nil {
		return nil, err
	}

	offers, err := c.GetSubscriptionIntroductoryOffers(subscriptionID, territoryIDs)
	if err != nil {
		return nil, fmt.Errorf("the introductory offers were accepted, but reading them back failed: %w", err)
	}

	created := make(map[string]models.SubscriptionIntroductoryOffer, len(territories))
	for _, offer := range offers {
		if existing[offer.ID] {
			continue
		}
		if offer.Relationships == nil || offer.Relationships.Territory == nil {
			continue
		}
		if territory := offer.Relationships.Territory.Data.ID; territory != "" {
			created[territory] = offer
		}
	}

	return created, nil
}

// UpdateSubscriptionIntroductoryOfferEndDate sets or clears the end date of one
// offer -- the only thing about an offer Apple allows to change. A nil endDate
// is sent as an explicit null, which is how an end date is removed.
func (c *Client) UpdateSubscriptionIntroductoryOfferEndDate(offerID string, endDate *string, authToken *string) (*models.SubscriptionIntroductoryOffer, error) {
	requestData := models.Request[models.SubscriptionIntroductoryOfferUpdateRequest]{
		Data: models.SubscriptionIntroductoryOfferUpdateRequest{
			Type:       "subscriptionIntroductoryOffers",
			ID:         offerID,
			Attributes: models.SubscriptionIntroductoryOfferUpdateAttributes{EndDate: endDate},
		},
	}

	rb, err := json.Marshal(requestData)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("PATCH", fmt.Sprintf("%s/v1/subscriptionIntroductoryOffers/%s", c.HostURL, offerID), strings.NewReader(string(rb)))
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(req, authToken)
	if err != nil {
		return nil, err
	}

	response := models.Response[models.SubscriptionIntroductoryOffer]{}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}

	return &response.Data, nil
}

// DeleteSubscriptionIntroductoryOffer deletes one offer in one territory.
//
// Apple also publishes DELETE /v1/subscriptions/{id}/relationships/introductoryOffers,
// which would remove many in one request, but whether removing the linkage
// deletes the offer is not documented, and this one says what it does.
func (c *Client) DeleteSubscriptionIntroductoryOffer(offerID string, authToken *string) error {
	req, err := http.NewRequest("DELETE", fmt.Sprintf("%s/v1/subscriptionIntroductoryOffers/%s", c.HostURL, offerID), nil)
	if err != nil {
		return err
	}

	_, err = c.doRequest(req, authToken)
	return err
}
