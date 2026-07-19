package config

// Created by PastureStack contributors for independent compatibility testing.

import "testing"

func TestSetFromEnvironmentUsesCompatibilityAPISettingsAndDefaults(t *testing.T) {
	t.Setenv("PLATFORM_URL", "https://platform.example.test/v1")
	t.Setenv("PLATFORM_ACCESS_KEY", "test-access-key")
	t.Setenv("PLATFORM_SECRET_KEY", "test-secret-key")
	t.Setenv("ROOT_DOMAIN", "example.test")
	t.Setenv("NAME_TEMPLATE", "")
	t.Setenv("TTL", "not-a-number")
	t.Setenv("METADATA_URL", "")
	t.Setenv("HEALTH_ADDRESS", "")

	SetFromEnvironment()

	if PlatformURL != "https://platform.example.test/v1" {
		t.Fatalf("unexpected platform URL: %q", PlatformURL)
	}
	if PlatformAccessKey != "test-access-key" {
		t.Fatalf("unexpected platform access key: %q", PlatformAccessKey)
	}
	if PlatformSecretKey != "test-secret-key" {
		t.Fatalf("unexpected platform secret key: %q", PlatformSecretKey)
	}
	if RootDomainName != "example.test." {
		t.Fatalf("expected normalized root domain, got %q", RootDomainName)
	}
	if NameTemplate != defaultNameTemplate {
		t.Fatalf("expected default name template %q, got %q", defaultNameTemplate, NameTemplate)
	}
	if TTL != 300 {
		t.Fatalf("expected default TTL 300, got %d", TTL)
	}
	if MetadataURL != "http://metadata/2015-12-19" {
		t.Fatalf("unexpected metadata URL: %q", MetadataURL)
	}
	if HealthAddress != ":10000" {
		t.Fatalf("unexpected health address: %q", HealthAddress)
	}
}

func TestSetFromEnvironmentAcceptsExplicitTemplateAndTTL(t *testing.T) {
	t.Setenv("PLATFORM_URL", "https://platform.example.test/v1")
	t.Setenv("PLATFORM_ACCESS_KEY", "test-access-key")
	t.Setenv("PLATFORM_SECRET_KEY", "test-secret-key")
	t.Setenv("ROOT_DOMAIN", "example.test.")
	t.Setenv("NAME_TEMPLATE", "%{{service_name}}.%{{environment_name}}")
	t.Setenv("TTL", "120")
	t.Setenv("METADATA_URL", "http://metadata.example.test/2026-07-25")
	t.Setenv("HEALTH_ADDRESS", ":18080")

	SetFromEnvironment()

	if NameTemplate != "%{{service_name}}.%{{environment_name}}" {
		t.Fatalf("unexpected explicit name template: %q", NameTemplate)
	}
	if TTL != 120 {
		t.Fatalf("expected TTL 120, got %d", TTL)
	}
	if MetadataURL != "http://metadata.example.test/2026-07-25" {
		t.Fatalf("unexpected explicit metadata URL: %q", MetadataURL)
	}
	if HealthAddress != ":18080" {
		t.Fatalf("unexpected explicit health address: %q", HealthAddress)
	}
}

func TestSetFromEnvironmentRejectsNonPositiveTTL(t *testing.T) {
	t.Setenv("PLATFORM_URL", "https://platform.example.test/v1")
	t.Setenv("PLATFORM_ACCESS_KEY", "test-access-key")
	t.Setenv("PLATFORM_SECRET_KEY", "test-secret-key")
	t.Setenv("ROOT_DOMAIN", "example.test")
	t.Setenv("TTL", "0")

	SetFromEnvironment()

	if TTL != 300 {
		t.Fatalf("expected default TTL 300, got %d", TTL)
	}
}
