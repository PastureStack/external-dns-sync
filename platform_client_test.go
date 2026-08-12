package main

// Created by PastureStack contributors for legacy event contract testing.

import (
	"testing"

	"github.com/PastureStack/external-dns-sync/utils"
)

func TestExternalDNSUpdateEventPreservesCompatibilityContract(t *testing.T) {
	record := utils.MetadataDnsRecord{
		ServiceName: "api",
		StackName:   "production",
		DnsRecord: utils.DnsRecord{
			Fqdn: "api.production.primary.example.test.",
		},
	}

	event := newExternalDNSUpdateEvent(record)

	if event.EventType != "dns.update" {
		t.Fatalf("unexpected event type: %q", event.EventType)
	}
	if event.ExternalID != record.DnsRecord.Fqdn {
		t.Fatalf("unexpected external ID: %q", event.ExternalID)
	}
	if event.ServiceName != "api" || event.StackName != "production" {
		t.Fatalf("unexpected service identity: %s/%s", event.StackName, event.ServiceName)
	}
	if event.FQDN != "api.production.primary.example.test" {
		t.Fatalf("expected compatibility API FQDN without trailing dot, got %q", event.FQDN)
	}
}
