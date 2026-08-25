package route53

import (
	"context"
	"strings"
	"testing"

	"github.com/PastureStack/external-dns-sync/utils"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsRoute53 "github.com/aws/aws-sdk-go-v2/service/route53"
	awsTypes "github.com/aws/aws-sdk-go-v2/service/route53/types"
	"github.com/juju/ratelimit"
)

type route53TestClient struct {
	changeInput *awsRoute53.ChangeResourceRecordSetsInput
	listPages   []*awsRoute53.ListResourceRecordSetsOutput
	listInputs  []*awsRoute53.ListResourceRecordSetsInput
}

func (client *route53TestClient) ChangeResourceRecordSets(
	_ context.Context,
	input *awsRoute53.ChangeResourceRecordSetsInput,
	_ ...func(*awsRoute53.Options),
) (*awsRoute53.ChangeResourceRecordSetsOutput, error) {
	client.changeInput = input
	return &awsRoute53.ChangeResourceRecordSetsOutput{}, nil
}

func (*route53TestClient) GetHostedZone(
	context.Context,
	*awsRoute53.GetHostedZoneInput,
	...func(*awsRoute53.Options),
) (*awsRoute53.GetHostedZoneOutput, error) {
	return &awsRoute53.GetHostedZoneOutput{
		HostedZone: &awsTypes.HostedZone{Name: aws.String("example.test.")},
	}, nil
}

func (*route53TestClient) GetHostedZoneCount(
	context.Context,
	*awsRoute53.GetHostedZoneCountInput,
	...func(*awsRoute53.Options),
) (*awsRoute53.GetHostedZoneCountOutput, error) {
	return &awsRoute53.GetHostedZoneCountOutput{}, nil
}

func (*route53TestClient) ListHostedZonesByName(
	context.Context,
	*awsRoute53.ListHostedZonesByNameInput,
	...func(*awsRoute53.Options),
) (*awsRoute53.ListHostedZonesByNameOutput, error) {
	return &awsRoute53.ListHostedZonesByNameOutput{
		HostedZones: []awsTypes.HostedZone{{
			Id:   aws.String("/hostedzone/ZTEST"),
			Name: aws.String("example.test."),
		}},
	}, nil
}

func (client *route53TestClient) ListResourceRecordSets(
	_ context.Context,
	input *awsRoute53.ListResourceRecordSetsInput,
	_ ...func(*awsRoute53.Options),
) (*awsRoute53.ListResourceRecordSetsOutput, error) {
	copyInput := *input
	client.listInputs = append(client.listInputs, &copyInput)
	index := len(client.listInputs) - 1
	if index >= len(client.listPages) {
		return &awsRoute53.ListResourceRecordSetsOutput{}, nil
	}
	return client.listPages[index], nil
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
	if got := aws.ToString(client.changeInput.HostedZoneId); got != "ZTEST" {
		t.Fatalf("hosted zone = %q", got)
	}
	if got := aws.ToString(client.changeInput.ChangeBatch.Comment); got != "Managed by PastureStack External DNS Sync" {
		t.Fatalf("comment = %q", got)
	}
	change := client.changeInput.ChangeBatch.Changes[0]
	if got := change.Action; got != awsTypes.ChangeActionUpsert {
		t.Fatalf("action = %q", got)
	}
	if got := aws.ToString(change.ResourceRecordSet.ResourceRecords[0].Value); got != "192.0.2.10" {
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
	got := aws.ToString(client.changeInput.ChangeBatch.Changes[0].ResourceRecordSet.ResourceRecords[0].Value)
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

func TestGetRecordsFollowsV2ContinuationToken(t *testing.T) {
	nextType := awsTypes.RRTypeTxt
	client := &route53TestClient{listPages: []*awsRoute53.ListResourceRecordSetsOutput{
		{
			IsTruncated:    true,
			NextRecordName: aws.String("txt.example.test."),
			NextRecordType: nextType,
			ResourceRecordSets: []awsTypes.ResourceRecordSet{{
				Name: aws.String("api.example.test."),
				Type: awsTypes.RRTypeA,
				TTL:  aws.Int64(60),
				ResourceRecords: []awsTypes.ResourceRecord{{
					Value: aws.String("192.0.2.10"),
				}},
			}},
		},
		{
			ResourceRecordSets: []awsTypes.ResourceRecordSet{{
				Name: aws.String("txt.example.test."),
				Type: awsTypes.RRTypeTxt,
				TTL:  aws.Int64(120),
				ResourceRecords: []awsTypes.ResourceRecord{{
					Value: aws.String(`"safe-value"`),
				}},
			}},
		},
	}}
	provider := &Route53Provider{
		client:       client,
		hostedZoneId: "ZTEST",
		limiter:      ratelimit.NewBucketWithRate(1000, 1),
	}

	records, err := provider.GetRecords()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || records[1].Records[0] != "safe-value" {
		t.Fatalf("records = %#v", records)
	}
	if len(client.listInputs) != 2 ||
		aws.ToString(client.listInputs[1].StartRecordName) != "txt.example.test." ||
		client.listInputs[1].StartRecordType != nextType {
		t.Fatalf("continuation input = %#v", client.listInputs)
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
