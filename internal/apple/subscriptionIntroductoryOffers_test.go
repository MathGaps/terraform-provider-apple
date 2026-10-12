// Copyright (c) AO Studio
// SPDX-License-Identifier: MPL-2.0

package apple

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/AhmedOsman00/terraform-provider-apple/internal/apple/models"
)

// introductoryOffersListBody is a realistic page of
// GET /v1/subscriptions/{id}/introductoryOffers with
// include=territory,subscriptionPricePoint.
const introductoryOffersListBody = `{
  "data": [
    {
      "type": "subscriptionIntroductoryOffers",
      "id": "old-usa",
      "attributes": {
        "startDate": "2026-01-01",
        "endDate": "2026-02-01",
        "duration": "ONE_WEEK",
        "offerMode": "FREE_TRIAL",
        "numberOfPeriods": 1
      },
      "relationships": {
        "territory": {"data": {"type": "territories", "id": "USA"}},
        "subscriptionPricePoint": {"links": {"self": "https://api.appstoreconnect.apple.com/v1/..."}}
      },
      "links": {"self": "https://api.appstoreconnect.apple.com/v1/subscriptionIntroductoryOffers/old-usa"}
    },
    {
      "type": "subscriptionIntroductoryOffers",
      "id": "new-usa",
      "attributes": {
        "endDate": "2027-06-30",
        "duration": "ONE_MONTH",
        "offerMode": "PAY_AS_YOU_GO",
        "numberOfPeriods": 3,
        "targetSubscriptionPlanType": "MONTHLY"
      },
      "relationships": {
        "territory": {"data": {"type": "territories", "id": "USA"}},
        "subscriptionPricePoint": {"data": {"type": "subscriptionPricePoints", "id": "point-usa"}}
      }
    },
    {
      "type": "subscriptionIntroductoryOffers",
      "id": "new-gbr",
      "attributes": {
        "endDate": "2027-06-30",
        "duration": "ONE_MONTH",
        "offerMode": "PAY_AS_YOU_GO",
        "numberOfPeriods": 3
      },
      "relationships": {
        "territory": {"data": {"type": "territories", "id": "GBR"}},
        "subscriptionPricePoint": {"data": {"type": "subscriptionPricePoints", "id": "point-gbr"}}
      }
    }
  ],
  "included": [
    {"type": "territories", "id": "USA", "attributes": {"currency": "USD"}},
    {"type": "territories", "id": "GBR", "attributes": {"currency": "GBP"}}
  ],
  "links": {"self": "https://api.appstoreconnect.apple.com/v1/subscriptions/sub-1/introductoryOffers"},
  "meta": {"paging": {"total": 3, "limit": 200}}
}`

// TestGetSubscriptionIntroductoryOffersRequestAndDecode covers the only read
// Apple offers. Without include the territory arrives as a link alone and an
// offer cannot be matched to the territory it was configured for.
func TestGetSubscriptionIntroductoryOffersRequestAndDecode(t *testing.T) {
	var (
		gotMethod string
		gotPath   string
		gotQuery  url.Values
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath, gotQuery = r.Method, r.URL.Path, r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, introductoryOffersListBody)
	}))
	defer srv.Close()

	offers, err := newTestClient(srv).GetSubscriptionIntroductoryOffers("sub-1", []string{"USA", "GBR"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotMethod != "GET" {
		t.Errorf("method = %q, want GET", gotMethod)
	}
	if want := "/v1/subscriptions/sub-1/introductoryOffers"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}
	if got := gotQuery.Get("include"); got != "territory,subscriptionPricePoint" {
		t.Errorf("include = %q, want %q", got, "territory,subscriptionPricePoint")
	}
	if got := gotQuery.Get("filter[territory]"); got != "USA,GBR" {
		t.Errorf("filter[territory] = %q, want %q", got, "USA,GBR")
	}
	if got := gotQuery.Get("limit"); got != "200" {
		t.Errorf("limit = %q, want %q", got, "200")
	}

	if len(offers) != 3 {
		t.Fatalf("got %d offers, want 3", len(offers))
	}

	trial := offers[0]
	if trial.Attributes.OfferMode != models.SubscriptionOfferModeFreeTrial {
		t.Errorf("offerMode = %q, want FREE_TRIAL", trial.Attributes.OfferMode)
	}
	if trial.Attributes.StartDate == nil || *trial.Attributes.StartDate != "2026-01-01" {
		t.Errorf("startDate = %v, want 2026-01-01", trial.Attributes.StartDate)
	}
	// A price point relationship carrying links alone decodes as no price.
	if rel := trial.Relationships.SubscriptionPricePoint; rel == nil || rel.Data.ID != "" {
		t.Errorf("free trial price point = %+v, want a relationship with no data", rel)
	}

	paid := offers[1]
	if paid.Attributes.NumberOfPeriods != 3 {
		t.Errorf("numberOfPeriods = %d, want 3", paid.Attributes.NumberOfPeriods)
	}
	if paid.Attributes.TargetSubscriptionPlanType == nil || *paid.Attributes.TargetSubscriptionPlanType != models.SubscriptionPlanTypeMonthly {
		t.Errorf("targetSubscriptionPlanType = %v, want MONTHLY", paid.Attributes.TargetSubscriptionPlanType)
	}
	if got := paid.Relationships.Territory.Data.ID; got != "USA" {
		t.Errorf("territory = %q, want USA", got)
	}
	if got := paid.Relationships.SubscriptionPricePoint.Data.ID; got != "point-usa" {
		t.Errorf("price point = %q, want point-usa", got)
	}
}

// TestGetSubscriptionIntroductoryOffersOmitsEmptyTerritoryFilter checks that
// an unfiltered read sends no filter rather than an empty one.
func TestGetSubscriptionIntroductoryOffersOmitsEmptyTerritoryFilter(t *testing.T) {
	var gotQuery url.Values

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		fmt.Fprint(w, `{"data":[]}`)
	}))
	defer srv.Close()

	if _, err := newTestClient(srv).GetSubscriptionIntroductoryOffers("sub-1", nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, present := gotQuery["filter[territory]"]; present {
		t.Error("filter[territory] was sent for an unfiltered request")
	}
}

// TestCreateSubscriptionIntroductoryOffersRequestShape covers the bulk write:
// one PATCH of the subscription carrying every new offer inline, a relationship
// that keeps the offers the subscription already had, and a read-back that
// tells the new offers apart from the old one in the same territory.
func TestCreateSubscriptionIntroductoryOffersRequestShape(t *testing.T) {
	var (
		patchBody  map[string]any
		patchCount int
		readQuery  url.Values
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == "GET" && r.URL.Path == "/v1/subscriptions/sub-1/relationships/introductoryOffers":
			fmt.Fprint(w, `{"data":[{"type":"subscriptionIntroductoryOffers","id":"old-usa"}],
				"links":{"self":"x"}}`)
		case r.Method == "PATCH" && r.URL.Path == "/v1/subscriptions/sub-1":
			patchCount++
			raw, _ := io.ReadAll(r.Body)
			if err := json.Unmarshal(raw, &patchBody); err != nil {
				t.Errorf("decoding request body: %s", err)
			}
			fmt.Fprint(w, `{"data":{"type":"subscriptions","id":"sub-1","attributes":{}}}`)
		case r.Method == "GET" && r.URL.Path == "/v1/subscriptions/sub-1/introductoryOffers":
			readQuery = r.URL.Query()
			fmt.Fprint(w, introductoryOffersListBody)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	endDate := "2027-06-30"
	planType := models.SubscriptionPlanTypeMonthly

	created, err := newTestClient(srv).CreateSubscriptionIntroductoryOffers("sub-1",
		models.SubscriptionIntroductoryOfferAttributes{
			EndDate:                    &endDate,
			Duration:                   models.SubscriptionOfferDurationOneMonth,
			OfferMode:                  models.SubscriptionOfferModePayAsYouGo,
			NumberOfPeriods:            3,
			TargetSubscriptionPlanType: &planType,
		},
		[]SubscriptionIntroductoryOfferTerritory{
			{TerritoryID: "USA", PricePointID: "point-usa"},
			{TerritoryID: "GBR", PricePointID: "point-gbr"},
		}, nil)
	if err != nil {
		t.Fatalf("CreateSubscriptionIntroductoryOffers: %s", err)
	}

	if patchCount != 1 {
		t.Fatalf("sent %d PATCH requests, want exactly 1", patchCount)
	}

	data, _ := patchBody["data"].(map[string]any)
	if data["type"] != "subscriptions" || data["id"] != "sub-1" {
		t.Errorf("data identifies %v/%v, want subscriptions/sub-1", data["type"], data["id"])
	}
	if _, present := data["attributes"]; present {
		t.Errorf("an attributes member was sent on the subscription: %v", data["attributes"])
	}

	relationships, _ := data["relationships"].(map[string]any)
	offersRel, _ := relationships["introductoryOffers"].(map[string]any)
	linkage, ok := offersRel["data"].([]any)
	if !ok {
		t.Fatalf("introductoryOffers is not a to-many linkage: %v", relationships)
	}

	// The offer the subscription already had is named first, or a PATCH that
	// replaces the relationship would delete it.
	wantLinkage := []string{"old-usa", "${offer0}", "${offer1}"}
	if len(linkage) != len(wantLinkage) {
		t.Fatalf("got %d linkages, want %d: %v", len(linkage), len(wantLinkage), linkage)
	}
	for i, want := range wantLinkage {
		entry, _ := linkage[i].(map[string]any)
		if entry["type"] != "subscriptionIntroductoryOffers" || entry["id"] != want {
			t.Errorf("linkage %d = %v/%v, want subscriptionIntroductoryOffers/%s", i, entry["type"], entry["id"], want)
		}
	}

	included, _ := patchBody["included"].([]any)
	if len(included) != 2 {
		t.Fatalf("got %d included members, want 2 -- the existing offer must not be re-created", len(included))
	}

	for i, territory := range []string{"USA", "GBR"} {
		entry, _ := included[i].(map[string]any)
		if entry["type"] != "subscriptionIntroductoryOffers" || entry["id"] != fmt.Sprintf("${offer%d}", i) {
			t.Errorf("included %d = %v/%v", i, entry["type"], entry["id"])
		}

		attrs, _ := entry["attributes"].(map[string]any)
		for name, want := range map[string]any{
			"offerMode":                  "PAY_AS_YOU_GO",
			"duration":                   "ONE_MONTH",
			"numberOfPeriods":            float64(3),
			"endDate":                    "2027-06-30",
			"targetSubscriptionPlanType": "MONTHLY",
		} {
			if attrs[name] != want {
				t.Errorf("included %d %s = %v, want %v", i, name, attrs[name], want)
			}
		}
		if _, present := attrs["startDate"]; present {
			t.Errorf("included %d sent a startDate nobody set: %v", i, attrs["startDate"])
		}

		rels, _ := entry["relationships"].(map[string]any)
		for name, want := range map[string][2]string{
			"subscription":           {"subscriptions", "sub-1"},
			"territory":              {"territories", territory},
			"subscriptionPricePoint": {"subscriptionPricePoints", "point-" + map[string]string{"USA": "usa", "GBR": "gbr"}[territory]},
		} {
			rel, _ := rels[name].(map[string]any)
			relData, _ := rel["data"].(map[string]any)
			if relData["type"] != want[0] || relData["id"] != want[1] {
				t.Errorf("included %d %s = %v, want %s/%s", i, name, rel, want[0], want[1])
			}
		}
	}

	if got := readQuery.Get("filter[territory]"); got != "USA,GBR" {
		t.Errorf("read-back filter[territory] = %q, want USA,GBR", got)
	}

	// The USA offer that existed before is not mistaken for the new one.
	if len(created) != 2 {
		t.Fatalf("got %d created offers, want 2: %v", len(created), created)
	}
	if got := created["USA"].ID; got != "new-usa" {
		t.Errorf("created USA offer = %q, want new-usa", got)
	}
	if got := created["GBR"].ID; got != "new-gbr" {
		t.Errorf("created GBR offer = %q, want new-gbr", got)
	}
}

// TestCreateSubscriptionIntroductoryOffersFreeTrialSendsNoPricePoint checks
// that a free trial names its territory and no price point, rather than an
// empty one: ResourceIdentifier marshals an unset value as
// {"type":"","id":""}, which Apple rejects.
func TestCreateSubscriptionIntroductoryOffersFreeTrialSendsNoPricePoint(t *testing.T) {
	var patchBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case "PATCH":
			if err := json.NewDecoder(r.Body).Decode(&patchBody); err != nil {
				t.Errorf("decoding request body: %s", err)
			}
			fmt.Fprint(w, `{"data":{"type":"subscriptions","id":"sub-1","attributes":{}}}`)
		default:
			fmt.Fprint(w, `{"data":[]}`)
		}
	}))
	defer srv.Close()

	if _, err := newTestClient(srv).CreateSubscriptionIntroductoryOffers("sub-1",
		models.SubscriptionIntroductoryOfferAttributes{
			Duration:        models.SubscriptionOfferDurationOneWeek,
			OfferMode:       models.SubscriptionOfferModeFreeTrial,
			NumberOfPeriods: 1,
		},
		[]SubscriptionIntroductoryOfferTerritory{{TerritoryID: "AUS"}}, nil); err != nil {
		t.Fatalf("CreateSubscriptionIntroductoryOffers: %s", err)
	}

	data, _ := patchBody["data"].(map[string]any)
	relationships, _ := data["relationships"].(map[string]any)
	offersRel, _ := relationships["introductoryOffers"].(map[string]any)
	if linkage, _ := offersRel["data"].([]any); len(linkage) != 1 {
		t.Errorf("got linkage %v, want only the new offer on a subscription that had none", offersRel["data"])
	}

	included, _ := patchBody["included"].([]any)
	if len(included) != 1 {
		t.Fatalf("got %d included members, want 1", len(included))
	}
	entry, _ := included[0].(map[string]any)

	attrs, _ := entry["attributes"].(map[string]any)
	if attrs["offerMode"] != "FREE_TRIAL" || attrs["numberOfPeriods"] != float64(1) {
		t.Errorf("attributes = %v", attrs)
	}
	for _, name := range []string{"endDate", "startDate", "targetSubscriptionPlanType"} {
		if _, present := attrs[name]; present {
			t.Errorf("%s sent for an offer that set none: %v", name, attrs[name])
		}
	}

	rels, _ := entry["relationships"].(map[string]any)
	if _, present := rels["subscriptionPricePoint"]; present {
		t.Errorf("price point relationship sent for a free trial: %v", rels["subscriptionPricePoint"])
	}
	territory, _ := rels["territory"].(map[string]any)
	if territoryData, _ := territory["data"].(map[string]any); territoryData["id"] != "AUS" {
		t.Errorf("territory = %v, want AUS", territory)
	}
}

// TestUpdateSubscriptionIntroductoryOfferEndDate covers the one update Apple
// allows, including removing an end date, which only an explicit null does.
func TestUpdateSubscriptionIntroductoryOfferEndDate(t *testing.T) {
	endDate := "2027-01-31"

	for name, tc := range map[string]struct {
		endDate *string
		want    any
	}{
		"set":   {endDate: &endDate, want: "2027-01-31"},
		"clear": {endDate: nil, want: nil},
	} {
		t.Run(name, func(t *testing.T) {
			var (
				method string
				path   string
				body   map[string]any
			)

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				method, path = r.Method, r.URL.Path
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Errorf("decoding request body: %s", err)
				}
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{"data":{"type":"subscriptionIntroductoryOffers","id":"offer-1",
					"attributes":{"duration":"ONE_WEEK","offerMode":"FREE_TRIAL","numberOfPeriods":1}}}`)
			}))
			defer srv.Close()

			offer, err := newTestClient(srv).UpdateSubscriptionIntroductoryOfferEndDate("offer-1", tc.endDate, nil)
			if err != nil {
				t.Fatalf("UpdateSubscriptionIntroductoryOfferEndDate: %s", err)
			}

			if method != "PATCH" || path != "/v1/subscriptionIntroductoryOffers/offer-1" {
				t.Errorf("request = %s %s, want PATCH /v1/subscriptionIntroductoryOffers/offer-1", method, path)
			}

			data, _ := body["data"].(map[string]any)
			if data["type"] != "subscriptionIntroductoryOffers" || data["id"] != "offer-1" {
				t.Errorf("data identifies %v/%v", data["type"], data["id"])
			}
			attrs, _ := data["attributes"].(map[string]any)
			got, present := attrs["endDate"]
			if !present {
				t.Fatalf("endDate member missing from %v -- omitting it leaves the old end date in place", attrs)
			}
			if got != tc.want {
				t.Errorf("endDate = %v, want %v", got, tc.want)
			}
			if len(attrs) != 1 {
				t.Errorf("attributes = %v, want endDate alone -- Apple's update request has no other member", attrs)
			}

			if offer.ID != "offer-1" {
				t.Errorf("decoded offer ID = %q", offer.ID)
			}
		})
	}
}

// TestDeleteSubscriptionIntroductoryOffer covers the per-record delete.
func TestDeleteSubscriptionIntroductoryOffer(t *testing.T) {
	var method, path string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	if err := newTestClient(srv).DeleteSubscriptionIntroductoryOffer("offer-1", nil); err != nil {
		t.Fatalf("DeleteSubscriptionIntroductoryOffer: %s", err)
	}

	if method != "DELETE" || path != "/v1/subscriptionIntroductoryOffers/offer-1" {
		t.Errorf("request = %s %s, want DELETE /v1/subscriptionIntroductoryOffers/offer-1", method, path)
	}
}
