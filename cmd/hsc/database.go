package main

import (
	"bytes"
	"database/sql"
	"errors"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/mike76-dev/hostscore/external"
	"github.com/mike76-dev/hostscore/hostdb"
	"github.com/mike76-dev/hostscore/internal/utils"
	rhpv2 "go.sia.tech/core/rhp/v2"
	rhpv3 "go.sia.tech/core/rhp/v3"
	"go.sia.tech/core/types"
	"go.uber.org/zap"
)

// insertUpdates updates the database with new records.
func (api *portalAPI) insertUpdates(node string, updates hostdb.HostUpdates) error {
	tx, err := api.db.Begin()
	if err != nil {
		return utils.AddContext(err, "couldn't start transaction")
	}

	hostStmt, err := tx.Prepare(`
		INSERT INTO hosts (
			id,
			network,
			public_key,
			first_seen,
			known_since,
			blocked,
			net_address,
			ip_nets,
			last_ip_change,
			settings,
			price_table
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) AS new
		ON DUPLICATE KEY UPDATE
			first_seen = new.first_seen,
			known_since = new.known_since,
			blocked = new.blocked,
			net_address = new.net_address,
			ip_nets = new.ip_nets,
			last_ip_change = new.last_ip_change,
			settings = new.settings,
			price_table = new.price_table
	`)
	if err != nil {
		tx.Rollback()
		return utils.AddContext(err, "couldn't prepare host statement")
	}
	defer hostStmt.Close()

	interactionsStmt, err := tx.Prepare(`
		INSERT INTO interactions (
			network,
			node,
			public_key,
			uptime,
			downtime,
			last_seen,
			active_hosts,
			historic_successful_interactions,
			historic_failed_interactions,
			recent_successful_interactions,
			recent_failed_interactions,
			last_update
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) AS new
		ON DUPLICATE KEY UPDATE
			uptime = new.uptime,
			downtime = new.downtime,
			last_seen = new.last_seen,
			active_hosts = new.active_hosts,
			historic_successful_interactions = new.historic_successful_interactions,
			historic_failed_interactions = new.historic_failed_interactions,
			recent_successful_interactions = new.recent_successful_interactions,
			recent_failed_interactions = new.recent_failed_interactions,
			last_update = new.last_update
	`)
	if err != nil {
		tx.Rollback()
		return utils.AddContext(err, "couldn't prepare interactions statement")
	}
	defer interactionsStmt.Close()

	scanSuccessStmt, err := tx.Prepare(`
		SELECT success
		FROM scans
		WHERE network = ?
		AND node = ?
		AND public_key = ?
		ORDER BY ran_at DESC
		LIMIT 1
	`)
	if err != nil {
		tx.Rollback()
		return utils.AddContext(err, "couldn't prepare scan success statement")
	}
	defer scanSuccessStmt.Close()

	scanStmt, err := tx.Prepare(`
		INSERT INTO scans (
			network,
			node,
			public_key,
			ran_at,
			success,
			latency,
			error,
			settings,
			price_table
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		tx.Rollback()
		return utils.AddContext(err, "couldn't prepare scan statement")
	}
	defer scanStmt.Close()

	benchmarkStmt, err := tx.Prepare(`
		INSERT INTO benchmarks (
			network,
			node,
			public_key,
			ran_at,
			success,
			upload_speed,
			download_speed,
			ttfb,
			error
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		tx.Rollback()
		return utils.AddContext(err, "couldn't prepare benchmark statement")
	}
	defer benchmarkStmt.Close()

	priceChangeCountStmt, err := tx.Prepare(`
		SELECT COUNT(*)
		FROM price_changes
		WHERE public_key = ?
	`)
	if err != nil {
		tx.Rollback()
		return utils.AddContext(err, "couldn't prepare price change count statement")
	}
	defer priceChangeCountStmt.Close()

	priceChangeStmt, err := tx.Prepare(`
		INSERT INTO price_changes (
			network,
			public_key,
			changed_at,
			remaining_storage,
			total_storage,
			collateral,
			storage_price,
			upload_price,
			download_price
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		tx.Rollback()
		return utils.AddContext(err, "couldn't prepare price change statement")
	}
	defer priceChangeStmt.Close()

	for _, host := range updates.Hosts {
		var settings, pt bytes.Buffer
		e := types.NewEncoder(&settings)
		if (host.Settings != rhpv2.HostSettings{}) {
			utils.EncodeSettings(&host.Settings, e)
			e.Flush()
		}
		e = types.NewEncoder(&pt)
		if (host.PriceTable != rhpv3.HostPriceTable{}) {
			utils.EncodePriceTable(&host.PriceTable, e)
			e.Flush()
		}
		_, err := hostStmt.Exec(
			host.ID,
			host.Network,
			host.PublicKey[:],
			host.FirstSeen.Unix(),
			host.KnownSince,
			host.Blocked,
			host.NetAddress,
			strings.Join(host.IPNets, ";"),
			host.LastIPChange.Unix(),
			settings.Bytes(),
			pt.Bytes(),
		)
		if err != nil {
			tx.Rollback()
			return utils.AddContext(err, "couldn't update host record")
		}
		_, err = interactionsStmt.Exec(
			host.Network,
			node,
			host.PublicKey[:],
			int64(host.Uptime.Seconds()),
			int64(host.Downtime.Seconds()),
			host.LastSeen.Unix(),
			host.ActiveHosts,
			host.Interactions.HistoricSuccesses,
			host.Interactions.HistoricFailures,
			host.Interactions.RecentSuccesses,
			host.Interactions.RecentFailures,
			host.Interactions.LastUpdate,
		)
		if err != nil {
			tx.Rollback()
			return utils.AddContext(err, "couldn't update host interactions")
		}
	}

	api.mu.Lock()
	for _, h := range updates.Hosts {
		var host *portalHost
		var exists bool
		if h.Network == "mainnet" {
			host, exists = api.hosts[h.PublicKey]
		} else {
			host, exists = api.hostsZen[h.PublicKey]
		}
		var count int
		if err := priceChangeCountStmt.QueryRow(h.PublicKey[:]).Scan(&count); err != nil {
			tx.Rollback()
			api.mu.Unlock()
			return utils.AddContext(err, "couldn't count price changes")
		}
		if !exists || count == 0 || pricesChanged(h.Settings, host.Settings) {
			var cb, spb, upb, dpb bytes.Buffer
			e := types.NewEncoder(&cb)
			types.V1Currency(h.Settings.Collateral).EncodeTo(e)
			e.Flush()
			e = types.NewEncoder(&spb)
			types.V1Currency(h.Settings.StoragePrice).EncodeTo(e)
			e.Flush()
			e = types.NewEncoder(&upb)
			types.V1Currency(h.Settings.UploadBandwidthPrice).EncodeTo(e)
			e.Flush()
			e = types.NewEncoder(&dpb)
			types.V1Currency(h.Settings.DownloadBandwidthPrice).EncodeTo(e)
			e.Flush()
			_, err := priceChangeStmt.Exec(
				h.Network,
				h.PublicKey[:],
				time.Now().Unix(),
				h.Settings.RemainingStorage,
				h.Settings.TotalStorage,
				cb.Bytes(),
				spb.Bytes(),
				upb.Bytes(),
				dpb.Bytes(),
			)
			if err != nil {
				api.log.Error("couldn't update price change", zap.String("network", h.Network), zap.Stringer("host", h.PublicKey), zap.Error(err))
			}
		}
		if exists {
			host.NetAddress = h.NetAddress
			host.Blocked = h.Blocked
			host.IPNets = h.IPNets
			host.LastIPChange = h.LastIPChange
			host.Settings = h.Settings
			host.PriceTable = h.PriceTable
			interactions := nodeInteractions{
				Uptime:      h.Uptime,
				Downtime:    h.Downtime,
				ScanHistory: h.ScanHistory,
				LastSeen:    h.LastSeen,
				ActiveHosts: h.ActiveHosts,
				HostInteractions: hostdb.HostInteractions{
					HistoricSuccesses: h.Interactions.HistoricSuccesses,
					HistoricFailures:  h.Interactions.HistoricFailures,
					RecentSuccesses:   h.Interactions.RecentSuccesses,
					RecentFailures:    h.Interactions.RecentFailures,
					LastUpdate:        h.Interactions.LastUpdate,
				},
			}
			host.Interactions[node] = interactions
		} else {
			host = &portalHost{
				ID:           h.ID,
				PublicKey:    h.PublicKey,
				FirstSeen:    h.FirstSeen,
				KnownSince:   h.KnownSince,
				NetAddress:   h.NetAddress,
				Blocked:      h.Blocked,
				Interactions: make(map[string]nodeInteractions),
				IPNets:       h.IPNets,
				LastIPChange: h.LastIPChange,
				Settings:     h.Settings,
				PriceTable:   h.PriceTable,
			}
			host.Interactions[node] = nodeInteractions{
				Uptime:      h.Uptime,
				Downtime:    h.Downtime,
				ScanHistory: h.ScanHistory,
				LastSeen:    h.LastSeen,
				ActiveHosts: h.ActiveHosts,
				HostInteractions: hostdb.HostInteractions{
					HistoricSuccesses: h.Interactions.HistoricSuccesses,
					HistoricFailures:  h.Interactions.HistoricFailures,
					RecentSuccesses:   h.Interactions.RecentSuccesses,
					RecentFailures:    h.Interactions.RecentFailures,
					LastUpdate:        h.Interactions.LastUpdate,
				},
			}
		}
		if h.Network == "mainnet" {
			api.hosts[h.PublicKey] = host
		} else if h.Network == "zen" {
			api.hostsZen[h.PublicKey] = host
		}
	}
	api.mu.Unlock()

	for _, scan := range updates.Scans {
		var settings, pt bytes.Buffer
		e := types.NewEncoder(&settings)
		if (scan.Settings != rhpv2.HostSettings{}) {
			utils.EncodeSettings(&scan.Settings, e)
			e.Flush()
		}
		e = types.NewEncoder(&pt)
		if (scan.PriceTable != rhpv3.HostPriceTable{}) {
			utils.EncodePriceTable(&scan.PriceTable, e)
			e.Flush()
		}
		_, err := scanStmt.Exec(
			scan.Network,
			node,
			scan.PublicKey[:],
			scan.Timestamp.Unix(),
			scan.Success,
			scan.Latency.Milliseconds(),
			scan.Error,
			settings.Bytes(),
			pt.Bytes(),
		)
		if err != nil {
			tx.Rollback()
			return utils.AddContext(err, "couldn't insert scan record")
		}
	}

	for _, benchmark := range updates.Benchmarks {
		_, err := benchmarkStmt.Exec(
			benchmark.Network,
			node,
			benchmark.PublicKey[:],
			benchmark.Timestamp.Unix(),
			benchmark.Success,
			benchmark.UploadSpeed,
			benchmark.DownloadSpeed,
			benchmark.TTFB.Milliseconds(),
			benchmark.Error,
		)
		if err != nil {
			tx.Rollback()
			return utils.AddContext(err, "couldn't insert benchmark record")
		}
	}

	if err := tx.Commit(); err != nil {
		return utils.AddContext(err, "couldn't commit transaction")
	}

	if err := api.clients[node].FinalizeUpdates(updates.ID); err != nil {
		return utils.AddContext(err, "couldn't finalize updates")
	}

	return nil
}

// isOnline returns true if the host is considered online by at least one node.
func (api *portalAPI) isOnline(host portalHost) bool {
	for _, interactions := range host.Interactions {
		history := interactions.ScanHistory
		if len(history) > 1 && history[0].Success && history[1].Success {
			return true
		}
		if len(history) == 1 && history[0].Success {
			return true
		}
	}
	return false
}

// pricesChanged returns true if any relevant part of the host's settings has changed.
func pricesChanged(os, ns rhpv2.HostSettings) bool {
	if ns.RemainingStorage != os.RemainingStorage || ns.TotalStorage != os.TotalStorage {
		return true
	}
	if ns.StoragePrice.Cmp(os.StoragePrice) != 0 || ns.Collateral.Cmp(os.Collateral) != 0 {
		return true
	}
	if ns.UploadBandwidthPrice.Cmp(os.UploadBandwidthPrice) != 0 {
		return true
	}
	if ns.DownloadBandwidthPrice.Cmp(os.DownloadBandwidthPrice) != 0 {
		return true
	}
	return false
}

// getHost retrieves the information about a specific host.
func (api *portalAPI) getHost(network string, pk types.PublicKey) (host portalHost, err error) {
	var h *portalHost
	var exists bool
	api.mu.RLock()
	if network == "mainnet" {
		h, exists = api.hosts[pk]
	} else if network == "zen" {
		h, exists = api.hostsZen[pk]
	}
	api.mu.RUnlock()
	if !exists {
		return portalHost{}, errors.New("host not found")
	}

	host = *h
	info, lastFetched, err := api.getLocation(pk, host.NetAddress)
	if err != nil {
		return portalHost{}, utils.AddContext(err, "couldn't get host location")
	} else if host.LastIPChange.After(lastFetched) {
		newInfo, err := external.FetchIPInfo(host.NetAddress, api.token)
		if err != nil {
			api.log.Error("couldn't fetch host location", zap.String("host", host.NetAddress), zap.Error(err))
		} else {
			if (newInfo != external.IPInfo{}) {
				info = newInfo
				err = api.saveLocation(pk, info)
				if err != nil {
					return portalHost{}, utils.AddContext(err, "couldn't update host location")
				}
			} else {
				api.log.Debug("empty host location received", zap.String("host", host.NetAddress))
			}
		}
	}

	host.IPInfo = info
	return
}

// getHosts retrieves the given number of host records.
func (api *portalAPI) getHosts(network string, all bool, offset, limit int, query string) (hosts []portalHost, more bool, total int, err error) {
	if offset < 0 {
		offset = 0
	}

	if all {
		query = "%" + query + "%"
		err = api.db.QueryRow(`
			SELECT COUNT(*)
			FROM hosts
			WHERE network = ?
			AND net_address LIKE ?
		`, network, query).Scan(&total)
		if err != nil {
			return nil, false, 0, utils.AddContext(err, "couldn't count hosts")
		}
		if total == 0 {
			return
		}
		more = limit > 0 && offset+limit < total
		if limit < 0 {
			limit = math.MaxInt64
		}

		rows, err := api.db.Query(`
			SELECT
				id,
				public_key,
				first_seen,
				known_since,
				blocked,
				net_address,
				ip_nets,
				last_ip_change,
				settings,
				price_table
			FROM hosts
			WHERE network = ?
			AND net_address LIKE ?
			ORDER BY id ASC
			LIMIT ?, ?
		`, network, query, offset, limit)
		if err != nil {
			return nil, false, 0, utils.AddContext(err, "couldn't query hosts")
		}

		for rows.Next() {
			var id int
			pk := make([]byte, 32)
			var ks uint64
			var b bool
			var na, ip string
			var fs, lc int64
			var settings, pt []byte
			if err := rows.Scan(&id, &pk, &fs, &ks, &b, &na, &ip, &lc, &settings, &pt); err != nil {
				rows.Close()
				return nil, false, 0, utils.AddContext(err, "couldn't decode host data")
			}
			host := portalHost{
				ID:           id,
				PublicKey:    types.PublicKey(pk),
				FirstSeen:    time.Unix(fs, 0),
				KnownSince:   ks,
				Blocked:      b,
				NetAddress:   na,
				IPNets:       strings.Split(ip, ";"),
				LastIPChange: time.Unix(lc, 0),
				Interactions: make(map[string]nodeInteractions),
			}
			if len(settings) > 0 {
				d := types.NewBufDecoder(settings)
				utils.DecodeSettings(&host.Settings, d)
				if err := d.Err(); err != nil {
					rows.Close()
					return nil, false, 0, utils.AddContext(err, "couldn't decode host settings")
				}
			}
			if len(pt) > 0 {
				d := types.NewBufDecoder(pt)
				utils.DecodePriceTable(&host.PriceTable, d)
				if err := d.Err(); err != nil {
					rows.Close()
					return nil, false, 0, utils.AddContext(err, "couldn't decode host price table")
				}
			}
			hosts = append(hosts, host)
		}
		rows.Close()

		for i := range hosts {
			rows, err = api.db.Query(`
				SELECT
					node,
					uptime,
					downtime,
					last_seen,
					active_hosts,
					historic_successful_interactions,
					historic_failed_interactions,
					recent_successful_interactions,
					recent_failed_interactions,
					last_update
				FROM interactions
				WHERE network = ?
				AND public_key = ?
			`, network, hosts[i].PublicKey[:])
			if err != nil {
				return nil, false, 0, utils.AddContext(err, "couldn't query interactions")
			}

			for rows.Next() {
				var lu uint64
				var ut, dt, ls int64
				var hsi, hfi, rsi, rfi float64
				var ah int
				var node string
				if err := rows.Scan(&node, &ut, &dt, &ls, &ah, &hsi, &hfi, &rsi, &rfi, &lu); err != nil {
					rows.Close()
					return nil, false, 0, utils.AddContext(err, "couldn't decode interactions")
				}
				interactions := nodeInteractions{
					Uptime:      time.Duration(ut) * time.Second,
					Downtime:    time.Duration(dt) * time.Second,
					LastSeen:    time.Unix(ls, 0),
					ActiveHosts: ah,
					HostInteractions: hostdb.HostInteractions{
						HistoricSuccesses: hsi,
						HistoricFailures:  hfi,
						RecentSuccesses:   rsi,
						RecentFailures:    rfi,
						LastUpdate:        lu,
					},
				}

				scanRows, err := api.db.Query(`
					SELECT ran_at, success, latency, error, settings, price_table
					FROM scans
					WHERE network = ?
					AND public_key = ?
					AND node = ?
					ORDER BY ran_at DESC
					LIMIT 2
				`, network, hosts[i].PublicKey[:], node)
				if err != nil {
					rows.Close()
					return nil, false, 0, utils.AddContext(err, "couldn't query scan history")
				}

				for scanRows.Next() {
					var ra int64
					var success bool
					var latency float64
					var msg string
					var settings, pt []byte
					if err := scanRows.Scan(&ra, &success, &latency, &msg, &settings, &pt); err != nil {
						scanRows.Close()
						rows.Close()
						return nil, false, 0, utils.AddContext(err, "couldn't decode scan history")
					}
					scan := hostdb.HostScan{
						Timestamp: time.Unix(ra, 0),
						Success:   success,
						Latency:   time.Duration(latency) * time.Millisecond,
						Error:     msg,
					}
					if len(settings) > 0 {
						d := types.NewBufDecoder(settings)
						utils.DecodeSettings(&scan.Settings, d)
						if err := d.Err(); err != nil {
							scanRows.Close()
							rows.Close()
							return nil, false, 0, utils.AddContext(err, "couldn't decode host settings")
						}
					}
					if len(pt) > 0 {
						d := types.NewBufDecoder(pt)
						utils.DecodePriceTable(&scan.PriceTable, d)
						if err := d.Err(); err != nil {
							scanRows.Close()
							rows.Close()
							return nil, false, 0, utils.AddContext(err, "couldn't decode host price table")
						}
					}
					interactions.ScanHistory = append([]hostdb.HostScan{scan}, interactions.ScanHistory...)
				}
				scanRows.Close()
				hosts[i].Interactions[node] = interactions
			}
			rows.Close()
		}
	} else {
		api.mu.RLock()
		if network == "mainnet" {
			for _, host := range api.hosts {
				if api.isOnline(*host) && (query == "" || strings.Contains(host.NetAddress, query)) {
					hosts = append(hosts, *host)
				}
			}
		} else if network == "zen" {
			for _, host := range api.hostsZen {
				if api.isOnline(*host) && (query == "" || strings.Contains(host.NetAddress, query)) {
					hosts = append(hosts, *host)
				}
			}
		}
		api.mu.RUnlock()

		slices.SortFunc(hosts, func(a, b portalHost) int { return a.ID - b.ID })

		if limit < 0 {
			limit = len(hosts)
		}
		if offset > len(hosts) {
			offset = len(hosts)
		}
		if offset+limit > len(hosts) {
			limit = len(hosts) - offset
		}
		total = len(hosts)
		more = offset+limit < total
		hosts = hosts[offset : offset+limit]
	}

	for i := range hosts {
		info, lastFetched, err := api.getLocation(hosts[i].PublicKey, hosts[i].NetAddress)
		if err != nil {
			return nil, false, 0, utils.AddContext(err, "couldn't get host location")
		} else if hosts[i].LastIPChange.After(lastFetched) {
			newInfo, err := external.FetchIPInfo(hosts[i].NetAddress, api.token)
			if err != nil {
				api.log.Error("couldn't fetch host location", zap.String("host", hosts[i].NetAddress), zap.Error(err))
			} else {
				if (newInfo != external.IPInfo{}) {
					info = newInfo
					err = api.saveLocation(hosts[i].PublicKey, info)
					if err != nil {
						return nil, false, 0, utils.AddContext(err, "couldn't update host location")
					}
				} else {
					api.log.Debug("empty host location received", zap.String("host", hosts[i].NetAddress))
				}
			}
		}

		hosts[i].IPInfo = info
	}

	return
}

// getLocation loads the host's geolocation from the database.
// If there is none present, the function tries to fetch it using the API.
func (api *portalAPI) getLocation(pk types.PublicKey, addr string) (info external.IPInfo, lastFetched time.Time, err error) {
	var lf int64
	err = api.db.QueryRow(`
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
	`, pk[:]).Scan(
		&info.IP,
		&info.HostName,
		&info.City,
		&info.Region,
		&info.Country,
		&info.Location,
		&info.ISP,
		&info.ZIP,
		&info.TimeZone,
		&lf,
	)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return external.IPInfo{}, time.Time{}, utils.AddContext(err, "couldn't query locations")
	}
	if err != nil {
		info, err = external.FetchIPInfo(addr, api.token)
		if err != nil {
			return external.IPInfo{}, time.Time{}, utils.AddContext(err, "couldn't fetch location")
		}
		if err := api.saveLocation(pk, info); err != nil {
			return external.IPInfo{}, time.Time{}, utils.AddContext(err, "couldn't save location")
		}
		return info, time.Now(), nil
	}
	lastFetched = time.Unix(lf, 0)
	return
}

// saveLocation saves the host's geolocation in the database.
func (api *portalAPI) saveLocation(pk types.PublicKey, info external.IPInfo) error {
	_, err := api.db.Exec(`
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

// getScans returns the scan history according to the criteria provided.
func (api *portalAPI) getScans(network string, pk types.PublicKey, from, to time.Time, num int, successful bool) (scans []hostdb.ScanHistory, err error) {
	if to.IsZero() {
		to = time.Now()
	}
	if num < 0 {
		num = 0
	}

	rows, err := api.db.Query(`
		SELECT node, ran_at, success, latency, error, settings, price_table
		FROM scans
		WHERE network = ?
		AND public_key = ?
		AND ran_at > ?
		AND ran_at < ?
		AND (? OR success = TRUE)
		ORDER BY ran_at DESC
		LIMIT ?
	`,
		network,
		pk[:],
		from.Unix(),
		to.Unix(),
		!successful,
		num,
	)
	if err != nil {
		return nil, utils.AddContext(err, "couldn't query scan history")
	}
	defer rows.Close()

	for rows.Next() {
		var ra int64
		var success bool
		var latency float64
		var node, msg string
		var settings, pt []byte
		if err := rows.Scan(&node, &ra, &success, &latency, &msg, &settings, &pt); err != nil {
			return nil, utils.AddContext(err, "couldn't decode scan history")
		}
		scan := hostdb.ScanHistory{
			HostScan: hostdb.HostScan{
				Timestamp: time.Unix(ra, 0),
				Success:   success,
				Latency:   time.Duration(latency) * time.Millisecond,
				Error:     msg,
			},
			PublicKey: pk,
			Network:   network,
			Node:      node,
		}
		if len(settings) > 0 {
			d := types.NewBufDecoder(settings)
			utils.DecodeSettings(&scan.Settings, d)
			if err := d.Err(); err != nil {
				return nil, utils.AddContext(err, "couldn't decode host settings")
			}
		}
		if len(pt) > 0 {
			d := types.NewBufDecoder(pt)
			utils.DecodePriceTable(&scan.PriceTable, d)
			if err := d.Err(); err != nil {
				return nil, utils.AddContext(err, "couldn't decode host price table")
			}
		}
		scans = append(scans, scan)
	}

	return
}

// getBenchmarks returns the benchmark history according to the criteria provided.
func (api *portalAPI) getBenchmarks(network string, pk types.PublicKey, from, to time.Time, num int, successful bool) (benchmarks []hostdb.BenchmarkHistory, err error) {
	if to.IsZero() {
		to = time.Now()
	}
	if num < 0 {
		num = 0
	}

	rows, err := api.db.Query(`
		SELECT node, ran_at, success, upload_speed, download_speed, ttfb, error
		FROM benchmarks
		WHERE network = ?
		AND public_key = ?
		AND ran_at > ?
		AND ran_at < ?
		AND (? OR success = TRUE)
		ORDER BY ran_at DESC
		LIMIT ?
	`,
		network,
		pk[:],
		from.Unix(),
		to.Unix(),
		!successful,
		num,
	)
	if err != nil {
		return nil, utils.AddContext(err, "couldn't query benchmark history")
	}
	defer rows.Close()

	for rows.Next() {
		var ra int64
		var success bool
		var ul, dl, ttfb float64
		var node, msg string
		if err := rows.Scan(&node, &ra, &success, &ul, &dl, &ttfb, &msg); err != nil {
			return nil, utils.AddContext(err, "couldn't query benchmark history")
		}
		benchmark := hostdb.BenchmarkHistory{
			HostBenchmark: hostdb.HostBenchmark{
				Timestamp:     time.Unix(ra, 0),
				Success:       success,
				UploadSpeed:   ul,
				DownloadSpeed: dl,
				TTFB:          time.Duration(ttfb) * time.Millisecond,
				Error:         msg,
			},
			PublicKey: pk,
			Network:   network,
			Node:      node,
		}
		benchmarks = append(benchmarks, benchmark)
	}

	return
}

// load loads the online hosts map from the database.
func (api *portalAPI) load() error {
	hostStmt, err := api.db.Prepare(`
		SELECT
			id,
			network,
			public_key,
			first_seen,
			known_since,
			blocked,
			net_address,
			ip_nets,
			last_ip_change,
			settings,
			price_table
		FROM hosts
	`)
	if err != nil {
		return utils.AddContext(err, "couldn't prepare hosts statement")
	}
	defer hostStmt.Close()

	rows, err := hostStmt.Query()
	if err != nil {
		return utils.AddContext(err, "couldn't query hosts")
	}

	for rows.Next() {
		var id int
		var network, netaddress, ipNets string
		pk := make([]byte, 32)
		var fs, lc int64
		var ks uint64
		var blocked bool
		var settings, pt []byte
		if err := rows.Scan(&id, &network, &pk, &fs, &ks, &blocked, &netaddress, &ipNets, &lc, &settings, &pt); err != nil {
			rows.Close()
			return utils.AddContext(err, "couldn't decode host data")
		}
		host := &portalHost{
			ID:           id,
			PublicKey:    types.PublicKey(pk),
			FirstSeen:    time.Unix(fs, 0),
			KnownSince:   ks,
			Blocked:      blocked,
			NetAddress:   netaddress,
			IPNets:       strings.Split(ipNets, ";"),
			LastIPChange: time.Unix(lc, 0),
			Interactions: make(map[string]nodeInteractions),
		}
		if len(settings) > 0 {
			d := types.NewBufDecoder(settings)
			utils.DecodeSettings(&host.Settings, d)
			if err := d.Err(); err != nil {
				rows.Close()
				return utils.AddContext(err, "couldn't decode host settings")
			}
		}
		if len(pt) > 0 {
			d := types.NewBufDecoder(pt)
			utils.DecodePriceTable(&host.PriceTable, d)
			if err := d.Err(); err != nil {
				rows.Close()
				return utils.AddContext(err, "couldn't decode host price table")
			}
		}

		if network == "mainnet" {
			api.hosts[host.PublicKey] = host
		} else if network == "zen" {
			api.hostsZen[host.PublicKey] = host
		}
	}
	rows.Close()

	if err := api.loadInteractions(api.hosts, "mainnet"); err != nil {
		return utils.AddContext(err, "couldn't load mainnet interactions")
	}

	if err := api.loadInteractions(api.hostsZen, "zen"); err != nil {
		return utils.AddContext(err, "couldn't load zen interactions")
	}

	return nil
}

func (api *portalAPI) loadInteractions(hosts map[types.PublicKey]*portalHost, network string) error {
	intStmt, err := api.db.Prepare(`
		SELECT
			node,
			uptime,
			downtime,
			last_seen,
			active_hosts,
			historic_successful_interactions,
			historic_failed_interactions,
			recent_successful_interactions,
			recent_failed_interactions,
			last_update
		FROM interactions
		WHERE network = ?
		AND public_key = ?
	`)
	if err != nil {
		return utils.AddContext(err, "couldn't prepare interactions statement")
	}
	defer intStmt.Close()

	for _, host := range hosts {
		rows, err := intStmt.Query(network, host.PublicKey[:])
		if err != nil {
			return utils.AddContext(err, "couldn't query interactions")
		}

		for rows.Next() {
			var node string
			var lu uint64
			var ut, dt, ls int64
			var hsi, hfi, rsi, rfi float64
			var ah int
			if err := rows.Scan(&node, &ut, &dt, &ls, &ah, &hsi, &hfi, &rsi, &rfi, &lu); err != nil {
				rows.Close()
				return utils.AddContext(err, "couldn't decode interactions")
			}
			interactions := nodeInteractions{
				Uptime:      time.Duration(ut) * time.Second,
				Downtime:    time.Duration(dt) * time.Second,
				LastSeen:    time.Unix(ls, 0),
				ActiveHosts: ah,
				HostInteractions: hostdb.HostInteractions{
					HistoricSuccesses: hsi,
					HistoricFailures:  hfi,
					RecentSuccesses:   rsi,
					RecentFailures:    rfi,
					LastUpdate:        lu,
				},
			}
			host.Interactions[node] = interactions
		}
		rows.Close()
	}

	return api.loadScans(hosts, network)
}

func (api *portalAPI) loadScans(hosts map[types.PublicKey]*portalHost, network string) error {
	scanStmt, err := api.db.Prepare(`
		SELECT
			ran_at,
			success,
			latency,
			error,
			settings,
			price_table
		FROM scans
		WHERE network = ?
		AND node = ?
		AND public_key = ?
		ORDER BY ran_at DESC
		LIMIT 2
	`)
	if err != nil {
		return utils.AddContext(err, "couldn't prepare scan statement")
	}
	defer scanStmt.Close()

	for _, host := range hosts {
		for node, int := range host.Interactions {
			rows, err := scanStmt.Query(network, node, host.PublicKey[:])
			if err != nil {
				return utils.AddContext(err, "couldn't query scan history")
			}

			for rows.Next() {
				var ra int64
				var success bool
				var latency float64
				var msg string
				var settings, pt []byte
				if err := rows.Scan(&ra, &success, &latency, &msg, &settings, &pt); err != nil {
					rows.Close()
					return utils.AddContext(err, "couldn't decode scan history")
				}
				scan := hostdb.HostScan{
					Timestamp: time.Unix(ra, 0),
					Success:   success,
					Latency:   time.Duration(latency) * time.Millisecond,
					Error:     msg,
				}
				if len(settings) > 0 {
					d := types.NewBufDecoder(settings)
					utils.DecodeSettings(&scan.Settings, d)
					if err := d.Err(); err != nil {
						rows.Close()
						return utils.AddContext(err, "couldn't decode host settings")
					}
				}
				if len(pt) > 0 {
					d := types.NewBufDecoder(pt)
					utils.DecodePriceTable(&scan.PriceTable, d)
					if err := d.Err(); err != nil {
						rows.Close()
						return utils.AddContext(err, "couldn't decode host price table")
					}
				}
				int.ScanHistory = append([]hostdb.HostScan{scan}, int.ScanHistory...)
			}
			rows.Close()
			host.Interactions[node] = int
		}
	}

	return nil
}

// getPriceChanges retrieves the historic price changes of the given host.
func (api *portalAPI) getPriceChanges(network string, pk types.PublicKey) (pcs []priceChange, err error) {
	rows, err := api.db.Query(`
		SELECT
			changed_at,
			remaining_storage,
			total_storage,
			collateral,
			storage_price,
			upload_price,
			download_price
		FROM price_changes
		WHERE network = ?
		AND public_key = ?
		AND changed_at >= ?
		ORDER BY changed_at ASC
	`, network, pk[:], time.Now().AddDate(-1, 0, 0))
	if err != nil {
		return nil, utils.AddContext(err, "couldn't query price changes")
	}
	defer rows.Close()

	for rows.Next() {
		var ca int64
		var rs, ts uint64
		var cb, spb, upb, dpb []byte
		if err := rows.Scan(&ca, &rs, &ts, &cb, &spb, &upb, &dpb); err != nil {
			return nil, utils.AddContext(err, "couldn't decode price change")
		}

		pc := priceChange{
			Timestamp:        time.Unix(ca, 0),
			RemainingStorage: rs,
			TotalStorage:     ts,
		}

		d := types.NewBufDecoder(cb)
		if (*types.V1Currency)(&pc.Collateral).DecodeFrom(d); d.Err() != nil {
			return nil, utils.AddContext(err, "couldn't decode collateral")
		}
		d = types.NewBufDecoder(spb)
		if (*types.V1Currency)(&pc.StoragePrice).DecodeFrom(d); d.Err() != nil {
			return nil, utils.AddContext(err, "couldn't decode storage price")
		}
		d = types.NewBufDecoder(upb)
		if (*types.V1Currency)(&pc.UploadPrice).DecodeFrom(d); d.Err() != nil {
			return nil, utils.AddContext(err, "couldn't decode upload price")
		}
		d = types.NewBufDecoder(dpb)
		if (*types.V1Currency)(&pc.DownloadPrice).DecodeFrom(d); d.Err() != nil {
			return nil, utils.AddContext(err, "couldn't decode download price")
		}

		pcs = append(pcs, pc)
	}

	return
}

// pruneOldRecords periodically cleans the database from old scan and benchmarks.
func (api *portalAPI) pruneOldRecords() {
	for {
		select {
		case <-api.stopChan:
			return
		case <-time.After(24 * time.Hour):
		}

		_, err := api.db.Exec(`
			DELETE FROM scans
			WHERE ran_at < ?
		`, time.Now().AddDate(0, 0, -14).Unix())
		if err != nil {
			api.log.Error("couldn't delete old scans", zap.Error(err))
		}

		_, err = api.db.Exec(`
			DELETE FROM benchmarks
			WHERE ran_at < ?
		`, time.Now().AddDate(0, 0, -56).Unix())
		if err != nil {
			api.log.Error("couldn't delete old benchmarks", zap.Error(err))
		}
	}
}
