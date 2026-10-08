// Copyright (c) AO Studio
// SPDX-License-Identifier: MPL-2.0

// Package main serves the Apple Terraform provider using the Terraform Plugin Framework.
// This provider enables management of Apple App Store Connect resources through Terraform.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/AhmedOsman00/terraform-provider-apple/internal/provider"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var (
	// version is set by the goreleaser configuration to appropriate values for the compiled binary.
	// Defaults to "dev" for local development builds.
	// See: https://goreleaser.com/cookbooks/using-main.version/
	version string = "dev"
)

func main() {
	var debug bool

	flag.BoolVar(&debug, "debug", false, "set to true to run the provider with support for debuggers like delve")
	flag.Parse()

	// Create a context that can be cancelled for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		tflog.Info(ctx, "Received shutdown signal", map[string]interface{}{
			"signal": sig.String(),
		})
		cancel()
	}()

	opts := providerserver.ServeOpts{
		// The OpenTofu Registry address MathGaps's fork is published under.
		// Terraform normalizes source addresses to lower case, so the registry
		// namespace "MathGaps" is written here as "mathgaps".
		Address: "registry.opentofu.org/mathgaps/apple",
		Debug:   debug,
	}

	tflog.Info(ctx, "Starting Apple Terraform provider", map[string]interface{}{
		"version": version,
		"address": opts.Address,
		"debug":   debug,
	})

	err := providerserver.Serve(ctx, provider.New(version), opts)
	if err != nil {
		tflog.Error(ctx, "Failed to serve provider", map[string]interface{}{
			"error": err.Error(),
		})
		fmt.Fprintf(os.Stderr, "Error: Failed to serve Apple Terraform provider: %v\n", err)
		os.Exit(1)
	}

	tflog.Info(ctx, "Apple Terraform provider shutdown complete")
}
