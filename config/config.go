package config

// Modified by PastureStack contributors for independent maintenance and rebranding.

import (
	"os"
	"strconv"

	"github.com/PastureStack/external-dns-sync/utils"
	"github.com/sirupsen/logrus"
)

const (
	defaultNameTemplate = "%{{service_name}}.%{{stack_name}}.%{{environment_name}}"
)

var (
	RootDomainName    string
	TTL               int
	PlatformURL       string
	PlatformAccessKey string
	PlatformSecretKey string
	NameTemplate      string
	MetadataURL       string
	HealthAddress     string
)

func SetFromEnvironment() {
	PlatformURL = getEnv("PLATFORM_URL")
	PlatformAccessKey = getEnv("PLATFORM_ACCESS_KEY")
	PlatformSecretKey = getEnv("PLATFORM_SECRET_KEY")
	RootDomainName = utils.Fqdn(getEnv("ROOT_DOMAIN"))
	MetadataURL = getOptionalEnv("METADATA_URL", "http://metadata/2015-12-19")
	HealthAddress = getOptionalEnv("HEALTH_ADDRESS", ":10000")
	NameTemplate = os.Getenv("NAME_TEMPLATE")
	if len(NameTemplate) == 0 {
		NameTemplate = defaultNameTemplate
	}

	TTLEnv := os.Getenv("TTL")
	i, err := strconv.Atoi(TTLEnv)
	if err != nil || i <= 0 {
		TTL = 300
	} else {
		TTL = i
	}
}

func getEnv(name string) string {
	envVar := os.Getenv(name)
	if len(envVar) == 0 {
		logrus.Fatalf("Environment variable '%s' is not set", name)
	}
	return envVar
}

func getOptionalEnv(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
