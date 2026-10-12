// Copyright (c) AO Studio
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// Apple creates win-back offers only for a subscription App Review has
// approved, and the API cannot get one approved. These tests therefore need an
// approved subscription that already exists, named here, the way the
// subscription tests need APPLE_TEST_APP_ID. A subscription the suite creates
// itself is never approved, so it cannot stand in.
const testAccWinBackSubscriptionEnvVar = "APPLE_TEST_APPROVED_SUBSCRIPTION_ID"

// testAccPreCheckWinBackOffer skips unless credentials and an approved
// subscription are both available.
func testAccPreCheckWinBackOffer(t *testing.T) {
	t.Helper()
	testAccPreCheck(t)

	if os.Getenv(testAccWinBackSubscriptionEnvVar) == "" {
		t.Skipf("skipping win-back offer acceptance test: %s must name a subscription App Review has "+
			"approved. Apple refuses win-back offers on any other, and the API cannot approve one.",
			testAccWinBackSubscriptionEnvVar)
	}
}

// testAccWinBackOfferID returns an offer_id unique to this run.
//
// Apple requires an offer_id that "must not have already been used for another
// offer in the same subscription group" -- used, not in use -- so a deleted
// offer's identifier is treated as reserved, the way product identifiers are.
// A fixed one would pass once per group and fail on every run afterwards.
func testAccWinBackOfferID() string {
	return "tf_winback_" + strings.ToLower(acctest.RandString(8))
}

// testAccWinBackOfferDate returns a plain date days from now. Apple accepts no
// start earlier than tomorrow, so a fixed date would expire.
func testAccWinBackOfferDate(days int) string {
	return time.Now().UTC().AddDate(0, 0, days).Format("2006-01-02")
}

// TestAccWinBackOfferResource_basic creates a win-back offer on an approved
// subscription, changes it in place, reads it through the listing data source,
// and imports it by offer_id.
//
// The offer starts a month out, so it is never live during the test and the
// in-place changes are ones Apple accepts before an offer goes live. Destroy
// deletes it; its offer_id stays used.
func TestAccWinBackOfferResource_basic(t *testing.T) {
	offerID := testAccWinBackOfferID()
	referenceName := "Terraform Win-Back " + acctest.RandString(6)
	start := testAccWinBackOfferDate(30)
	end := testAccWinBackOfferDate(60)
	laterEnd := testAccWinBackOfferDate(90)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckWinBackOffer(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckWinBackOfferDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccWinBackOfferConfig(offerID, referenceName, start, end, "NORMAL", false),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckWinBackOfferExists("apple_subscription_win_back_offer.test"),
					resource.TestCheckResourceAttr("apple_subscription_win_back_offer.test", "offer_id", offerID),
					resource.TestCheckResourceAttr("apple_subscription_win_back_offer.test", "priority", "NORMAL"),
					resource.TestCheckResourceAttr("apple_subscription_win_back_offer.test",
						"customer_eligibility_time_since_last_subscribed_in_months.minimum", "2"),
					resource.TestCheckResourceAttr("apple_subscription_win_back_offer.test", "prices.#", "1"),
					resource.TestCheckResourceAttrSet("apple_subscription_win_back_offer.test", "promotion_intent"),
				),
			},
			// The eligibility rules, the dates and the priority are the members
			// of Apple's update request, so changing them must not replace the
			// offer -- a replacement would need a new offer_id.
			{
				Config: testAccWinBackOfferConfig(offerID, referenceName, start, laterEnd, "HIGH", true),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("apple_subscription_win_back_offer.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckWinBackOfferExists("apple_subscription_win_back_offer.test"),
					resource.TestCheckResourceAttr("apple_subscription_win_back_offer.test", "priority", "HIGH"),
					resource.TestCheckResourceAttr("apple_subscription_win_back_offer.test", "end_date", laterEnd),
					resource.TestCheckResourceAttr("apple_subscription_win_back_offer.test",
						"customer_eligibility_wait_between_offers_in_months", "6"),
					resource.TestCheckResourceAttr("data.apple_subscription_win_back_offers.test", "win_back_offers.#", "1"),
					resource.TestCheckResourceAttrPair(
						"data.apple_subscription_win_back_offers.test", "win_back_offers.0.id",
						"apple_subscription_win_back_offer.test", "id"),
				),
			},
			// Import by offer_id, the identifier a person has: Apple cannot
			// report the subscription, so it leads the import ID.
			{
				ResourceName:      "apple_subscription_win_back_offer.test",
				ImportState:       true,
				ImportStateIdFunc: testAccWinBackOfferImportID("apple_subscription_win_back_offer.test"),
				ImportStateVerify: true,
			},
		},
	})
}

// TestAccWinBackOfferResource_validation exercises the plan-time checks. It
// creates nothing at Apple: every step fails at plan time.
func TestAccWinBackOfferResource_validation(t *testing.T) {
	config := func(paid, minimum, maximum, wait int, priority, end string) string {
		return fmt.Sprintf(`
resource "apple_subscription_win_back_offer" "test" {
  subscription_id = "12345"
  reference_name  = "Invalid"
  offer_id        = "invalid"
  duration        = "ONE_MONTH"
  offer_mode      = "PAY_AS_YOU_GO"
  period_count    = 1
  priority        = %[5]q
  start_date      = "2030-07-10"
  end_date        = %[6]q

  customer_eligibility_paid_subscription_duration_in_months = %[1]d
  customer_eligibility_time_since_last_subscribed_in_months = { minimum = %[2]d, maximum = %[3]d }
  customer_eligibility_wait_between_offers_in_months        = %[4]d

  prices = [{ price_point_id = "abcdef" }]
}`, paid, minimum, maximum, wait, priority, end)
	}

	tests := []struct {
		name        string
		config      string
		expectError *regexp.Regexp
	}{
		{"paid duration Apple does not offer", config(30, 2, 24, 2, "HIGH", "2030-08-10"), regexp.MustCompile(`Invalid Attribute Value`)},
		{"lapsed window beyond 24 months", config(6, 2, 36, 2, "HIGH", "2030-08-10"), regexp.MustCompile(`Invalid Attribute Value`)},
		{"lapsed window inverted", config(6, 12, 3, 2, "HIGH", "2030-08-10"), regexp.MustCompile(`(?s)minimum 12\s+above\s+maximum\s+3`)},
		{"wait shorter than two months", config(6, 2, 24, 1, "HIGH", "2030-08-10"), regexp.MustCompile(`Invalid Attribute Value`)},
		{"priority outside the enum", config(6, 2, 24, 2, "LOW", "2030-08-10"), regexp.MustCompile(`Invalid Attribute Value Match`)},
		{"end before start", config(6, 2, 24, 2, "HIGH", "2030-07-01"), regexp.MustCompile(`(?s)is\s+not\s+after\s+start_date`)},
		{"end as a timestamp", config(6, 2, 24, 2, "HIGH", "2030-08-10T00:00:00Z"), regexp.MustCompile(`Invalid Attribute Value Match`)},
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

// testAccWinBackOfferConfig prices the offer in the United States at the
// subscription's lowest price point, which is below whatever the approved
// subscription costs.
func testAccWinBackOfferConfig(offerID, referenceName, start, end, priority string, withWait bool) string {
	wait := ""
	listing := ""
	if withWait {
		wait = "customer_eligibility_wait_between_offers_in_months = 6"
		listing = fmt.Sprintf(`
data "apple_subscription_win_back_offers" "test" {
  subscription_id = %[1]q
  offer_id        = apple_subscription_win_back_offer.test.offer_id
}
`, os.Getenv(testAccWinBackSubscriptionEnvVar))
	}

	return fmt.Sprintf(`
data "apple_subscription_price_points" "offer" {
  subscription_id = %[1]q
  territories     = ["USA"]
  customer_price  = "0.99"
  limit           = 1
}

resource "apple_subscription_win_back_offer" "test" {
  subscription_id = %[1]q
  reference_name  = %[2]q
  offer_id        = %[3]q
  duration        = "ONE_MONTH"
  offer_mode      = "PAY_AS_YOU_GO"
  period_count    = 1
  start_date      = %[4]q
  end_date        = %[5]q
  priority        = %[6]q

  customer_eligibility_paid_subscription_duration_in_months = 1
  customer_eligibility_time_since_last_subscribed_in_months = { minimum = 2, maximum = 24 }
  %[7]s

  prices = [{
    territory_id   = "USA"
    price_point_id = data.apple_subscription_price_points.offer.price_points[0].id
  }]
}
%[8]s`, os.Getenv(testAccWinBackSubscriptionEnvVar), referenceName, offerID, start, end, priority, wait, listing)
}

func testAccWinBackOfferImportID(resourceName string) resource.ImportStateIdFunc {
	return func(s *terraform.State) (string, error) {
		rs, err := testAccStateResource(s, resourceName)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s/%s", rs.Attributes["subscription_id"], rs.Attributes["offer_id"]), nil
	}
}

// testAccCheckWinBackOfferExists asks Apple, and checks the offer is listed
// under the subscription state says it belongs to -- the one fact Apple never
// reports on the offer itself.
func testAccCheckWinBackOfferExists(resourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, err := testAccStateResource(s, resourceName)
		if err != nil {
			return err
		}

		client, err := testAccAPIClient()
		if err != nil {
			return fmt.Errorf("could not build API client: %w", err)
		}

		offer, err := client.GetWinBackOffer(rs.ID)
		if err != nil {
			return fmt.Errorf("win-back offer %s not found at Apple: %w", rs.ID, err)
		}
		if offer.Attributes.OfferID != rs.Attributes["offer_id"] {
			return fmt.Errorf("Apple reports offer_id %q, state holds %q", offer.Attributes.OfferID, rs.Attributes["offer_id"])
		}

		prices, err := client.GetWinBackOfferPrices(rs.ID)
		if err != nil {
			return fmt.Errorf("reading prices of win-back offer %s: %w", rs.ID, err)
		}
		if got, want := fmt.Sprint(len(prices)), rs.Attributes["prices.#"]; got != want {
			return fmt.Errorf("win-back offer %s has %s prices at Apple, state holds %s", rs.ID, got, want)
		}

		offers, err := client.GetWinBackOffers(rs.Attributes["subscription_id"])
		if err != nil {
			return fmt.Errorf("listing win-back offers of subscription %s: %w", rs.Attributes["subscription_id"], err)
		}
		for _, listed := range offers {
			if listed.ID == rs.ID {
				return nil
			}
		}
		return fmt.Errorf("win-back offer %s is not listed under subscription %s", rs.ID, rs.Attributes["subscription_id"])
	}
}

func testAccCheckWinBackOfferDestroy(s *terraform.State) error {
	client, err := testAccAPIClient()
	if err != nil {
		return fmt.Errorf("could not build API client: %w", err)
	}

	for name, rs := range s.RootModule().Resources {
		if rs.Type != "apple_subscription_win_back_offer" {
			continue
		}

		if _, err := client.GetWinBackOffer(rs.Primary.ID); err == nil {
			return fmt.Errorf("win-back offer %s (%s) still exists at Apple", rs.Primary.ID, name)
		} else if !testAccIsNotFound(err) {
			return fmt.Errorf("unexpected error checking win-back offer %s: %w", rs.Primary.ID, err)
		}
	}

	return nil
}
