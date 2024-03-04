package hostdb

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"time"
)

const (
	ipInfoAPI             = "https://ipinfo.io/"
	locationCheckInterval = 24 * time.Hour
)

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

func fetchIPInfo(addr string) (IPInfo, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return IPInfo{}, err
	}

	ips, err := net.LookupHost(host)
	if err != nil {
		return IPInfo{}, err
	}
	if len(ips) == 0 {
		return IPInfo{}, errors.New("unable to resolve netaddress")
	}

	resp, err := http.Get(ipInfoAPI + ips[0])
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

func (hdb *HostDB) fetchLocations() {
	getLocations := func() {
		hosts, _ := hdb.s.getHosts(false, 0, -1, "")
		hostsZen, _ := hdb.sZen.getHosts(false, 0, -1, "")
		hosts = append(hosts, hostsZen...)
		for _, host := range hosts {
			info, err := fetchIPInfo(host.NetAddress)
			if err == nil {
				if err := hdb.saveLocation(&host, info); err != nil {
					hdb.log.Println("[ERROR] couldn't save location:", err)
				}
			}
		}
	}
	if err := hdb.tg.Add(); err != nil {
		hdb.log.Println("[ERROR] couldn't add a thread:", err)
		return
	}
	defer hdb.tg.Done()

	getLocations()

	for {
		select {
		case <-hdb.tg.StopChan():
			return
		case <-time.After(locationCheckInterval):
			getLocations()
		}
	}
}
