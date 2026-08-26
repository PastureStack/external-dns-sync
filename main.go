package main

// Modified by PastureStack contributors for independent maintenance and rebranding.

import (
	"flag"
	"os"
	"reflect"
	"time"

	"github.com/PastureStack/external-dns-sync/config"
	"github.com/PastureStack/external-dns-sync/internal/logsafe"
	"github.com/PastureStack/external-dns-sync/metadata"
	"github.com/PastureStack/external-dns-sync/providers"
	_ "github.com/PastureStack/external-dns-sync/providers/route53"
	"github.com/PastureStack/external-dns-sync/utils"
	"github.com/sirupsen/logrus"
)

const (
	pollIntervalSeconds = 1
	// if metadata wasn't updated in 1 min, force update would be executed
	forceUpdateIntervalMinutes = 1
)

type Op struct {
	Name string
}

var (
	Add    = Op{Name: "Add"}
	Remove = Op{Name: "Remove"}
	Update = Op{Name: "Update"}
)

// set at build time
var Version string

var (
	providerName = flag.String("provider", "route53", "External provider name (published runtime: route53)")
	debug        = flag.Bool("debug", false, "Debug")
	logFile      = flag.String("log", "", "Log file")

	provider    providers.Provider
	m           *metadata.MetadataClient
	platformAPI *PlatformClient

	metadataRecsCached = make(map[string]utils.MetadataDnsRecord)
)

func setEnv() {
	flag.Parse()
	if *debug {
		logrus.SetLevel(logrus.DebugLevel)
	}
	if *logFile != "" {
		if output, err := os.OpenFile(*logFile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666); err != nil {
			logrus.Fatalf("Failed to log to file %s: %s", logsafe.Value(*logFile), logsafe.Value(err))
		} else {
			logrus.SetOutput(output)
			formatter := &logrus.TextFormatter{
				FullTimestamp: true,
			}
			logrus.SetFormatter(formatter)
		}
	}

	// get config from environment variables
	config.SetFromEnvironment()

	var err error
	// configure metadata client
	m, err = metadata.NewMetadataClient()
	if err != nil {
		logrus.Fatalf("Failed to configure metadata client: %s", logsafe.Value(err))
	}

	// Configure the legacy-compatible platform API client.
	platformAPI, err = NewPlatformClient(config.PlatformURL, config.PlatformAccessKey, config.PlatformSecretKey)
	if err != nil {
		logrus.Fatalf("Failed to configure platform API client: %s", logsafe.Value(err))
	}

	// get provider
	provider, err = providers.GetProvider(*providerName, config.RootDomainName)
	if err != nil {
		logrus.Fatalf("Failed to get provider '%s': %s", logsafe.Value(*providerName), logsafe.Value(err))
	}
}

func main() {
	logrus.Infof("Starting PastureStack External DNS Sync %s", logsafe.Value(Version))
	setEnv()

	go startHealthCheck()
	if err := EnsureUpgradeToStateRRSet(); err != nil {
		logrus.Fatalf("Failed to ensure upgrade: %s", logsafe.Value(err))
	}

	currentVersion := "init"
	lastUpdated := time.Now()
	hasSynchronized := false

	for {
		update, updateForced := false, false
		newVersion, err := m.GetVersion()
		if err != nil {
			logrus.Errorf("Failed to get metadata version: %s", logsafe.Value(err))
			goto sleep
		} else if currentVersion != newVersion {
			logrus.Debugf("Metadata version changed. Old: %s New: %s.", logsafe.Value(currentVersion), logsafe.Value(newVersion))
			currentVersion = newVersion
			update = true
		} else {
			if time.Since(lastUpdated).Minutes() >= forceUpdateIntervalMinutes {
				logrus.Debugf("Executing force update as metadata version hasn't changed in: %d minutes",
					forceUpdateIntervalMinutes)
				updateForced = true
			}
		}

		if update || updateForced {
			// get records from metadata
			metadataRecs, err := m.GetMetadataDnsRecords()
			if err != nil {
				logrus.Errorf("Failed to get DNS records from metadata: %s", logsafe.Value(err))
				goto sleep
			}

			logrus.WithField("recordCount", len(metadataRecs)).Debug("DNS records received from metadata")

			// A flapping service might cause the metadata version to change
			// in short intervals. Caching the previous metadata DNS records
			// allows us to check if the actual records have changed before
			// querying the provider records.
			if shouldSynchronize(hasSynchronized, updateForced, metadataRecs, metadataRecsCached) {
				// update the provider
				updatedRecords, err := UpdateProviderDnsRecords(metadataRecs)
				if err != nil {
					logrus.Errorf("Failed to update provider with new DNS records: %s", logsafe.Value(err))
					goto sleep
				}

				// Update the service FQDN through the platform compatibility API.
				for _, mRec := range updatedRecords {
					if mRec.ServiceName != "" && mRec.StackName != "" {
						logrus.Debugf("Updating platform service FQDN for %s/%s", logsafe.Value(mRec.ServiceName), logsafe.Value(mRec.StackName))
						if err := platformAPI.UpdateServiceDomainName(mRec); err != nil {
							logrus.Errorf("Failed to update platform service FQDN: %s", logsafe.Value(err))
						}
					}
				}

				metadataRecsCached = metadataRecs
				lastUpdated = time.Now()
				hasSynchronized = true
			} else {
				logrus.Debugf("DNS records from metadata did not change")
			}
		}
	sleep:
		time.Sleep(pollIntervalSeconds * time.Second)
	}
}

func shouldSynchronize(
	hasSynchronized bool,
	updateForced bool,
	metadataRecords map[string]utils.MetadataDnsRecord,
	cachedRecords map[string]utils.MetadataDnsRecord,
) bool {
	return !hasSynchronized || updateForced || !reflect.DeepEqual(metadataRecords, cachedRecords)
}
