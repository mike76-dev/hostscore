// Package external contains the API of all services
// external to Sia.
package external

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"

	"github.com/mike76-dev/hostscore/internal/utils"
)

// IPInfo contains the geolocation data of a host.
type IPInfo struct {
	IP       string `json:"ip"`
	HostName string `json:"hostname"`
	City     string `json:"city"`
	Region   string `json:"region"`
	Country  string `json:"country"`
	Location string `json:"loc"`
	ISP      string `json:"org"`
	ZIP      string `json:"postal"`
	TimeZone string `json:"timezone"`
}

const (
	// marketAPI is the endpoint of the Siacoin exchange rate API.
	marketAPI = "https://api.siacentral.com/v2/market/exchange-rate"

	// ipInfoAPI is the endpoint of the IPInfo geolocation API.
	ipInfoAPI = "https://ipinfo.io/"
)

type (
	// marketResponse holds the market API response.
	marketResponse struct {
		Message string             `json:"message"`
		Type    string             `json:"type"`
		Price   map[string]float64 `json:"price"`
	}
)

// FetchSCRates retrieves the Siacoin exchange rates.
func FetchSCRates() (map[string]float64, error) {
	resp, err := http.Get(marketAPI)
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, errors.New("falied to fetch SC exchange rates")
		}
		var data marketResponse
		dec := json.NewDecoder(resp.Body)
		err = dec.Decode(&data)
		if err != nil {
			return nil, errors.New("wrong format of SC exchange rates")
		}
		return data.Price, nil
	}
	return nil, utils.AddContext(err, "falied to fetch SC exchange rates")
}

// FetchIPInfo uses the IPInfo API to fetch the host's geolocation.
func FetchIPInfo(addr, token string) (IPInfo, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return IPInfo{}, err
	}

	ips, err := net.LookupHost(host)
	if err != nil {
		return IPInfo{}, nil
	}
	if len(ips) == 0 {
		return IPInfo{}, nil
	}

	resp, err := http.Get(ipInfoAPI + ips[0] + "?token=" + token)
	if err != nil {
		return IPInfo{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return IPInfo{}, errors.New("failed to fetch host location")
	}

	var data IPInfo
	dec := json.NewDecoder(resp.Body)
	err = dec.Decode(&data)

	return data, err
}
