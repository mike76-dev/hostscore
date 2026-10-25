package persist

import (
	"os"
	"path/filepath"
	"testing"
)

// writeConfig writes contents to hsdconfig.yml in a temp directory and
// returns the directory.
func writeConfig(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, configFilename), []byte(contents), 0660); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestLoadConfig(t *testing.T) {
	dir := writeConfig(t, `config:
  mainnet: :9981
  zen: :9881
  api: :9980
  dir: /var/lib/hsd
  dbUser: hsuser
  dbName: hostscore
priceLimits:
  maxContractPriceSC: 1SC
  maxUploadPriceUSD: 6.0
  maxDownloadPriceUSD: 18.0
  maxStoragePriceUSD: 6.0
`)

	var config HSDConfig
	if err := config.Load(dir); err != nil {
		t.Fatalf("Load: %v", err)
	}

	want := HSDConfig{
		Config: ConfigFields{
			GatewayMainnet: ":9981",
			GatewayZen:     ":9881",
			APIAddr:        ":9980",
			Dir:            "/var/lib/hsd",
			DBUser:         "hsuser",
			DBName:         "hostscore",
		},
		Limits: PriceLimits{
			MaxContractPriceSC:  "1SC",
			MaxUploadPriceUSD:   6.0,
			MaxDownloadPriceUSD: 18.0,
			MaxStoragePriceUSD:  6.0,
		},
	}
	if config != want {
		t.Errorf("got %+v, want %+v", config, want)
	}
}

func TestLoadConfigPartial(t *testing.T) {
	// Omitted fields keep whatever values the struct already holds, so the
	// caller's defaults survive a partial config.
	dir := writeConfig(t, `config:
  dbUser: hsuser
  dbName: hostscore
`)

	config := HSDConfig{
		Config: ConfigFields{
			GatewayMainnet: ":9981",
			GatewayZen:     ":9881",
			APIAddr:        ":9980",
			Dir:            "default-dir",
		},
		Limits: PriceLimits{
			MaxContractPriceSC: "1SC",
			MaxUploadPriceUSD:  6.0,
		},
	}
	if err := config.Load(dir); err != nil {
		t.Fatalf("Load: %v", err)
	}

	if config.Config.DBUser != "hsuser" || config.Config.DBName != "hostscore" {
		t.Errorf("config fields not loaded: %+v", config.Config)
	}
	if config.Config.GatewayMainnet != ":9981" || config.Config.APIAddr != ":9980" || config.Config.Dir != "default-dir" {
		t.Errorf("defaults not preserved: %+v", config.Config)
	}
	if config.Limits.MaxContractPriceSC != "1SC" || config.Limits.MaxUploadPriceUSD != 6.0 {
		t.Errorf("default limits not preserved: %+v", config.Limits)
	}
}

func TestLoadConfigMissingFile(t *testing.T) {
	var config HSDConfig
	err := config.Load(t.TempDir())
	if !os.IsNotExist(err) {
		t.Errorf("got %v, want a not-exist error", err)
	}
}

func TestLoadConfigUnknownField(t *testing.T) {
	// The decoder runs with KnownFields, so typos are rejected rather than
	// silently ignored.
	dir := writeConfig(t, `config:
  dbUsr: hsuser
`)

	var config HSDConfig
	if err := config.Load(dir); err == nil {
		t.Error("expected an error for an unknown field")
	}
}

func TestLoadConfigMalformed(t *testing.T) {
	dir := writeConfig(t, "config: [not: a mapping\n")

	var config HSDConfig
	if err := config.Load(dir); err == nil {
		t.Error("expected an error for malformed YAML")
	}
}
