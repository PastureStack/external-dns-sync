package main

// Modified by PastureStack contributors for independent maintenance and rebranding.

import (
	"fmt"
	"sort"
	"strings"

	"github.com/PastureStack/external-dns-sync/config"
	"github.com/PastureStack/external-dns-sync/utils"
	"github.com/sirupsen/logrus"
)

func UpdateProviderDnsRecords(metadataRecs map[string]utils.MetadataDnsRecord) ([]utils.MetadataDnsRecord, error) {
	var updated []utils.MetadataDnsRecord
	ourRecords, allRecords, err := getProviderDnsRecords()
	if err != nil {
		return nil, fmt.Errorf("Provider error reading dns entries: %v", err)
	}
	logrus.Debugf("DNS records from provider: %v", ourRecords)

	if _, err := removeExtraRecords(metadataRecs, ourRecords); err != nil {
		return nil, err
	}

	added, err := addMissingRecords(metadataRecs, allRecords)
	if err != nil {
		return nil, err
	}
	updated = append(updated, added...)

	changed, err := updateExistingRecords(metadataRecs, allRecords)
	if err != nil {
		return nil, err
	}
	updated = append(updated, changed...)

	return updated, nil
}

func addMissingRecords(metadataRecs map[string]utils.MetadataDnsRecord, providerRecs map[string]utils.DnsRecord) ([]utils.MetadataDnsRecord, error) {
	var toAdd []utils.MetadataDnsRecord
	for _, key := range sortedKeys(metadataRecs) {
		if _, ok := providerRecs[key]; !ok {
			toAdd = append(toAdd, metadataRecs[key])
		}
	}
	if len(toAdd) == 0 {
		logrus.Debug("No DNS records to add")
	} else {
		logrus.Debugf("DNS records to add: %v", toAdd)
	}

	return updateRecords(toAdd, &Add)
}

func updateRecords(toChange []utils.MetadataDnsRecord, op *Op) ([]utils.MetadataDnsRecord, error) {
	var changed []utils.MetadataDnsRecord
	for _, value := range toChange {
		var err error
		switch *op {
		case Add:
			logrus.Infof("Adding dns record: %v", value)
			err = provider.AddRecord(value.DnsRecord)
		case Remove:
			logrus.Infof("Removing dns record: %v", value)
			err = provider.RemoveRecord(value.DnsRecord)
		case Update:
			logrus.Infof("Updating dns record: %v", value)
			err = provider.UpdateRecord(value.DnsRecord)
		default:
			return changed, fmt.Errorf("unsupported DNS operation %q", op.Name)
		}
		if err != nil {
			return changed, fmt.Errorf("%s DNS record %s: %w", strings.ToLower(op.Name), value.DnsRecord.Fqdn, err)
		}
		if *op != Remove {
			changed = append(changed, value)
		}
	}
	return changed, nil
}

func updateExistingRecords(metadataRecs map[string]utils.MetadataDnsRecord, providerRecs map[string]utils.DnsRecord) ([]utils.MetadataDnsRecord, error) {
	var toUpdate []utils.MetadataDnsRecord
	for _, key := range sortedKeys(metadataRecs) {
		if _, ok := providerRecs[key]; ok {
			metadataR := make(map[string]struct{}, len(metadataRecs[key].DnsRecord.Records))
			for _, s := range metadataRecs[key].DnsRecord.Records {
				metadataR[s] = struct{}{}
			}

			providerR := make(map[string]struct{}, len(providerRecs[key].Records))
			for _, s := range providerRecs[key].Records {
				providerR[s] = struct{}{}
			}
			var update bool
			if len(metadataR) != len(providerR) {
				update = true
			} else {
				for key := range metadataR {
					if _, ok := providerR[key]; !ok {
						update = true
					}
				}
				for key := range providerR {
					if _, ok := metadataR[key]; !ok {
						update = true
					}
				}
			}
			if update {
				toUpdate = append(toUpdate, metadataRecs[key])
			}
		}
	}

	if len(toUpdate) == 0 {
		logrus.Debug("No DNS records to update")
	} else {
		logrus.Debugf("DNS records to update: %v", toUpdate)
	}

	return updateRecords(toUpdate, &Update)
}

func removeExtraRecords(metadataRecs map[string]utils.MetadataDnsRecord, providerRecs map[string]utils.DnsRecord) ([]utils.MetadataDnsRecord, error) {
	var toRemove []utils.MetadataDnsRecord
	for _, key := range sortedDNSRecordKeys(providerRecs) {
		if _, ok := metadataRecs[key]; !ok {
			toRemove = append(toRemove, utils.MetadataDnsRecord{
				ServiceName: "",
				StackName:   "",
				DnsRecord:   providerRecs[key],
			})
		}
	}

	if len(toRemove) == 0 {
		logrus.Debug("No DNS records to remove")
	} else {
		logrus.Debugf("DNS records to remove: %v", toRemove)
	}

	return updateRecords(toRemove, &Remove)
}

func sortedKeys(records map[string]utils.MetadataDnsRecord) []string {
	keys := make([]string, 0, len(records))
	for key := range records {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		left := records[keys[i]].DnsRecord
		right := records[keys[j]].DnsRecord
		if left.Type != right.Type {
			return left.Type != "TXT"
		}
		return keys[i] < keys[j]
	})
	return keys
}

func sortedDNSRecordKeys(records map[string]utils.DnsRecord) []string {
	keys := make([]string, 0, len(records))
	for key := range records {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		left := records[keys[i]]
		right := records[keys[j]]
		if left.Type != right.Type {
			return left.Type != "TXT"
		}
		return keys[i] < keys[j]
	})
	return keys
}

func getProviderDnsRecords() (map[string]utils.DnsRecord, map[string]utils.DnsRecord, error) {
	providerRecords, err := provider.GetRecords()
	if err != nil {
		return nil, nil, err
	}

	allRecords := make(map[string]utils.DnsRecord)
	ourRecords := make(map[string]utils.DnsRecord)
	if len(providerRecords) == 0 {
		return ourRecords, allRecords, nil
	}

	stateFqdn := utils.StateFqdn(m.EnvironmentUUID, config.RootDomainName)
	ourFqdns := make(map[string]struct{})

	// Get the FQDNs that were created by us from the state RRSet
	for _, rec := range providerRecords {
		if rec.Fqdn == stateFqdn && rec.Type == "TXT" {
			logrus.Debugf("FQDNs from state RRSet: %v", rec.Records)
			for _, value := range rec.Records {
				ourFqdns[value] = struct{}{}
			}
			ourRecords[stateFqdn] = rec
			break
		}
	}

	for _, rec := range providerRecords {
		if rec.Type == "A" {
			allRecords[rec.Fqdn] = rec
			if _, ok := ourFqdns[rec.Fqdn]; ok {
				ourRecords[rec.Fqdn] = rec
			}
		}
	}

	if stateRec, ok := ourRecords[stateFqdn]; ok {
		allRecords[stateFqdn] = stateRec
	}

	return ourRecords, allRecords, nil
}

// upgrade path from previous versions of external-dns.
// checks for any pre-existing A records with names matching the legacy
// suffix and TTLs matching the value of config.TTL. If any are found,
// a state RRSet is created in the zone using the FQDNs of the records
// as values.
func EnsureUpgradeToStateRRSet() error {
	allRecords, err := provider.GetRecords()
	if err != nil {
		return err
	}

	stateFqdn := utils.StateFqdn(m.EnvironmentUUID, config.RootDomainName)
	logrus.Debugf("Checking for state RRSet %s", stateFqdn)
	for _, rec := range allRecords {
		if rec.Fqdn == stateFqdn && rec.Type == "TXT" {
			logrus.Debugf("Found state RRSet with %d records", len(rec.Records))
			return nil
		}
	}

	logrus.Debug("State RRSet not found")
	ourFqdns := make(map[string]struct{})
	// records created by previous versions will match this suffix
	joins := []string{m.EnvironmentName, config.RootDomainName}
	suffix := "." + strings.ToLower(strings.Join(joins, "."))
	for _, rec := range allRecords {
		if rec.Type == "A" && strings.HasSuffix(rec.Fqdn, suffix) && rec.TTL == config.TTL {
			ourFqdns[rec.Fqdn] = struct{}{}
		}
	}

	if len(ourFqdns) > 0 {
		logrus.Infof("Creating RRSet '%s TXT' for %d pre-existing records", stateFqdn, len(ourFqdns))
		stateRec := utils.StateRecord(stateFqdn, config.TTL, ourFqdns)
		if err := provider.AddRecord(stateRec); err != nil {
			return fmt.Errorf("Failed to add RRSet to provider %v: %v", stateRec, err)
		}
	}

	return nil
}
