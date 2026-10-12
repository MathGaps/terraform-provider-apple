// Copyright (c) AO Studio
// SPDX-License-Identifier: MPL-2.0

package subscription

import (
	"context"
	"regexp"
	"sort"
	"sync"

	"github.com/AhmedOsman00/terraform-provider-apple/internal/apple/models"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// subscriptionIntroductoryOfferModel maps the introductory offer schema: one
// offer, described once, across a set of territories.
//
// Apple keeps a record per territory. The territories are carried in exactly
// one of two attributes, chosen by whether the offer has a price: a free trial
// lists them in territory_ids, a paid offer keys its price points by them in
// price_point_ids. offer_ids maps each territory to the Apple record behind it.
type subscriptionIntroductoryOfferModel struct {
	ID                         types.String `tfsdk:"id"`
	SubscriptionID             types.String `tfsdk:"subscription_id"`
	OfferMode                  types.String `tfsdk:"offer_mode"`
	Duration                   types.String `tfsdk:"duration"`
	NumberOfPeriods            types.Int64  `tfsdk:"number_of_periods"`
	StartDate                  types.String `tfsdk:"start_date"`
	EndDate                    types.String `tfsdk:"end_date"`
	TargetSubscriptionPlanType types.String `tfsdk:"target_subscription_plan_type"`
	TerritoryIDs               types.Set    `tfsdk:"territory_ids"`
	PricePointIDs              types.Map    `tfsdk:"price_point_ids"`
	OfferIDs                   types.Map    `tfsdk:"offer_ids"`
}

// subscriptionIntroductoryOfferListModel is one Apple record, as the listing
// data source reports it: one offer in one territory.
type subscriptionIntroductoryOfferListModel struct {
	ID                         types.String `tfsdk:"id"`
	TerritoryID                types.String `tfsdk:"territory_id"`
	PricePointID               types.String `tfsdk:"price_point_id"`
	OfferMode                  types.String `tfsdk:"offer_mode"`
	Duration                   types.String `tfsdk:"duration"`
	NumberOfPeriods            types.Int64  `tfsdk:"number_of_periods"`
	StartDate                  types.String `tfsdk:"start_date"`
	EndDate                    types.String `tfsdk:"end_date"`
	TargetSubscriptionPlanType types.String `tfsdk:"target_subscription_plan_type"`
}

// subscriptionIntroductoryOffersDataSourceModel maps the listing data source.
type subscriptionIntroductoryOffersDataSourceModel struct {
	// Scope
	SubscriptionID types.String `tfsdk:"subscription_id"`

	// Filter configuration. Territories are passed to Apple; offer_mode is
	// applied in memory.
	Territories types.Set    `tfsdk:"territories"`
	OfferMode   types.String `tfsdk:"offer_mode"`

	// Output
	IntroductoryOffers []subscriptionIntroductoryOfferListModel `tfsdk:"introductory_offers"`

	// Computed metadata
	TotalCount types.Int64 `tfsdk:"total_count"`
}

// Introductory offer validators.
var (
	// OfferModeValidator validates an introductory offer mode.
	OfferModeValidator = stringvalidator.OneOf(models.ValidSubscriptionOfferModes...)

	// OfferDurationValidator validates an introductory offer duration.
	OfferDurationValidator = stringvalidator.OneOf(models.ValidSubscriptionOfferDurations...)

	// OfferDateValidator validates the plain dates an offer starts and ends
	// on. Like a price's start date, Apple takes a date, never a timestamp.
	OfferDateValidator = stringvalidator.RegexMatches(
		regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`),
		"Date must be a plain date in YYYY-MM-DD form, not a timestamp",
	)
)

// Introductory offer writes are serialized per subscription.
//
// Creating offers is a read-modify-write of the subscription's whole
// introductoryOffers relationship: the existing linkages are read and sent
// back alongside the new ones, so that a PATCH which replaces the relationship
// keeps them. Two resources on one subscription are independent to Terraform
// and are applied in parallel, and an interleaving -- both read, one writes,
// the other writes from a stale read -- would name an offer the first had just
// deleted, or leave out one it had just created. The lock is per provider
// instance, the same scope as Terraform's parallelism, as the Bundle ID
// capability lock is.
var (
	introductoryOfferLocks sync.Map // subscription ID -> *sync.Mutex

	// introductoryOfferFallbackLock guards the unreachable case in which the
	// map holds something other than a mutex.
	introductoryOfferFallbackLock sync.Mutex
)

// lockSubscriptionIntroductoryOffers serializes introductory offer writes for
// one subscription and returns the function that releases the lock.
func lockSubscriptionIntroductoryOffers(subscriptionID string) func() {
	value, _ := introductoryOfferLocks.LoadOrStore(subscriptionID, &sync.Mutex{})

	mutex, ok := value.(*sync.Mutex)
	if !ok {
		mutex = &introductoryOfferFallbackLock
	}

	mutex.Lock()

	return mutex.Unlock
}

// offerTerritories reads the configured territories out of a model, mapping
// each to its price point -- the empty string for a free trial, which has none.
func offerTerritories(ctx context.Context, model subscriptionIntroductoryOfferModel) (map[string]string, diag.Diagnostics) {
	var diags diag.Diagnostics
	territories := map[string]string{}

	if !model.PricePointIDs.IsNull() && !model.PricePointIDs.IsUnknown() {
		pricePoints := map[string]string{}
		diags.Append(model.PricePointIDs.ElementsAs(ctx, &pricePoints, false)...)
		for territory, pricePoint := range pricePoints {
			territories[territory] = pricePoint
		}
	}

	if !model.TerritoryIDs.IsNull() && !model.TerritoryIDs.IsUnknown() {
		var ids []string
		diags.Append(model.TerritoryIDs.ElementsAs(ctx, &ids, false)...)
		for _, territory := range ids {
			territories[territory] = ""
		}
	}

	return territories, diags
}

// stateOfferIDs reads offer_ids, the territory -> Apple record map.
func stateOfferIDs(ctx context.Context, model subscriptionIntroductoryOfferModel) (map[string]string, diag.Diagnostics) {
	offerIDs := map[string]string{}
	if model.OfferIDs.IsNull() || model.OfferIDs.IsUnknown() {
		return offerIDs, nil
	}

	diags := model.OfferIDs.ElementsAs(ctx, &offerIDs, false)
	return offerIDs, diags
}

// setOfferTerritories writes territories and their offer IDs back into a model.
//
// The territories go into whichever attribute the offer is described by --
// price_point_ids when it has prices, territory_ids when it does not -- so a
// read that drops a territory keeps the configuration's shape rather than
// moving it to the other attribute and planning a change nobody made.
func setOfferTerritories(ctx context.Context, model *subscriptionIntroductoryOfferModel, territories, offerIDs map[string]string) diag.Diagnostics {
	var diags diag.Diagnostics

	usesPricePoints := !model.PricePointIDs.IsNull()
	if model.PricePointIDs.IsNull() && model.TerritoryIDs.IsNull() {
		// Neither is set, which is an import: an offer with any price point
		// is a paid one.
		for _, pricePoint := range territories {
			if pricePoint != "" {
				usesPricePoints = true
				break
			}
		}
	}

	if usesPricePoints {
		value, d := types.MapValueFrom(ctx, types.StringType, territories)
		diags.Append(d...)
		model.PricePointIDs = value
		model.TerritoryIDs = types.SetNull(types.StringType)
	} else {
		value, d := types.SetValueFrom(ctx, types.StringType, sortedKeys(territories))
		diags.Append(d...)
		model.TerritoryIDs = value
		model.PricePointIDs = types.MapNull(types.StringType)
	}

	value, d := types.MapValueFrom(ctx, types.StringType, offerIDs)
	diags.Append(d...)
	model.OfferIDs = value

	return diags
}

// offerChanges works out what an update has to do to move from the offers in
// state to the territories in the plan.
//
// A territory in both with the same price point is kept, and only its end date
// can change. A territory whose price point changed is deleted and created
// again, because Apple's update request carries the end date alone. Deletions
// come back as territory -> offer ID; creations as a sorted list of territories,
// so the request body and the placeholders in it do not depend on Go's map
// order.
func offerChanges(stateTerritories, stateOfferIDs, planTerritories map[string]string) (toDelete map[string]string, toCreate []string, kept []string) {
	toDelete = map[string]string{}

	for territory, offerID := range stateOfferIDs {
		planned, inPlan := planTerritories[territory]
		if inPlan && planned == stateTerritories[territory] {
			kept = append(kept, territory)
			continue
		}
		toDelete[territory] = offerID
	}

	for territory := range planTerritories {
		if _, replaced := toDelete[territory]; replaced {
			toCreate = append(toCreate, territory)
			continue
		}
		if _, inState := stateOfferIDs[territory]; !inState {
			toCreate = append(toCreate, territory)
		}
	}

	sort.Strings(toCreate)
	sort.Strings(kept)

	return toDelete, toCreate, kept
}

// survivingOffers matches the offers state knows about against what Apple
// reports, by record ID, and returns the ones that still exist keyed by
// territory.
//
// Matching is by ID rather than by territory on purpose: a territory can hold
// other introductory offers -- an earlier one that has ended, a later one
// scheduled after this -- and those are not this resource's.
func survivingOffers(offerIDs map[string]string, reported []models.SubscriptionIntroductoryOffer) map[string]models.SubscriptionIntroductoryOffer {
	byID := make(map[string]models.SubscriptionIntroductoryOffer, len(reported))
	for _, offer := range reported {
		byID[offer.ID] = offer
	}

	surviving := make(map[string]models.SubscriptionIntroductoryOffer, len(offerIDs))
	for territory, offerID := range offerIDs {
		if offer, ok := byID[offerID]; ok {
			surviving[territory] = offer
		}
	}

	return surviving
}

// endDatesDisagree is what Read records when the surviving records hold
// different end dates. It is no valid configuration value -- the validator
// takes YYYY-MM-DD or null -- so the next plan always differs from it and sends
// the configured end date to every record. Any one record's own date could
// equal the configuration and hide the others: an end-date update that Apple
// refused in some territories and not in others leaves exactly that.
const endDatesDisagree = ""

// driftedEndDate reports the end date Apple holds when it differs from the one
// in state. When every surviving record agrees that is their date; when they
// disagree it is endDatesDisagree, so an end date changed by hand in one
// territory, or a partial update, plans an update that puts every territory
// back.
func driftedEndDate(current *string, surviving map[string]models.SubscriptionIntroductoryOffer) (*string, bool) {
	first := true
	var agreed *string
	for _, offer := range surviving {
		reported := offer.Attributes.EndDate
		if first {
			agreed, first = reported, false
			continue
		}
		if !sameDate(agreed, reported) {
			disagree := endDatesDisagree
			return &disagree, true
		}
	}

	if first || sameDate(current, agreed) {
		return current, false
	}
	return agreed, true
}

// sameOfferTerms reports whether two records are the same offer in different
// territories: every attribute Apple holds on an offer, which is what App Store
// Connect's UI copies to each territory it creates one in.
func sameOfferTerms(a, b models.SubscriptionIntroductoryOfferAttributes) bool {
	return a.OfferMode == b.OfferMode &&
		a.Duration == b.Duration &&
		a.NumberOfPeriods == b.NumberOfPeriods &&
		sameDate(a.StartDate, b.StartDate) &&
		sameDate(a.EndDate, b.EndDate) &&
		samePlanType(a.TargetSubscriptionPlanType, b.TargetSubscriptionPlanType)
}

// groupOfferTerritories finds the offer anchorID names and every other record
// on the subscription with the same terms, keyed by territory -- the set an
// import adopts as one resource. found is false when anchorID is not among
// the offers.
func groupOfferTerritories(offers []models.SubscriptionIntroductoryOffer, anchorID string) (anchor models.SubscriptionIntroductoryOffer, group map[string]models.SubscriptionIntroductoryOffer, found bool) {
	for _, offer := range offers {
		if offer.ID == anchorID {
			anchor, found = offer, true
			break
		}
	}
	if !found {
		return anchor, nil, false
	}

	group = map[string]models.SubscriptionIntroductoryOffer{}
	for _, offer := range offers {
		territory := offerTerritoryID(offer)
		if territory == "" || !sameOfferTerms(anchor.Attributes, offer.Attributes) {
			continue
		}
		// Two records with the same terms in one territory would overlap,
		// which Apple does not allow; if it ever reports them, the one the
		// import named wins.
		if existing, taken := group[territory]; taken && existing.ID == anchorID {
			continue
		}
		group[territory] = offer
	}

	return anchor, group, true
}

// offerTerritoryID reads the territory of a record, or "" when Apple did not
// populate the linkage.
func offerTerritoryID(offer models.SubscriptionIntroductoryOffer) string {
	if offer.Relationships == nil || offer.Relationships.Territory == nil {
		return ""
	}
	return offer.Relationships.Territory.Data.ID
}

// offerPricePointID reads the price point of a record, or "" for a free trial
// or when Apple did not populate the linkage.
func offerPricePointID(offer models.SubscriptionIntroductoryOffer) string {
	if offer.Relationships == nil || offer.Relationships.SubscriptionPricePoint == nil {
		return ""
	}
	return offer.Relationships.SubscriptionPricePoint.Data.ID
}

func sameDate(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func samePlanType(a, b *models.SubscriptionPlanType) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
