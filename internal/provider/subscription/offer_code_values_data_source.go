// Copyright (c) AO Studio
// SPDX-License-Identifier: MPL-2.0

package subscription

import (
	"context"
	"fmt"

	"github.com/AhmedOsman00/terraform-provider-apple/internal/apple"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var (
	_ datasource.DataSource              = &subscriptionOfferCodeValuesDataSource{}
	_ datasource.DataSourceWithConfigure = &subscriptionOfferCodeValuesDataSource{}
)

func NewSubscriptionOfferCodeOneTimeUseCodeValuesDataSource() datasource.DataSource {
	return &subscriptionOfferCodeValuesDataSource{}
}

type subscriptionOfferCodeValuesDataSource struct {
	client *apple.Client
}

// Metadata returns the data source type name.
func (d *subscriptionOfferCodeValuesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_subscription_offer_code_one_time_use_code_values"
}

// Schema defines the schema for the data source.
func (d *subscriptionOfferCodeValuesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads the codes Apple generated for an " +
			"`apple_subscription_offer_code_one_time_use_code` batch — the same file App Store Connect " +
			"offers for download.\n\n" +
			"This is a separate, opt-in data source rather than an attribute of the batch on purpose: every " +
			"code is a secret anyone can redeem, and a batch holds up to 25,000 of them. Declare it only in the " +
			"configuration that distributes the codes.\n\n" +
			"~> **The codes land in Terraform state in plain text.** `csv` is marked sensitive, which keeps it " +
			"out of plan and console output, not out of the state file. Use a state backend that is encrypted " +
			"and access-controlled, and prefer handing the value straight to wherever the codes go (a secret " +
			"store, a mail-merge file) over outputting it.\n\n" +
			"Apple says codes can take up to an hour to become redeemable after the batch is created.",

		Attributes: map[string]schema.Attribute{
			"one_time_use_code_id": schema.StringAttribute{
				MarkdownDescription: "The ID of the `apple_subscription_offer_code_one_time_use_code` batch.",
				Required:            true,
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "The batch ID, repeated.",
				Computed:            true,
			},
			"csv": schema.StringAttribute{
				MarkdownDescription: "The codes as the CSV document Apple returns, unmodified — Apple's API " +
					"specification gives its media type as `text/csv` but does not describe its columns, so " +
					"the provider does not parse it. Inspect it once, then split it with `csvdecode()` (if it " +
					"has a header row) or `split(\"\\n\", ...)`.",
				Computed:  true,
				Sensitive: true,
			},
		},
	}
}

// Read downloads the batch's codes.
func (d *subscriptionOfferCodeValuesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config subscriptionOfferCodeValuesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	batchID := config.OneTimeUseCodeID.ValueString()
	// Never log the codes themselves.
	tflog.Debug(ctx, "Reading one-time-use code values", map[string]interface{}{"one_time_use_code_id": batchID})

	csv, err := d.client.GetSubscriptionOfferCodeOneTimeUseCodeValues(batchID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Read One-Time-Use Codes",
			fmt.Sprintf("Could not download the codes of batch '%s': %s\n\n"+
				"Apple generates the codes after the batch is created and says they can take up to an hour "+
				"to become redeemable; a batch read too soon may not have them yet.", batchID, err.Error()),
		)
		return
	}

	config.ID = types.StringValue(batchID)
	config.CSV = types.StringValue(csv)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

// Configure adds the provider configured client to the data source.
func (d *subscriptionOfferCodeValuesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*apple.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *apple.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	d.client = client
}
