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

// SubscriptionPromotionalOfferPrice is one territory's price to set on a
// promotional offer.
//
// It is an input helper rather than a wire type: the request Apple wants sends
// each price twice, once as a placeholder in the relationship and once inline
// in "included", and promotionalOfferPrices assembles both from this. An empty
// PricePointID omits the price point, which is how a territory carries a free
// trial.
type SubscriptionPromotionalOfferPrice struct {
	TerritoryID  string
	PricePointID string
}

// GetSubscriptionPromotionalOffers lists every promotional offer of a
// subscription.
//
// Apple publishes no top-level collection: GET /v1/subscriptionPromotionalOffers
// is not an operation, so the offers are reached through their subscription.
func (c *Client) GetSubscriptionPromotionalOffers(subscriptionID string) ([]models.SubscriptionPromotionalOffer, error) {
	return getAllPages[models.SubscriptionPromotionalOffer](
		c,
		fmt.Sprintf("/v1/subscriptions/%s/promotionalOffers", subscriptionID),
		defaultPageSize,
	)
}

// GetSubscriptionPromotionalOffer retrieves one promotional offer by its ID.
//
// include=subscription populates the parent linkage, which import needs to
// write complete state from a bare offer ID.
func (c *Client) GetSubscriptionPromotionalOffer(offerID string) (*models.SubscriptionPromotionalOffer, error) {
	params := url.Values{}
	params.Set("include", "subscription")

	req, err := http.NewRequest("GET", fmt.Sprintf("%s/v1/subscriptionPromotionalOffers/%s?%s", c.HostURL, offerID, params.Encode()), nil)
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(req, nil)
	if err != nil {
		return nil, err
	}

	response := models.Response[models.SubscriptionPromotionalOffer]{}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}

	return &response.Data, nil
}

// GetSubscriptionPromotionalOfferByCode finds a subscription's offer by its
// offer code, scanning the subscription's collection: Apple publishes no
// filter[offerCode].
func (c *Client) GetSubscriptionPromotionalOfferByCode(subscriptionID, offerCode string) (*models.SubscriptionPromotionalOffer, error) {
	offers, err := c.GetSubscriptionPromotionalOffers(subscriptionID)
	if err != nil {
		return nil, err
	}

	for _, offer := range offers {
		if offer.Attributes.OfferCode == offerCode {
			return &offer, nil
		}
	}

	return nil, fmt.Errorf("promotional offer with code '%s' not found on subscription '%s'", offerCode, subscriptionID)
}

// GetSubscriptionPromotionalOfferPrices reads every territory's price of an
// offer.
//
// This is its own paginated collection rather than include=prices on the
// offer, because that include is capped at limit[prices]=50 -- an offer
// available in every storefront has three times as many. include is required
// for the result to be usable: without it Apple reports the territory and the
// price point as links alone.
func (c *Client) GetSubscriptionPromotionalOfferPrices(offerID string) ([]models.SubscriptionPromotionalOfferPrice, error) {
	params := url.Values{}
	params.Set("include", "territory,subscriptionPricePoint")

	return getAllPagesQuery[models.SubscriptionPromotionalOfferPrice](
		c,
		fmt.Sprintf("/v1/subscriptionPromotionalOffers/%s/prices", offerID),
		defaultPageSize,
		params,
	)
}

// CreateSubscriptionPromotionalOffer creates a promotional offer with its
// prices in one request.
//
// The prices do not exist yet, so each travels inline in "included" under a
// placeholder ID that the prices relationship references; Apple substitutes
// real IDs as it commits. Apple requires the prices relationship on every
// create.
func (c *Client) CreateSubscriptionPromotionalOffer(
	subscriptionID string,
	attributes models.SubscriptionPromotionalOfferCreateAttributes,
	prices []SubscriptionPromotionalOfferPrice,
	authToken *string,
) (*models.SubscriptionPromotionalOffer, error) {
	placeholders, included := promotionalOfferPrices(prices)

	requestData := models.SubscriptionPromotionalOfferCreateRequest{
		Data: models.SubscriptionPromotionalOfferCreateData{
			Type:       "subscriptionPromotionalOffers",
			Attributes: attributes,
			Relationships: models.SubscriptionPromotionalOfferCreateRelationships{
				Subscription: models.ResourceIdentifier{
					Data: models.ResourceData{Type: "subscriptions", ID: subscriptionID},
				},
				Prices: models.ResourceIdentifiers{Data: placeholders},
			},
		},
		Included: included,
	}

	rb, err := json.Marshal(requestData)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", fmt.Sprintf("%s/v1/subscriptionPromotionalOffers", c.HostURL), strings.NewReader(string(rb)))
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(req, authToken)
	if err != nil {
		return nil, err
	}

	response := models.Response[models.SubscriptionPromotionalOffer]{}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}

	return &response.Data, nil
}

// SetSubscriptionPromotionalOfferPrices replaces an offer's prices.
//
// This is the only change Apple's update request can express: it carries the
// prices relationship and no attributes. The relationship names the complete
// set the offer should have afterwards.
func (c *Client) SetSubscriptionPromotionalOfferPrices(
	offerID string,
	prices []SubscriptionPromotionalOfferPrice,
	authToken *string,
) (*models.SubscriptionPromotionalOffer, error) {
	placeholders, included := promotionalOfferPrices(prices)

	requestData := models.SubscriptionPromotionalOfferUpdateRequest{
		Data: models.SubscriptionPromotionalOfferUpdateData{
			Type: "subscriptionPromotionalOffers",
			ID:   offerID,
			Relationships: models.SubscriptionPromotionalOfferUpdateRelationships{
				Prices: models.ResourceIdentifiers{Data: placeholders},
			},
		},
		Included: included,
	}

	rb, err := json.Marshal(requestData)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("PATCH", fmt.Sprintf("%s/v1/subscriptionPromotionalOffers/%s", c.HostURL, offerID), strings.NewReader(string(rb)))
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(req, authToken)
	if err != nil {
		return nil, err
	}

	response := models.Response[models.SubscriptionPromotionalOffer]{}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}

	return &response.Data, nil
}

// DeleteSubscriptionPromotionalOffer deletes a promotional offer.
func (c *Client) DeleteSubscriptionPromotionalOffer(offerID string, authToken *string) error {
	req, err := http.NewRequest("DELETE", fmt.Sprintf("%s/v1/subscriptionPromotionalOffers/%s", c.HostURL, offerID), nil)
	if err != nil {
		return err
	}

	_, err = c.doRequest(req, authToken)
	return err
}

// promotionalOfferPrices builds the placeholder relationship and the inline
// prices it references.
//
// The territory is always sent: it is what a free-trial price consists of. The
// price point is sent only when one is named.
func promotionalOfferPrices(prices []SubscriptionPromotionalOfferPrice) ([]models.ResourceData, []models.SubscriptionPromotionalOfferPriceInlineCreate) {
	placeholders := make([]models.ResourceData, 0, len(prices))
	included := make([]models.SubscriptionPromotionalOfferPriceInlineCreate, 0, len(prices))

	for i, price := range prices {
		placeholderID := fmt.Sprintf("${price%d}", i)

		placeholders = append(placeholders, models.ResourceData{
			Type: "subscriptionPromotionalOfferPrices",
			ID:   placeholderID,
		})

		relationships := models.SubscriptionPromotionalOfferPriceInlineRelationships{}
		if price.TerritoryID != "" {
			relationships.Territory = &models.ResourceIdentifier{
				Data: models.ResourceData{Type: "territories", ID: price.TerritoryID},
			}
		}
		if price.PricePointID != "" {
			relationships.SubscriptionPricePoint = &models.ResourceIdentifier{
				Data: models.ResourceData{Type: "subscriptionPricePoints", ID: price.PricePointID},
			}
		}

		included = append(included, models.SubscriptionPromotionalOfferPriceInlineCreate{
			Type:          "subscriptionPromotionalOfferPrices",
			ID:            placeholderID,
			Relationships: relationships,
		})
	}

	return placeholders, included
}
