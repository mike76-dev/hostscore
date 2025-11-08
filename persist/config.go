package persist

import (
	"os"
	"path/filepath"

	"go.sia.tech/core/types"
	"gopkg.in/yaml.v3"
)

// configFilename is the name of the configuration file.
const configFilename = "hsdconfig.yml"

// ConfigFields contains all information needed to start hsd.
type ConfigFields struct {
	GatewayMainnet string `yaml:"mainnet,omitempty"`
	GatewayZen     string `yaml:"zen,omitempty"`
	APIAddr        string `yaml:"api"`
	Dir            string `yaml:"dir"`
	DBUser         string `yaml:"dbUser"`
	DBName         string `yaml:"dbName"`
}

// PriceLimits specifies the price limit settings in SC and fiat.
type PriceLimits struct {
	MaxContractPriceSC  string  `yaml:"maxContractPriceSC"`
	MaxUploadPriceSC    string  `yaml:"maxUploadPriceSC"`
	MaxUploadPriceUSD   float64 `yaml:"maxUploadPriceUSD"`
	MaxDownloadPriceSC  string  `yaml:"maxDownloadPriceSC"`
	MaxDownloadPriceUSD float64 `yaml:"maxDownloadPriceUSD"`
	MaxStoragePriceSC   string  `yaml:"maxStoragePriceSC"`
	MaxStoragePriceUSD  float64 `yaml:"maxStoragePriceUSD"`
}

// ParsedLimits lists the SC limits in `types.Currency` rather than `string`.
type ParsedLimits struct {
	MaxContractPriceSC  types.Currency
	MaxUploadPriceSC    types.Currency
	MaxUploadPriceUSD   float64
	MaxDownloadPriceSC  types.Currency
	MaxDownloadPriceUSD float64
	MaxStoragePriceSC   types.Currency
	MaxStoragePriceUSD  float64
}

// HSDConfig contains the fields that are passed on to the new node.
type HSDConfig struct {
	Config ConfigFields `yaml:"config"`
	Limits PriceLimits  `yaml:"priceLimits,omitempty"`
}

// Config contains the parsed fields.
type Config struct {
	Config ConfigFields
	Limits ParsedLimits
}

// Load loads the configuration from disk.
func (hsdc *HSDConfig) Load(dir string) error {
	path := filepath.Join(dir, configFilename)
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	dec := yaml.NewDecoder(f)
	dec.KnownFields(true)

	if err := dec.Decode(hsdc); err != nil {
		return err
	}

	return nil
}
