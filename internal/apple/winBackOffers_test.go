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

// winBackOfferResponse is a realistic WinBackOfferResponse, modelled on the
// response in Apple's "Create a Win-Back Offer" documentation.
const winBackOfferResponse = `{
  "data": {
    "type": "winBackOffers",
    "id": "10778326500",
    "attributes": {
      "referenceName": "6 Months for 3 A",
      "offerId": "6Monthfor3_a",
      "duration": "SIX_MONTHS",
      "offerMode": "PAY_UP_FRONT",
      "periodCount": 1,
      "customerEligibilityPaidSubscriptionDurationInMonths": 6,
      "customerEligibilityTimeSinceLastSubscribedInMonths": {"minimum": 2, "maximum": 24},
      "customerEligibilityWaitBetweenOffersInMonths": 2,
      "startDate": "2024-07-01",
      "endDate": "2024-07-31",
      "priority": "HIGH",
      "promotionIntent": "USE_AUTO_GENERATED_ASSETS"
    },
    "relationships": {
      "prices": {
        "data": [
          {"type": "winBackOfferPrices", "id": "eyJvIjoiMTA3NzgzMjY1MDAiLCJ0IjoiQ0FOIiwicCI6IjEwMTQyIn0"}
        ]
      }
    },
    "links": {"self": "https://api.appstoreconnect.apple.com/v1/winBackOffers/10778326500"}
  },
  "links": {"self": "https://api.appstoreconnect.apple.com/v1/winBackOffers"}
}`

func intPtr(i int) *int { return &i }

// TestCreateWinBackOfferRequestShape covers the whole create document, which is
// the one place the provider departs from Apple's OpenAPI schema: the schema
// gives an inline win-back price no relationships at all, while Apple's own
// example request sends the price point on each. Everything else is load-
// bearing too -- the placeholders in "included" have to be the strings the
// "prices" relationship references, the eligibility range is an object rather
// than two numbers, and an optional member left unset must be absent rather
// than null.
func TestCreateWinBackOfferRequestShape(t *testing.T) {
	var (
		body   map[string]any
		method string
		path   string
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decoding request body: %s", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, winBackOfferResponse)
	}))
	defer srv.Close()

	offer, err := newTestClient(srv).CreateWinBackOffer("6447497832", models.WinBackOfferAttributes{
		ReferenceName: "6 Months for 3 A",
		OfferID:       "6Monthfor3_a",
		Duration:      "SIX_MONTHS",
		OfferMode:     "PAY_UP_FRONT",
		PeriodCount:   1,
		CustomerEligibilityPaidSubscriptionDurationInMonths: 6,
		CustomerEligibilityTimeSinceLastSubscribedInMonths: &models.IntegerRange{
			Minimum: intPtr(2),
			Maximum: intPtr(24),
		},
		StartDate: "2024-07-01",
		Priority:  "HIGH",
	}, []WinBackOfferPriceInput{
		{PricePointID: "point-usa"},
		{PricePointID: "point-can", TerritoryID: "CAN"},
	}, nil)
	if err != nil {
		t.Fatalf("CreateWinBackOffer: %s", err)
	}

	if method != "POST" || path != "/v1/winBackOffers" {
		t.Errorf("request = %s %s, want POST /v1/winBackOffers", method, path)
	}

	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatalf("request has no data member: %v", body)
	}
	if data["type"] != "winBackOffers" {
		t.Errorf("data.type = %v, want winBackOffers", data["type"])
	}

	attrs, _ := data["attributes"].(map[string]any)
	wantAttrs := map[string]any{
		"referenceName": "6 Months for 3 A",
		"offerId":       "6Monthfor3_a",
		"duration":      "SIX_MONTHS",
		"offerMode":     "PAY_UP_FRONT",
		"periodCount":   float64(1),
		"customerEligibilityPaidSubscriptionDurationInMonths": float64(6),
		"startDate": "2024-07-01",
		"priority":  "HIGH",
	}
	for name, want := range wantAttrs {
		if attrs[name] != want {
			t.Errorf("attributes.%s = %v, want %v", name, attrs[name], want)
		}
	}
	window, ok := attrs["customerEligibilityTimeSinceLastSubscribedInMonths"].(map[string]any)
	if !ok {
		t.Fatalf("time-since-last-subscribed is not an IntegerRange object: %v",
			attrs["customerEligibilityTimeSinceLastSubscribedInMonths"])
	}
	if window["minimum"] != float64(2) || window["maximum"] != float64(24) {
		t.Errorf("time-since-last-subscribed = %v, want {minimum: 2, maximum: 24}", window)
	}
	for _, unset := range []string{
		"endDate", "promotionIntent", "customerEligibilityWaitBetweenOffersInMonths", "targetSubscriptionPlanType",
	} {
		if _, present := attrs[unset]; present {
			t.Errorf("attributes.%s sent although it was not set: %v", unset, attrs[unset])
		}
	}

	relationships, _ := data["relationships"].(map[string]any)
	subscription, _ := relationships["subscription"].(map[string]any)
	subscriptionData, _ := subscription["data"].(map[string]any)
	if subscriptionData["type"] != "subscriptions" || subscriptionData["id"] != "6447497832" {
		t.Errorf("subscription relationship = %v, want subscriptions/6447497832", subscriptionData)
	}

	pricesRel, _ := relationships["prices"].(map[string]any)
	linkage, ok := pricesRel["data"].([]any)
	if !ok || len(linkage) != 2 {
		t.Fatalf("prices relationship = %v, want a to-many linkage of 2", relationships["prices"])
	}

	included, ok := body["included"].([]any)
	if !ok || len(included) != 2 {
		t.Fatalf("included = %v, want 2 inline prices", body["included"])
	}
	byID := make(map[string]map[string]any, len(included))
	for _, member := range included {
		entry, _ := member.(map[string]any)
		if entry["type"] != "winBackOfferPrices" {
			t.Errorf("included member type = %v, want winBackOfferPrices", entry["type"])
		}
		if _, present := entry["attributes"]; present {
			t.Errorf("inline price carries an attributes member, which Apple's schema has none of: %v", entry)
		}
		byID[fmt.Sprint(entry["id"])] = entry
	}
	for i, member := range linkage {
		entry, _ := member.(map[string]any)
		want := fmt.Sprintf("${price%d}", i)
		if entry["type"] != "winBackOfferPrices" || entry["id"] != want {
			t.Errorf("linkage %d = %v, want winBackOfferPrices/%s", i, entry, want)
		}
		if _, ok := byID[want]; !ok {
			t.Errorf("linkage %d references %q, which is not in included", i, want)
		}
	}

	pricePointOf := func(entry map[string]any) (string, map[string]any) {
		rels, _ := entry["relationships"].(map[string]any)
		point, _ := rels["subscriptionPricePoint"].(map[string]any)
		pointData, _ := point["data"].(map[string]any)
		if pointData["type"] != "subscriptionPricePoints" {
			t.Errorf("subscriptionPricePoint type = %v, want subscriptionPricePoints", pointData["type"])
		}
		return fmt.Sprint(pointData["id"]), rels
	}

	first, firstRels := pricePointOf(byID["${price0}"])
	if first != "point-usa" {
		t.Errorf("first price point = %q, want point-usa", first)
	}
	if _, present := firstRels["territory"]; present {
		t.Errorf("territory relationship sent for a price that names none: %v", firstRels["territory"])
	}
	if _, present := firstRels["subscription"]; present {
		t.Errorf("inline price repeats the subscription, which Apple's example does not: %v", firstRels)
	}

	second, secondRels := pricePointOf(byID["${price1}"])
	if second != "point-can" {
		t.Errorf("second price point = %q, want point-can", second)
	}
	territory, _ := secondRels["territory"].(map[string]any)
	territoryData, _ := territory["data"].(map[string]any)
	if territoryData["type"] != "territories" || territoryData["id"] != "CAN" {
		t.Errorf("territory relationship = %v, want territories/CAN", territoryData)
	}

	if offer.ID != "10778326500" {
		t.Errorf("decoded offer ID = %q, want 10778326500", offer.ID)
	}
}

// TestCreateWinBackOfferSendsOptionalMembers checks the optional members reach
// Apple under their wire names when they are set.
func TestCreateWinBackOfferSendsOptionalMembers(t *testing.T) {
	var body map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decoding request body: %s", err)
		}
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, winBackOfferResponse)
	}))
	defer srv.Close()

	endDate := "2024-07-31"
	intent := "NOT_PROMOTED"
	planType := "UPFRONT"

	if _, err := newTestClient(srv).CreateWinBackOffer("sub-1", models.WinBackOfferAttributes{
		ReferenceName: "r", OfferID: "o", Duration: "ONE_MONTH", OfferMode: "FREE_TRIAL", PeriodCount: 1,
		CustomerEligibilityPaidSubscriptionDurationInMonths: 36,
		CustomerEligibilityTimeSinceLastSubscribedInMonths:  &models.IntegerRange{Minimum: intPtr(1), Maximum: intPtr(3)},
		CustomerEligibilityWaitBetweenOffersInMonths:        intPtr(6),
		StartDate:                  "2024-07-01",
		EndDate:                    &endDate,
		Priority:                   "NORMAL",
		PromotionIntent:            &intent,
		TargetSubscriptionPlanType: &planType,
	}, []WinBackOfferPriceInput{{PricePointID: "p"}}, nil); err != nil {
		t.Fatalf("CreateWinBackOffer: %s", err)
	}

	data, _ := body["data"].(map[string]any)
	attrs, _ := data["attributes"].(map[string]any)
	for name, want := range map[string]any{
		"endDate":         endDate,
		"promotionIntent": intent,
		"customerEligibilityWaitBetweenOffersInMonths": float64(6),
		"targetSubscriptionPlanType":                   planType,
	} {
		if attrs[name] != want {
			t.Errorf("attributes.%s = %v, want %v", name, attrs[name], want)
		}
	}
}

// TestUpdateWinBackOfferSendsOnlyWhatChanged covers the PATCH body. A member
// omitted keeps Apple's value, so removing an end date has to be an explicit
// null -- omitting it would leave the offer running to its old date -- while
// members that did not change are not sent at all, because Apple refuses some
// changes once an offer is live and a resent unchanged value must not be one.
func TestUpdateWinBackOfferSendsOnlyWhatChanged(t *testing.T) {
	var (
		body   map[string]any
		method string
		path   string
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decoding request body: %s", err)
		}
		fmt.Fprint(w, winBackOfferResponse)
	}))
	defer srv.Close()

	priority := "NORMAL"
	if _, err := newTestClient(srv).UpdateWinBackOffer("10778326500", models.WinBackOfferUpdateAttributes{
		Priority:     &priority,
		ClearEndDate: true,
	}, nil); err != nil {
		t.Fatalf("UpdateWinBackOffer: %s", err)
	}

	if method != "PATCH" || path != "/v1/winBackOffers/10778326500" {
		t.Errorf("request = %s %s, want PATCH /v1/winBackOffers/10778326500", method, path)
	}

	data, _ := body["data"].(map[string]any)
	if data["type"] != "winBackOffers" || data["id"] != "10778326500" {
		t.Errorf("data identifies %v/%v, want winBackOffers/10778326500", data["type"], data["id"])
	}
	if _, present := data["relationships"]; present {
		t.Errorf("update sent relationships, which Apple's update request has none of: %v", data["relationships"])
	}

	attrs, ok := data["attributes"].(map[string]any)
	if !ok {
		t.Fatalf("update has no attributes member: %v", data)
	}
	if len(attrs) != 2 {
		t.Errorf("attributes = %v, want exactly priority and endDate", attrs)
	}
	if attrs["priority"] != "NORMAL" {
		t.Errorf("priority = %v, want NORMAL", attrs["priority"])
	}
	endDate, present := attrs["endDate"]
	if !present || endDate != nil {
		t.Errorf("endDate = %v (present %v), want an explicit null", endDate, present)
	}
}

// TestGetWinBackOfferPricesRequestsIncludes covers the prices read. A
// WinBackOfferPrice has no attributes, so without the include it is an ID and
// two links, and nothing in it can be compared with configuration.
func TestGetWinBackOfferPricesRequestsIncludes(t *testing.T) {
	var (
		path  string
		query url.Values
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, query = r.URL.Path, r.URL.Query()
		fmt.Fprint(w, `{"data":[{"type":"winBackOfferPrices","id":"eyJvIjoiMTA3NzgzMjY1MDAiLCJ0IjoiQ0FOIiwicCI6IjEwMTQyIn0",
			"relationships":{
				"territory":{"data":{"type":"territories","id":"CAN"}},
				"subscriptionPricePoint":{"data":{"type":"subscriptionPricePoints","id":"point-can"}}}}],
			"links":{"self":"x"}}`)
	}))
	defer srv.Close()

	prices, err := newTestClient(srv).GetWinBackOfferPrices("10778326500")
	if err != nil {
		t.Fatalf("GetWinBackOfferPrices: %s", err)
	}

	if path != "/v1/winBackOffers/10778326500/prices" {
		t.Errorf("path = %q, want /v1/winBackOffers/10778326500/prices", path)
	}
	if got := query.Get("include"); got != "territory,subscriptionPricePoint" {
		t.Errorf("include = %q, want territory,subscriptionPricePoint", got)
	}
	if got := query.Get("limit"); got != "200" {
		t.Errorf("limit = %q, want 200", got)
	}

	if len(prices) != 1 || prices[0].Relationships == nil || prices[0].Relationships.SubscriptionPricePoint == nil {
		t.Fatalf("price point relationship was not decoded: %+v", prices)
	}
	if got := prices[0].Relationships.SubscriptionPricePoint.Data.ID; got != "point-can" {
		t.Errorf("price point = %q, want point-can", got)
	}
	if got := prices[0].Relationships.Territory.Data.ID; got != "CAN" {
		t.Errorf("territory = %q, want CAN", got)
	}
}

// TestGetWinBackOffersListsThroughSubscription covers the listing, which is the
// only way to learn which subscription an offer belongs to: the offer itself
// never names it.
func TestGetWinBackOffersListsThroughSubscription(t *testing.T) {
	var path string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		var single map[string]json.RawMessage
		if err := json.Unmarshal([]byte(winBackOfferResponse), &single); err != nil {
			t.Fatalf("fixture: %s", err)
		}
		fmt.Fprintf(w, `{"data":[%s],"links":{"self":"x"}}`, single["data"])
	}))
	defer srv.Close()

	offers, err := newTestClient(srv).GetWinBackOffers("6447497832")
	if err != nil {
		t.Fatalf("GetWinBackOffers: %s", err)
	}

	if path != "/v1/subscriptions/6447497832/winBackOffers" {
		t.Errorf("path = %q, want /v1/subscriptions/6447497832/winBackOffers", path)
	}
	if len(offers) != 1 {
		t.Fatalf("got %d offers, want 1", len(offers))
	}

	attrs := offers[0].Attributes
	if attrs.OfferID != "6Monthfor3_a" || attrs.Duration != "SIX_MONTHS" || attrs.PeriodCount != 1 {
		t.Errorf("decoded attributes = %+v", attrs)
	}
	window := attrs.CustomerEligibilityTimeSinceLastSubscribedInMonths
	if window == nil || window.Minimum == nil || *window.Minimum != 2 || window.Maximum == nil || *window.Maximum != 24 {
		t.Errorf("time-since-last-subscribed = %+v, want 2..24", window)
	}
	if attrs.EndDate == nil || *attrs.EndDate != "2024-07-31" {
		t.Errorf("endDate = %v, want 2024-07-31", attrs.EndDate)
	}
	if attrs.PromotionIntent == nil || *attrs.PromotionIntent != "USE_AUTO_GENERATED_ASSETS" {
		t.Errorf("promotionIntent = %v, want USE_AUTO_GENERATED_ASSETS", attrs.PromotionIntent)
	}
}

// TestGetAndDeleteWinBackOfferPaths covers the two single-record requests.
func TestGetAndDeleteWinBackOfferPaths(t *testing.T) {
	var requests []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		if r.Method == "DELETE" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		fmt.Fprint(w, winBackOfferResponse)
	}))
	defer srv.Close()

	client := newTestClient(srv)

	offer, err := client.GetWinBackOffer("10778326500")
	if err != nil {
		t.Fatalf("GetWinBackOffer: %s", err)
	}
	if offer.Attributes.Priority != "HIGH" {
		t.Errorf("priority = %q, want HIGH", offer.Attributes.Priority)
	}

	if err := client.DeleteWinBackOffer("10778326500", nil); err != nil {
		t.Fatalf("DeleteWinBackOffer: %s", err)
	}

	want := []string{"GET /v1/winBackOffers/10778326500", "DELETE /v1/winBackOffers/10778326500"}
	if fmt.Sprint(requests) != fmt.Sprint(want) {
		t.Errorf("requests = %v, want %v", requests, want)
	}
}
