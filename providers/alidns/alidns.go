package alidns

// Modified by PastureStack contributors for independent maintenance and rebranding.

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/PastureStack/external-dns-sync/internal/logsafe"
	"github.com/PastureStack/external-dns-sync/providers"
	"github.com/PastureStack/external-dns-sync/utils"
	alidns "github.com/alibabacloud-go/alidns-20150109/v5/client"
	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/utils"
	"github.com/sirupsen/logrus"
)

const alidnsEndpoint = "alidns.aliyuncs.com"

type alidnsClient interface {
	DescribeDomainInfo(*alidns.DescribeDomainInfoRequest) (*alidns.DescribeDomainInfoResponse, error)
	DescribeDomainRecords(*alidns.DescribeDomainRecordsRequest) (*alidns.DescribeDomainRecordsResponse, error)
	AddDomainRecord(*alidns.AddDomainRecordRequest) (*alidns.AddDomainRecordResponse, error)
	DeleteDomainRecord(*alidns.DeleteDomainRecordRequest) (*alidns.DeleteDomainRecordResponse, error)
}

type AlidnsProvider struct {
	client         alidnsClient
	rootDomainName string
}

func init() {
	providers.RegisterProvider("alidns", &AlidnsProvider{})
}

func (a *AlidnsProvider) Init(rootDomainName string) error {
	accessKey := strings.TrimSpace(os.Getenv("ALICLOUD_ACCESS_KEY_ID"))
	if accessKey == "" {
		return fmt.Errorf("ALICLOUD_ACCESS_KEY_ID is not set")
	}

	secretKey := strings.TrimSpace(os.Getenv("ALICLOUD_ACCESS_KEY_SECRET"))
	if secretKey == "" {
		return fmt.Errorf("ALICLOUD_ACCESS_KEY_SECRET is not set")
	}

	client, err := alidns.NewClient(&openapi.Config{
		AccessKeyId:     stringPointer(accessKey),
		AccessKeySecret: stringPointer(secretKey),
		Endpoint:        stringPointer(alidnsEndpoint),
	})
	if err != nil {
		return fmt.Errorf("failed to create Alibaba Cloud DNS client: %w", err)
	}
	a.client = client
	a.rootDomainName = utils.UnFqdn(rootDomainName)

	if _, err := a.client.DescribeDomainInfo(&alidns.DescribeDomainInfoRequest{
		DomainName: stringPointer(a.rootDomainName),
	}); err != nil {
		return fmt.Errorf("failed to describe root domain %q: %w", a.rootDomainName, err)
	}

	logrus.Infof("Configured %s with zone '%s'", logsafe.Value(a.GetName()), logsafe.Value(a.rootDomainName))
	return nil
}

func (*AlidnsProvider) GetName() string {
	return "AliDNS"
}

func (a *AlidnsProvider) HealthCheck() error {
	_, err := a.client.DescribeDomainInfo(&alidns.DescribeDomainInfoRequest{
		DomainName: stringPointer(a.rootDomainName),
	})
	return err
}

func (a *AlidnsProvider) AddRecord(record utils.DnsRecord) error {
	for _, value := range record.Records {
		request, err := a.prepareRecord(record, value)
		if err != nil {
			return err
		}
		if _, err := a.client.AddDomainRecord(request); err != nil {
			return fmt.Errorf("Alibaba Cloud API call has failed: %w", err)
		}
	}
	return nil
}

func (a *AlidnsProvider) UpdateRecord(record utils.DnsRecord) error {
	if err := a.RemoveRecord(record); err != nil {
		return err
	}
	return a.AddRecord(record)
}

func (a *AlidnsProvider) RemoveRecord(record utils.DnsRecord) error {
	records, err := a.findRecords(record)
	if err != nil {
		return err
	}

	for _, current := range records {
		if current == nil || current.RecordId == nil || strings.TrimSpace(*current.RecordId) == "" {
			return fmt.Errorf("Alibaba Cloud returned a matching DNS record without an ID")
		}
		if _, err := a.client.DeleteDomainRecord(&alidns.DeleteDomainRecordRequest{
			RecordId: current.RecordId,
		}); err != nil {
			return fmt.Errorf("Alibaba Cloud API call has failed: %w", err)
		}
	}
	return nil
}

func (a *AlidnsProvider) GetRecords() ([]utils.DnsRecord, error) {
	apiRecords, err := a.listRecords()
	if err != nil {
		return nil, err
	}

	type recordKey struct {
		fqdn       string
		recordType string
	}
	grouped := make(map[recordKey]*utils.DnsRecord)
	for _, current := range apiRecords {
		if current == nil || current.RR == nil || current.Type == nil ||
			current.Value == nil || current.TTL == nil {
			logrus.Warn("Skipping incomplete AliDNS record response")
			continue
		}
		rr := strings.TrimSpace(*current.RR)
		var fqdn string
		if rr == "" || rr == "@" {
			fqdn = a.rootDomainName + "."
		} else {
			fqdn = fmt.Sprintf("%s.%s.", rr, a.rootDomainName)
		}
		key := recordKey{fqdn: fqdn, recordType: *current.Type}
		record, exists := grouped[key]
		if !exists {
			record = &utils.DnsRecord{Fqdn: fqdn, Type: *current.Type, TTL: int(*current.TTL)}
			grouped[key] = record
		}
		record.Records = append(record.Records, *current.Value)
	}

	keys := make([]recordKey, 0, len(grouped))
	for key := range grouped {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].fqdn == keys[j].fqdn {
			return keys[i].recordType < keys[j].recordType
		}
		return keys[i].fqdn < keys[j].fqdn
	})
	records := make([]utils.DnsRecord, 0, len(keys))
	for _, key := range keys {
		records = append(records, *grouped[key])
	}
	return records, nil
}

func (a *AlidnsProvider) parseName(record utils.DnsRecord) (string, error) {
	fqdn := utils.UnFqdn(record.Fqdn)
	if fqdn == a.rootDomainName {
		return "@", nil
	}
	suffix := "." + a.rootDomainName
	if !strings.HasSuffix(fqdn, suffix) {
		return "", fmt.Errorf("AliDNS record %q is outside managed zone %q", record.Fqdn, a.rootDomainName)
	}
	rr := strings.TrimSuffix(fqdn, suffix)
	if rr == "" {
		return "@", nil
	}
	return rr, nil
}

func (a *AlidnsProvider) prepareRecord(record utils.DnsRecord, value string) (*alidns.AddDomainRecordRequest, error) {
	ttl, err := checkedTTL(record.TTL)
	if err != nil {
		return nil, err
	}
	rr, err := a.parseName(record)
	if err != nil {
		return nil, err
	}
	return &alidns.AddDomainRecordRequest{
		DomainName: stringPointer(a.rootDomainName),
		RR:         stringPointer(rr),
		Type:       stringPointer(record.Type),
		Value:      stringPointer(value),
		TTL:        &ttl,
	}, nil
}

func checkedTTL(ttl int) (int64, error) {
	if ttl <= 0 {
		return 0, fmt.Errorf("AliDNS TTL must be greater than zero")
	}
	return int64(ttl), nil
}

func (a *AlidnsProvider) findRecords(record utils.DnsRecord) ([]*alidns.DescribeDomainRecordsResponseBodyDomainRecordsRecord, error) {
	records, err := a.listRecords()
	if err != nil {
		return nil, err
	}
	name, err := a.parseName(record)
	if err != nil {
		return nil, err
	}
	matching := make([]*alidns.DescribeDomainRecordsResponseBodyDomainRecordsRecord, 0)
	for _, current := range records {
		if current != nil && current.RR != nil && current.Type != nil &&
			*current.RR == name && *current.Type == record.Type {
			matching = append(matching, current)
		}
	}
	return matching, nil
}

func (a *AlidnsProvider) listRecords() ([]*alidns.DescribeDomainRecordsResponseBodyDomainRecordsRecord, error) {
	const pageSize int64 = 500
	all := make([]*alidns.DescribeDomainRecordsResponseBodyDomainRecordsRecord, 0)
	for pageNumber := int64(1); ; pageNumber++ {
		response, err := a.client.DescribeDomainRecords(&alidns.DescribeDomainRecordsRequest{
			DomainName: stringPointer(a.rootDomainName),
			PageNumber: &pageNumber,
			PageSize:   int64Pointer(pageSize),
		})
		if err != nil {
			return nil, fmt.Errorf("Alibaba Cloud API call has failed: %w", err)
		}
		if response == nil || response.Body == nil || response.Body.DomainRecords == nil {
			return nil, fmt.Errorf("Alibaba Cloud returned an incomplete DNS record response")
		}
		pageRecords := response.Body.DomainRecords.Record
		all = append(all, pageRecords...)
		if response.Body.TotalCount != nil {
			if int64(len(all)) >= *response.Body.TotalCount {
				break
			}
			if len(pageRecords) == 0 {
				return nil, fmt.Errorf("Alibaba Cloud pagination ended before TotalCount was reached")
			}
			continue
		}
		if len(pageRecords) < int(pageSize) {
			break
		}
	}
	return all, nil
}

func stringPointer(value string) *string {
	return &value
}

func int64Pointer(value int64) *int64 {
	return &value
}
