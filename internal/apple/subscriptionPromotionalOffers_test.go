// Copyright (c) AO Studio
// SPDX-License-Identifier: MPL-2.0

package apple

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/AhmedOsman00/terraform-provider-apple/internal/apple/models"
)

// promotionalOfferRecorder captures the one request a test server receives.
type promotionalOfferRecorder struct {
	method string
	path   string
	query  url.Values
	body   map[string]any
}

func newPromotionalOfferServer(t *testing.T, rec *promotionalOfferRecorder, status int, response string) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.method, rec.path, rec.query = r.Method, r.URL.Path, r.URL.Query()
		rec.body = nil
		if r.Body != nil && r.ContentLength != 0 {
			if err := json.NewDecoder(r.Body).Decode(&rec.body); err != nil {
				t.Errorf("decoding request body: %s", err)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		fmt.Fprint(w, response)
	}))
}

const promotionalOfferResponse = `{"data":{"type":"subscriptionPromotionalOffers","id":"offer-1",
	"attributes":{"duration":"THREE_MONTHS","name":"Win back","numberOfPeriods":3,
		"offerCode":"winback50","offerMode":"PAY_AS_YOU_GO","targetSubscriptionPlanType":"MONTHLY"},
	"relationships":{"subscription":{"data":{"type":"subscriptions","id":"sub-1"}},
		"prices":{"links":{"related":"https://example.invalid"}}}},
	"links":{"self":"https://example.invalid"}}`

// object walks a decoded JSON document by member names, failing the test when
// a member is missing or is not an object.
func object(t *testing.T, doc map[string]any, path ...string) map[string]any {
	t.Helper()

	current := doc
	for _, member := range path {
		next, ok := current[member].(map[string]any)
		if !ok {
			t.Fatalf("member %q missing or not an object in %v", member, current)
		}
		current = next
	}
	return current
}

// TestCreateSubscriptionPromotionalOfferRequest pins the create document Apple
// receives: the five required attributes, the subscription and placeholder
// prices relationships, and each price inline under the same placeholder with
// its territory and price point.
func TestCreateSubscriptionPromotionalOfferRequest(t *testing.T) {
	rec := &promotionalOfferRecorder{}
	srv := newPromotionalOfferServer(t, rec, http.StatusCreated, promotionalOfferResponse)
	defer srv.Close()

	planType := models.SubscriptionPlanTypeMonthly
	offer, err := newTestClient(srv).CreateSubscriptionPromotionalOffer("sub-1",
		models.SubscriptionPromotionalOfferCreateAttributes{
			Duration:                   models.SubscriptionOfferDurationThreeMonths,
			Name:                       "Win back",
			NumberOfPeriods:            3,
			OfferCode:                  "winback50",
			OfferMode:                  models.SubscriptionOfferModePayAsYouGo,
			TargetSubscriptionPlanType: &planType,
		},
		[]SubscriptionPromotionalOfferPrice{
			{TerritoryID: "USA", PricePointID: "point-usa"},
			{TerritoryID: "GBR", PricePointID: "point-gbr"},
		},
		nil,
	)
	if err != nil {
		t.Fatalf("CreateSubscriptionPromotionalOffer: %s", err)
	}

	if rec.method != "POST" || rec.path != "/v1/subscriptionPromotionalOffers" {
		t.Errorf("request = %s %s, want POST /v1/subscriptionPromotionalOffers", rec.method, rec.path)
	}

	data := object(t, rec.body, "data")
	if data["type"] != "subscriptionPromotionalOffers" {
		t.Errorf("type = %v, want subscriptionPromotionalOffers", data["type"])
	}

	attrs := object(t, data, "attributes")
	want := map[string]any{
		"duration":                   "THREE_MONTHS",
		"name":                       "Win back",
		"numberOfPeriods":            float64(3),
		"offerCode":                  "winback50",
		"offerMode":                  "PAY_AS_YOU_GO",
		"targetSubscriptionPlanType": "MONTHLY",
	}
	for key, value := range want {
		if attrs[key] != value {
			t.Errorf("attributes.%s = %v, want %v", key, attrs[key], value)
		}
	}

	subscription := object(t, data, "relationships", "subscription", "data")
	if subscription["type"] != "subscriptions" || subscription["id"] != "sub-1" {
		t.Errorf("subscription relationship = %v, want type subscriptions id sub-1", subscription)
	}

	placeholders, _ := object(t, data, "relationships", "prices")["data"].([]any)
	if len(placeholders) != 2 {
		t.Fatalf("prices relationship has %d members, want 2", len(placeholders))
	}
	first, _ := placeholders[0].(map[string]any)
	if first["type"] != "subscriptionPromotionalOfferPrices" || first["id"] != "${price0}" {
		t.Errorf("first placeholder = %v, want type subscriptionPromotionalOfferPrices id ${price0}", first)
	}

	included, _ := rec.body["included"].([]any)
	if len(included) != 2 {
		t.Fatalf("included has %d members, want 2", len(included))
	}
	second, _ := included[1].(map[string]any)
	if second["type"] != "subscriptionPromotionalOfferPrices" || second["id"] != "${price1}" {
		t.Errorf("second inline price = %v, want type subscriptionPromotionalOfferPrices id ${price1}", second)
	}
	territory := object(t, second, "relationships", "territory", "data")
	if territory["type"] != "territories" || territory["id"] != "GBR" {
		t.Errorf("territory = %v, want type territories id GBR", territory)
	}
	point := object(t, second, "relationships", "subscriptionPricePoint", "data")
	if point["type"] != "subscriptionPricePoints" || point["id"] != "point-gbr" {
		t.Errorf("price point = %v, want type subscriptionPricePoints id point-gbr", point)
	}
	if _, present := object(t, second, "relationships")["subscription"]; present {
		t.Error("inline price repeats the subscription, which Apple's inline schema does not have")
	}

	if offer.ID != "offer-1" || offer.Attributes.OfferCode != "winback50" {
		t.Errorf("decoded offer = %+v, want id offer-1 code winback50", offer)
	}
	if offer.Attributes.NumberOfPeriods == nil || *offer.Attributes.NumberOfPeriods != 3 {
		t.Errorf("numberOfPeriods = %v, want 3", offer.Attributes.NumberOfPeriods)
	}
}

// TestCreateSubscriptionPromotionalOfferFreeTrial checks the two optional
// members are absent rather than empty: an unset plan type, and the price point
// of a territory that carries a free trial.
func TestCreateSubscriptionPromotionalOfferFreeTrial(t *testing.T) {
	rec := &promotionalOfferRecorder{}
	srv := newPromotionalOfferServer(t, rec, http.StatusCreated, promotionalOfferResponse)
	defer srv.Close()

	_, err := newTestClient(srv).CreateSubscriptionPromotionalOffer("sub-1",
		models.SubscriptionPromotionalOfferCreateAttributes{
			Duration:        models.SubscriptionOfferDurationOneMonth,
			Name:            "Free month",
			NumberOfPeriods: 1,
			OfferCode:       "freemonth",
			OfferMode:       models.SubscriptionOfferModeFreeTrial,
		},
		[]SubscriptionPromotionalOfferPrice{{TerritoryID: "USA"}},
		nil,
	)
	if err != nil {
		t.Fatalf("CreateSubscriptionPromotionalOffer: %s", err)
	}

	attrs := object(t, rec.body, "data", "attributes")
	if _, present := attrs["targetSubscriptionPlanType"]; present {
		t.Errorf("targetSubscriptionPlanType sent despite being unset: %v", attrs["targetSubscriptionPlanType"])
	}

	included, _ := rec.body["included"].([]any)
	if len(included) != 1 {
		t.Fatalf("included has %d members, want 1", len(included))
	}
	price, _ := included[0].(map[string]any)
	relationships := object(t, price, "relationships")
	if _, present := relationships["subscriptionPricePoint"]; present {
		t.Errorf("subscriptionPricePoint sent for a free-trial territory: %v", relationships["subscriptionPricePoint"])
	}
	if territory := object(t, relationships, "territory", "data"); territory["id"] != "USA" {
		t.Errorf("territory = %v, want USA", territory)
	}
}

// TestSetSubscriptionPromotionalOfferPricesRequest pins the update: a PATCH to
// the offer carrying only the prices relationship and the inline prices, with
// no attributes member -- Apple's update request has none.
func TestSetSubscriptionPromotionalOfferPricesRequest(t *testing.T) {
	rec := &promotionalOfferRecorder{}
	srv := newPromotionalOfferServer(t, rec, http.StatusOK, promotionalOfferResponse)
	defer srv.Close()

	_, err := newTestClient(srv).SetSubscriptionPromotionalOfferPrices("offer-1",
		[]SubscriptionPromotionalOfferPrice{{TerritoryID: "USA", PricePointID: "point-usa"}}, nil)
	if err != nil {
		t.Fatalf("SetSubscriptionPromotionalOfferPrices: %s", err)
	}

	if rec.method != "PATCH" || rec.path != "/v1/subscriptionPromotionalOffers/offer-1" {
		t.Errorf("request = %s %s, want PATCH /v1/subscriptionPromotionalOffers/offer-1", rec.method, rec.path)
	}

	data := object(t, rec.body, "data")
	if data["type"] != "subscriptionPromotionalOffers" || data["id"] != "offer-1" {
		t.Errorf("data = type %v id %v, want subscriptionPromotionalOffers offer-1", data["type"], data["id"])
	}
	if _, present := data["attributes"]; present {
		t.Errorf("attributes sent on an update, which Apple's request does not accept: %v", data["attributes"])
	}

	placeholders, _ := object(t, data, "relationships", "prices")["data"].([]any)
	if len(placeholders) != 1 {
		t.Fatalf("prices relationship has %d members, want 1", len(placeholders))
	}

	included, _ := rec.body["included"].([]any)
	if len(included) != 1 {
		t.Fatalf("included has %d members, want 1", len(included))
	}
	price, _ := included[0].(map[string]any)
	if point := object(t, price, "relationships", "subscriptionPricePoint", "data"); point["id"] != "point-usa" {
		t.Errorf("price point = %v, want point-usa", point)
	}
}

// TestGetSubscriptionPromotionalOfferRequestsSubscription covers the include
// that lets a bare offer ID be imported: without it Apple reports the
// subscription as links alone.
func TestGetSubscriptionPromotionalOfferRequestsSubscription(t *testing.T) {
	rec := &promotionalOfferRecorder{}
	srv := newPromotionalOfferServer(t, rec, http.StatusOK, promotionalOfferResponse)
	defer srv.Close()

	offer, err := newTestClient(srv).GetSubscriptionPromotionalOffer("offer-1")
	if err != nil {
		t.Fatalf("GetSubscriptionPromotionalOffer: %s", err)
	}

	if rec.method != "GET" || rec.path != "/v1/subscriptionPromotionalOffers/offer-1" {
		t.Errorf("request = %s %s, want GET /v1/subscriptionPromotionalOffers/offer-1", rec.method, rec.path)
	}
	if got := rec.query.Get("include"); got != "subscription" {
		t.Errorf("include = %q, want subscription", got)
	}

	if offer.Relationships == nil || offer.Relationships.Subscription == nil {
		t.Fatal("subscription relationship was not decoded")
	}
	if got := offer.Relationships.Subscription.Data.ID; got != "sub-1" {
		t.Errorf("subscription = %q, want sub-1", got)
	}
	if offer.Attributes.OfferMode == nil || *offer.Attributes.OfferMode != models.SubscriptionOfferModePayAsYouGo {
		t.Errorf("offerMode = %v, want PAY_AS_YOU_GO", offer.Attributes.OfferMode)
	}
	if offer.Attributes.TargetSubscriptionPlanType == nil || *offer.Attributes.TargetSubscriptionPlanType != models.SubscriptionPlanTypeMonthly {
		t.Errorf("targetSubscriptionPlanType = %v, want MONTHLY", offer.Attributes.TargetSubscriptionPlanType)
	}
}

// TestGetSubscriptionPromotionalOfferPricesIncludesLinkages covers the prices
// read: its own collection, both includes, and a decode where one territory
// carries a price point and a free-trial territory does not.
func TestGetSubscriptionPromotionalOfferPricesIncludesLinkages(t *testing.T) {
	rec := &promotionalOfferRecorder{}
	srv := newPromotionalOfferServer(t, rec, http.StatusOK, `{"data":[
		{"type":"subscriptionPromotionalOfferPrices","id":"p-usa",
			"relationships":{"territory":{"data":{"type":"territories","id":"USA"}},
				"subscriptionPricePoint":{"data":{"type":"subscriptionPricePoints","id":"point-usa"}}}},
		{"type":"subscriptionPromotionalOfferPrices","id":"p-gbr",
			"relationships":{"territory":{"data":{"type":"territories","id":"GBR"}}}}],
		"included":[{"type":"territories","id":"USA","attributes":{"currency":"USD"}}],
		"links":{"self":"https://example.invalid"}}`)
	defer srv.Close()

	prices, err := newTestClient(srv).GetSubscriptionPromotionalOfferPrices("offer-1")
	if err != nil {
		t.Fatalf("GetSubscriptionPromotionalOfferPrices: %s", err)
	}

	if rec.path != "/v1/subscriptionPromotionalOffers/offer-1/prices" {
		t.Errorf("path = %q, want /v1/subscriptionPromotionalOffers/offer-1/prices", rec.path)
	}
	if got := rec.query.Get("include"); got != "territory,subscriptionPricePoint" {
		t.Errorf("include = %q, want territory,subscriptionPricePoint", got)
	}
	if got := rec.query.Get("limit"); got != "200" {
		t.Errorf("limit = %q, want 200", got)
	}

	if len(prices) != 2 {
		t.Fatalf("got %d prices, want 2", len(prices))
	}
	if prices[0].Relationships == nil || prices[0].Relationships.SubscriptionPricePoint == nil {
		t.Fatal("price point relationship was not decoded")
	}
	if got := prices[0].Relationships.SubscriptionPricePoint.Data.ID; got != "point-usa" {
		t.Errorf("price point = %q, want point-usa", got)
	}
	if prices[1].Relationships.SubscriptionPricePoint != nil {
		t.Errorf("free-trial territory decoded a price point: %+v", prices[1].Relationships.SubscriptionPricePoint)
	}
}

// TestGetSubscriptionPromotionalOfferByCode covers the lookup import's
// composite form depends on: the subscription's collection, scanned in memory.
func TestGetSubscriptionPromotionalOfferByCode(t *testing.T) {
	rec := &promotionalOfferRecorder{}
	srv := newPromotionalOfferServer(t, rec, http.StatusOK, `{"data":[
		{"type":"subscriptionPromotionalOffers","id":"offer-1","attributes":{"offerCode":"other"}},
		{"type":"subscriptionPromotionalOffers","id":"offer-2","attributes":{"offerCode":"winback50"}}]}`)
	defer srv.Close()

	client := newTestClient(srv)

	offer, err := client.GetSubscriptionPromotionalOfferByCode("sub-1", "winback50")
	if err != nil {
		t.Fatalf("GetSubscriptionPromotionalOfferByCode: %s", err)
	}
	if rec.path != "/v1/subscriptions/sub-1/promotionalOffers" {
		t.Errorf("path = %q, want /v1/subscriptions/sub-1/promotionalOffers", rec.path)
	}
	if offer.ID != "offer-2" {
		t.Errorf("found offer %q, want offer-2", offer.ID)
	}

	if _, err := client.GetSubscriptionPromotionalOfferByCode("sub-1", "missing"); err == nil {
		t.Error("expected an error for an offer code the subscription does not have")
	}
}

// TestDeleteSubscriptionPromotionalOffer pins the delete request.
func TestDeleteSubscriptionPromotionalOffer(t *testing.T) {
	rec := &promotionalOfferRecorder{}
	srv := newPromotionalOfferServer(t, rec, http.StatusNoContent, "")
	defer srv.Close()

	if err := newTestClient(srv).DeleteSubscriptionPromotionalOffer("offer-1", nil); err != nil {
		t.Fatalf("DeleteSubscriptionPromotionalOffer: %s", err)
	}

	if rec.method != "DELETE" || rec.path != "/v1/subscriptionPromotionalOffers/offer-1" {
		t.Errorf("request = %s %s, want DELETE /v1/subscriptionPromotionalOffers/offer-1", rec.method, rec.path)
	}
}
