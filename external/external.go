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
	// rateAPI is the endpoint of the Siacoin exchange rate API.
	rateAPI = "https://api.siascan.com/exchange-rate/siacoin/usd"

	// ipInfoAPI is the endpoint of the IPInfo geolocation API.
	ipInfoAPI = "https://ipinfo.io/"
)

// FetchSCRate retrieves the Siacoin exchange rate.
func FetchSCRate() (rate float64, err error) {
	resp, err := http.Get(rateAPI)
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return 0, errors.New("falied to fetch SC exchange rate")
		}
		dec := json.NewDecoder(resp.Body)
		err = dec.Decode(&rate)
		if err != nil {
			return 0, errors.New("wrong format of SC exchange rate")
		}
		return
	}
	return 0, utils.AddContext(err, "falied to fetch SC exchange rate")
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
