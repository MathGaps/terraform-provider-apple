// Copyright (c) AO Studio
// SPDX-License-Identifier: MPL-2.0

package subscription

import (
	"context"
	"reflect"
	"testing"

	"github.com/AhmedOsman00/terraform-provider-apple/internal/apple/models"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TestIntroductoryOfferSchemas runs the introductory offer resource and data
// source through the framework's own schema rules and pins their type names,
// since renaming one is breaking. The framework otherwise validates a schema
// only when the provider server starts, which happens in the acceptance tier.
func TestIntroductoryOfferSchemas(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	r := NewSubscriptionIntroductoryOfferResource()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "apple"}, metaResp)
	if metaResp.TypeName != "apple_subscription_introductory_offer" {
		t.Errorf("resource type name = %q", metaResp.TypeName)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("resource schema returned errors: %+v", schemaResp.Diagnostics)
	}
	if diags := schemaResp.Schema.ValidateImplementation(ctx); diags.HasError() {
		t.Fatalf("resource schema failed validation: %+v", diags)
	}

	d := NewSubscriptionIntroductoryOffersDataSource()
	dsMetaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "apple"}, dsMetaResp)
	if dsMetaResp.TypeName != "apple_subscription_introductory_offers" {
		t.Errorf("data source type name = %q", dsMetaResp.TypeName)
	}
	dsSchemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, dsSchemaResp)
	if dsSchemaResp.Diagnostics.HasError() {
		t.Fatalf("data source schema returned errors: %+v", dsSchemaResp.Diagnostics)
	}
	if diags := dsSchemaResp.Schema.ValidateImplementation(ctx); diags.HasError() {
		t.Fatalf("data source schema failed validation: %+v", diags)
	}
}

func strPtr(s string) *string { return &s }

func offer(id, territory, pricePoint string, attrs models.SubscriptionIntroductoryOfferAttributes) models.SubscriptionIntroductoryOffer {
	rels := &models.SubscriptionIntroductoryOfferRelationships{}
	if territory != "" {
		rels.Territory = &models.ResourceIdentifier{Data: models.ResourceData{Type: "territories", ID: territory}}
	}
	if pricePoint != "" {
		rels.SubscriptionPricePoint = &models.ResourceIdentifier{Data: models.ResourceData{Type: "subscriptionPricePoints", ID: pricePoint}}
	}
	return models.SubscriptionIntroductoryOffer{Type: "subscriptionIntroductoryOffers", ID: id, Attributes: attrs, Relationships: rels}
}

var weekTrial = models.SubscriptionIntroductoryOfferAttributes{
	Duration:        models.SubscriptionOfferDurationOneWeek,
	OfferMode:       models.SubscriptionOfferModeFreeTrial,
	NumberOfPeriods: 1,
}

func TestOfferChanges(t *testing.T) {
	t.Parallel()

	stateTerritories := map[string]string{"USA": "p-usa", "GBR": "p-gbr", "AUS": "p-aus"}
	stateOfferIDs := map[string]string{"USA": "o-usa", "GBR": "o-gbr", "AUS": "o-aus"}
	planTerritories := map[string]string{
		"USA": "p-usa",     // unchanged: kept
		"GBR": "p-gbr-new", // repriced: deleted and created again
		"CAN": "p-can",     // added
		// AUS removed
	}

	toDelete, toCreate, kept := offerChanges(stateTerritories, stateOfferIDs, planTerritories)

	if want := map[string]string{"GBR": "o-gbr", "AUS": "o-aus"}; !reflect.DeepEqual(toDelete, want) {
		t.Errorf("toDelete = %v, want %v", toDelete, want)
	}
	if want := []string{"CAN", "GBR"}; !reflect.DeepEqual(toCreate, want) {
		t.Errorf("toCreate = %v, want %v (sorted)", toCreate, want)
	}
	if want := []string{"USA"}; !reflect.DeepEqual(kept, want) {
		t.Errorf("kept = %v, want %v", kept, want)
	}
}

func TestOfferChangesFreeTrialHasNoPricePoints(t *testing.T) {
	t.Parallel()

	toDelete, toCreate, kept := offerChanges(
		map[string]string{"USA": "", "GBR": ""},
		map[string]string{"USA": "o-usa", "GBR": "o-gbr"},
		map[string]string{"USA": "", "GBR": ""},
	)

	if len(toDelete) != 0 || len(toCreate) != 0 {
		t.Errorf("an unchanged free trial plans deletes %v and creates %v", toDelete, toCreate)
	}
	if want := []string{"GBR", "USA"}; !reflect.DeepEqual(kept, want) {
		t.Errorf("kept = %v, want %v", kept, want)
	}
}

// TestSurvivingOffersMatchesByID checks that another offer in the same
// territory -- an earlier one that has ended, say -- is not mistaken for this
// resource's record.
func TestSurvivingOffersMatchesByID(t *testing.T) {
	t.Parallel()

	reported := []models.SubscriptionIntroductoryOffer{
		offer("someone-elses-usa", "USA", "", weekTrial),
		offer("o-gbr", "GBR", "", weekTrial),
	}

	surviving := survivingOffers(map[string]string{"USA": "o-usa", "GBR": "o-gbr"}, reported)

	if len(surviving) != 1 {
		t.Fatalf("got %d surviving offers, want 1: %v", len(surviving), surviving)
	}
	if surviving["GBR"].ID != "o-gbr" {
		t.Errorf("GBR = %q, want o-gbr", surviving["GBR"].ID)
	}
	if _, ok := surviving["USA"]; ok {
		t.Error("a different record in USA was taken for this resource's deleted one")
	}
}

func TestDriftedEndDate(t *testing.T) {
	t.Parallel()

	withEnd := func(end *string) models.SubscriptionIntroductoryOfferAttributes {
		attrs := weekTrial
		attrs.EndDate = end
		return attrs
	}

	surviving := map[string]models.SubscriptionIntroductoryOffer{
		"USA": offer("o-usa", "USA", "", withEnd(strPtr("2027-01-31"))),
		"GBR": offer("o-gbr", "GBR", "", withEnd(strPtr("2027-01-31"))),
	}

	if got, drifted := driftedEndDate(strPtr("2027-01-31"), surviving); drifted || *got != "2027-01-31" {
		t.Errorf("matching end dates reported as drifted to %v", got)
	}

	// One territory edited by hand is enough to plan an update.
	surviving["GBR"] = offer("o-gbr", "GBR", "", withEnd(strPtr("2026-12-31")))
	got, drifted := driftedEndDate(strPtr("2027-01-31"), surviving)
	if !drifted || got == nil || *got != "2026-12-31" {
		t.Errorf("driftedEndDate = %v, %v, want 2026-12-31, true", got, drifted)
	}

	// An end date removed at Apple drifts to null.
	surviving["GBR"] = offer("o-gbr", "GBR", "", withEnd(nil))
	if got, drifted := driftedEndDate(strPtr("2027-01-31"), surviving); !drifted || got != nil {
		t.Errorf("driftedEndDate = %v, %v, want nil, true", got, drifted)
	}
}

// TestGroupOfferTerritories covers import: the records App Store Connect makes
// for one offer share every term, so they are adopted together, and an offer
// with different terms in the same territories is left alone.
func TestGroupOfferTerritories(t *testing.T) {
	t.Parallel()

	later := weekTrial
	later.StartDate = strPtr("2027-06-01")

	offers := []models.SubscriptionIntroductoryOffer{
		offer("trial-usa", "USA", "", weekTrial),
		offer("trial-gbr", "GBR", "", weekTrial),
		offer("later-usa", "USA", "", later),
		offer("orphan", "", "", weekTrial),
	}

	anchor, group, found := groupOfferTerritories(offers, "trial-gbr")
	if !found {
		t.Fatal("anchor not found")
	}
	if anchor.ID != "trial-gbr" {
		t.Errorf("anchor = %q", anchor.ID)
	}
	if len(group) != 2 || group["USA"].ID != "trial-usa" || group["GBR"].ID != "trial-gbr" {
		t.Errorf("group = %v, want trial-usa and trial-gbr", group)
	}

	if _, _, found := groupOfferTerritories(offers, "missing"); found {
		t.Error("an ID that is not on the subscription was found")
	}

	_, group, _ = groupOfferTerritories(offers, "later-usa")
	if len(group) != 1 || group["USA"].ID != "later-usa" {
		t.Errorf("group for the later offer = %v, want later-usa alone", group)
	}
}

// TestSetOfferTerritoriesKeepsShape checks that a read writes territories back
// into the attribute the configuration uses, and that an import picks the
// attribute from whether the offer has prices.
func TestSetOfferTerritoriesKeepsShape(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	paid := subscriptionIntroductoryOfferModel{
		PricePointIDs: types.MapValueMust(types.StringType, map[string]attr.Value{"USA": types.StringValue("p-usa")}),
		TerritoryIDs:  types.SetNull(types.StringType),
	}
	if diags := setOfferTerritories(ctx, &paid, map[string]string{"USA": "p-usa"}, map[string]string{"USA": "o-usa"}); diags.HasError() {
		t.Fatalf("setOfferTerritories: %v", diags)
	}
	if paid.PricePointIDs.IsNull() || !paid.TerritoryIDs.IsNull() {
		t.Errorf("paid offer moved attributes: price_point_ids=%v territory_ids=%v", paid.PricePointIDs, paid.TerritoryIDs)
	}

	imported := subscriptionIntroductoryOfferModel{
		PricePointIDs: types.MapNull(types.StringType),
		TerritoryIDs:  types.SetNull(types.StringType),
	}
	if diags := setOfferTerritories(ctx, &imported, map[string]string{"USA": "", "GBR": ""}, map[string]string{"USA": "o-usa", "GBR": "o-gbr"}); diags.HasError() {
		t.Fatalf("setOfferTerritories: %v", diags)
	}
	if imported.TerritoryIDs.IsNull() || !imported.PricePointIDs.IsNull() {
		t.Errorf("imported free trial: territory_ids=%v price_point_ids=%v", imported.TerritoryIDs, imported.PricePointIDs)
	}
	if got := len(imported.OfferIDs.Elements()); got != 2 {
		t.Errorf("offer_ids has %d entries, want 2", got)
	}
}

func TestFilterIntroductoryOffers(t *testing.T) {
	t.Parallel()

	paid := weekTrial
	paid.OfferMode = models.SubscriptionOfferModePayUpFront

	later := weekTrial
	later.StartDate = strPtr("2027-06-01")

	offers := []models.SubscriptionIntroductoryOffer{
		offer("c", "USA", "", later),
		offer("b", "USA", "", weekTrial),
		offer("a", "GBR", "p-gbr", paid),
	}

	all := FilterIntroductoryOffers(offers, "")
	var ids []string
	for _, o := range all {
		ids = append(ids, o.ID)
	}
	if want := []string{"a", "b", "c"}; !reflect.DeepEqual(ids, want) {
		t.Errorf("order = %v, want %v (territory, then start date)", ids, want)
	}

	trials := FilterIntroductoryOffers(offers, "FREE_TRIAL")
	if len(trials) != 2 {
		t.Errorf("got %d free trials, want 2", len(trials))
	}
}
