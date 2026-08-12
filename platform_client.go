package main

// Modified by PastureStack contributors for independent maintenance and rebranding.

import (
	"context"

	"github.com/PastureStack/external-dns-sync/internal/platformapi"
	"github.com/PastureStack/external-dns-sync/utils"
)

const externalDNSUpdateEventType = "dns.update"

// PlatformClient wraps the environment API required by this service.
type PlatformClient struct {
	client *platformapi.Client
}

func NewPlatformClient(platformURL string, accessKey string, secretKey string) (*PlatformClient, error) {
	client, err := platformapi.NewClient(platformURL, accessKey, secretKey)
	if err != nil {
		return nil, err
	}

	return &PlatformClient{client: client}, nil
}

func (client *PlatformClient) UpdateServiceDomainName(metadataRecord utils.MetadataDnsRecord) error {
	event := newExternalDNSUpdateEvent(metadataRecord)
	return client.client.CreateExternalDNSEvent(context.Background(), event)
}

func newExternalDNSUpdateEvent(metadataRecord utils.MetadataDnsRecord) *platformapi.ExternalDNSEvent {
	return &platformapi.ExternalDNSEvent{
		EventType:   externalDNSUpdateEventType,
		ExternalID:  metadataRecord.DnsRecord.Fqdn,
		ServiceName: metadataRecord.ServiceName,
		StackName:   metadataRecord.StackName,
		FQDN:        utils.UnFqdn(metadataRecord.DnsRecord.Fqdn),
	}
}

func (client *PlatformClient) TestConnect() error {
	return client.client.TestConnection(context.Background())
}
