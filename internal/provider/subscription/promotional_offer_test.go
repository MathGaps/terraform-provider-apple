// Copyright (c) AO Studio
// SPDX-License-Identifier: MPL-2.0

package subscription

import (
	"context"
	"testing"

	"github.com/AhmedOsman00/terraform-provider-apple/internal/apple/models"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TestPromotionalOfferSchemas runs the promotional offer resource and data
// source schemas through the framework's own rules and pins their type names.
//
// The framework validates a schema only when the provider server starts, so a
// mistake would otherwise surface only in the acceptance tier, which needs
// credentials and a real app and does not run in CI. A rename is breaking.
func TestPromotionalOfferSchemas(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	r := NewSubscriptionPromotionalOfferResource()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("resource schema returned errors: %+v", schemaResp.Diagnostics)
	}
	if diags := schemaResp.Schema.ValidateImplementation(ctx); diags.HasError() {
		t.Fatalf("resource schema failed validation: %+v", diags)
	}

	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "apple"}, metaResp)
	if metaResp.TypeName != "apple_subscription_promotional_offer" {
		t.Errorf("resource type name = %q, want apple_subscription_promotional_offer", metaResp.TypeName)
	}

	d := NewSubscriptionPromotionalOffersDataSource()
	dsSchemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, dsSchemaResp)
	if dsSchemaResp.Diagnostics.HasError() {
		t.Fatalf("data source schema returned errors: %+v", dsSchemaResp.Diagnostics)
	}
	if diags := dsSchemaResp.Schema.ValidateImplementation(ctx); diags.HasError() {
		t.Fatalf("data source schema failed validation: %+v", diags)
	}

	dsMetaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "apple"}, dsMetaResp)
	if dsMetaResp.TypeName != "apple_subscription_promotional_offers" {
		t.Errorf("data source type name = %q, want apple_subscription_promotional_offers", dsMetaResp.TypeName)
	}
}

func offerPrice(id, territory, point string) models.SubscriptionPromotionalOfferPrice {
	price := models.SubscriptionPromotionalOfferPrice{
		Type: "subscriptionPromotionalOfferPrices",
		ID:   id,
		Relationships: &models.SubscriptionPromotionalOfferPriceRelationships{
			Territory: &models.ResourceIdentifier{Data: models.ResourceData{Type: "territories", ID: territory}},
		},
	}
	if point != "" {
		price.Relationships.SubscriptionPricePoint = &models.ResourceIdentifier{
			Data: models.ResourceData{Type: "subscriptionPricePoints", ID: point},
		}
	}
	return price
}

func statePrice(territory, point string) promotionalOfferPriceModel {
	price := promotionalOfferPriceModel{TerritoryID: types.StringValue(territory), PricePointID: types.StringNull()}
	if point != "" {
		price.PricePointID = types.StringValue(point)
	}
	return price
}

func pricesByTerritory(t *testing.T, prices []promotionalOfferPriceModel) map[string]types.String {
	t.Helper()
	byTerritory := make(map[string]types.String, len(prices))
	for _, price := range prices {
		territory := price.TerritoryID.ValueString()
		if _, dup := byTerritory[territory]; dup {
			t.Errorf("territory %s reconciled twice", territory)
		}
		byTerritory[territory] = price.PricePointID
	}
	return byTerritory
}

// TestReconcilePromotionalOfferPrices covers the four outcomes of matching
// Apple's prices against state by territory.
func TestReconcilePromotionalOfferPrices(t *testing.T) {
	t.Parallel()

	state := []promotionalOfferPriceModel{
		statePrice("USA", "point-usa"), // unchanged at Apple
		statePrice("GBR", "point-gbr"), // repriced in App Store Connect
		statePrice("CAN", ""),          // a free-trial territory
		statePrice("AUS", "point-aus"), // removed at Apple
	}
	reported := []models.SubscriptionPromotionalOfferPrice{
		offerPrice("p1", "USA", "point-usa"),
		offerPrice("p2", "GBR", "point-gbr-new"),
		offerPrice("p3", "CAN", "point-can-free"),
		offerPrice("p4", "DEU", "point-deu"),                   // added at Apple
		{Type: "subscriptionPromotionalOfferPrices", ID: "p5"}, // no linkage: ignored
	}

	got := pricesByTerritory(t, reconcilePromotionalOfferPrices(state, reported))

	want := map[string]types.String{
		"USA": types.StringValue("point-usa"),
		"GBR": types.StringValue("point-gbr-new"),
		"CAN": types.StringNull(),
		"DEU": types.StringValue("point-deu"),
	}
	if len(got) != len(want) {
		t.Fatalf("reconciled %d territories %v, want %d %v", len(got), got, len(want), want)
	}
	for territory, point := range want {
		if !got[territory].Equal(point) {
			t.Errorf("%s = %v, want %v", territory, got[territory], point)
		}
	}
}

// TestReconcilePromotionalOfferPricesImport covers import, where there is no
// state: everything Apple reports is adopted, and a territory with no price
// point -- a free trial -- adopts a null rather than an empty string.
func TestReconcilePromotionalOfferPricesImport(t *testing.T) {
	t.Parallel()

	got := pricesByTerritory(t, reconcilePromotionalOfferPrices(nil, []models.SubscriptionPromotionalOfferPrice{
		offerPrice("p1", "USA", "point-usa"),
		offerPrice("p2", "GBR", ""),
	}))

	if !got["USA"].Equal(types.StringValue("point-usa")) {
		t.Errorf("USA = %v, want point-usa", got["USA"])
	}
	if point, ok := got["GBR"]; !ok || !point.IsNull() {
		t.Errorf("GBR = %v (present %v), want a null price point", point, ok)
	}
}

// TestPromotionalOfferPricesToAPI checks the request is built in territory
// order, so the same set always produces the same body, and that a free-trial
// territory sends no price point.
func TestPromotionalOfferPricesToAPI(t *testing.T) {
	t.Parallel()

	got := promotionalOfferPricesToAPI([]promotionalOfferPriceModel{
		statePrice("USA", "point-usa"),
		statePrice("AUS", ""),
	})

	if len(got) != 2 {
		t.Fatalf("got %d prices, want 2", len(got))
	}
	if got[0].TerritoryID != "AUS" || got[0].PricePointID != "" {
		t.Errorf("first price = %+v, want AUS with no price point", got[0])
	}
	if got[1].TerritoryID != "USA" || got[1].PricePointID != "point-usa" {
		t.Errorf("second price = %+v, want USA point-usa", got[1])
	}
}
