// Copyright (c) AO Studio
// SPDX-License-Identifier: MPL-2.0

package subscription

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/AhmedOsman00/terraform-provider-apple/internal/apple"
	"github.com/AhmedOsman00/terraform-provider-apple/internal/apple/models"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type seenRequest struct {
	method, path, body string
}

// priceScheduleServer answers the reads write() makes and records every
// request. prices is the JSON "data" array of the subscription's price listing.
func priceScheduleServer(t *testing.T, state, prices string) (*apple.Client, func() []seenRequest) {
	t.Helper()
	var mu sync.Mutex
	var seen []seenRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		seen = append(seen, seenRequest{r.Method, r.URL.Path, string(body)})
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/subscriptions/sub1":
			fmt.Fprintf(w, `{"data":{"type":"subscriptions","id":"sub1","attributes":{"state":%q}}}`, state)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/subscriptions/sub1/prices":
			fmt.Fprintf(w, `{"data":%s}`, prices)
		default:
			fmt.Fprint(w, `{"data":{"type":"subscriptionPrices","id":"new"}}`)
		}
	}))
	t.Cleanup(srv.Close)

	client := &apple.Client{HostURL: srv.URL, HTTPClient: srv.Client(), Token: "t"}
	return client, func() []seenRequest {
		mu.Lock()
		defer mu.Unlock()
		return append([]seenRequest(nil), seen...)
	}
}

func priceRecord(id, territory, point string, startDate *string, preserved bool) string {
	start := "null"
	if startDate != nil {
		start = fmt.Sprintf("%q", *startDate)
	}
	return fmt.Sprintf(`{"type":"subscriptionPrices","id":%q,"attributes":{"startDate":%s,"preserved":%t},`+
		`"relationships":{"territory":{"data":{"type":"territories","id":%q}},`+
		`"subscriptionPricePoint":{"data":{"type":"subscriptionPricePoints","id":%q}}}}`,
		id, start, preserved, territory, point)
}

func schedulePlan(prices ...subscriptionSchedulePriceModel) *subscriptionPriceScheduleModel {
	return &subscriptionPriceScheduleModel{SubscriptionID: types.StringValue("sub1"), Prices: prices}
}

func schedulePrice(territory, point string) subscriptionSchedulePriceModel {
	return subscriptionSchedulePriceModel{
		PricePointID:         types.StringValue(point),
		TerritoryID:          types.StringValue(territory),
		StartDate:            types.StringNull(),
		PreserveCurrentPrice: types.BoolNull(),
		PlanType:             types.StringNull(),
	}
}

func TestWriteInMetadataStatePatches(t *testing.T) {
	t.Parallel()
	client, requests := priceScheduleServer(t, "READY_TO_SUBMIT", `[]`)
	r := &subscriptionPriceScheduleResource{client: client}

	var diags diag.Diagnostics
	r.write(context.Background(), schedulePlan(schedulePrice("USA", "pp9")), &diags)
	if diags.HasError() {
		t.Fatalf("write failed: %v", diags)
	}

	var patches int
	for _, req := range requests() {
		if req.method == http.MethodPost {
			t.Errorf("unexpected POST %s", req.path)
		}
		if req.method == http.MethodPatch {
			patches++
			if req.path != "/v1/subscriptions/sub1" {
				t.Errorf("PATCH path = %s", req.path)
			}
		}
	}
	if patches != 1 {
		t.Errorf("PATCH count = %d, want 1", patches)
	}
}

func TestWriteApprovedSubscriptionPostsChangedTerritories(t *testing.T) {
	t.Parallel()
	listing := "[" + strings.Join([]string{
		priceRecord("a", "USA", "pp8", nil, false),
		priceRecord("b", "GBR", "gbp7", nil, false),
	}, ",") + "]"
	client, requests := priceScheduleServer(t, "APPROVED", listing)
	r := &subscriptionPriceScheduleResource{client: client}

	usa := schedulePrice("USA", "pp9")
	usa.PreserveCurrentPrice = types.BoolValue(true)
	usa.StartDate = types.StringValue("2026-12-01")
	gbr := schedulePrice("GBR", "gbp7") // unchanged

	var diags diag.Diagnostics
	r.write(context.Background(), schedulePlan(usa, gbr), &diags)
	if diags.HasError() {
		t.Fatalf("write failed: %v", diags)
	}

	var posts []seenRequest
	for _, req := range requests() {
		if req.method == http.MethodPatch {
			t.Errorf("unexpected PATCH %s", req.path)
		}
		if req.method == http.MethodPost {
			posts = append(posts, req)
		}
	}
	if len(posts) != 1 {
		t.Fatalf("POST count = %d, want 1", len(posts))
	}
	if posts[0].path != "/v1/subscriptionPrices" {
		t.Errorf("POST path = %s", posts[0].path)
	}
	var sent struct {
		Data struct {
			Attributes    map[string]any `json:"attributes"`
			Relationships struct {
				SubscriptionPricePoint struct {
					Data struct{ ID string } `json:"data"`
				} `json:"subscriptionPricePoint"`
			} `json:"relationships"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(posts[0].body), &sent); err != nil {
		t.Fatal(err)
	}
	if got := sent.Data.Relationships.SubscriptionPricePoint.Data.ID; got != "pp9" {
		t.Errorf("price point = %q, want pp9", got)
	}
	if sent.Data.Attributes["preserveCurrentPrice"] != true || sent.Data.Attributes["startDate"] != "2026-12-01" {
		t.Errorf("attributes = %v", sent.Data.Attributes)
	}
}

func TestReconcileIgnoresPreservedAndPendingPrices(t *testing.T) {
	t.Parallel()
	pending := "2026-12-01"
	var reported []models.SubscriptionPrice
	listing := "[" + strings.Join([]string{
		priceRecord("old", "USA", "pp8", nil, true),
		priceRecord("pending", "USA", "pp9", &pending, false),
	}, ",") + "]"
	if err := json.Unmarshal([]byte(listing), &reported); err != nil {
		t.Fatal(err)
	}

	got := reconcileSchedulePrices(nil, reported, "2026-11-12")
	if len(got) != 1 {
		t.Fatalf("got %d prices, want 1: %v", len(got), got)
	}
	want := subscriptionSchedulePriceModel{
		PricePointID:         types.StringValue("pp9"),
		TerritoryID:          types.StringValue("USA"),
		StartDate:            types.StringNull(),
		PreserveCurrentPrice: types.BoolValue(true),
		PlanType:             types.StringNull(),
	}
	if got[0] != want {
		t.Errorf("got %v, want %v", got[0], want)
	}

	// State holding the configured 9 stays as configured, and the preserved 8
	// is not adopted beside it.
	configured := want
	if got := reconcileSchedulePrices([]subscriptionSchedulePriceModel{configured}, reported, "2026-11-12"); len(got) != 1 || got[0] != configured {
		t.Errorf("state not kept: %v", got)
	}
}

func TestWriteApprovedKeepsPlansApart(t *testing.T) {
	t.Parallel()
	monthly := `{"type":"subscriptionPrices","id":"m","attributes":{"startDate":null,"preserved":false,"planType":"MONTHLY"},` +
		`"relationships":{"territory":{"data":{"type":"territories","id":"USA"}},"subscriptionPricePoint":{"data":{"type":"subscriptionPricePoints","id":"pp9"}}}}`
	upfront := `{"type":"subscriptionPrices","id":"u","attributes":{"startDate":null,"preserved":false,"planType":"UPFRONT"},` +
		`"relationships":{"territory":{"data":{"type":"territories","id":"USA"}},"subscriptionPricePoint":{"data":{"type":"subscriptionPricePoints","id":"up1"}}}}`
	client, requests := priceScheduleServer(t, "APPROVED", "["+monthly+","+upfront+"]")
	r := &subscriptionPriceScheduleResource{client: client}

	m := schedulePrice("USA", "pp9")
	m.PlanType = types.StringValue("MONTHLY")
	u := schedulePrice("USA", "up1") // plan unset reads as UPFRONT, as Apple reports it

	var diags diag.Diagnostics
	r.write(context.Background(), schedulePlan(m, u), &diags)
	if diags.HasError() {
		t.Fatalf("write failed: %v", diags)
	}
	for _, req := range requests() {
		if req.method == http.MethodPost || req.method == http.MethodPatch {
			t.Errorf("unexpected %s %s", req.method, req.path)
		}
	}
}

func TestWriteApprovedUndatedChangeCarriesAStartDate(t *testing.T) {
	t.Parallel()
	listing := "[" + priceRecord("a", "USA", "pp8", nil, false) + "]"
	client, requests := priceScheduleServer(t, "APPROVED", listing)
	r := &subscriptionPriceScheduleResource{client: client}

	var diags diag.Diagnostics
	r.write(context.Background(), schedulePlan(schedulePrice("USA", "pp9")), &diags)
	if diags.HasError() {
		t.Fatalf("write failed: %v", diags)
	}

	var posts []seenRequest
	for _, req := range requests() {
		if req.method == http.MethodPost {
			posts = append(posts, req)
		}
	}
	if len(posts) != 1 {
		t.Fatalf("POST count = %d, want 1", len(posts))
	}
	var sent struct {
		Data struct {
			Attributes map[string]any `json:"attributes"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(posts[0].body), &sent); err != nil {
		t.Fatal(err)
	}
	if got, _ := sent.Data.Attributes["startDate"].(string); got == "" {
		t.Errorf("startDate missing from %s: Apple reads an undated price as an initial price", posts[0].body)
	}
}
