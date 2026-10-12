// Copyright (c) AO Studio
// SPDX-License-Identifier: MPL-2.0

package subscription

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/AhmedOsman00/terraform-provider-apple/internal/apple/models"
)

// OfferCodeFilters is the filtering configuration for the offer code listing.
// Active is nil when the listing should not filter on it.
type OfferCodeFilters struct {
	NamePattern string
	Active      *bool
}

// FilterOfferCodes filters a subscription's offer codes and sorts the result
// by name, then ID, so the listing is stable between plans. The name pattern
// is a regular expression matched as a substring unless anchored, the
// convention the rest of the subscription package follows.
func FilterOfferCodes(offers []models.SubscriptionOfferCode, filters OfferCodeFilters) ([]models.SubscriptionOfferCode, error) {
	var pattern *regexp.Regexp
	if filters.NamePattern != "" {
		compiled, err := regexp.Compile(filters.NamePattern)
		if err != nil {
			return nil, fmt.Errorf("invalid name_pattern %q: %w", filters.NamePattern, err)
		}
		pattern = compiled
	}

	filtered := make([]models.SubscriptionOfferCode, 0, len(offers))
	for _, offer := range offers {
		if pattern != nil && !pattern.MatchString(offer.Attributes.Name) {
			continue
		}
		if filters.Active != nil {
			// An offer whose state Apple did not report is not known to be
			// either, so it matches neither filter value.
			if offer.Attributes.Active == nil || *offer.Attributes.Active != *filters.Active {
				continue
			}
		}
		filtered = append(filtered, offer)
	}

	sort.SliceStable(filtered, func(i, j int) bool {
		a, b := strings.ToLower(filtered[i].Attributes.Name), strings.ToLower(filtered[j].Attributes.Name)
		if a != b {
			return a < b
		}
		return filtered[i].ID < filtered[j].ID
	})

	return filtered, nil
}
