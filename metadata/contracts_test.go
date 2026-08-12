package metadata

import "testing"

func TestNeutralMetadataContracts(t *testing.T) {
	if defaultMetadataURL != "http://metadata/2015-12-19" {
		t.Fatalf("metadata URL = %q", defaultMetadataURL)
	}
	wantLabels := map[string]string{
		"service policy":   "io.pasturestack.service.external_dns",
		"service template": "io.pasturestack.service.external_dns_name_template",
		"host policy":      "io.pasturestack.host.external_dns",
		"host address":     "io.pasturestack.host.external_dns_ip",
	}
	gotLabels := map[string]string{
		"service policy":   servicePolicyLabel,
		"service template": serviceNameTemplateLabel,
		"host policy":      hostPolicyLabel,
		"host address":     hostExternalIPLabel,
	}
	for name, want := range wantLabels {
		if got := gotLabels[name]; got != want {
			t.Errorf("%s label = %q, want %q", name, got, want)
		}
	}
}

func TestLegacyMetadataContractsRemainReadable(t *testing.T) {
	wantLabels := map[string]string{
		"service policy":   "io.rancher.service.external_dns",
		"service template": "io.rancher.service.external_dns_name_template",
		"host policy":      "io.rancher.host.external_dns",
		"host address":     "io.rancher.host.external_dns_ip",
	}
	gotLabels := map[string]string{
		"service policy":   legacyServicePolicyLabel,
		"service template": legacyServiceNameTemplateLabel,
		"host policy":      legacyHostPolicyLabel,
		"host address":     legacyHostExternalIPLabel,
	}
	for name, want := range wantLabels {
		if got := gotLabels[name]; got != want {
			t.Errorf("%s label = %q, want %q", name, got, want)
		}
	}
}

func TestCompatibilityLabelValuePrefersNeutralLabel(t *testing.T) {
	labels := map[string]string{
		servicePolicyLabel:       "never",
		legacyServicePolicyLabel: "always",
	}
	value, ok := compatibilityLabelValue(
		labels, servicePolicyLabel, legacyServicePolicyLabel)
	if !ok || value != "never" {
		t.Fatalf("value = %q, ok = %v", value, ok)
	}
}

func TestCompatibilityLabelValueFallsBackToLegacyLabel(t *testing.T) {
	value, ok := compatibilityLabelValue(
		map[string]string{legacyHostExternalIPLabel: "192.0.2.20"},
		hostExternalIPLabel,
		legacyHostExternalIPLabel,
	)
	if !ok || value != "192.0.2.20" {
		t.Fatalf("value = %q, ok = %v", value, ok)
	}
}
