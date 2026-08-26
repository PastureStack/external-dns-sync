package route53

// Modified by PastureStack contributors for independent maintenance and rebranding.

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/PastureStack/external-dns-sync/internal/logsafe"
	"github.com/PastureStack/external-dns-sync/providers"
	"github.com/PastureStack/external-dns-sync/utils"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awsRoute53 "github.com/aws/aws-sdk-go-v2/service/route53"
	awsTypes "github.com/aws/aws-sdk-go-v2/service/route53/types"
	"github.com/juju/ratelimit"
	"github.com/sirupsen/logrus"
)

var route53MaxRetries = 3

type Route53Provider struct {
	client       route53Client
	hostedZoneId string
	limiter      *ratelimit.Bucket
}

type route53Client interface {
	ChangeResourceRecordSets(context.Context, *awsRoute53.ChangeResourceRecordSetsInput, ...func(*awsRoute53.Options)) (*awsRoute53.ChangeResourceRecordSetsOutput, error)
	GetHostedZone(context.Context, *awsRoute53.GetHostedZoneInput, ...func(*awsRoute53.Options)) (*awsRoute53.GetHostedZoneOutput, error)
	GetHostedZoneCount(context.Context, *awsRoute53.GetHostedZoneCountInput, ...func(*awsRoute53.Options)) (*awsRoute53.GetHostedZoneCountOutput, error)
	ListHostedZonesByName(context.Context, *awsRoute53.ListHostedZonesByNameInput, ...func(*awsRoute53.Options)) (*awsRoute53.ListHostedZonesByNameOutput, error)
	ListResourceRecordSets(context.Context, *awsRoute53.ListResourceRecordSetsInput, ...func(*awsRoute53.Options)) (*awsRoute53.ListResourceRecordSetsOutput, error)
}

func init() {
	providers.RegisterProvider("route53", &Route53Provider{})
}

// Init creates a Route 53 client using the AWS SDK v2 default credential chain.
// The historical AWS_ACCESS_KEY/AWS_SECRET_KEY pair remains supported for
// compatibility, but the standard AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY names
// and workload credentials are preferred.
func (r *Route53Provider) Init(rootDomainName string) error {
	r.limiter = ratelimit.NewBucketWithRate(5.0, 1)

	if envVal := os.Getenv("ROUTE53_MAX_RETRIES"); envVal != "" {
		i, err := strconv.Atoi(envVal)
		if err == nil && i >= 0 && i <= 10 {
			route53MaxRetries = i
		} else {
			logrus.Warn("Invalid value for ROUTE53_MAX_RETRIES; using 3")
			route53MaxRetries = 3
		}
	}

	region := strings.TrimSpace(os.Getenv("AWS_REGION"))
	if region == "" {
		region = strings.TrimSpace(os.Getenv("AWS_DEFAULT_REGION"))
	}
	if region == "" {
		region = "us-east-1"
	}

	loadOptions := []func(*awsconfig.LoadOptions) error{awsconfig.WithRegion(region)}
	legacyAccessKey := strings.TrimSpace(os.Getenv("AWS_ACCESS_KEY"))
	legacySecretKey := strings.TrimSpace(os.Getenv("AWS_SECRET_KEY"))
	if legacyAccessKey != "" || legacySecretKey != "" {
		if legacyAccessKey == "" || legacySecretKey == "" {
			return fmt.Errorf("AWS_ACCESS_KEY and AWS_SECRET_KEY must be set together")
		}
		loadOptions = append(loadOptions, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(
				legacyAccessKey,
				legacySecretKey,
				strings.TrimSpace(os.Getenv("AWS_SESSION_TOKEN")),
			),
		))
	}

	cfg, err := awsconfig.LoadDefaultConfig(context.Background(), loadOptions...)
	if err != nil {
		return fmt.Errorf("failed to load Route 53 configuration: %w", err)
	}

	endpoint := strings.TrimSpace(os.Getenv("ROUTE53_ENDPOINT_URL"))
	if endpoint != "" {
		if err := validateEndpointURL(endpoint); err != nil {
			return err
		}
	}
	r.client = awsRoute53.NewFromConfig(cfg, func(options *awsRoute53.Options) {
		// RetryMaxAttempts includes the initial request, whereas the existing
		// ROUTE53_MAX_RETRIES setting counts retries only.
		options.RetryMaxAttempts = route53MaxRetries + 1
		if endpoint != "" {
			options.BaseEndpoint = aws.String(endpoint)
		}
	})

	if err := r.setHostedZone(rootDomainName); err != nil {
		return fmt.Errorf("failed to configure hosted zone: %w", err)
	}

	logrus.Infof("Configured %s with hosted zone %s", logsafe.Value(r.GetName()), logsafe.Value(rootDomainName))
	return nil
}

func (r *Route53Provider) setHostedZone(rootDomainName string) error {
	if envVal := os.Getenv("ROUTE53_ZONE_ID"); envVal != "" {
		r.hostedZoneId = strings.TrimSpace(envVal)
		return r.validateHostedZoneId(rootDomainName)
	}

	r.limiter.Wait(1)
	maxItems := int32(1)
	resp, err := r.client.ListHostedZonesByName(context.Background(), &awsRoute53.ListHostedZonesByNameInput{
		DNSName:  aws.String(utils.UnFqdn(rootDomainName)),
		MaxItems: &maxItems,
	})
	if err != nil {
		return fmt.Errorf("could not list hosted zones: %w", err)
	}
	if len(resp.HostedZones) == 0 || aws.ToString(resp.HostedZones[0].Name) != rootDomainName {
		return fmt.Errorf("hosted zone for %q not found", rootDomainName)
	}

	zoneID := aws.ToString(resp.HostedZones[0].Id)
	if zoneID == "" {
		return fmt.Errorf("hosted zone response did not include an ID")
	}
	r.hostedZoneId = strings.TrimPrefix(zoneID, "/hostedzone/")
	return nil
}

func (r *Route53Provider) validateHostedZoneId(rootDomainName string) error {
	r.limiter.Wait(1)
	resp, err := r.client.GetHostedZone(context.Background(), &awsRoute53.GetHostedZoneInput{
		Id: aws.String(r.hostedZoneId),
	})
	if err != nil {
		return fmt.Errorf("could not look up configured hosted zone: %w", err)
	}
	if resp.HostedZone == nil || resp.HostedZone.Name == nil {
		return fmt.Errorf("hosted zone response did not include a name")
	}
	if aws.ToString(resp.HostedZone.Name) != rootDomainName {
		return fmt.Errorf("configured hosted zone does not match root domain %q", rootDomainName)
	}
	return nil
}

func (*Route53Provider) GetName() string {
	return "Route 53"
}

func (r *Route53Provider) HealthCheck() error {
	_, err := r.client.GetHostedZoneCount(context.Background(), &awsRoute53.GetHostedZoneCountInput{})
	return err
}

func (r *Route53Provider) AddRecord(record utils.DnsRecord) error {
	return r.changeRecord(record, awsTypes.ChangeActionUpsert)
}

func (r *Route53Provider) UpdateRecord(record utils.DnsRecord) error {
	return r.changeRecord(record, awsTypes.ChangeActionUpsert)
}

func (r *Route53Provider) RemoveRecord(record utils.DnsRecord) error {
	return r.changeRecord(record, awsTypes.ChangeActionDelete)
}

func (r *Route53Provider) changeRecord(record utils.DnsRecord, action awsTypes.ChangeAction) error {
	if len(record.Records) == 0 {
		return fmt.Errorf("DNS record %q has no values", record.Fqdn)
	}
	r.limiter.Wait(1)
	records := make([]awsTypes.ResourceRecord, len(record.Records))
	for idx, value := range record.Records {
		if record.Type == "TXT" {
			value = `"` + value + `"`
		}
		records[idx] = awsTypes.ResourceRecord{Value: aws.String(value)}
	}

	_, err := r.client.ChangeResourceRecordSets(context.Background(), &awsRoute53.ChangeResourceRecordSetsInput{
		HostedZoneId: aws.String(r.hostedZoneId),
		ChangeBatch: &awsTypes.ChangeBatch{
			Comment: aws.String("Managed by PastureStack External DNS Sync"),
			Changes: []awsTypes.Change{{
				Action: action,
				ResourceRecordSet: &awsTypes.ResourceRecordSet{
					Name:            aws.String(record.Fqdn),
					Type:            awsTypes.RRType(record.Type),
					TTL:             aws.Int64(int64(record.TTL)),
					ResourceRecords: records,
				},
			}},
		},
	})
	return err
}

func (r *Route53Provider) GetRecords() ([]utils.DnsRecord, error) {
	r.limiter.Wait(1)
	dnsRecords := []utils.DnsRecord{}
	maxItems := int32(100)
	params := &awsRoute53.ListResourceRecordSetsInput{
		HostedZoneId: aws.String(r.hostedZoneId),
		MaxItems:     &maxItems,
	}

	var rrSets []awsTypes.ResourceRecordSet
	for {
		page, err := r.client.ListResourceRecordSets(context.Background(), params)
		if err != nil {
			return dnsRecords, fmt.Errorf("Route 53 API call has failed: %w", err)
		}
		rrSets = append(rrSets, page.ResourceRecordSets...)
		if !page.IsTruncated {
			break
		}
		if page.NextRecordName == nil || page.NextRecordType == "" {
			return dnsRecords, fmt.Errorf("Route 53 returned a truncated page without a continuation token")
		}
		r.limiter.Wait(1)
		params.StartRecordName = page.NextRecordName
		params.StartRecordType = page.NextRecordType
		params.StartRecordIdentifier = page.NextRecordIdentifier
	}

	for idx := range rrSets {
		rrSet := &rrSets[idx]
		if rrSet.Name == nil || rrSet.Type == "" {
			logrus.Warn("Skipping incomplete Route 53 record-set response")
			continue
		}
		if IsProprietary(rrSet) {
			logrus.WithFields(logrus.Fields{
				"name": logsafe.Value(aws.ToString(rrSet.Name)),
				"type": logsafe.Value(rrSet.Type),
			}).Debug("Skipped proprietary Route 53 record")
			continue
		}

		records := []string{}
		for _, rr := range rrSet.ResourceRecords {
			if rr.Value == nil {
				continue
			}
			value := aws.ToString(rr.Value)
			if rrSet.Type == awsTypes.RRTypeTxt {
				value = strings.Trim(value, `"`)
			}
			records = append(records, value)
		}
		if rrSet.TTL == nil {
			logrus.Warnf("Skipping Route 53 record %s without a TTL", logsafe.Value(aws.ToString(rrSet.Name)))
			continue
		}
		dnsRecords = append(dnsRecords, utils.DnsRecord{
			Fqdn:    aws.ToString(rrSet.Name),
			Records: records,
			Type:    string(rrSet.Type),
			TTL:     int(aws.ToInt64(rrSet.TTL)),
		})
	}

	return dnsRecords, nil
}

func IsProprietary(rr *awsTypes.ResourceRecordSet) bool {
	return rr != nil && (rr.AliasTarget != nil || rr.TrafficPolicyInstanceId != nil)
}

func validateEndpointURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("parse Route 53 endpoint URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("Route 53 endpoint URL must use http or https")
	}
	if parsed.Host == "" {
		return fmt.Errorf("Route 53 endpoint URL must include a host")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("Route 53 endpoint URL must not contain user information, a query, or a fragment")
	}
	return nil
}
