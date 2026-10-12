// Copyright (c) AO Studio
// SPDX-License-Identifier: MPL-2.0

package subscription

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/AhmedOsman00/terraform-provider-apple/internal/apple/models"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TestOfferCodeSchemas runs the offer code schemas through the framework's own
// rules and pins their type names. The framework validates a schema only when
// the provider server starts, so a Required+Computed attribute or a default on
// a non-computed one would otherwise surface only in the acceptance tier, which
// needs credentials and does not run in CI; a rename is breaking.
func TestOfferCodeSchemas(t *testing.T) {
	t.Parallel()

	resources := map[string]func() resource.Resource{
		"apple_subscription_offer_code":                   NewSubscriptionOfferCodeResource,
		"apple_subscription_offer_code_custom_code":       NewSubscriptionOfferCodeCustomCodeResource,
		"apple_subscription_offer_code_one_time_use_code": NewSubscriptionOfferCodeOneTimeUseCodeResource,
	}

	for name, newResource := range resources {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()

			meta := &resource.MetadataResponse{}
			newResource().Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "apple"}, meta)
			if meta.TypeName != name {
				t.Errorf("type name = %q, want %q", meta.TypeName, name)
			}

			resp := &resource.SchemaResponse{}
			newResource().Schema(ctx, resource.SchemaRequest{}, resp)
			if resp.Diagnostics.HasError() {
				t.Fatalf("schema returned errors: %+v", resp.Diagnostics)
			}
			if diags := resp.Schema.ValidateImplementation(ctx); diags.HasError() {
				t.Fatalf("schema failed validation: %+v", diags)
			}
		})
	}

	dataSources := map[string]func() datasource.DataSource{
		"apple_subscription_offer_codes":                         NewSubscriptionOfferCodesDataSource,
		"apple_subscription_offer_code_one_time_use_code_values": NewSubscriptionOfferCodeOneTimeUseCodeValuesDataSource,
	}

	for name, newDataSource := range dataSources {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()

			meta := &datasource.MetadataResponse{}
			newDataSource().Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "apple"}, meta)
			if meta.TypeName != name {
				t.Errorf("type name = %q, want %q", meta.TypeName, name)
			}

			resp := &datasource.SchemaResponse{}
			newDataSource().Schema(ctx, datasource.SchemaRequest{}, resp)
			if resp.Diagnostics.HasError() {
				t.Fatalf("schema returned errors: %+v", resp.Diagnostics)
			}
			if diags := resp.Schema.ValidateImplementation(ctx); diags.HasError() {
				t.Fatalf("schema failed validation: %+v", diags)
			}
		})
	}
}

// TestValuesAreSensitive pins the one property the values data source exists
// for: the codes are secrets and must never print in a plan.
func TestValuesAreSensitive(t *testing.T) {
	resp := &datasource.SchemaResponse{}
	NewSubscriptionOfferCodeOneTimeUseCodeValuesDataSource().Schema(context.Background(), datasource.SchemaRequest{}, resp)

	if !resp.Schema.Attributes["csv"].IsSensitive() {
		t.Error("csv is not marked sensitive")
	}
}

func TestIsReactivation(t *testing.T) {
	cases := []struct {
		name        string
		state, plan types.Bool
		want        bool
	}{
		{"inactive to active replaces", types.BoolValue(false), types.BoolValue(true), true},
		{"active to inactive updates in place", types.BoolValue(true), types.BoolValue(false), false},
		{"unchanged active", types.BoolValue(true), types.BoolValue(true), false},
		{"unchanged inactive", types.BoolValue(false), types.BoolValue(false), false},
		{"create", types.BoolNull(), types.BoolValue(true), false},
		{"unknown plan", types.BoolValue(false), types.BoolUnknown(), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isReactivation(tc.state, tc.plan); got != tc.want {
				t.Errorf("isReactivation = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCheckExpirationWithinSixMonths(t *testing.T) {
	now := time.Date(2026, 10, 12, 15, 0, 0, 0, time.UTC)

	cases := []struct {
		date    string
		wantErr string
	}{
		{"2026-11-01", ""},
		{"2027-04-12", ""},
		{"2027-04-13", ""}, // one day of slack for month-end arithmetic
		{"2027-04-14", "more than six months away"},
		{"2028-01-01", "more than six months away"},
		{"2020-01-01", ""}, // the past is never refused: a finished campaign must keep planning
		{"2027-02-30", "not a valid date"},
	}

	for _, tc := range cases {
		t.Run(tc.date, func(t *testing.T) {
			err := checkExpirationWithinSixMonths(tc.date, now)
			if tc.wantErr == "" && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Errorf("error = %v, want one containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestCheckOneTimeUseCodeCount(t *testing.T) {
	cases := []struct {
		environment string
		count       int64
		ok          bool
	}{
		{models.OfferCodeEnvironmentProduction, 499, false},
		{models.OfferCodeEnvironmentProduction, 500, true},
		{models.OfferCodeEnvironmentProduction, 25000, true},
		{models.OfferCodeEnvironmentProduction, 25001, false},
		{models.OfferCodeEnvironmentSandbox, 9, false},
		{models.OfferCodeEnvironmentSandbox, 10, true},
		{models.OfferCodeEnvironmentSandbox, 10000, true},
		{models.OfferCodeEnvironmentSandbox, 10001, false},
	}

	for _, tc := range cases {
		err := checkOneTimeUseCodeCount(tc.environment, tc.count)
		if (err == nil) != tc.ok {
			t.Errorf("%s %d: error = %v, want ok=%v", tc.environment, tc.count, err, tc.ok)
		}
	}
}

func TestCheckOfferCodePrices(t *testing.T) {
	price := func(territory, point string) offerCodePriceModel {
		p := offerCodePriceModel{TerritoryID: types.StringValue(territory), PricePointID: types.StringNull()}
		if point != "" {
			p.PricePointID = types.StringValue(point)
		}
		return p
	}

	if problems := checkOfferCodePrices("PAY_AS_YOU_GO", []offerCodePriceModel{price("USA", "a"), price("AUS", "b")}); len(problems) != 0 {
		t.Errorf("valid paid prices reported %v", problems)
	}
	if problems := checkOfferCodePrices("FREE_TRIAL", []offerCodePriceModel{price("USA", ""), price("AUS", "")}); len(problems) != 0 {
		t.Errorf("free trial without price points reported %v", problems)
	}

	problems := checkOfferCodePrices("PAY_UP_FRONT", []offerCodePriceModel{price("USA", "a"), price("AUS", "")})
	if len(problems) != 1 || !strings.Contains(problems[0], "AUS has no price_point_id") {
		t.Errorf("missing price point: problems = %v", problems)
	}

	problems = checkOfferCodePrices("FREE_TRIAL", []offerCodePriceModel{price("USA", ""), price("USA", "")})
	if len(problems) != 1 || !strings.Contains(problems[0], "USA is priced more than once") {
		t.Errorf("duplicate territory: problems = %v", problems)
	}

	unknown := offerCodePriceModel{TerritoryID: types.StringUnknown(), PricePointID: types.StringNull()}
	if problems := checkOfferCodePrices("PAY_AS_YOU_GO", []offerCodePriceModel{unknown}); len(problems) != 0 {
		t.Errorf("unknown territory should be skipped, got %v", problems)
	}
}

func TestOfferCodePricesFromAPI(t *testing.T) {
	ref := func(typ, id string) *models.ResourceIdentifier {
		return &models.ResourceIdentifier{Data: models.ResourceData{Type: typ, ID: id}}
	}

	got := offerCodePricesFromAPI([]models.SubscriptionOfferCodePrice{
		{ID: "p2", Relationships: &models.SubscriptionOfferCodePriceRelationships{
			Territory: ref("territories", "USA"), SubscriptionPricePoint: ref("subscriptionPricePoints", "pp-usa")}},
		{ID: "p1", Relationships: &models.SubscriptionOfferCodePriceRelationships{
			Territory: ref("territories", "AUS")}},
		{ID: "orphan"},
	})

	if len(got) != 2 {
		t.Fatalf("got %d prices, want 2 (the one without a territory is skipped): %+v", len(got), got)
	}
	if got[0].TerritoryID.ValueString() != "AUS" || !got[0].PricePointID.IsNull() {
		t.Errorf("first = %+v, want AUS with a null price point (a free trial)", got[0])
	}
	if got[1].TerritoryID.ValueString() != "USA" || got[1].PricePointID.ValueString() != "pp-usa" {
		t.Errorf("second = %+v, want USA/pp-usa", got[1])
	}
}

func TestFilterOfferCodes(t *testing.T) {
	yes, no := true, false
	offers := []models.SubscriptionOfferCode{
		{ID: "3", Attributes: models.SubscriptionOfferCodeAttributes{Name: "school spring", Active: &yes}},
		{ID: "1", Attributes: models.SubscriptionOfferCodeAttributes{Name: "School Autumn", Active: &no}},
		{ID: "2", Attributes: models.SubscriptionOfferCodeAttributes{Name: "Winback", Active: &yes}},
		{ID: "4", Attributes: models.SubscriptionOfferCodeAttributes{Name: "School unknown"}},
	}

	all, err := FilterOfferCodes(offers, OfferCodeFilters{})
	if err != nil {
		t.Fatal(err)
	}
	if ids := offerIDs(all); ids != "1,3,4,2" {
		t.Errorf("unfiltered order = %s, want 1,3,4,2 (by name, case-insensitive)", ids)
	}

	active, _ := FilterOfferCodes(offers, OfferCodeFilters{NamePattern: "(?i)^school", Active: &yes})
	if ids := offerIDs(active); ids != "3" {
		t.Errorf("active school offers = %s, want 3", ids)
	}

	inactive, _ := FilterOfferCodes(offers, OfferCodeFilters{Active: &no})
	if ids := offerIDs(inactive); ids != "1" {
		t.Errorf("inactive offers = %s, want 1 (an unreported state matches neither)", ids)
	}

	if _, err := FilterOfferCodes(offers, OfferCodeFilters{NamePattern: "("}); err == nil {
		t.Error("malformed pattern was accepted")
	}
}

func offerIDs(offers []models.SubscriptionOfferCode) string {
	ids := make([]string, len(offers))
	for i, o := range offers {
		ids[i] = o.ID
	}
	return strings.Join(ids, ",")
}
