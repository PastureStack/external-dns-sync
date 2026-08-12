package providers

// Created by PastureStack contributors for provider registry testing without a real DNS zone.

import (
	"testing"

	"github.com/PastureStack/external-dns-sync/utils"
)

type registryTestProvider struct {
	initializedRoot string
}

func (provider *registryTestProvider) Init(rootDomainName string) error {
	provider.initializedRoot = rootDomainName
	return nil
}

func (*registryTestProvider) GetName() string                        { return "registry-test" }
func (*registryTestProvider) HealthCheck() error                     { return nil }
func (*registryTestProvider) AddRecord(utils.DnsRecord) error        { return nil }
func (*registryTestProvider) RemoveRecord(utils.DnsRecord) error     { return nil }
func (*registryTestProvider) UpdateRecord(utils.DnsRecord) error     { return nil }
func (*registryTestProvider) GetRecords() ([]utils.DnsRecord, error) { return nil, nil }

func TestProviderRegistryInitializesSelectedProvider(t *testing.T) {
	const providerName = "pasturestack-unit-test"
	delete(providers, providerName)
	defer delete(providers, providerName)

	registered := &registryTestProvider{}
	RegisterProvider(providerName, registered)

	selected, err := GetProvider(providerName, "example.test.")
	if err != nil {
		t.Fatalf("expected registered provider, got error: %v", err)
	}
	if selected != registered {
		t.Fatalf("registry returned a different provider: %#v", selected)
	}
	if registered.initializedRoot != "example.test." {
		t.Fatalf("provider initialized with %q", registered.initializedRoot)
	}
}

func TestProviderRegistryRejectsUnknownProvider(t *testing.T) {
	if _, err := GetProvider("provider-that-does-not-exist", "example.test."); err == nil {
		t.Fatal("expected an error for an unknown provider")
	}
}
