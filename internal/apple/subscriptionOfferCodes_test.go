// Copyright (c) AO Studio
// SPDX-License-Identifier: MPL-2.0

package apple

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"

	"github.com/AhmedOsman00/terraform-provider-apple/internal/apple/models"
)

// offerCodeRequest is what the test server saw.
type offerCodeRequest struct {
	method string
	path   string
	query  url.Values
	accept string
	body   []byte
}

// newOfferCodeServer records the one request a test sends and answers it with
// the given status, content type and body.
func newOfferCodeServer(t *testing.T, status int, contentType, response string) (*httptest.Server, *offerCodeRequest) {
	t.Helper()
	got := &offerCodeRequest{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("reading request body: %v", err)
		}
		got.method = r.Method
		got.path = r.URL.Path
		got.query = r.URL.Query()
		got.accept = r.Header.Get("Accept")
		got.body = body

		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(status)
		_, _ = io.WriteString(w, response)
	}))
	t.Cleanup(srv.Close)

	return srv, got
}

// assertJSONBody compares the request body Apple would receive against the
// expected document, structurally: key order and whitespace do not matter,
// every member and every value does -- including a member that must be absent.
func assertJSONBody(t *testing.T, got []byte, want string) {
	t.Helper()

	var gotDoc, wantDoc interface{}
	if err := json.Unmarshal(got, &gotDoc); err != nil {
		t.Fatalf("request body is not JSON: %v\n%s", err, got)
	}
	if err := json.Unmarshal([]byte(want), &wantDoc); err != nil {
		t.Fatalf("expected body is not JSON: %v", err)
	}

	if !reflect.DeepEqual(gotDoc, wantDoc) {
		gotPretty, _ := json.MarshalIndent(gotDoc, "", "  ")
		wantPretty, _ := json.MarshalIndent(wantDoc, "", "  ")
		t.Errorf("request body mismatch\n got: %s\nwant: %s", gotPretty, wantPretty)
	}
}

// TestCreateSubscriptionOfferCodeSendsOfferWithInlinePrices covers the one
// write that sets an offer's terms. The prices do not exist yet, so each goes
// inline under a placeholder the relationship references; a paid territory
// names its price point and a territory with no price point omits the member
// rather than sending it empty.
func TestCreateSubscriptionOfferCodeSendsOfferWithInlinePrices(t *testing.T) {
	srv, got := newOfferCodeServer(t, http.StatusCreated, "application/json", `{"data":{
		"type":"subscriptionOfferCodes","id":"oc-1",
		"attributes":{"name":"School 2026","customerEligibilities":["NEW","EXPIRED"],
			"offerEligibility":"STACK_WITH_INTRO_OFFERS","duration":"ONE_MONTH","offerMode":"PAY_AS_YOU_GO",
			"numberOfPeriods":3,"totalNumberOfCodes":0,"productionCodeCount":0,"sandboxCodeCount":0,
			"active":true,"autoRenewEnabled":true,"targetSubscriptionPlanType":"MONTHLY"}},
		"links":{"self":"x"}}`)

	autoRenew := true
	offer, err := newTestClient(srv).CreateSubscriptionOfferCode("sub-1", models.SubscriptionOfferCodeCreateAttributes{
		Name:                  "School 2026",
		CustomerEligibilities: []string{"EXPIRED", "NEW"},
		OfferEligibility:      "STACK_WITH_INTRO_OFFERS",
		Duration:              "ONE_MONTH",
		OfferMode:             "PAY_AS_YOU_GO",
		NumberOfPeriods:       3,
		AutoRenewEnabled:      &autoRenew,
	}, []SubscriptionOfferCodePriceInput{
		{TerritoryID: "AUS", PricePointID: "pp-aus"},
		{TerritoryID: "USA"},
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.method != http.MethodPost || got.path != "/v1/subscriptionOfferCodes" {
		t.Errorf("request = %s %s, want POST /v1/subscriptionOfferCodes", got.method, got.path)
	}

	assertJSONBody(t, got.body, `{
		"data": {
			"type": "subscriptionOfferCodes",
			"attributes": {
				"name": "School 2026",
				"customerEligibilities": ["EXPIRED", "NEW"],
				"offerEligibility": "STACK_WITH_INTRO_OFFERS",
				"duration": "ONE_MONTH",
				"offerMode": "PAY_AS_YOU_GO",
				"numberOfPeriods": 3,
				"autoRenewEnabled": true
			},
			"relationships": {
				"subscription": {"data": {"type": "subscriptions", "id": "sub-1"}},
				"prices": {"data": [
					{"type": "subscriptionOfferCodePrices", "id": "${price0}"},
					{"type": "subscriptionOfferCodePrices", "id": "${price1}"}
				]}
			}
		},
		"included": [
			{
				"type": "subscriptionOfferCodePrices",
				"id": "${price0}",
				"relationships": {
					"territory": {"data": {"type": "territories", "id": "AUS"}},
					"subscriptionPricePoint": {"data": {"type": "subscriptionPricePoints", "id": "pp-aus"}}
				}
			},
			{
				"type": "subscriptionOfferCodePrices",
				"id": "${price1}",
				"relationships": {
					"territory": {"data": {"type": "territories", "id": "USA"}}
				}
			}
		]
	}`)

	if offer.ID != "oc-1" || offer.Attributes.Active == nil || !*offer.Attributes.Active {
		t.Errorf("decoded offer = %+v, want oc-1 active", offer)
	}
	if offer.Attributes.NumberOfPeriods == nil || *offer.Attributes.NumberOfPeriods != 3 {
		t.Errorf("numberOfPeriods not decoded: %+v", offer.Attributes)
	}
	if offer.Attributes.TargetSubscriptionPlanType == nil || *offer.Attributes.TargetSubscriptionPlanType != "MONTHLY" {
		t.Errorf("targetSubscriptionPlanType not decoded: %+v", offer.Attributes)
	}
}

// TestSetOfferCodeActiveSendsFalse covers the deactivation every destroy
// performs. "active": false has to be on the wire -- an omitempty bool would
// send an empty attributes object, which deactivates nothing.
func TestSetOfferCodeActiveSendsFalse(t *testing.T) {
	cases := []struct {
		name     string
		call     func(c *Client) error
		wantPath string
		wantType string
	}{
		{
			name: "offer code",
			call: func(c *Client) error {
				_, err := c.SetSubscriptionOfferCodeActive("id-1", false, nil)
				return err
			},
			wantPath: "/v1/subscriptionOfferCodes/id-1",
			wantType: "subscriptionOfferCodes",
		},
		{
			name: "custom code",
			call: func(c *Client) error {
				_, err := c.SetSubscriptionOfferCodeCustomCodeActive("id-1", false, nil)
				return err
			},
			wantPath: "/v1/subscriptionOfferCodeCustomCodes/id-1",
			wantType: "subscriptionOfferCodeCustomCodes",
		},
		{
			name: "one-time-use codes",
			call: func(c *Client) error {
				_, err := c.SetSubscriptionOfferCodeOneTimeUseCodeActive("id-1", false, nil)
				return err
			},
			wantPath: "/v1/subscriptionOfferCodeOneTimeUseCodes/id-1",
			wantType: "subscriptionOfferCodeOneTimeUseCodes",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, got := newOfferCodeServer(t, http.StatusOK, "application/json",
				`{"data":{"type":"`+tc.wantType+`","id":"id-1","attributes":{"active":false}},"links":{"self":"x"}}`)

			if err := tc.call(newTestClient(srv)); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got.method != http.MethodPatch || got.path != tc.wantPath {
				t.Errorf("request = %s %s, want PATCH %s", got.method, got.path, tc.wantPath)
			}
			assertJSONBody(t, got.body,
				`{"data":{"type":"`+tc.wantType+`","id":"id-1","attributes":{"active":false}}}`)
		})
	}
}

// TestGetSubscriptionOfferCodeIncludesSubscription covers the read import
// depends on: without include=subscription Apple reports the relationship as
// links alone, and an imported offer would have no subscription_id.
func TestGetSubscriptionOfferCodeIncludesSubscription(t *testing.T) {
	srv, got := newOfferCodeServer(t, http.StatusOK, "application/json", `{"data":{
		"type":"subscriptionOfferCodes","id":"oc-1",
		"attributes":{"name":"Spring","customerEligibilities":["EXISTING"],"active":false,
			"offerMode":"FREE_TRIAL","duration":"ONE_WEEK","numberOfPeriods":1,
			"totalNumberOfCodes":500,"productionCodeCount":500,"sandboxCodeCount":0},
		"relationships":{"subscription":{"data":{"type":"subscriptions","id":"sub-9"}}}},
		"included":[{"type":"subscriptions","id":"sub-9","attributes":{"name":"Pro"}}],
		"links":{"self":"x"}}`)

	offer, err := newTestClient(srv).GetSubscriptionOfferCode("oc-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.method != http.MethodGet || got.path != "/v1/subscriptionOfferCodes/oc-1" {
		t.Errorf("request = %s %s", got.method, got.path)
	}
	if inc := got.query.Get("include"); inc != "subscription" {
		t.Errorf("include = %q, want subscription", inc)
	}

	if offer.Relationships == nil || offer.Relationships.Subscription == nil ||
		offer.Relationships.Subscription.Data.ID != "sub-9" {
		t.Fatalf("subscription linkage not decoded: %+v", offer.Relationships)
	}
	if offer.Attributes.Active == nil || *offer.Attributes.Active {
		t.Errorf("active = %v, want false", offer.Attributes.Active)
	}
	if offer.Attributes.ProductionCodeCount == nil || *offer.Attributes.ProductionCodeCount != 500 {
		t.Errorf("productionCodeCount not decoded: %+v", offer.Attributes)
	}
}

// TestGetSubscriptionOfferCodePricesIncludesLinkages covers the price read an
// import adopts: both linkages are populated only on request.
func TestGetSubscriptionOfferCodePricesIncludesLinkages(t *testing.T) {
	srv, got := newOfferCodeServer(t, http.StatusOK, "application/json", `{"data":[
		{"type":"subscriptionOfferCodePrices","id":"p1","relationships":{
			"territory":{"data":{"type":"territories","id":"AUS"}},
			"subscriptionPricePoint":{"data":{"type":"subscriptionPricePoints","id":"pp-aus"}}}}],
		"links":{"self":"x"}}`)

	prices, err := newTestClient(srv).GetSubscriptionOfferCodePrices("oc-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.path != "/v1/subscriptionOfferCodes/oc-1/prices" {
		t.Errorf("path = %q", got.path)
	}
	if inc := got.query.Get("include"); inc != "territory,subscriptionPricePoint" {
		t.Errorf("include = %q, want territory,subscriptionPricePoint", inc)
	}
	if got.query.Get("limit") != "200" {
		t.Errorf("limit = %q, want 200", got.query.Get("limit"))
	}

	if len(prices) != 1 || prices[0].Relationships == nil ||
		prices[0].Relationships.SubscriptionPricePoint == nil ||
		prices[0].Relationships.SubscriptionPricePoint.Data.ID != "pp-aus" {
		t.Errorf("prices not decoded: %+v", prices)
	}
}

// TestGetSubscriptionOfferCodesListsThroughSubscription covers the listing:
// Apple publishes no top-level collection, so it hangs off the subscription.
func TestGetSubscriptionOfferCodesListsThroughSubscription(t *testing.T) {
	srv, got := newOfferCodeServer(t, http.StatusOK, "application/json", `{"data":[
		{"type":"subscriptionOfferCodes","id":"oc-1","attributes":{"name":"A","active":true}},
		{"type":"subscriptionOfferCodes","id":"oc-2","attributes":{"name":"B","active":false}}],
		"links":{"self":"x"}}`)

	offers, err := newTestClient(srv).GetSubscriptionOfferCodes("sub-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.method != http.MethodGet || got.path != "/v1/subscriptions/sub-1/offerCodes" {
		t.Errorf("request = %s %s", got.method, got.path)
	}
	if len(offers) != 2 {
		t.Errorf("got %d offers, want 2", len(offers))
	}
}

// TestCreateCustomCodeOmitsUnsetExpiration covers the custom code create: an
// expiration date is nullable, and a code without one never expires -- so an
// unset one must be absent, not "".
func TestCreateCustomCodeOmitsUnsetExpiration(t *testing.T) {
	srv, got := newOfferCodeServer(t, http.StatusCreated, "application/json", `{"data":{
		"type":"subscriptionOfferCodeCustomCodes","id":"cc-1",
		"attributes":{"customCode":"SCHOOL2026","numberOfCodes":1000,"createdDate":"2026-10-12T00:00:00Z","active":true}},
		"links":{"self":"x"}}`)

	code, err := newTestClient(srv).CreateSubscriptionOfferCodeCustomCode("oc-1",
		models.SubscriptionOfferCodeCustomCodeCreateAttrs{CustomCode: "SCHOOL2026", NumberOfCodes: 1000}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.method != http.MethodPost || got.path != "/v1/subscriptionOfferCodeCustomCodes" {
		t.Errorf("request = %s %s", got.method, got.path)
	}
	assertJSONBody(t, got.body, `{"data":{
		"type":"subscriptionOfferCodeCustomCodes",
		"attributes":{"customCode":"SCHOOL2026","numberOfCodes":1000},
		"relationships":{"offerCode":{"data":{"type":"subscriptionOfferCodes","id":"oc-1"}}}}}`)

	if code.ID != "cc-1" || code.Attributes.CreatedDate == nil {
		t.Errorf("decoded code = %+v", code)
	}
}

// TestGetCustomCodeIncludesOfferCode covers the read a bare-ID import of a
// custom code depends on.
func TestGetCustomCodeIncludesOfferCode(t *testing.T) {
	srv, got := newOfferCodeServer(t, http.StatusOK, "application/json", `{"data":{
		"type":"subscriptionOfferCodeCustomCodes","id":"cc-1",
		"attributes":{"customCode":"SCHOOL2026","numberOfCodes":1000,"expirationDate":"2027-03-31","active":true},
		"relationships":{"offerCode":{"data":{"type":"subscriptionOfferCodes","id":"oc-1"}}}},
		"links":{"self":"x"}}`)

	code, err := newTestClient(srv).GetSubscriptionOfferCodeCustomCode("cc-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.path != "/v1/subscriptionOfferCodeCustomCodes/cc-1" || got.query.Get("include") != "offerCode" {
		t.Errorf("request = %s?%s", got.path, got.query.Encode())
	}
	if code.Relationships == nil || code.Relationships.OfferCode == nil || code.Relationships.OfferCode.Data.ID != "oc-1" {
		t.Errorf("offer linkage not decoded: %+v", code.Relationships)
	}
	if code.Attributes.ExpirationDate == nil || *code.Attributes.ExpirationDate != "2027-03-31" {
		t.Errorf("expirationDate not decoded: %+v", code.Attributes)
	}
}

// TestCreateOneTimeUseCodesSendsEnvironment covers the batch create, whose
// expiration date is required and whose environment decides the batch size
// Apple accepts.
func TestCreateOneTimeUseCodesSendsEnvironment(t *testing.T) {
	srv, got := newOfferCodeServer(t, http.StatusCreated, "application/json", `{"data":{
		"type":"subscriptionOfferCodeOneTimeUseCodes","id":"otu-1",
		"attributes":{"numberOfCodes":10,"expirationDate":"2027-01-31","environment":"SANDBOX","active":true}},
		"links":{"self":"x"}}`)

	environment := models.OfferCodeEnvironmentSandbox
	batch, err := newTestClient(srv).CreateSubscriptionOfferCodeOneTimeUseCode("oc-1",
		models.SubscriptionOfferCodeOneTimeUseCodeCreateAttrs{
			NumberOfCodes: 10, ExpirationDate: "2027-01-31", Environment: &environment,
		}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.method != http.MethodPost || got.path != "/v1/subscriptionOfferCodeOneTimeUseCodes" {
		t.Errorf("request = %s %s", got.method, got.path)
	}
	assertJSONBody(t, got.body, `{"data":{
		"type":"subscriptionOfferCodeOneTimeUseCodes",
		"attributes":{"numberOfCodes":10,"expirationDate":"2027-01-31","environment":"SANDBOX"},
		"relationships":{"offerCode":{"data":{"type":"subscriptionOfferCodes","id":"oc-1"}}}}}`)

	if batch.Attributes.Environment == nil || *batch.Attributes.Environment != "SANDBOX" {
		t.Errorf("environment not decoded: %+v", batch.Attributes)
	}
}

// TestGetOneTimeUseCodeValuesReturnsCSV covers the values download, whose
// success body Apple's spec gives as text/csv rather than JSON: the request
// must accept CSV, and the document must come back unmodified.
func TestGetOneTimeUseCodeValuesReturnsCSV(t *testing.T) {
	const document = "Code\nABCD1234EFGH5678IJ\nKLMN9012OPQR3456ST\n"
	srv, got := newOfferCodeServer(t, http.StatusOK, "text/csv", document)

	csv, err := newTestClient(srv).GetSubscriptionOfferCodeOneTimeUseCodeValues("otu-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.method != http.MethodGet || got.path != "/v1/subscriptionOfferCodeOneTimeUseCodes/otu-1/values" {
		t.Errorf("request = %s %s", got.method, got.path)
	}
	if got.accept != "text/csv, application/json" {
		t.Errorf("Accept = %q, want text/csv first", got.accept)
	}
	if csv != document {
		t.Errorf("csv = %q, want the document unmodified", csv)
	}
}

// TestGetOneTimeUseCodeValuesReportsAppleError checks that an error on the CSV
// endpoint still surfaces Apple's own JSON message.
func TestGetOneTimeUseCodeValuesReportsAppleError(t *testing.T) {
	srv, _ := newOfferCodeServer(t, http.StatusNotFound, "application/json",
		`{"errors":[{"status":"404","code":"NOT_FOUND","title":"The specified resource does not exist","detail":"no batch"}]}`)

	_, err := newTestClient(srv).GetSubscriptionOfferCodeOneTimeUseCodeValues("missing")
	if err == nil {
		t.Fatal("expected an error")
	}
	if want := "API error (status 404): The specified resource does not exist - no batch"; err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}
