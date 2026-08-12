package alidns

import (
	"testing"

	"github.com/PastureStack/external-dns-sync/utils"
)

func TestPrepareRecordPreservesValidTTL(t *testing.T) {
	provider := &AlidnsProvider{rootDomainName: "example.test"}
	for _, ttl := range []int{1, 300, 1<<31 - 1} {
		prepared, err := provider.prepareRecord(utils.DnsRecord{
			Fqdn: "api.example.test.",
			Type: "A",
			TTL:  ttl,
		}, "192.0.2.10")
		if err != nil {
			t.Fatalf("valid TTL %d was rejected: %v", ttl, err)
		}
		if int64(prepared.TTL) != int64(ttl) {
			t.Fatalf("TTL was truncated: got %d, want %d", prepared.TTL, ttl)
		}
	}
}

func TestPrepareRecordRejectsTTLThatWouldTruncate(t *testing.T) {
	provider := &AlidnsProvider{rootDomainName: "example.test"}
	for _, ttl := range []int{0, -1, int(uint64(1) << 31)} {
		if _, err := provider.prepareRecord(utils.DnsRecord{
			Fqdn: "api.example.test.",
			Type: "A",
			TTL:  ttl,
		}, "192.0.2.10"); err == nil {
			t.Fatalf("unsafe TTL %d was accepted", ttl)
		}
	}
}
