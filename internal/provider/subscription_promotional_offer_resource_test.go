// Copyright (c) AO Studio
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// testAccPromotionalOfferCode returns an offer code unique to this run.
//
// An offer code identifies its offer, and Apple's API documents neither its
// uniqueness rule nor whether deleting the offer releases the code. Every test here
// makes a fresh subscription, so a fixed code would most likely be fine -- but
// "most likely" is the bet a fixed product identifier lost, so the code is
// randomised the way testAccSubscriptionProductID is.
func testAccPromotionalOfferCode(base string) string {
	return fmt.Sprintf("%s%s", base, strings.ToLower(acctest.RandString(8)))
}

// TestAccSubscriptionPromotionalOfferResource_basic creates a paid offer priced
// in two territories, narrows its prices in place, imports it both ways and
// lists it through the data source.
func TestAccSubscriptionPromotionalOfferResource_basic(t *testing.T) {
	referenceName := "Terraform Promo " + acctest.RandString(6)
	productID := testAccSubscriptionProductID()
	name := testAccProductName("Pro Promo")
	offerCode := testAccPromotionalOfferCode("winback")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckSubscription(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testAccCheckDestroyAll(
			testAccCheckSubscriptionPromotionalOfferDestroy,
			testAccCheckSubscriptionDestroy,
			testAccCheckSubscriptionGroupDestroy,
		),
		Steps: []resource.TestStep{
			{
				Config: testAccSubscriptionPromotionalOfferConfig(referenceName, productID, name, offerCode, true),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckSubscriptionPromotionalOfferPrices("apple_subscription_promotional_offer.test", 2),
					resource.TestCheckResourceAttr("apple_subscription_promotional_offer.test", "offer_code", offerCode),
					resource.TestCheckResourceAttr("apple_subscription_promotional_offer.test", "offer_mode", "PAY_AS_YOU_GO"),
					resource.TestCheckResourceAttr("apple_subscription_promotional_offer.test", "number_of_periods", "3"),
					resource.TestCheckResourceAttr("apple_subscription_promotional_offer.test", "prices.#", "2"),
					resource.TestCheckResourceAttrPair(
						"apple_subscription_promotional_offer.test", "subscription_id",
						"apple_subscription.test", "id"),
				),
			},
			// The prices are the one thing Apple's update request carries, so
			// dropping a territory changes the offer in place rather than
			// replacing it.
			{
				Config: testAccSubscriptionPromotionalOfferConfig(referenceName, productID, name, offerCode, false),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("apple_subscription_promotional_offer.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckSubscriptionPromotionalOfferPrices("apple_subscription_promotional_offer.test", 1),
					resource.TestCheckResourceAttr("apple_subscription_promotional_offer.test", "prices.#", "1"),
					resource.TestCheckResourceAttr("data.apple_subscription_promotional_offers.test", "filtered_count", "1"),
					resource.TestCheckResourceAttrPair(
						"data.apple_subscription_promotional_offers.test", "promotional_offers.0.id",
						"apple_subscription_promotional_offer.test", "id"),
				),
			},
			// A bare ID suffices: the provider reads with include=subscription.
			{
				ResourceName:      "apple_subscription_promotional_offer.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// And the form an offer made in App Store Connect is known by.
			{
				ResourceName:      "apple_subscription_promotional_offer.test",
				ImportState:       true,
				ImportStateIdFunc: testAccSubscriptionPromotionalOfferCodeImportID("apple_subscription_promotional_offer.test"),
				ImportStateVerify: true,
			},
		},
	})
}

// TestAccSubscriptionPromotionalOfferResource_freeTrial creates a free-trial
// offer whose one territory names no price point -- the shape Apple's inline
// price schema permits and the one place this resource leans on that.
func TestAccSubscriptionPromotionalOfferResource_freeTrial(t *testing.T) {
	referenceName := "Terraform Trial " + acctest.RandString(6)
	productID := testAccSubscriptionProductID()
	name := testAccProductName("Pro Trial")
	offerCode := testAccPromotionalOfferCode("freemonth")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckSubscription(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testAccCheckDestroyAll(
			testAccCheckSubscriptionPromotionalOfferDestroy,
			testAccCheckSubscriptionDestroy,
			testAccCheckSubscriptionGroupDestroy,
		),
		Steps: []resource.TestStep{
			{
				Config: testAccSubscriptionConfig(referenceName, productID, name, "ONE_MONTH") + fmt.Sprintf(`
resource "apple_subscription_availability" "test" {
  subscription_id       = apple_subscription.test.id
  available_territories = ["USA"]
}

resource "apple_subscription_promotional_offer" "test" {
  subscription_id   = apple_subscription.test.id
  name              = "Free month"
  offer_code        = %[1]q
  duration          = "ONE_MONTH"
  offer_mode        = "FREE_TRIAL"
  number_of_periods = 1

  prices = [{ territory_id = "USA" }]

  depends_on = [apple_subscription_availability.test]
}
`, offerCode),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckSubscriptionPromotionalOfferPrices("apple_subscription_promotional_offer.test", 1),
					resource.TestCheckResourceAttr("apple_subscription_promotional_offer.test", "offer_mode", "FREE_TRIAL"),
					resource.TestCheckResourceAttr("apple_subscription_promotional_offer.test", "prices.#", "1"),
				),
			},
		},
	})
}

// TestAccSubscriptionPromotionalOfferResource_validation covers what is
// rejected at plan time and so never reaches Apple.
func TestAccSubscriptionPromotionalOfferResource_validation(t *testing.T) {
	tests := []struct {
		name        string
		config      string
		expectError *regexp.Regexp
	}{
		{
			name: "paid offer without a price point",
			config: `
resource "apple_subscription_promotional_offer" "test" {
  subscription_id   = "12345"
  name              = "Win back"
  offer_code        = "winback"
  duration          = "ONE_MONTH"
  offer_mode        = "PAY_AS_YOU_GO"
  number_of_periods = 3
  prices            = [{ territory_id = "USA" }]
}`,
			expectError: regexp.MustCompile(`Missing Price Point`),
		},
		{
			name: "offer duration Apple does not offer",
			config: `
resource "apple_subscription_promotional_offer" "test" {
  subscription_id   = "12345"
  name              = "Win back"
  offer_code        = "winback"
  duration          = "TWO_YEARS"
  offer_mode        = "FREE_TRIAL"
  number_of_periods = 1
  prices            = [{ territory_id = "USA" }]
}`,
			expectError: regexp.MustCompile(`Invalid Attribute Value Match`),
		},
		{
			name: "offer code with a slash",
			config: `
resource "apple_subscription_promotional_offer" "test" {
  subscription_id   = "12345"
  name              = "Win back"
  offer_code        = "win/back"
  duration          = "ONE_MONTH"
  offer_mode        = "FREE_TRIAL"
  number_of_periods = 1
  prices            = [{ territory_id = "USA" }]
}`,
			expectError: regexp.MustCompile(`Invalid Attribute Value Match`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resource.Test(t, resource.TestCase{
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config:      testAccProviderConfig + tt.config,
						PlanOnly:    true,
						ExpectError: tt.expectError,
					},
				},
			})
		})
	}
}

// testAccSubscriptionPromotionalOfferConfig prices a paid offer in USA, and in
// GBR too when withGBR is set, from one looked-up price point and its
// equalization.
func testAccSubscriptionPromotionalOfferConfig(referenceName, productID, name, offerCode string, withGBR bool) string {
	gbr := ""
	if withGBR {
		gbr = `,
    {
      territory_id   = "GBR"
      price_point_id = data.apple_subscription_price_point_equalizations.test.price_point_ids["GBR"]
    }`
	}

	return testAccSubscriptionConfig(referenceName, productID, name, "ONE_MONTH") + fmt.Sprintf(`
resource "apple_subscription_availability" "test" {
  subscription_id       = apple_subscription.test.id
  available_territories = ["USA", "GBR"]
}

data "apple_subscription_price_points" "test" {
  subscription_id = apple_subscription.test.id
  territories     = ["USA"]
  customer_price  = "0.99"
  limit           = 1
}

data "apple_subscription_price_point_equalizations" "test" {
  price_point_id = data.apple_subscription_price_points.test.price_points[0].id
  territories    = ["GBR"]
}

resource "apple_subscription_promotional_offer" "test" {
  subscription_id               = apple_subscription.test.id
  name                          = "Win back"
  offer_code                    = %[1]q
  duration                      = "ONE_MONTH"
  offer_mode                    = "PAY_AS_YOU_GO"
  number_of_periods             = 3
  target_subscription_plan_type = "MONTHLY"

  prices = [
    {
      territory_id   = "USA"
      price_point_id = data.apple_subscription_price_points.test.price_points[0].id
    }%[2]s
  ]

  depends_on = [apple_subscription_availability.test]
}

data "apple_subscription_promotional_offers" "test" {
  subscription_id = apple_subscription.test.id
  offer_code      = apple_subscription_promotional_offer.test.offer_code
}
`, offerCode, gbr)
}

// testAccSubscriptionPromotionalOfferCodeImportID imports by
// "<subscription_id>/<offer_code>".
func testAccSubscriptionPromotionalOfferCodeImportID(resourceName string) resource.ImportStateIdFunc {
	return func(s *terraform.State) (string, error) {
		rs, err := testAccStateResource(s, resourceName)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s/%s", rs.Attributes["subscription_id"], rs.Attributes["offer_code"]), nil
	}
}

// testAccCheckSubscriptionPromotionalOfferPrices asks Apple whether the offer
// exists and how many territories it is priced in. State alone would pass on a
// write Apple accepted but committed partially.
func testAccCheckSubscriptionPromotionalOfferPrices(resourceName string, want int) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, err := testAccStateResource(s, resourceName)
		if err != nil {
			return err
		}

		client, err := testAccAPIClient()
		if err != nil {
			return fmt.Errorf("could not build API client: %w", err)
		}

		offer, err := client.GetSubscriptionPromotionalOffer(rs.ID)
		if err != nil {
			return fmt.Errorf("promotional offer %s not found at Apple: %w", rs.ID, err)
		}
		if offer.ID != rs.ID {
			return fmt.Errorf("Apple returned promotional offer %s, expected %s", offer.ID, rs.ID)
		}

		prices, err := client.GetSubscriptionPromotionalOfferPrices(rs.ID)
		if err != nil {
			return fmt.Errorf("reading prices of promotional offer %s: %w", rs.ID, err)
		}
		if len(prices) != want {
			return fmt.Errorf("promotional offer %s has %d prices at Apple, want %d", rs.ID, len(prices), want)
		}

		return nil
	}
}

func testAccCheckSubscriptionPromotionalOfferDestroy(s *terraform.State) error {
	client, err := testAccAPIClient()
	if err != nil {
		return fmt.Errorf("could not build API client: %w", err)
	}

	for name, rs := range s.RootModule().Resources {
		if rs.Type != "apple_subscription_promotional_offer" {
			continue
		}

		if _, err := client.GetSubscriptionPromotionalOffer(rs.Primary.ID); err == nil {
			return fmt.Errorf("promotional offer %s (%s) still exists at Apple", rs.Primary.ID, name)
		} else if !testAccIsNotFound(err) {
			return fmt.Errorf("unexpected error checking promotional offer %s: %w", rs.Primary.ID, err)
		}
	}

	return nil
}
