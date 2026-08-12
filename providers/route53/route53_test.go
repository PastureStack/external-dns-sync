package route53

import (
	"strings"
	"testing"

	"github.com/PastureStack/external-dns-sync/utils"
	"github.com/aws/aws-sdk-go/aws"
	awsRoute53 "github.com/aws/aws-sdk-go/service/route53"
	"github.com/juju/ratelimit"
)

type route53TestClient struct {
	changeInput *awsRoute53.ChangeResourceRecordSetsInput
}

func (client *route53TestClient) ChangeResourceRecordSets(input *awsRoute53.ChangeResourceRecordSetsInput) (*awsRoute53.ChangeResourceRecordSetsOutput, error) {
	client.changeInput = input
	return &awsRoute53.ChangeResourceRecordSetsOutput{}, nil
}

func (*route53TestClient) GetHostedZone(*awsRoute53.GetHostedZoneInput) (*awsRoute53.GetHostedZoneOutput, error) {
	return &awsRoute53.GetHostedZoneOutput{
		HostedZone: &awsRoute53.HostedZone{Name: aws.String("example.test.")},
	}, nil
}

func (*route53TestClient) GetHostedZoneCount(*awsRoute53.GetHostedZoneCountInput) (*awsRoute53.GetHostedZoneCountOutput, error) {
	return &awsRoute53.GetHostedZoneCountOutput{}, nil
}

func (*route53TestClient) ListHostedZonesByName(*awsRoute53.ListHostedZonesByNameInput) (*awsRoute53.ListHostedZonesByNameOutput, error) {
	return &awsRoute53.ListHostedZonesByNameOutput{
		HostedZones: []*awsRoute53.HostedZone{{
			Id:   aws.String("/hostedzone/ZTEST"),
			Name: aws.String("example.test."),
		}},
	}, nil
}

func (*route53TestClient) ListResourceRecordSetsPages(
	*awsRoute53.ListResourceRecordSetsInput,
	func(*awsRoute53.ListResourceRecordSetsOutput, bool) bool,
) error {
	return nil
}

func TestChangeRecordUsesReviewedZoneAndPastureStackComment(t *testing.T) {
	client := &route53TestClient{}
	provider := &Route53Provider{
		client:       client,
		hostedZoneId: "ZTEST",
		limiter:      ratelimit.NewBucketWithRate(1000, 1),
	}
	record := utils.DnsRecord{
		Fqdn:    "api.example.test.",
		Type:    "A",
		TTL:     60,
		Records: []string{"192.0.2.10"},
	}

	if err := provider.AddRecord(record); err != nil {
		t.Fatal(err)
	}
	if got := aws.StringValue(client.changeInput.HostedZoneId); got != "ZTEST" {
		t.Fatalf("hosted zone = %q", got)
	}
	if got := aws.StringValue(client.changeInput.ChangeBatch.Comment); got != "Managed by PastureStack External DNS Sync" {
		t.Fatalf("comment = %q", got)
	}
	change := client.changeInput.ChangeBatch.Changes[0]
	if got := aws.StringValue(change.Action); got != "UPSERT" {
		t.Fatalf("action = %q", got)
	}
	if got := aws.StringValue(change.ResourceRecordSet.ResourceRecords[0].Value); got != "192.0.2.10" {
		t.Fatalf("record value = %q", got)
	}
}

func TestChangeRecordQuotesTXTValue(t *testing.T) {
	client := &route53TestClient{}
	provider := &Route53Provider{
		client:       client,
		hostedZoneId: "ZTEST",
		limiter:      ratelimit.NewBucketWithRate(1000, 1),
	}

	if err := provider.AddRecord(utils.DnsRecord{
		Fqdn:    "external-dns-env.example.test.",
		Type:    "TXT",
		TTL:     60,
		Records: []string{"api.example.test."},
	}); err != nil {
		t.Fatal(err)
	}
	got := aws.StringValue(client.changeInput.ChangeBatch.Changes[0].ResourceRecordSet.ResourceRecords[0].Value)
	if got != `"api.example.test."` {
		t.Fatalf("TXT value = %q", got)
	}
}

func TestChangeRecordRejectsEmptyValues(t *testing.T) {
	provider := &Route53Provider{
		client:       &route53TestClient{},
		hostedZoneId: "ZTEST",
		limiter:      ratelimit.NewBucketWithRate(1000, 1),
	}
	if err := provider.AddRecord(utils.DnsRecord{Fqdn: "empty.example.test.", Type: "A"}); err == nil {
		t.Fatal("expected an empty-record error")
	}
}

func TestEndpointURLValidation(t *testing.T) {
	for _, accepted := range []string{
		"http://route53.example.test",
		"https://route53.example.test/api",
	} {
		if err := validateEndpointURL(accepted); err != nil {
			t.Fatalf("expected %q to be accepted: %v", accepted, err)
		}
	}
	for _, rejected := range []string{
		"/relative",
		"file:///tmp/socket",
		"https://user:secret@route53.example.test",
		"https://route53.example.test?token=secret",
		"https://route53.example.test#fragment",
	} {
		if err := validateEndpointURL(rejected); err == nil {
			t.Fatalf("expected %q to be rejected", rejected)
		}
	}
}

func TestConfiguredZoneMismatchDoesNotExposeZoneID(t *testing.T) {
	client := &route53TestClient{}
	provider := &Route53Provider{
		client:       client,
		hostedZoneId: "PRIVATE-ZONE-ID",
		limiter:      ratelimit.NewBucketWithRate(1000, 1),
	}
	err := provider.validateHostedZoneId("different.example.test.")
	if err == nil {
		t.Fatal("expected a root-domain mismatch")
	}
	if strings.Contains(err.Error(), "PRIVATE-ZONE-ID") {
		t.Fatalf("error exposed configured zone ID: %v", err)
	}
}
