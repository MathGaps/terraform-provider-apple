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

// SubscriptionOfferCodePriceInput is one territory's price for an offer code.
//
// It is an input helper rather than a wire type: the create request sends each
// price twice, once as a placeholder in the "prices" relationship and once
// inline in "included", and CreateSubscriptionOfferCode assembles both. An
// empty PricePointID omits the price point, which is what a FREE_TRIAL sends.
type SubscriptionOfferCodePriceInput struct {
	TerritoryID  string
	PricePointID string
}

// GetSubscriptionOfferCodes lists the offer codes of a subscription.
//
// Apple publishes no top-level collection, so a subscription's offers are
// reachable only through it -- the same shape as every other subscription
// child.
func (c *Client) GetSubscriptionOfferCodes(subscriptionID string) ([]models.SubscriptionOfferCode, error) {
	return getAllPages[models.SubscriptionOfferCode](
		c, fmt.Sprintf("/v1/subscriptions/%s/offerCodes", subscriptionID), defaultPageSize)
}

// GetSubscriptionOfferCode reads one offer code.
//
// include=subscription is requested so the response carries the subscription
// linkage: without it Apple reports the relationship as links alone, and an
// import would have no way to learn which subscription the offer belongs to.
func (c *Client) GetSubscriptionOfferCode(offerCodeID string) (*models.SubscriptionOfferCode, error) {
	params := url.Values{}
	params.Set("include", "subscription")

	return offerCodeGet[models.SubscriptionOfferCode](c,
		fmt.Sprintf("%s/v1/subscriptionOfferCodes/%s?%s", c.HostURL, offerCodeID, params.Encode()))
}

// GetSubscriptionOfferCodePrices lists an offer code's per-territory prices.
//
// include=territory,subscriptionPricePoint populates both linkages; without it
// a price carries neither, and cannot be told apart from any other.
func (c *Client) GetSubscriptionOfferCodePrices(offerCodeID string) ([]models.SubscriptionOfferCodePrice, error) {
	params := url.Values{}
	params.Set("include", "territory,subscriptionPricePoint")

	return getAllPagesQuery[models.SubscriptionOfferCodePrice](
		c, fmt.Sprintf("/v1/subscriptionOfferCodes/%s/prices", offerCodeID), defaultPageSize, params)
}

// CreateSubscriptionOfferCode creates an offer code with its prices.
//
// This is the only write that sets anything but "active": Apple's update
// request carries nothing else, so every attribute and every price is fixed
// from here on. The prices travel inline under "${price0}"-style placeholders
// that the "prices" relationship references, the JSON:API shape the price
// schedules already use.
func (c *Client) CreateSubscriptionOfferCode(
	subscriptionID string,
	attributes models.SubscriptionOfferCodeCreateAttributes,
	prices []SubscriptionOfferCodePriceInput,
	authToken *string,
) (*models.SubscriptionOfferCode, error) {
	placeholders := make([]models.ResourceData, 0, len(prices))
	included := make([]models.SubscriptionOfferCodePriceInlineCreate, 0, len(prices))

	for i, price := range prices {
		placeholderID := fmt.Sprintf("${price%d}", i)
		placeholders = append(placeholders, models.ResourceData{Type: "subscriptionOfferCodePrices", ID: placeholderID})

		relationships := models.SubscriptionOfferCodePriceInlineCreateRelations{
			Territory: models.ResourceIdentifier{Data: models.ResourceData{Type: "territories", ID: price.TerritoryID}},
		}
		if price.PricePointID != "" {
			relationships.SubscriptionPricePoint = &models.ResourceIdentifier{
				Data: models.ResourceData{Type: "subscriptionPricePoints", ID: price.PricePointID},
			}
		}

		included = append(included, models.SubscriptionOfferCodePriceInlineCreate{
			Type:          "subscriptionOfferCodePrices",
			ID:            placeholderID,
			Relationships: relationships,
		})
	}

	requestData := models.SubscriptionOfferCodeCreateRequest{
		Data: models.SubscriptionOfferCodeCreateData{
			Type:       "subscriptionOfferCodes",
			Attributes: attributes,
			Relationships: models.SubscriptionOfferCodeCreateRelationships{
				Subscription: models.ResourceIdentifier{
					Data: models.ResourceData{Type: "subscriptions", ID: subscriptionID},
				},
				Prices: models.ResourceIdentifiers{Data: placeholders},
			},
		},
		Included: included,
	}

	return offerCodeSend[models.SubscriptionOfferCode](c, "POST", c.HostURL+"/v1/subscriptionOfferCodes", requestData, authToken)
}

// SetSubscriptionOfferCodeActive turns an offer code on or off.
//
// "active" is the only member of Apple's update request. App Store Connect's
// help says a deactivated offer "can't be reactivated later" and that every
// code under it is invalidated, so in practice this only ever goes one way.
func (c *Client) SetSubscriptionOfferCodeActive(offerCodeID string, active bool, authToken *string) (*models.SubscriptionOfferCode, error) {
	return offerCodeSend[models.SubscriptionOfferCode](c, "PATCH",
		fmt.Sprintf("%s/v1/subscriptionOfferCodes/%s", c.HostURL, offerCodeID),
		activeUpdate("subscriptionOfferCodes", offerCodeID, active), authToken)
}

// GetSubscriptionOfferCodeCustomCode reads one custom code, with its offer
// linkage populated through include=offerCode.
func (c *Client) GetSubscriptionOfferCodeCustomCode(customCodeID string) (*models.SubscriptionOfferCodeCustomCode, error) {
	params := url.Values{}
	params.Set("include", "offerCode")

	return offerCodeGet[models.SubscriptionOfferCodeCustomCode](c,
		fmt.Sprintf("%s/v1/subscriptionOfferCodeCustomCodes/%s?%s", c.HostURL, customCodeID, params.Encode()))
}

// CreateSubscriptionOfferCodeCustomCode creates a shared code string for an
// offer. Apple refuses a string another active offer of the same app already
// uses; it can be reused once that one is deactivated.
func (c *Client) CreateSubscriptionOfferCodeCustomCode(
	offerCodeID string,
	attributes models.SubscriptionOfferCodeCustomCodeCreateAttrs,
	authToken *string,
) (*models.SubscriptionOfferCodeCustomCode, error) {
	requestData := models.Request[models.SubscriptionOfferCodeCustomCodeCreateRequest]{
		Data: models.SubscriptionOfferCodeCustomCodeCreateRequest{
			Type:          "subscriptionOfferCodeCustomCodes",
			Attributes:    attributes,
			Relationships: offerCodeRelation(offerCodeID),
		},
	}

	return offerCodeSend[models.SubscriptionOfferCodeCustomCode](c, "POST",
		c.HostURL+"/v1/subscriptionOfferCodeCustomCodes", requestData, authToken)
}

// SetSubscriptionOfferCodeCustomCodeActive turns a custom code on or off; its
// update request carries "active" alone.
func (c *Client) SetSubscriptionOfferCodeCustomCodeActive(customCodeID string, active bool, authToken *string) (*models.SubscriptionOfferCodeCustomCode, error) {
	return offerCodeSend[models.SubscriptionOfferCodeCustomCode](c, "PATCH",
		fmt.Sprintf("%s/v1/subscriptionOfferCodeCustomCodes/%s", c.HostURL, customCodeID),
		activeUpdate("subscriptionOfferCodeCustomCodes", customCodeID, active), authToken)
}

// GetSubscriptionOfferCodeOneTimeUseCode reads one batch of one-time-use
// codes, with its offer linkage populated through include=offerCode. The codes
// themselves are not part of the record; see
// GetSubscriptionOfferCodeOneTimeUseCodeValues.
func (c *Client) GetSubscriptionOfferCodeOneTimeUseCode(batchID string) (*models.SubscriptionOfferCodeOneTimeUseCode, error) {
	params := url.Values{}
	params.Set("include", "offerCode")

	return offerCodeGet[models.SubscriptionOfferCodeOneTimeUseCode](c,
		fmt.Sprintf("%s/v1/subscriptionOfferCodeOneTimeUseCodes/%s?%s", c.HostURL, batchID, params.Encode()))
}

// CreateSubscriptionOfferCodeOneTimeUseCode asks Apple to generate a batch of
// one-time-use codes for an offer.
func (c *Client) CreateSubscriptionOfferCodeOneTimeUseCode(
	offerCodeID string,
	attributes models.SubscriptionOfferCodeOneTimeUseCodeCreateAttrs,
	authToken *string,
) (*models.SubscriptionOfferCodeOneTimeUseCode, error) {
	requestData := models.Request[models.SubscriptionOfferCodeOneTimeUseCodeCreateRequest]{
		Data: models.SubscriptionOfferCodeOneTimeUseCodeCreateRequest{
			Type:          "subscriptionOfferCodeOneTimeUseCodes",
			Attributes:    attributes,
			Relationships: offerCodeRelation(offerCodeID),
		},
	}

	return offerCodeSend[models.SubscriptionOfferCodeOneTimeUseCode](c, "POST",
		c.HostURL+"/v1/subscriptionOfferCodeOneTimeUseCodes", requestData, authToken)
}

// SetSubscriptionOfferCodeOneTimeUseCodeActive turns a batch on or off; its
// update request carries "active" alone.
func (c *Client) SetSubscriptionOfferCodeOneTimeUseCodeActive(batchID string, active bool, authToken *string) (*models.SubscriptionOfferCodeOneTimeUseCode, error) {
	return offerCodeSend[models.SubscriptionOfferCodeOneTimeUseCode](c, "PATCH",
		fmt.Sprintf("%s/v1/subscriptionOfferCodeOneTimeUseCodes/%s", c.HostURL, batchID),
		activeUpdate("subscriptionOfferCodeOneTimeUseCodes", batchID, active), authToken)
}

// GetSubscriptionOfferCodeOneTimeUseCodeValues downloads a batch's generated
// codes.
//
// Apple's spec gives this endpoint's success response as text/csv, not JSON --
// the same file App Store Connect offers for download -- so it is returned as
// the document Apple sent, unparsed. The spec does not describe the columns,
// and the provider does not guess at them.
func (c *Client) GetSubscriptionOfferCodeOneTimeUseCodeValues(batchID string) (string, error) {
	req, err := http.NewRequest("GET",
		fmt.Sprintf("%s/v1/subscriptionOfferCodeOneTimeUseCodes/%s/values", c.HostURL, batchID), nil)
	if err != nil {
		return "", err
	}

	// Errors still come back as JSON, so both are acceptable.
	body, err := c.doRequestAccepting(req, nil, "text/csv, application/json")
	if err != nil {
		return "", err
	}

	return string(body), nil
}

// offerCodeRelation is the relationship a custom code and a one-time-use batch
// both name their offer through.
func offerCodeRelation(offerCodeID string) models.SubscriptionOfferCodeChildCreateRelations {
	return models.SubscriptionOfferCodeChildCreateRelations{
		OfferCode: models.ResourceIdentifier{
			Data: models.ResourceData{Type: "subscriptionOfferCodes", ID: offerCodeID},
		},
	}
}

// activeUpdate builds the update document all three records share.
func activeUpdate(resourceType, id string, active bool) models.Request[models.SubscriptionOfferCodeActiveUpdateRequest] {
	return models.Request[models.SubscriptionOfferCodeActiveUpdateRequest]{
		Data: models.SubscriptionOfferCodeActiveUpdateRequest{
			Type:       resourceType,
			ID:         id,
			Attributes: models.SubscriptionOfferCodeActiveOnly{Active: active},
		},
	}
}

// offerCodeGet GETs a single-resource document and returns its data member.
func offerCodeGet[T any](c *Client, fullURL string) (*T, error) {
	req, err := http.NewRequest("GET", fullURL, nil)
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(req, nil)
	if err != nil {
		return nil, err
	}

	response := models.Response[T]{}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}

	return &response.Data, nil
}

// offerCodeSend marshals a request document, sends it and returns the data member
// of the single-resource response.
func offerCodeSend[T any](c *Client, method, fullURL string, document any, authToken *string) (*T, error) {
	rb, err := json.Marshal(document)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(method, fullURL, strings.NewReader(string(rb)))
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(req, authToken)
	if err != nil {
		return nil, err
	}

	response := models.Response[T]{}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}

	return &response.Data, nil
}
