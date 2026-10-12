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
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// Offer codes are the one part of the subscription suite that cannot be
// cleaned up. Apple publishes no DELETE for an offer, a custom code or a batch
// of one-time-use codes; destroy deactivates them, and they stay listed under
// the subscription forever. An offer attached to a subscription may also stop
// that subscription from being deleted. So these tests do not create a
// subscription of their own: they need one that already exists, named by
// APPLE_TEST_OFFER_SUBSCRIPTION_ID, and naming one is the statement that
// leaving deactivated offers on it is acceptable -- credentials and a test app
// are deliberately not enough, the same guard the app pricing tests are behind.
const testAccOfferSubscriptionEnvVar = "APPLE_TEST_OFFER_SUBSCRIPTION_ID"

func testAccPreCheckOfferCode(t *testing.T) {
	t.Helper()
	testAccPreCheck(t)

	if os.Getenv(testAccOfferSubscriptionEnvVar) == "" {
		t.Skipf("skipping offer code acceptance test: %s must name an existing subscription that is available "+
			"in the USA. Apple cannot delete an offer code, so every run leaves deactivated offers on it for good.",
			testAccOfferSubscriptionEnvVar)
	}
}

// testAccCustomCode returns a custom code string unique to this run.
//
// App Store Connect refuses a custom code another active offer of the same app
// uses, and a run interrupted before its destroy leaves one active, so a fixed
// string would fail every later run until somebody deactivated it by hand.
func testAccCustomCode() string {
	return "TF" + strings.ToUpper(acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum))
}

// TestAccSubscriptionOfferCodeResource_basic creates a free-trial offer, a
// custom code and a sandbox batch of one-time-use codes, reads the codes back,
// deactivates the custom code in place, and imports all three.
//
// It is a FREE_TRIAL so it needs no price point, and the batch is SANDBOX at
// Apple's minimum of 10 so it costs the app's quarterly code quota as little
// as possible. Everything it creates stays at Apple, deactivated.
func TestAccSubscriptionOfferCodeResource_basic(t *testing.T) {
	name := "Terraform Offer " + acctest.RandString(6)
	customCode := testAccCustomCode()
	expiration := time.Now().UTC().AddDate(0, 1, 0).Format("2006-01-02")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckOfferCode(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckOfferCodesDeactivated,
		Steps: []resource.TestStep{
			{
				Config: testAccSubscriptionOfferCodeConfig(name, customCode, expiration, true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("apple_subscription_offer_code.test", "id"),
					resource.TestCheckResourceAttr("apple_subscription_offer_code.test", "active", "true"),
					resource.TestCheckResourceAttr("apple_subscription_offer_code.test", "prices.#", "1"),
					resource.TestCheckResourceAttr("apple_subscription_offer_code_custom_code.test", "custom_code", customCode),
					resource.TestCheckResourceAttr("apple_subscription_offer_code_custom_code.test", "active", "true"),
					resource.TestCheckResourceAttr("apple_subscription_offer_code_one_time_use_code.test", "environment", "SANDBOX"),
					resource.TestCheckResourceAttr("apple_subscription_offer_code_one_time_use_code.test", "number_of_codes", "10"),
					testAccCheckOfferCodeActive("apple_subscription_offer_code.test", true),
				),
			},
			// Reading the generated codes is opt-in and separate from the
			// batch, so it is its own step: Apple generates them after the
			// batch is created, and this is the first thing a run should
			// confirm about the values endpoint.
			{
				Config: testAccSubscriptionOfferCodeConfig(name, customCode, expiration, true) + `
data "apple_subscription_offer_code_one_time_use_code_values" "test" {
  one_time_use_code_id = apple_subscription_offer_code_one_time_use_code.test.id
}
`,
				Check: resource.TestCheckResourceAttrSet(
					"data.apple_subscription_offer_code_one_time_use_code_values.test", "csv"),
			},
			// active is the one member of Apple's update request, so ending a
			// custom code is an in-place update rather than a replacement.
			{
				Config: testAccSubscriptionOfferCodeConfig(name, customCode, expiration, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("apple_subscription_offer_code_custom_code.test", "active", "false"),
					testAccCheckCustomCodeActive("apple_subscription_offer_code_custom_code.test", false),
				),
			},
			{
				ResourceName:      "apple_subscription_offer_code.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				ResourceName:      "apple_subscription_offer_code_custom_code.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				ResourceName:      "apple_subscription_offer_code_one_time_use_code.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// TestAccSubscriptionOfferCodeResource_validation covers what the provider
// refuses at plan time. Nothing reaches Apple.
func TestAccSubscriptionOfferCodeResource_validation(t *testing.T) {
	farFuture := time.Now().UTC().AddDate(1, 0, 0).Format("2006-01-02")

	tests := []struct {
		name        string
		config      string
		expectError *regexp.Regexp
	}{
		{
			name: "paid offer with a territory that has no price point",
			config: `
resource "apple_subscription_offer_code" "test" {
  subscription_id        = "12345"
  name                   = "Invalid"
  customer_eligibilities = ["NEW"]
  offer_eligibility      = "STACK_WITH_INTRO_OFFERS"
  duration               = "ONE_MONTH"
  offer_mode             = "PAY_AS_YOU_GO"
  number_of_periods      = 3
  prices                 = [{ territory_id = "USA" }]
}`,
			expectError: regexp.MustCompile(`USA has no price_point_id`),
		},
		{
			name: "custom code with a special character",
			config: `
resource "apple_subscription_offer_code_custom_code" "test" {
  offer_code_id   = "12345"
  custom_code     = "SCHOOL-2026"
  number_of_codes = 100
}`,
			expectError: regexp.MustCompile(`Invalid Attribute Value Match`),
		},
		{
			name: "production batch below Apple's minimum",
			config: `
resource "apple_subscription_offer_code_one_time_use_code" "test" {
  offer_code_id   = "12345"
  number_of_codes = 10
  expiration_date = "2026-12-31"
}`,
			expectError: regexp.MustCompile(`between 500 and 25000`),
		},
		{
			name: "expiration more than six months away",
			config: fmt.Sprintf(`
resource "apple_subscription_offer_code_one_time_use_code" "test" {
  offer_code_id   = "12345"
  number_of_codes = 10
  environment     = "SANDBOX"
  expiration_date = %q
}`, farFuture),
			expectError: regexp.MustCompile(`more than six months away`),
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

func testAccSubscriptionOfferCodeConfig(name, customCode, expiration string, customCodeActive bool) string {
	return testAccProviderConfig + fmt.Sprintf(`
resource "apple_subscription_offer_code" "test" {
  subscription_id        = %[1]q
  name                   = %[2]q
  customer_eligibilities = ["NEW", "EXPIRED"]
  offer_eligibility      = "STACK_WITH_INTRO_OFFERS"
  duration               = "ONE_WEEK"
  offer_mode             = "FREE_TRIAL"
  number_of_periods      = 1

  # A free trial has no price, only the territories it is offered in.
  prices = [{ territory_id = "USA" }]
}

resource "apple_subscription_offer_code_custom_code" "test" {
  offer_code_id   = apple_subscription_offer_code.test.id
  custom_code     = %[3]q
  number_of_codes = 1
  expiration_date = %[4]q
  active          = %[5]t
}

resource "apple_subscription_offer_code_one_time_use_code" "test" {
  offer_code_id   = apple_subscription_offer_code.test.id
  number_of_codes = 10
  environment     = "SANDBOX"
  expiration_date = %[4]q
}
`, os.Getenv(testAccOfferSubscriptionEnvVar), name, customCode, expiration, customCodeActive)
}

// testAccCheckOfferCodeActive asks Apple whether the offer is active.
func testAccCheckOfferCodeActive(resourceName string, want bool) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, err := testAccStateResource(s, resourceName)
		if err != nil {
			return err
		}
		client, err := testAccAPIClient()
		if err != nil {
			return fmt.Errorf("could not build API client: %w", err)
		}

		offer, err := client.GetSubscriptionOfferCode(rs.ID)
		if err != nil {
			return fmt.Errorf("reading offer code %s: %w", rs.ID, err)
		}
		if offer.Attributes.Active == nil || *offer.Attributes.Active != want {
			return fmt.Errorf("offer code %s active = %v at Apple, want %t", rs.ID, offer.Attributes.Active, want)
		}
		return nil
	}
}

// testAccCheckCustomCodeActive asks Apple whether the custom code is active.
func testAccCheckCustomCodeActive(resourceName string, want bool) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, err := testAccStateResource(s, resourceName)
		if err != nil {
			return err
		}
		client, err := testAccAPIClient()
		if err != nil {
			return fmt.Errorf("could not build API client: %w", err)
		}

		code, err := client.GetSubscriptionOfferCodeCustomCode(rs.ID)
		if err != nil {
			return fmt.Errorf("reading custom code %s: %w", rs.ID, err)
		}
		if code.Attributes.Active == nil || *code.Attributes.Active != want {
			return fmt.Errorf("custom code %s active = %v at Apple, want %t", rs.ID, code.Attributes.Active, want)
		}
		return nil
	}
}

// testAccCheckOfferCodesDeactivated is the destroy check for records Apple
// cannot delete: each must still exist, and be inactive. A check that only
// read Terraform state would pass even if the deactivation never happened.
func testAccCheckOfferCodesDeactivated(s *terraform.State) error {
	client, err := testAccAPIClient()
	if err != nil {
		return fmt.Errorf("could not build API client: %w", err)
	}

	for _, rs := range s.RootModule().Resources {
		var active *bool
		switch rs.Type {
		case "apple_subscription_offer_code":
			offer, err := client.GetSubscriptionOfferCode(rs.Primary.ID)
			if err != nil {
				return fmt.Errorf("reading offer code %s after destroy: %w", rs.Primary.ID, err)
			}
			active = offer.Attributes.Active
		case "apple_subscription_offer_code_custom_code":
			code, err := client.GetSubscriptionOfferCodeCustomCode(rs.Primary.ID)
			if err != nil {
				return fmt.Errorf("reading custom code %s after destroy: %w", rs.Primary.ID, err)
			}
			active = code.Attributes.Active
		case "apple_subscription_offer_code_one_time_use_code":
			batch, err := client.GetSubscriptionOfferCodeOneTimeUseCode(rs.Primary.ID)
			if err != nil {
				return fmt.Errorf("reading one-time-use codes %s after destroy: %w", rs.Primary.ID, err)
			}
			active = batch.Attributes.Active
		default:
			continue
		}

		if active == nil || *active {
			return fmt.Errorf("%s %s is still active at Apple after destroy", rs.Type, rs.Primary.ID)
		}
	}

	return nil
}
