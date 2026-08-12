package main

import (
	"errors"
	"reflect"
	"testing"

	"github.com/PastureStack/external-dns-sync/utils"
)

type lifecycleProvider struct {
	records   []utils.DnsRecord
	changes   []string
	failFQDN  string
	failError error
}

func (*lifecycleProvider) Init(string) error  { return nil }
func (*lifecycleProvider) GetName() string    { return "lifecycle-test" }
func (*lifecycleProvider) HealthCheck() error { return nil }
func (p *lifecycleProvider) GetRecords() ([]utils.DnsRecord, error) {
	return append([]utils.DnsRecord(nil), p.records...), nil
}
func (p *lifecycleProvider) AddRecord(record utils.DnsRecord) error {
	return p.change("add", record)
}
func (p *lifecycleProvider) RemoveRecord(record utils.DnsRecord) error {
	return p.change("remove", record)
}
func (p *lifecycleProvider) UpdateRecord(record utils.DnsRecord) error {
	return p.change("update", record)
}
func (p *lifecycleProvider) change(action string, record utils.DnsRecord) error {
	p.changes = append(p.changes, action+":"+record.Type+":"+record.Fqdn)
	if record.Fqdn == p.failFQDN {
		return p.failError
	}
	return nil
}

func TestUpdateProviderAddsAddressBeforeOwnershipState(t *testing.T) {
	previousProvider := provider
	defer func() { provider = previousProvider }()

	fake := &lifecycleProvider{}
	provider = fake
	records := map[string]utils.MetadataDnsRecord{
		"external-dns-env.example.test.": {
			DnsRecord: utils.DnsRecord{
				Fqdn:    "external-dns-env.example.test.",
				Type:    "TXT",
				TTL:     60,
				Records: []string{"api.production.example.test."},
			},
		},
		"api.production.example.test.": {
			ServiceName: "api",
			StackName:   "production",
			DnsRecord: utils.DnsRecord{
				Fqdn:    "api.production.example.test.",
				Type:    "A",
				TTL:     60,
				Records: []string{"192.0.2.10"},
			},
		},
	}

	updated, err := UpdateProviderDnsRecords(records)
	if err != nil {
		t.Fatal(err)
	}
	if len(updated) != 2 {
		t.Fatalf("updated records = %d, want 2", len(updated))
	}
	want := []string{
		"add:A:api.production.example.test.",
		"add:TXT:external-dns-env.example.test.",
	}
	if !reflect.DeepEqual(fake.changes, want) {
		t.Fatalf("changes = %#v, want %#v", fake.changes, want)
	}
}

func TestUpdateProviderStopsBeforeOwnershipStateWhenAddressFails(t *testing.T) {
	previousProvider := provider
	defer func() { provider = previousProvider }()

	fake := &lifecycleProvider{
		failFQDN:  "api.production.example.test.",
		failError: errors.New("provider unavailable"),
	}
	provider = fake
	records := map[string]utils.MetadataDnsRecord{
		"external-dns-env.example.test.": {
			DnsRecord: utils.DnsRecord{
				Fqdn:    "external-dns-env.example.test.",
				Type:    "TXT",
				TTL:     60,
				Records: []string{"api.production.example.test."},
			},
		},
		"api.production.example.test.": {
			DnsRecord: utils.DnsRecord{
				Fqdn:    "api.production.example.test.",
				Type:    "A",
				TTL:     60,
				Records: []string{"192.0.2.10"},
			},
		},
	}

	if _, err := UpdateProviderDnsRecords(records); err == nil {
		t.Fatal("expected provider failure")
	}
	want := []string{"add:A:api.production.example.test."}
	if !reflect.DeepEqual(fake.changes, want) {
		t.Fatalf("changes = %#v, want %#v", fake.changes, want)
	}
}

func TestFirstSynchronizationReconcilesAnEmptyDesiredState(t *testing.T) {
	empty := map[string]utils.MetadataDnsRecord{}
	if !shouldSynchronize(false, false, empty, empty) {
		t.Fatal("first synchronization must reconcile provider state even when metadata is empty")
	}
	if shouldSynchronize(true, false, empty, empty) {
		t.Fatal("an unchanged state after the first synchronization should be skipped")
	}
	if !shouldSynchronize(true, true, empty, empty) {
		t.Fatal("a forced synchronization must reconcile unchanged state")
	}
}
