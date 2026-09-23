package cmd

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/runtimeunit"
)

func TestRootHelpShowsBannerAndNoResetCommand(t *testing.T) {
	originalVersion := Version
	t.Cleanup(func() {
		SetVersion(originalVersion)
		rootCmd.SetOut(os.Stdout)
		rootCmd.SetErr(os.Stderr)
	})

	SetVersion("9.8.7-test")

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)

	if err := rootCmd.Help(); err != nil {
		t.Fatalf("render root help: %v", err)
	}

	help := out.String()
	for _, want := range []string{
		"Chatto is a self-hostable chat server for teams and communities.",
		"Version: 9.8.7-test | Self-hosting docs: https://docs.chatto.run",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("root help missing %q:\n%s", want, help)
		}
	}

	if strings.Contains(help, "\n  reset ") {
		t.Fatalf("root help should not list reset command:\n%s", help)
	}
}

func TestRootRegistersExporterCommand(t *testing.T) {
	cmd, _, err := rootCmd.Find([]string{"exporter", "--help"})
	if err != nil {
		t.Fatalf("find exporter command: %v", err)
	}
	if cmd == nil || cmd.Use != "exporter" {
		t.Fatalf("root command did not register exporter, got %#v", cmd)
	}
}

func TestRootRegistersSearchProviderCommand(t *testing.T) {
	command, _, err := rootCmd.Find([]string{"search-provider"})
	if err != nil {
		t.Fatalf("find search-provider command: %v", err)
	}
	if command != searchProviderCmd {
		t.Fatalf("found command %q, want search-provider", command.Name())
	}
}

func TestRootRegistersAssetProcessingCommand(t *testing.T) {
	command, _, err := rootCmd.Find([]string{"asset-processing"})
	if err != nil {
		t.Fatalf("find asset-processing command: %v", err)
	}
	if command != assetProcessingCmd {
		t.Fatalf("found command %q, want asset-processing", command.Name())
	}
}

func TestAssetProcessingRuntimeRegistrationUsesDedicatedConfig(t *testing.T) {
	var registration *runtimeunit.Registration
	registrations := runtimeUnitRegistrations()
	for i := range registrations {
		if registrations[i].Unit.Name() == "asset-processing" {
			registration = &registrations[i]
			break
		}
	}
	if registration == nil {
		t.Fatal("asset-processing runtime unit is not registered")
	}
	if registration.Enabled(config.ChattoConfig{Video: config.VideoConfig{Enabled: true}}) {
		t.Fatal("video uploads must not enable the built-in asset-processing worker")
	}
	if !registration.Enabled(config.ChattoConfig{AssetProcessing: config.AssetProcessingConfig{Enabled: true}}) {
		t.Fatal("asset_processing.enabled should enable the built-in worker")
	}
}
