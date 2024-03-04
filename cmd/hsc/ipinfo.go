package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/mike76-dev/hostscore/hostdb"
	"github.com/mike76-dev/hostscore/internal/utils"
	"go.sia.tech/core/types"
)

const ipInfoAPI = "https://ipinfo.io/"

// fetchIPInfo uses the IPInfo API to fetch the host's geolocation.
func fetchIPInfo(addr, token string) (hostdb.IPInfo, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return hostdb.IPInfo{}, err
	}

	ips, err := net.LookupHost(host)
	if err != nil {
		return hostdb.IPInfo{}, err
	}
	if len(ips) == 0 {
		return hostdb.IPInfo{}, errors.New("unable to resolve netaddress")
	}

	resp, err := http.Get(ipInfoAPI + ips[0] + "?token=" + token)
	if err != nil {
		return hostdb.IPInfo{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return hostdb.IPInfo{}, errors.New("failed to fetch host location")
	}

	var data hostdb.IPInfo
	dec := json.NewDecoder(resp.Body)
	err = dec.Decode(&data)

	return data, err
}

// saveLocation saves the host's geolocation in the database.
func saveLocation(db *sql.DB, pk types.PublicKey, info hostdb.IPInfo) error {
	_, err := db.Exec(`
		INSERT INTO locations (
			public_key,
			ip,
			host_name,
			city,
			region,
			country,
			loc,
			isp,
			zip,
			time_zone,
			fetched_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) AS new
		ON DUPLICATE KEY UPDATE
			ip = new.ip,
			host_name = new.host_name,
			city = new.city,
			region = new.region,
			country = new.country,
			loc = new.loc,
			isp = new.isp,
			zip = new.zip,
			time_zone = new.time_zone,
			fetched_at = new.fetched_at
	`,
		pk[:],
		info.IP,
		info.HostName,
		info.City,
		info.Region,
		info.Country,
		info.Location,
		info.ISP,
		info.ZIP,
		info.TimeZone,
		time.Now().Unix(),
	)

	return err
}

// getLocation loads the host's geolocation from the database.
// If there is none present, the function tries to fetch it using the API.
func getLocation(db *sql.DB, host hostdb.HostDBEntry, token string) (info hostdb.IPInfo, lastFetched time.Time, err error) {
	var lf int64
	err = db.QueryRow(`
		SELECT
			ip,
			host_name,
			city,
			region,
			country,
			loc,
			isp,
			zip,
			time_zone,
			fetched_at
		FROM locations
		WHERE public_key = ?
	`, host.PublicKey[:]).Scan(
		info.IP,
		info.HostName,
		info.City,
		info.Region,
		info.Country,
		info.Location,
		info.ISP,
		info.ZIP,
		info.TimeZone,
		lf,
	)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return hostdb.IPInfo{}, time.Time{}, utils.AddContext(err, "couldn't query locations")
	}
	if err != nil {
		info, err = fetchIPInfo(host.NetAddress, token)
		if err != nil {
			return hostdb.IPInfo{}, time.Time{}, utils.AddContext(err, "couldn't fetch location")
		}
		if err := saveLocation(db, host.PublicKey, info); err != nil {
			return hostdb.IPInfo{}, time.Time{}, utils.AddContext(err, "couldn't save location")
		}
		return info, time.Now(), nil
	}
	lastFetched = time.Unix(lf, 0)
	return
}
