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

// TestAccSubscriptionIntroductoryOfferResource_freeTrial covers the free trial
// across a set of territories: one PATCH creating a record per territory, an
// import that adopts every territory from any one record, and an in-place
// update that moves the end date and swaps a territory.
//
// The subscription is made sellable first -- available and priced in every
// territory the offer names -- because nothing in an offer references either,
// and the order has to be declared.
func TestAccSubscriptionIntroductoryOfferResource_freeTrial(t *testing.T) {
	referenceName := "Terraform Intro " + acctest.RandString(6)
	productID := testAccSubscriptionProductID()
	name := testAccProductName("Pro Intro")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckSubscription(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testAccCheckDestroyAll(
			testAccCheckSubscriptionIntroductoryOfferDestroy,
			testAccCheckSubscriptionDestroy,
			testAccCheckSubscriptionGroupDestroy,
		),
		Steps: []resource.TestStep{
			{
				Config: testAccSubscriptionIntroductoryOfferFreeTrialConfig(referenceName, productID, name,
					[]string{"USA", "GBR"}, ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair(
						"apple_subscription_introductory_offer.test", "id",
						"apple_subscription.test", "id"),
					resource.TestCheckResourceAttr("apple_subscription_introductory_offer.test", "territory_ids.#", "2"),
					resource.TestCheckResourceAttr("apple_subscription_introductory_offer.test", "offer_ids.%", "2"),
					resource.TestCheckResourceAttrSet("apple_subscription_introductory_offer.test", "offer_ids.USA"),
					resource.TestCheckResourceAttrSet("apple_subscription_introductory_offer.test", "offer_ids.GBR"),
					resource.TestCheckResourceAttr("apple_subscription_introductory_offer.test", "number_of_periods", "1"),
					// Both records reached Apple in one PATCH, and nothing else
					// on the subscription was disturbed by it.
					testAccCheckIntroductoryOfferCount("apple_subscription_introductory_offer.test", 2),
					// The listing reports one record per territory.
					resource.TestCheckResourceAttr(
						"data.apple_subscription_introductory_offers.test", "introductory_offers.#", "2"),
					resource.TestCheckResourceAttr(
						"data.apple_subscription_introductory_offers.test", "introductory_offers.0.offer_mode", "FREE_TRIAL"),
				),
			},
			// Any one territory's record imports the whole offer.
			{
				ResourceName:      "apple_subscription_introductory_offer.test",
				ImportState:       true,
				ImportStateIdFunc: testAccIntroductoryOfferImportID("apple_subscription_introductory_offer.test", "GBR"),
				ImportStateVerify: true,
				// Import adopts every term Apple reports, and Apple reports a plan
				// type on prices that never named one; if it fills either of these
				// on an offer that set neither, that is Apple's value rather than
				// a mismatch. Read never refreshes them, for the same reason.
				ImportStateVerifyIgnore: []string{"start_date", "target_subscription_plan_type"},
			},
			// The end date changes in place on the record that stays; GBR's
			// record is deleted and CAN's created, all without replacement.
			{
				Config: testAccSubscriptionIntroductoryOfferFreeTrialConfig(referenceName, productID, name,
					[]string{"USA", "CAN"}, "2030-12-31"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("apple_subscription_introductory_offer.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("apple_subscription_introductory_offer.test", "end_date", "2030-12-31"),
					resource.TestCheckResourceAttr("apple_subscription_introductory_offer.test", "offer_ids.%", "2"),
					resource.TestCheckNoResourceAttr("apple_subscription_introductory_offer.test", "offer_ids.GBR"),
					testAccCheckIntroductoryOfferCount("apple_subscription_introductory_offer.test", 2),
					testAccCheckIntroductoryOfferEndDates("apple_subscription_introductory_offer.test", "2030-12-31"),
				),
			},
		},
	})
}

// TestAccSubscriptionIntroductoryOfferResource_payAsYouGo covers a paid offer:
// three discounted months, priced per territory through the equalization of
// one base price point, the way the price schedule is.
func TestAccSubscriptionIntroductoryOfferResource_payAsYouGo(t *testing.T) {
	referenceName := "Terraform Intro Paid " + acctest.RandString(6)
	productID := testAccSubscriptionProductID()
	name := testAccProductName("Pro Paid")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckSubscription(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testAccCheckDestroyAll(
			testAccCheckSubscriptionIntroductoryOfferDestroy,
			testAccCheckSubscriptionDestroy,
			testAccCheckSubscriptionGroupDestroy,
		),
		Steps: []resource.TestStep{
			{
				Config: testAccSubscriptionIntroductoryOfferPaidConfig(referenceName, productID, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("apple_subscription_introductory_offer.test", "offer_mode", "PAY_AS_YOU_GO"),
					resource.TestCheckResourceAttr("apple_subscription_introductory_offer.test", "number_of_periods", "3"),
					resource.TestCheckResourceAttr("apple_subscription_introductory_offer.test", "price_point_ids.%", "2"),
					resource.TestCheckResourceAttr("apple_subscription_introductory_offer.test", "offer_ids.%", "2"),
					testAccCheckIntroductoryOfferCount("apple_subscription_introductory_offer.test", 2),
				),
			},
			{
				ResourceName:      "apple_subscription_introductory_offer.test",
				ImportState:       true,
				ImportStateIdFunc: testAccIntroductoryOfferImportID("apple_subscription_introductory_offer.test", "USA"),
				ImportStateVerify: true,
				// Import adopts every term Apple reports, and Apple reports a plan
				// type on prices that never named one; if it fills either of these
				// on an offer that set neither, that is Apple's value rather than
				// a mismatch. Read never refreshes them, for the same reason.
				ImportStateVerifyIgnore: []string{"start_date", "target_subscription_plan_type"},
			},
		},
	})
}

// TestAccSubscriptionIntroductoryOfferResource_validation covers the plan-time
// rules. Every step is rejected before a request reaches Apple, so this
// creates nothing and needs no credentials.
func TestAccSubscriptionIntroductoryOfferResource_validation(t *testing.T) {
	tests := []struct {
		name        string
		config      string
		expectError *regexp.Regexp
	}{
		{
			name: "free trial with price points",
			config: `
resource "apple_subscription_introductory_offer" "test" {
  subscription_id = "12345"
  offer_mode      = "FREE_TRIAL"
  duration        = "ONE_WEEK"
  price_point_ids = { USA = "abcdef" }
}`,
			expectError: regexp.MustCompile(`Free Trial Takes No Price`),
		},
		{
			name: "free trial with no territories",
			config: `
resource "apple_subscription_introductory_offer" "test" {
  subscription_id = "12345"
  offer_mode      = "FREE_TRIAL"
  duration        = "ONE_WEEK"
}`,
			expectError: regexp.MustCompile(`Missing Territories`),
		},
		{
			name: "paid offer with bare territories",
			config: `
resource "apple_subscription_introductory_offer" "test" {
  subscription_id = "12345"
  offer_mode      = "PAY_UP_FRONT"
  duration        = "THREE_MONTHS"
  territory_ids   = ["USA"]
}`,
			expectError: regexp.MustCompile(`Paid Offer Needs Price Points`),
		},
		{
			name: "both territory attributes",
			config: `
resource "apple_subscription_introductory_offer" "test" {
  subscription_id = "12345"
  offer_mode      = "PAY_AS_YOU_GO"
  duration        = "ONE_MONTH"
  territory_ids   = ["USA"]
  price_point_ids = { USA = "abcdef" }
}`,
			expectError: regexp.MustCompile(`Conflicting Territory Attributes`),
		},
		{
			name: "duration Apple does not offer",
			config: `
resource "apple_subscription_introductory_offer" "test" {
  subscription_id = "12345"
  offer_mode      = "FREE_TRIAL"
  duration        = "TEN_DAYS"
  territory_ids   = ["USA"]
}`,
			expectError: regexp.MustCompile(`value must be one of`),
		},
		{
			name: "end date given as a timestamp",
			config: `
resource "apple_subscription_introductory_offer" "test" {
  subscription_id = "12345"
  offer_mode      = "FREE_TRIAL"
  duration        = "ONE_WEEK"
  territory_ids   = ["USA"]
  end_date        = "2030-12-31T00:00:00Z"
}`,
			expectError: regexp.MustCompile(`Invalid Attribute Value Match`),
		},
		{
			name: "two-letter territory key",
			config: `
resource "apple_subscription_introductory_offer" "test" {
  subscription_id = "12345"
  offer_mode      = "PAY_UP_FRONT"
  duration        = "ONE_MONTH"
  price_point_ids = { US = "abcdef" }
}`,
			expectError: regexp.MustCompile(`Invalid Attribute Value Match`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config:      tt.config,
						PlanOnly:    true,
						ExpectError: tt.expectError,
					},
				},
			})
		})
	}
}

// --- configurations ---

// testAccSellableSubscriptionConfig makes a subscription available and priced
// in USA, GBR and CAN -- everything an introductory offer in any of them
// needs -- with the price schedule fanned out from one base point.
func testAccSellableSubscriptionConfig(referenceName, productID, name string) string {
	return testAccSubscriptionConfig(referenceName, productID, name, "ONE_MONTH") + `
data "apple_subscription_price_points" "base" {
  subscription_id = apple_subscription.test.id
  territories     = ["USA"]
  customer_price  = "9.99"
  limit           = 1
}

data "apple_subscription_price_point_equalizations" "base" {
  price_point_id = data.apple_subscription_price_points.base.price_points[0].id
  territories    = ["GBR", "CAN"]
}

resource "apple_subscription_availability" "test" {
  subscription_id       = apple_subscription.test.id
  available_territories = ["USA", "GBR", "CAN"]
}

resource "apple_subscription_price_schedule" "test" {
  subscription_id = apple_subscription.test.id

  prices = [
    for territory, price_point in merge(
      { USA = data.apple_subscription_price_points.base.price_points[0].id },
      data.apple_subscription_price_point_equalizations.base.price_point_ids,
    ) : { territory_id = territory, price_point_id = price_point }
  ]

  depends_on = [apple_subscription_availability.test]
}
`
}

func testAccSubscriptionIntroductoryOfferFreeTrialConfig(referenceName, productID, name string, territories []string, endDate string) string {
	quoted := make([]string, len(territories))
	for i, territory := range territories {
		quoted[i] = fmt.Sprintf("%q", territory)
	}

	endDateLine := ""
	if endDate != "" {
		endDateLine = fmt.Sprintf("end_date        = %q", endDate)
	}

	return testAccSellableSubscriptionConfig(referenceName, productID, name) + fmt.Sprintf(`
resource "apple_subscription_introductory_offer" "test" {
  subscription_id = apple_subscription.test.id
  offer_mode      = "FREE_TRIAL"
  duration        = "ONE_WEEK"
  territory_ids   = [%[1]s]
  %[2]s

  depends_on = [apple_subscription_price_schedule.test]
}

data "apple_subscription_introductory_offers" "test" {
  subscription_id = apple_subscription.test.id

  depends_on = [apple_subscription_introductory_offer.test]
}
`, strings.Join(quoted, ", "), endDateLine)
}

// testAccSubscriptionIntroductoryOfferPaidConfig offers three months at 0.99
// in the United States, and its equalized equivalent in Great Britain.
func testAccSubscriptionIntroductoryOfferPaidConfig(referenceName, productID, name string) string {
	return testAccSellableSubscriptionConfig(referenceName, productID, name) + `
data "apple_subscription_price_points" "intro" {
  subscription_id = apple_subscription.test.id
  territories     = ["USA"]
  customer_price  = "0.99"
  limit           = 1
}

data "apple_subscription_price_point_equalizations" "intro" {
  price_point_id = data.apple_subscription_price_points.intro.price_points[0].id
  territories    = ["GBR"]
}

resource "apple_subscription_introductory_offer" "test" {
  subscription_id   = apple_subscription.test.id
  offer_mode        = "PAY_AS_YOU_GO"
  duration          = "ONE_MONTH"
  number_of_periods = 3

  price_point_ids = merge(
    { USA = data.apple_subscription_price_points.intro.price_points[0].id },
    data.apple_subscription_price_point_equalizations.intro.price_point_ids,
  )

  depends_on = [apple_subscription_price_schedule.test]
}
`
}

// --- import ID, existence and destroy checks ---

// testAccIntroductoryOfferImportID imports through one territory's record.
func testAccIntroductoryOfferImportID(resourceName, territory string) resource.ImportStateIdFunc {
	return func(s *terraform.State) (string, error) {
		rs, err := testAccStateResource(s, resourceName)
		if err != nil {
			return "", err
		}
		offerID := rs.Attributes["offer_ids."+territory]
		if offerID == "" {
			return "", fmt.Errorf("%s has no offer in %s", resourceName, territory)
		}
		return fmt.Sprintf("%s/%s", rs.Attributes["subscription_id"], offerID), nil
	}
}

// testAccStateOfferIDs reads offer_ids out of flatmapped state.
func testAccStateOfferIDs(attributes map[string]string) map[string]string {
	offerIDs := map[string]string{}
	for key, value := range attributes {
		territory, ok := strings.CutPrefix(key, "offer_ids.")
		if !ok || territory == "%" {
			continue
		}
		offerIDs[territory] = value
	}
	return offerIDs
}

// testAccCheckIntroductoryOfferCount asks Apple how many introductory offer
// records the subscription carries in all, and that every one in state is
// among them. Counting the whole subscription is what catches a bulk write
// that replaced the relationship and dropped a record it should have kept, or
// a removed territory whose record was never deleted.
func testAccCheckIntroductoryOfferCount(resourceName string, want int) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, err := testAccStateResource(s, resourceName)
		if err != nil {
			return err
		}

		client, err := testAccAPIClient()
		if err != nil {
			return fmt.Errorf("could not build API client: %w", err)
		}

		subscriptionID := rs.Attributes["subscription_id"]
		offers, err := client.GetSubscriptionIntroductoryOffers(subscriptionID, nil)
		if err != nil {
			return fmt.Errorf("reading introductory offers of subscription %s: %w", subscriptionID, err)
		}

		if len(offers) != want {
			return fmt.Errorf("subscription %s has %d introductory offer records at Apple, want %d",
				subscriptionID, len(offers), want)
		}

		atApple := map[string]bool{}
		for _, offer := range offers {
			atApple[offer.ID] = true
		}
		for territory, offerID := range testAccStateOfferIDs(rs.Attributes) {
			if !atApple[offerID] {
				return fmt.Errorf("the %s offer %s in state is not at Apple", territory, offerID)
			}
		}

		return nil
	}
}

// testAccCheckIntroductoryOfferEndDates checks the end date Apple holds on
// every record in state, since an update patches each one separately.
func testAccCheckIntroductoryOfferEndDates(resourceName, want string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, err := testAccStateResource(s, resourceName)
		if err != nil {
			return err
		}

		client, err := testAccAPIClient()
		if err != nil {
			return fmt.Errorf("could not build API client: %w", err)
		}

		subscriptionID := rs.Attributes["subscription_id"]
		offers, err := client.GetSubscriptionIntroductoryOffers(subscriptionID, nil)
		if err != nil {
			return fmt.Errorf("reading introductory offers of subscription %s: %w", subscriptionID, err)
		}

		inState := map[string]bool{}
		for _, offerID := range testAccStateOfferIDs(rs.Attributes) {
			inState[offerID] = true
		}

		for _, offer := range offers {
			if !inState[offer.ID] {
				continue
			}
			if offer.Attributes.EndDate == nil || *offer.Attributes.EndDate != want {
				return fmt.Errorf("offer %s ends %v at Apple, want %s", offer.ID, offer.Attributes.EndDate, want)
			}
		}

		return nil
	}
}

// testAccCheckSubscriptionIntroductoryOfferDestroy asks Apple whether any
// record in state survived. A subscription that is itself gone takes its
// offers with it.
func testAccCheckSubscriptionIntroductoryOfferDestroy(s *terraform.State) error {
	client, err := testAccAPIClient()
	if err != nil {
		return fmt.Errorf("could not build API client: %w", err)
	}

	for name, rs := range s.RootModule().Resources {
		if rs.Type != "apple_subscription_introductory_offer" {
			continue
		}

		subscriptionID := rs.Primary.Attributes["subscription_id"]
		offers, err := client.GetSubscriptionIntroductoryOffers(subscriptionID, nil)
		if err != nil {
			if testAccIsNotFound(err) {
				continue
			}
			return fmt.Errorf("unexpected error checking introductory offers of %s: %w", subscriptionID, err)
		}

		inState := map[string]bool{}
		for _, offerID := range testAccStateOfferIDs(rs.Primary.Attributes) {
			inState[offerID] = true
		}
		for _, offer := range offers {
			if inState[offer.ID] {
				return fmt.Errorf("introductory offer %s (%s) still exists at Apple", offer.ID, name)
			}
		}
	}

	return nil
}
