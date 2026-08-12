package route53

// Modified by PastureStack contributors for independent maintenance and rebranding.

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/PastureStack/external-dns-sync/providers"
	"github.com/PastureStack/external-dns-sync/utils"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/credentials/ec2rolecreds"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/session"
	awsRoute53 "github.com/aws/aws-sdk-go/service/route53"
	"github.com/juju/ratelimit"
	"github.com/sirupsen/logrus"
)

var (
	route53MaxRetries int = 3
)

type Route53Provider struct {
	client       route53Client
	hostedZoneId string
	limiter      *ratelimit.Bucket
}

type route53Client interface {
	ChangeResourceRecordSets(*awsRoute53.ChangeResourceRecordSetsInput) (*awsRoute53.ChangeResourceRecordSetsOutput, error)
	GetHostedZone(*awsRoute53.GetHostedZoneInput) (*awsRoute53.GetHostedZoneOutput, error)
	GetHostedZoneCount(*awsRoute53.GetHostedZoneCountInput) (*awsRoute53.GetHostedZoneCountOutput, error)
	ListHostedZonesByName(*awsRoute53.ListHostedZonesByNameInput) (*awsRoute53.ListHostedZonesByNameOutput, error)
	ListResourceRecordSetsPages(*awsRoute53.ListResourceRecordSetsInput, func(*awsRoute53.ListResourceRecordSetsOutput, bool) bool) error
}

func init() {
	providers.RegisterProvider("route53", &Route53Provider{})
}

// Init creates a Route53 client with credentials from one of these
// two locations in that priority order:
// 1) Environment variables: AWS_ACCESS_KEY, AWS_SECRET_KEY
// 2) EC2 IAM role
func (r *Route53Provider) Init(rootDomainName string) error {
	// Comply with the API's 5 req/s rate limit. If there are other
	// clients using the same account the AWS SDK will throttle the
	// requests automatically if the global rate limit is exhausted.
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

	creds := credentials.NewChainCredentials(
		[]credentials.Provider{
			&credentials.EnvProvider{},
			&ec2rolecreds.EC2RoleProvider{
				Client: ec2metadata.New(session.Must(session.NewSession())),
			},
		})

	config := aws.NewConfig().WithMaxRetries(route53MaxRetries).
		WithCredentials(creds)
	if endpoint := strings.TrimSpace(os.Getenv("ROUTE53_ENDPOINT_URL")); endpoint != "" {
		if err := validateEndpointURL(endpoint); err != nil {
			return err
		}
		region := strings.TrimSpace(os.Getenv("AWS_REGION"))
		if region == "" {
			region = "us-east-1"
		}
		config = config.WithEndpoint(endpoint).WithRegion(region)
	}

	sess, err := session.NewSession(config)
	if err != nil {
		return fmt.Errorf("Failed to create Route53 session: %v", err)
	}

	r.client = awsRoute53.New(sess)
	if err := r.setHostedZone(rootDomainName); err != nil {
		return fmt.Errorf("Failed to configure hosted zone: %v", err)
	}

	logrus.Infof("Configured %s with hosted zone %s",
		r.GetName(), rootDomainName)

	return nil
}

func (r *Route53Provider) setHostedZone(rootDomainName string) error {
	if envVal := os.Getenv("ROUTE53_ZONE_ID"); envVal != "" {
		r.hostedZoneId = strings.TrimSpace(envVal)
		if err := r.validateHostedZoneId(rootDomainName); err != nil {
			return err
		}
		return nil
	}

	r.limiter.Wait(1)
	params := &awsRoute53.ListHostedZonesByNameInput{
		DNSName:  aws.String(utils.UnFqdn(rootDomainName)),
		MaxItems: aws.String("1"),
	}
	resp, err := r.client.ListHostedZonesByName(params)
	if err != nil {
		return fmt.Errorf("Could not list hosted zones: %v", err)
	}

	if len(resp.HostedZones) == 0 || *resp.HostedZones[0].Name != rootDomainName {
		return fmt.Errorf("Hosted zone for '%s' not found", rootDomainName)
	}

	if resp.HostedZones[0].Id == nil {
		return fmt.Errorf("hosted zone response did not include an ID")
	}
	zoneId := *resp.HostedZones[0].Id
	if strings.HasPrefix(zoneId, "/hostedzone/") {
		zoneId = strings.TrimPrefix(zoneId, "/hostedzone/")
	}

	r.hostedZoneId = zoneId
	return nil
}

func (r *Route53Provider) validateHostedZoneId(rootDomainName string) error {
	r.limiter.Wait(1)
	params := &awsRoute53.GetHostedZoneInput{
		Id: aws.String(r.hostedZoneId),
	}
	resp, err := r.client.GetHostedZone(params)
	if err != nil {
		return fmt.Errorf("could not look up configured hosted zone: %v", err)
	}

	if resp.HostedZone == nil || resp.HostedZone.Name == nil {
		return fmt.Errorf("hosted zone response did not include a name")
	}
	if *resp.HostedZone.Name != rootDomainName {
		return fmt.Errorf("configured hosted zone does not match root domain %q", rootDomainName)
	}

	return nil
}

func (*Route53Provider) GetName() string {
	return "Route 53"
}

func (r *Route53Provider) HealthCheck() error {
	var params *awsRoute53.GetHostedZoneCountInput
	_, err := r.client.GetHostedZoneCount(params)
	return err
}

func (r *Route53Provider) AddRecord(record utils.DnsRecord) error {
	return r.changeRecord(record, "UPSERT")
}

func (r *Route53Provider) UpdateRecord(record utils.DnsRecord) error {
	return r.changeRecord(record, "UPSERT")
}

func (r *Route53Provider) RemoveRecord(record utils.DnsRecord) error {
	return r.changeRecord(record, "DELETE")
}

func (r *Route53Provider) changeRecord(record utils.DnsRecord, action string) error {
	if len(record.Records) == 0 {
		return fmt.Errorf("DNS record %q has no values", record.Fqdn)
	}
	r.limiter.Wait(1)
	records := make([]*awsRoute53.ResourceRecord, len(record.Records))
	for idx, value := range record.Records {
		if record.Type == "TXT" {
			value = `"` + value + `"`
		}
		records[idx] = &awsRoute53.ResourceRecord{
			Value: aws.String(value),
		}
	}

	params := &awsRoute53.ChangeResourceRecordSetsInput{
		HostedZoneId: aws.String(r.hostedZoneId),
		ChangeBatch: &awsRoute53.ChangeBatch{
			Comment: aws.String("Managed by PastureStack External DNS Sync"),
			Changes: []*awsRoute53.Change{
				{
					Action: aws.String(action),
					ResourceRecordSet: &awsRoute53.ResourceRecordSet{
						Name:            aws.String(record.Fqdn),
						Type:            aws.String(record.Type),
						TTL:             aws.Int64(int64(record.TTL)),
						ResourceRecords: records,
					},
				},
			},
		},
	}

	_, err := r.client.ChangeResourceRecordSets(params)
	return err
}

func (r *Route53Provider) GetRecords() ([]utils.DnsRecord, error) {
	r.limiter.Wait(1)
	dnsRecords := []utils.DnsRecord{}
	rrSets := []*awsRoute53.ResourceRecordSet{}
	params := &awsRoute53.ListResourceRecordSetsInput{
		HostedZoneId: aws.String(r.hostedZoneId),
		MaxItems:     aws.String("100"),
	}

	err := r.client.ListResourceRecordSetsPages(params,
		func(page *awsRoute53.ListResourceRecordSetsOutput, lastPage bool) bool {
			rrSets = append(rrSets, page.ResourceRecordSets...)
			if !lastPage {
				r.limiter.Wait(1)
			}
			return !lastPage
		})
	if err != nil {
		return dnsRecords, fmt.Errorf("Route 53 API call has failed: %v", err)
	}

	for _, rrSet := range rrSets {
		if rrSet == nil || rrSet.Name == nil || rrSet.Type == nil {
			logrus.Warn("Skipping incomplete Route 53 record-set response")
			continue
		}
		// skip proprietary Route 53 resource record sets
		if IsProprietary(rrSet) {
			logrus.Debugf("skipped properietary rrSet: %s", rrSet)
			continue
		}

		records := []string{}
		for _, rr := range rrSet.ResourceRecords {
			if rr == nil || rr.Value == nil {
				continue
			}
			value := *rr.Value
			if *rrSet.Type == "TXT" {
				value = strings.Trim(value, `"`)
			}
			records = append(records, value)
		}

		logrus.Debugf("rrSet: %s", rrSet)
		logrus.Debugf("records: %s", records)

		if rrSet.TTL == nil {
			logrus.Warnf("Skipping Route 53 record %s without a TTL", *rrSet.Name)
			continue
		}
		dnsRecord := utils.DnsRecord{
			Fqdn:    *rrSet.Name,
			Records: records,
			Type:    *rrSet.Type,
			TTL:     int(*rrSet.TTL),
		}
		dnsRecords = append(dnsRecords, dnsRecord)
	}

	return dnsRecords, nil
}

func IsProprietary(rr *awsRoute53.ResourceRecordSet) bool {
	return (rr.AliasTarget != nil || rr.TrafficPolicyInstanceId != nil)
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
