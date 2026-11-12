// Copyright (c) AO Studio
// SPDX-License-Identifier: MPL-2.0

package subscription

import (
	"testing"

	"github.com/AhmedOsman00/terraform-provider-apple/internal/apple/models"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func state(s models.SubscriptionState) *models.SubscriptionState { return &s }
func period(p models.SubscriptionPeriod) *models.SubscriptionPeriod {
	return &p
}
func level(i int) *int { return &i }

func subscriptions() []models.Subscription {
	return []models.Subscription{
		{ID: "1", Attributes: models.SubscriptionAttributes{
			Name:               "Pro Monthly",
			ProductID:          "com.example.app.pro.monthly",
			State:              state(models.SubscriptionStateApproved),
			SubscriptionPeriod: period(models.SubscriptionPeriodOneMonth),
			GroupLevel:         level(1),
		}},
		{ID: "2", Attributes: models.SubscriptionAttributes{
			Name:               "Pro Yearly",
			ProductID:          "com.example.app.pro.yearly",
			State:              state(models.SubscriptionStateReadyToSubmit),
			SubscriptionPeriod: period(models.SubscriptionPeriodOneYear),
			GroupLevel:         level(1),
		}},
		{ID: "3", Attributes: models.SubscriptionAttributes{
			Name:               "Basic Monthly",
			ProductID:          "com.example.app.basic.monthly",
			State:              state(models.SubscriptionStateMissingMetadata),
			SubscriptionPeriod: period(models.SubscriptionPeriodOneMonth),
			GroupLevel:         level(2),
		}},
		// Apple omits attributes it has no value for. A subscription without a
		// state or period must not match a filter that asks for one, and must
		// not panic the sort comparators either.
		{ID: "4", Attributes: models.SubscriptionAttributes{
			Name:      "Bare",
			ProductID: "com.example.app.bare",
		}},
	}
}

func productIDs(list []models.Subscription) []string {
	out := make([]string, 0, len(list))
	for _, s := range list {
		out = append(out, s.Attributes.ProductID)
	}
	return out
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestFilterSubscriptions(t *testing.T) {
	tests := []struct {
		name    string
		filters SubscriptionFilters
		want    []string
	}{
		{
			name: "no filters returns everything",
			want: []string{
				"com.example.app.pro.monthly",
				"com.example.app.pro.yearly",
				"com.example.app.basic.monthly",
				"com.example.app.bare",
			},
		},
		{
			name:    "name pattern matches as substring",
			filters: SubscriptionFilters{NamePattern: "Pro"},
			want:    []string{"com.example.app.pro.monthly", "com.example.app.pro.yearly"},
		},
		{
			name:    "anchored name pattern",
			filters: SubscriptionFilters{NamePattern: "^Basic Monthly$"},
			want:    []string{"com.example.app.basic.monthly"},
		},
		{
			name:    "product id pattern",
			filters: SubscriptionFilters{ProductIDPattern: `\.monthly$`},
			want:    []string{"com.example.app.pro.monthly", "com.example.app.basic.monthly"},
		},
		{
			name:    "state is an exact match",
			filters: SubscriptionFilters{State: string(models.SubscriptionStateApproved)},
			want:    []string{"com.example.app.pro.monthly"},
		},
		{
			name:    "period is an exact match",
			filters: SubscriptionFilters{Period: string(models.SubscriptionPeriodOneYear)},
			want:    []string{"com.example.app.pro.yearly"},
		},
		{
			name: "filters combine",
			filters: SubscriptionFilters{
				ProductIDPattern: `^com\.example\.app\.pro`,
				Period:           string(models.SubscriptionPeriodOneMonth),
			},
			want: []string{"com.example.app.pro.monthly"},
		},
		{
			name:    "a subscription without a state never matches a state filter",
			filters: SubscriptionFilters{State: string(models.SubscriptionStateMissingMetadata)},
			want:    []string{"com.example.app.basic.monthly"},
		},
		{
			name:    "no matches yields an empty result rather than everything",
			filters: SubscriptionFilters{NamePattern: "nothing matches this"},
			want:    []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FilterSubscriptions(subscriptions(), tt.filters)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !equalStrings(productIDs(got), tt.want) {
				t.Errorf("got %v, want %v", productIDs(got), tt.want)
			}
		})
	}
}

func TestFilterSubscriptionsRejectsMalformedPatterns(t *testing.T) {
	tests := []struct {
		name    string
		filters SubscriptionFilters
	}{
		{name: "name pattern", filters: SubscriptionFilters{NamePattern: "["}},
		{name: "product id pattern", filters: SubscriptionFilters{ProductIDPattern: "(unclosed"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := FilterSubscriptions(subscriptions(), tt.filters); err == nil {
				t.Fatal("expected an error for a malformed pattern, got nil")
			}
		})
	}
}

func TestSortSubscriptions(t *testing.T) {
	tests := []struct {
		name      string
		sortBy    string
		sortOrder string
		want      []string
	}{
		{
			name: "defaults to product id ascending",
			want: []string{
				"com.example.app.bare",
				"com.example.app.basic.monthly",
				"com.example.app.pro.monthly",
				"com.example.app.pro.yearly",
			},
		},
		{
			name:      "product id descending",
			sortBy:    "product_id",
			sortOrder: "desc",
			want: []string{
				"com.example.app.pro.yearly",
				"com.example.app.pro.monthly",
				"com.example.app.basic.monthly",
				"com.example.app.bare",
			},
		},
		{
			name:   "name is case-insensitive",
			sortBy: "name",
			want: []string{
				"com.example.app.bare",
				"com.example.app.basic.monthly",
				"com.example.app.pro.monthly",
				"com.example.app.pro.yearly",
			},
		},
		{
			name:   "group level, with a missing level treated as 1",
			sortBy: "group_level",
			want: []string{
				"com.example.app.pro.monthly",
				"com.example.app.pro.yearly",
				"com.example.app.bare",
				"com.example.app.basic.monthly",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			list := subscriptions()
			SortSubscriptions(list, tt.sortBy, tt.sortOrder)
			if !equalStrings(productIDs(list), tt.want) {
				t.Errorf("got %v, want %v", productIDs(list), tt.want)
			}
		})
	}
}

func TestLimitIsAppliedAfterSorting(t *testing.T) {
	// The data sources limit after sorting rather than before, so a limit of 1
	// must yield the first record in sort order, not an arbitrary one.
	list := subscriptions()
	SortSubscriptions(list, "product_id", "asc")
	if got := productIDs(list)[:1]; !equalStrings(got, []string{"com.example.app.bare"}) {
		t.Errorf("got %v, want [com.example.app.bare]", got)
	}
}

func groups() []models.SubscriptionGroup {
	return []models.SubscriptionGroup{
		{ID: "10", Attributes: models.SubscriptionGroupAttributes{ReferenceName: "Premium"}},
		{ID: "11", Attributes: models.SubscriptionGroupAttributes{ReferenceName: "legacy tiers"}},
		{ID: "12", Attributes: models.SubscriptionGroupAttributes{ReferenceName: "Trials"}},
	}
}

func referenceNames(list []models.SubscriptionGroup) []string {
	out := make([]string, 0, len(list))
	for _, g := range list {
		out = append(out, g.Attributes.ReferenceName)
	}
	return out
}

func TestFilterSubscriptionGroups(t *testing.T) {
	tests := []struct {
		name    string
		filters SubscriptionGroupFilters
		want    []string
	}{
		{
			name: "no filters returns everything",
			want: []string{"Premium", "legacy tiers", "Trials"},
		},
		{
			name:    "substring match",
			filters: SubscriptionGroupFilters{ReferenceNamePattern: "ium"},
			want:    []string{"Premium"},
		},
		{
			name:    "anchored match",
			filters: SubscriptionGroupFilters{ReferenceNamePattern: "^Trials$"},
			want:    []string{"Trials"},
		},
		{
			name:    "matching is case-sensitive unless the pattern says otherwise",
			filters: SubscriptionGroupFilters{ReferenceNamePattern: "LEGACY"},
			want:    []string{},
		},
		{
			name:    "case-insensitive via an inline flag",
			filters: SubscriptionGroupFilters{ReferenceNamePattern: "(?i)LEGACY"},
			want:    []string{"legacy tiers"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FilterSubscriptionGroups(groups(), tt.filters)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !equalStrings(referenceNames(got), tt.want) {
				t.Errorf("got %v, want %v", referenceNames(got), tt.want)
			}
		})
	}
}

func TestFilterSubscriptionGroupsRejectsMalformedPattern(t *testing.T) {
	if _, err := FilterSubscriptionGroups(groups(), SubscriptionGroupFilters{ReferenceNamePattern: "["}); err == nil {
		t.Fatal("expected an error for a malformed pattern, got nil")
	}
}

func TestSortSubscriptionGroups(t *testing.T) {
	// Sorting is case-insensitive, so "legacy tiers" sorts between "Premium"
	// and "Trials" rather than after both as a byte comparison would put it.
	list := groups()
	SortSubscriptionGroups(list, "reference_name", "asc")
	if want := []string{"legacy tiers", "Premium", "Trials"}; !equalStrings(referenceNames(list), want) {
		t.Errorf("got %v, want %v", referenceNames(list), want)
	}

	SortSubscriptionGroups(list, "id", "desc")
	if want := []string{"Trials", "legacy tiers", "Premium"}; !equalStrings(referenceNames(list), want) {
		t.Errorf("got %v, want %v", referenceNames(list), want)
	}
}

func pricePoints() []models.SubscriptionPricePoint {
	return []models.SubscriptionPricePoint{
		{ID: "p1", Attributes: models.SubscriptionPricePointAttributes{CustomerPrice: "9.99", Proceeds: "6.99"}},
		{ID: "p2", Attributes: models.SubscriptionPricePointAttributes{CustomerPrice: "19.99", Proceeds: "13.99"}},
		{ID: "p3", Attributes: models.SubscriptionPricePointAttributes{CustomerPrice: "9.99", Proceeds: "7.49"}},
	}
}

func TestFilterSubscriptionPricePoints(t *testing.T) {
	tests := []struct {
		name    string
		filters SubscriptionPricePointFilters
		want    []string
	}{
		{
			name: "no filter returns everything",
			want: []string{"p1", "p2", "p3"},
		},
		{
			name:    "exact price match can return several territories",
			filters: SubscriptionPricePointFilters{CustomerPrice: "9.99"},
			want:    []string{"p1", "p3"},
		},
		{
			name:    "a price that is not in the catalogue matches nothing",
			filters: SubscriptionPricePointFilters{CustomerPrice: "9.9"},
			want:    []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FilterSubscriptionPricePoints(pricePoints(), tt.filters)
			ids := make([]string, 0, len(got))
			for _, p := range got {
				ids = append(ids, p.ID)
			}
			if !equalStrings(ids, tt.want) {
				t.Errorf("got %v, want %v", ids, tt.want)
			}
		})
	}
}

// territoryPoint builds a price point carrying a territory linkage, the shape
// an equalization read returns.
func territoryPoint(id, territory string) models.SubscriptionPricePoint {
	return models.SubscriptionPricePoint{
		ID: id,
		Relationships: &models.SubscriptionPricePointRelationships{
			Territory: &models.ResourceIdentifier{
				Data: models.ResourceData{Type: "territories", ID: territory},
			},
		},
	}
}

func TestPricePointIDsByTerritory(t *testing.T) {
	tests := []struct {
		name   string
		points []models.SubscriptionPricePoint
		want   map[string]string
	}{
		{
			name:   "no points is an empty map, not nil",
			points: nil,
			want:   map[string]string{},
		},
		{
			name: "one entry per territory",
			points: []models.SubscriptionPricePoint{
				territoryPoint("eq-gbr", "GBR"),
				territoryPoint("eq-egy", "EGY"),
			},
			want: map[string]string{"GBR": "eq-gbr", "EGY": "eq-egy"},
		},
		{
			name:   "a point with no territory linkage is skipped",
			points: append(pricePoints(), territoryPoint("eq-gbr", "GBR")),
			want:   map[string]string{"GBR": "eq-gbr"},
		},
		{
			name: "an empty territory ID is skipped rather than keyed on the empty string",
			points: []models.SubscriptionPricePoint{
				territoryPoint("eq-blank", ""),
				territoryPoint("eq-gbr", "GBR"),
			},
			want: map[string]string{"GBR": "eq-gbr"},
		},
		{
			name: "a repeated territory keeps the first, so response order cannot change the result",
			points: []models.SubscriptionPricePoint{
				territoryPoint("eq-gbr", "GBR"),
				territoryPoint("eq-gbr-again", "GBR"),
			},
			want: map[string]string{"GBR": "eq-gbr"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PricePointIDsByTerritory(tt.points)

			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for territory, id := range tt.want {
				if got[territory] != id {
					t.Errorf("territory %s = %q, want %q", territory, got[territory], id)
				}
			}
		})
	}
}

// reportedPrice builds one price as Apple returns it from
// GET /v1/subscriptions/{id}/prices with include=territory,subscriptionPricePoint.
func reportedPrice(id, pointID, territory string, startDate *string, planType *models.SubscriptionPlanType) models.SubscriptionPrice {
	return models.SubscriptionPrice{
		Type: "subscriptionPrices",
		ID:   id,
		Attributes: models.SubscriptionPriceAttributes{
			StartDate: startDate,
			PlanType:  planType,
		},
		Relationships: &models.SubscriptionPriceRelationships{
			Territory: &models.ResourceIdentifier{
				Data: models.ResourceData{Type: "territories", ID: territory},
			},
			SubscriptionPricePoint: &models.ResourceIdentifier{
				Data: models.ResourceData{Type: "subscriptionPricePoints", ID: pointID},
			},
		},
	}
}

// TestReconcileSchedulePricesKeepsConfiguredValues covers the reason this
// function exists: Apple reports a territory and a plan type on every price
// whether or not one was sent, so adopting what it returns would show as a
// permanent diff against a configuration that set neither.
func TestReconcileSchedulePricesKeepsConfiguredValues(t *testing.T) {
	configured := []subscriptionSchedulePriceModel{
		{PricePointID: types.StringValue("point-usa")},
	}

	monthly := models.SubscriptionPlanType("MONTHLY")
	reconciled := reconcileSchedulePrices(configured, []models.SubscriptionPrice{
		reportedPrice("price-1", "point-usa", "USA", nil, &monthly),
	}, "2026-11-12")

	if len(reconciled) != 1 {
		t.Fatalf("got %d prices, want 1", len(reconciled))
	}
	if !reconciled[0].TerritoryID.IsNull() {
		t.Errorf("territory_id = %v, want null: the configuration named none", reconciled[0].TerritoryID)
	}
	if !reconciled[0].PlanType.IsNull() {
		t.Errorf("plan_type = %v, want null: the configuration named none", reconciled[0].PlanType)
	}
}

// TestReconcileSchedulePricesDropsPricesAppleNoLongerReports checks that a
// price deleted outside Terraform surfaces as drift rather than persisting.
func TestReconcileSchedulePricesDropsPricesAppleNoLongerReports(t *testing.T) {
	configured := []subscriptionSchedulePriceModel{
		{PricePointID: types.StringValue("point-usa")},
		{PricePointID: types.StringValue("point-gbr")},
	}

	reconciled := reconcileSchedulePrices(configured, []models.SubscriptionPrice{
		reportedPrice("price-1", "point-usa", "USA", nil, nil),
	}, "2026-11-12")

	if len(reconciled) != 1 {
		t.Fatalf("got %d prices, want 1", len(reconciled))
	}
	if got := reconciled[0].PricePointID.ValueString(); got != "point-usa" {
		t.Errorf("surviving price point = %q, want point-usa", got)
	}
}

// TestReconcileSchedulePricesAdoptsUnknownPrices checks that a price added in
// App Store Connect appears with Apple's own values, so it shows up as drift
// instead of being silently discarded on the next plan.
func TestReconcileSchedulePricesAdoptsUnknownPrices(t *testing.T) {
	startDate := "2027-01-01"
	upfront := models.SubscriptionPlanType("UPFRONT")

	reconciled := reconcileSchedulePrices(nil, []models.SubscriptionPrice{
		reportedPrice("price-2", "point-gbr", "GBR", &startDate, &upfront),
	}, "2027-06-01")

	if len(reconciled) != 1 {
		t.Fatalf("got %d prices, want 1", len(reconciled))
	}
	adopted := reconciled[0]
	if got := adopted.PricePointID.ValueString(); got != "point-gbr" {
		t.Errorf("price_point_id = %q, want point-gbr", got)
	}
	if got := adopted.TerritoryID.ValueString(); got != "GBR" {
		t.Errorf("territory_id = %q, want GBR", got)
	}
	if got := adopted.StartDate.ValueString(); got != startDate {
		t.Errorf("start_date = %q, want %q", got, startDate)
	}
	if got := adopted.PlanType.ValueString(); got != "UPFRONT" {
		t.Errorf("plan_type = %q, want UPFRONT", got)
	}
	// preserveCurrentPrice is an instruction Apple never reports back.
	if !adopted.PreserveCurrentPrice.IsNull() {
		t.Errorf("preserve_current_price = %v, want null", adopted.PreserveCurrentPrice)
	}
}

// TestReconcileSchedulePricesSkipsPricesWithoutAPricePoint checks the read that
// forgot include=subscriptionPricePoint: a price whose point is unknown cannot
// be matched against configuration, and must not be keyed under the empty
// string.
func TestReconcileSchedulePricesSkipsPricesWithoutAPricePoint(t *testing.T) {
	reconciled := reconcileSchedulePrices(nil, []models.SubscriptionPrice{
		{Type: "subscriptionPrices", ID: "price-3"},
		{
			Type:          "subscriptionPrices",
			ID:            "price-4",
			Relationships: &models.SubscriptionPriceRelationships{},
		},
	}, "2026-11-12")

	if len(reconciled) != 0 {
		t.Fatalf("got %d prices, want 0: neither reported a price point", len(reconciled))
	}
}
