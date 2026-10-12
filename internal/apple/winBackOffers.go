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

// WinBackOfferPriceInput is one price to create a win-back offer with.
//
// It is an input helper rather than a wire type, like SubscriptionManualPrice:
// the request sends each price twice, once as a placeholder in the "prices"
// relationship and once inline in "included", and CreateWinBackOffer assembles
// both from this.
type WinBackOfferPriceInput struct {
	PricePointID string
	TerritoryID  string
}

// GetWinBackOffers lists every win-back offer of a subscription.
//
// Apple publishes no top-level GET /v1/winBackOffers collection -- an offer is
// listed only through the subscription it belongs to.
func (c *Client) GetWinBackOffers(subscriptionID string) ([]models.WinBackOffer, error) {
	return getAllPages[models.WinBackOffer](
		c,
		fmt.Sprintf("/v1/subscriptions/%s/winBackOffers", subscriptionID),
		defaultPageSize,
	)
}

// GetWinBackOffer retrieves one win-back offer by Apple's ID.
//
// The response never names the subscription: WinBackOffer has no subscription
// relationship, and include accepts only "prices".
func (c *Client) GetWinBackOffer(offerID string) (*models.WinBackOffer, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/v1/winBackOffers/%s", c.HostURL, offerID), nil)
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(req, nil)
	if err != nil {
		return nil, err
	}

	response := models.Response[models.WinBackOffer]{}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}

	return &response.Data, nil
}

// GetWinBackOfferPrices lists the prices of a win-back offer.
//
// include=territory,subscriptionPricePoint is what makes a price comparable
// with configuration: without it Apple reports both relationships as links
// alone, and a WinBackOfferPrice has no attributes of its own. An offer priced
// in every storefront has 175 of them, so the collection is paginated like any
// other.
func (c *Client) GetWinBackOfferPrices(offerID string) ([]models.WinBackOfferPrice, error) {
	params := url.Values{}
	params.Set("include", "territory,subscriptionPricePoint")

	return getAllPagesQuery[models.WinBackOfferPrice](
		c,
		fmt.Sprintf("/v1/winBackOffers/%s/prices", offerID),
		defaultPageSize,
		params,
	)
}

// CreateWinBackOffer creates a win-back offer and all of its prices in one
// request.
//
// The prices do not exist yet, so each travels inline in "included" under a
// placeholder ID of the form "${price0}" that the "prices" relationship
// references, and Apple substitutes real IDs on commit -- the shape the price
// schedules use. See models.WinBackOfferPriceInlineCreate for why the inline
// price carries a relationship Apple's schema does not declare.
//
// The offer's prices can never change afterwards: Apple's update request has no
// relationships at all.
func (c *Client) CreateWinBackOffer(
	subscriptionID string,
	attributes models.WinBackOfferAttributes,
	prices []WinBackOfferPriceInput,
	authToken *string,
) (*models.WinBackOffer, error) {
	placeholders := make([]models.ResourceData, 0, len(prices))
	included := make([]models.WinBackOfferPriceInlineCreate, 0, len(prices))

	for i, price := range prices {
		placeholderID := fmt.Sprintf("${price%d}", i)

		placeholders = append(placeholders, models.ResourceData{Type: "winBackOfferPrices", ID: placeholderID})

		relationships := models.WinBackOfferPriceInlineRelationships{
			SubscriptionPricePoint: models.ResourceIdentifier{
				Data: models.ResourceData{Type: "subscriptionPricePoints", ID: price.PricePointID},
			},
		}
		if price.TerritoryID != "" {
			relationships.Territory = &models.ResourceIdentifier{
				Data: models.ResourceData{Type: "territories", ID: price.TerritoryID},
			}
		}

		included = append(included, models.WinBackOfferPriceInlineCreate{
			Type:          "winBackOfferPrices",
			ID:            placeholderID,
			Relationships: relationships,
		})
	}

	requestData := models.WinBackOfferCreateRequest{
		Data: models.WinBackOfferCreateData{
			Type:       "winBackOffers",
			Attributes: attributes,
			Relationships: models.WinBackOfferCreateRelationships{
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

	req, err := http.NewRequest("POST", fmt.Sprintf("%s/v1/winBackOffers", c.HostURL), strings.NewReader(string(rb)))
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(req, authToken)
	if err != nil {
		return nil, err
	}

	response := models.Response[models.WinBackOffer]{}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}

	return &response.Data, nil
}

// UpdateWinBackOffer changes the eligibility rules, dates, priority or
// promotion of a win-back offer. Only the members set in attributes are sent.
func (c *Client) UpdateWinBackOffer(
	offerID string,
	attributes models.WinBackOfferUpdateAttributes,
	authToken *string,
) (*models.WinBackOffer, error) {
	requestData := models.Request[models.WinBackOfferUpdateRequest]{
		Data: models.WinBackOfferUpdateRequest{
			Type:       "winBackOffers",
			ID:         offerID,
			Attributes: attributes,
		},
	}

	rb, err := json.Marshal(requestData)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("PATCH", fmt.Sprintf("%s/v1/winBackOffers/%s", c.HostURL, offerID), strings.NewReader(string(rb)))
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(req, authToken)
	if err != nil {
		return nil, err
	}

	response := models.Response[models.WinBackOffer]{}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}

	return &response.Data, nil
}

// DeleteWinBackOffer deletes a win-back offer. Apple allows this at any time,
// live or not; a customer holding a link to a deleted offer is told it is no
// longer available.
func (c *Client) DeleteWinBackOffer(offerID string, authToken *string) error {
	req, err := http.NewRequest("DELETE", fmt.Sprintf("%s/v1/winBackOffers/%s", c.HostURL, offerID), nil)
	if err != nil {
		return err
	}

	_, err = c.doRequest(req, authToken)
	return err
}
