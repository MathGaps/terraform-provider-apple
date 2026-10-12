// Copyright (c) AO Studio
// SPDX-License-Identifier: MPL-2.0

package subscription

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

// TestWinBackOfferSchemas runs the win-back offer resource and data source
// schemas through the framework's own rules and pins their type names, which
// are breaking to rename. The framework otherwise validates a schema only when
// the provider server starts, which only the acceptance tier does -- and that
// tier needs an approved subscription, so it does not run in CI.
func TestWinBackOfferSchemas(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("resource", func(t *testing.T) {
		t.Parallel()

		r := NewWinBackOfferResource()

		meta := &resource.MetadataResponse{}
		r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "apple"}, meta)
		if meta.TypeName != "apple_subscription_win_back_offer" {
			t.Errorf("type name = %q, want apple_subscription_win_back_offer", meta.TypeName)
		}

		resp := &resource.SchemaResponse{}
		r.Schema(ctx, resource.SchemaRequest{}, resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("schema returned errors: %+v", resp.Diagnostics)
		}
		if diags := resp.Schema.ValidateImplementation(ctx); diags.HasError() {
			t.Fatalf("schema failed validation: %+v", diags)
		}
	})

	t.Run("data source", func(t *testing.T) {
		t.Parallel()

		d := NewWinBackOffersDataSource()

		meta := &datasource.MetadataResponse{}
		d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "apple"}, meta)
		if meta.TypeName != "apple_subscription_win_back_offers" {
			t.Errorf("type name = %q, want apple_subscription_win_back_offers", meta.TypeName)
		}

		resp := &datasource.SchemaResponse{}
		d.Schema(ctx, datasource.SchemaRequest{}, resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("schema returned errors: %+v", resp.Diagnostics)
		}
		if diags := resp.Schema.ValidateImplementation(ctx); diags.HasError() {
			t.Fatalf("schema failed validation: %+v", diags)
		}
	})
}
