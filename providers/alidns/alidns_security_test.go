package alidns

import (
	"testing"

	"github.com/PastureStack/external-dns-sync/utils"
	alidns "github.com/alibabacloud-go/alidns-20150109/v5/client"
)

type alidnsTestClient struct {
	pages    []*alidns.DescribeDomainRecordsResponse
	requests []*alidns.DescribeDomainRecordsRequest
}

func (*alidnsTestClient) DescribeDomainInfo(*alidns.DescribeDomainInfoRequest) (*alidns.DescribeDomainInfoResponse, error) {
	return &alidns.DescribeDomainInfoResponse{}, nil
}

func (client *alidnsTestClient) DescribeDomainRecords(request *alidns.DescribeDomainRecordsRequest) (*alidns.DescribeDomainRecordsResponse, error) {
	client.requests = append(client.requests, request)
	index := len(client.requests) - 1
	return client.pages[index], nil
}

func (*alidnsTestClient) AddDomainRecord(*alidns.AddDomainRecordRequest) (*alidns.AddDomainRecordResponse, error) {
	return &alidns.AddDomainRecordResponse{}, nil
}

func (*alidnsTestClient) DeleteDomainRecord(*alidns.DeleteDomainRecordRequest) (*alidns.DeleteDomainRecordResponse, error) {
	return &alidns.DeleteDomainRecordResponse{}, nil
}

func TestPrepareRecordPreservesValidTTL(t *testing.T) {
	provider := &AlidnsProvider{rootDomainName: "example.test"}
	for _, ttl := range []int{1, 300, 1 << 31} {
		prepared, err := provider.prepareRecord(utils.DnsRecord{
			Fqdn: "api.example.test.",
			Type: "A",
			TTL:  ttl,
		}, "192.0.2.10")
		if err != nil {
			t.Fatalf("valid TTL %d was rejected: %v", ttl, err)
		}
		if prepared.TTL == nil || *prepared.TTL != int64(ttl) {
			t.Fatalf("TTL was changed: got %v, want %d", prepared.TTL, ttl)
		}
	}
}

func TestPrepareRecordRejectsNonPositiveTTL(t *testing.T) {
	provider := &AlidnsProvider{rootDomainName: "example.test"}
	for _, ttl := range []int{0, -1} {
		if _, err := provider.prepareRecord(utils.DnsRecord{
			Fqdn: "api.example.test.",
			Type: "A",
			TTL:  ttl,
		}, "192.0.2.10"); err == nil {
			t.Fatalf("unsafe TTL %d was accepted", ttl)
		}
	}
}

func TestPrepareRecordUsesAtForZoneApexAndRejectsOutsideZone(t *testing.T) {
	provider := &AlidnsProvider{rootDomainName: "example.test"}
	prepared, err := provider.prepareRecord(utils.DnsRecord{
		Fqdn: "example.test.",
		Type: "A",
		TTL:  60,
	}, "192.0.2.10")
	if err != nil {
		t.Fatal(err)
	}
	if prepared.RR == nil || *prepared.RR != "@" {
		t.Fatalf("apex RR = %v", prepared.RR)
	}
	if _, err := provider.prepareRecord(utils.DnsRecord{
		Fqdn: "api.other.test.",
		Type: "A",
		TTL:  60,
	}, "192.0.2.10"); err == nil {
		t.Fatal("expected an outside-zone error")
	}
}

func TestGetRecordsFollowsAliDNSPaginationAndGroupsValues(t *testing.T) {
	total := int64(2)
	client := &alidnsTestClient{pages: []*alidns.DescribeDomainRecordsResponse{
		{
			Body: &alidns.DescribeDomainRecordsResponseBody{
				TotalCount: &total,
				DomainRecords: &alidns.DescribeDomainRecordsResponseBodyDomainRecords{
					Record: []*alidns.DescribeDomainRecordsResponseBodyDomainRecordsRecord{{
						RR:    stringPointer("api"),
						Type:  stringPointer("A"),
						TTL:   int64Pointer(60),
						Value: stringPointer("192.0.2.10"),
					}},
				},
			},
		},
		{
			Body: &alidns.DescribeDomainRecordsResponseBody{
				TotalCount: &total,
				DomainRecords: &alidns.DescribeDomainRecordsResponseBodyDomainRecords{
					Record: []*alidns.DescribeDomainRecordsResponseBodyDomainRecordsRecord{{
						RR:    stringPointer("api"),
						Type:  stringPointer("A"),
						TTL:   int64Pointer(60),
						Value: stringPointer("192.0.2.11"),
					}},
				},
			},
		},
	}}
	provider := &AlidnsProvider{client: client, rootDomainName: "example.test"}
	records, err := provider.GetRecords()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || len(records[0].Records) != 2 ||
		records[0].Fqdn != "api.example.test." {
		t.Fatalf("records = %#v", records)
	}
	if len(client.requests) != 2 ||
		client.requests[1].PageNumber == nil || *client.requests[1].PageNumber != 2 {
		t.Fatalf("pagination requests = %#v", client.requests)
	}
}
