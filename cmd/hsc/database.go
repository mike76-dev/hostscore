package main

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strconv"
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
			price_score,
			storage_score,
			collateral_score,
			interactions_score,
			uptime_score,
			age_score,
			version_score,
			latency_score,
			benchmarks_score,
			contracts_score,
			total_score,
			settings,
			price_table
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) AS new
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
			price_score,
			storage_score,
			collateral_score,
			interactions_score,
			uptime_score,
			age_score,
			version_score,
			latency_score,
			benchmarks_score,
			contracts_score,
			total_score,
			historic_successful_interactions,
			historic_failed_interactions,
			recent_successful_interactions,
			recent_failed_interactions,
			last_update
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) AS new
		ON DUPLICATE KEY UPDATE
			uptime = new.uptime,
			downtime = new.downtime,
			last_seen = new.last_seen,
			active_hosts = new.active_hosts,
			price_score = new.price_score,
			storage_score = new.storage_score,
			collateral_score = new.collateral_score,
			interactions_score = new.interactions_score,
			uptime_score = new.uptime_score,
			age_score = new.age_score,
			version_score = new.version_score,
			latency_score = new.latency_score,
			benchmarks_score = new.benchmarks_score,
			contracts_score = new.contracts_score,
			total_score = new.total_score,
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

	updateScoreStmt, err := tx.Prepare(`
		UPDATE hosts
		SET price_score = ?,
			storage_score = ?,
			collateral_score = ?,
			interactions_score = ?,
			uptime_score = ?,
			age_score = ?,
			version_score = ?,
			latency_score = ?,
			benchmarks_score = ?,
			contracts_score = ?,
			total_score = ?
		WHERE network = ?
		AND public_key = ?
	`)
	if err != nil {
		tx.Rollback()
		return utils.AddContext(err, "couldn't prepare score update statement")
	}
	defer updateScoreStmt.Close()

	for _, host := range updates.Hosts {
		api.mu.RLock()
		var oldHost *portalHost
		var exists bool
		if host.Network == "mainnet" {
			oldHost, exists = api.hosts[host.PublicKey]
		} else if host.Network == "zen" {
			oldHost, exists = api.hostsZen[host.PublicKey]
		}
		api.mu.RUnlock()
		var oldScans []portalScan
		var oldBenchmarks []hostdb.HostBenchmark
		if exists {
			interactions, ok := oldHost.Interactions[node]
			if ok {
				oldScans = interactions.ScanHistory
				oldBenchmarks = interactions.BenchmarkHistory
			}
		}
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
			0,
			0,
			0,
			0,
			0,
			0,
			0,
			0,
			0,
			0,
			0,
			settings.Bytes(),
			pt.Bytes(),
		)
		if err != nil {
			tx.Rollback()
			return utils.AddContext(err, "couldn't update host record")
		}
		sb := calculateScore(host, oldScans, oldBenchmarks)
		_, err = interactionsStmt.Exec(
			host.Network,
			node,
			host.PublicKey[:],
			int64(host.Uptime.Seconds()),
			int64(host.Downtime.Seconds()),
			host.LastSeen.Unix(),
			host.ActiveHosts,
			sb.PricesScore,
			sb.StorageScore,
			sb.CollateralScore,
			sb.InteractionsScore,
			sb.UptimeScore,
			sb.AgeScore,
			sb.VersionScore,
			sb.LatencyScore,
			sb.BenchmarksScore,
			sb.ContractsScore,
			sb.TotalScore,
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

		var newScans []portalScan
		for _, scan := range updates.Scans {
			if scan.Network == h.Network && scan.PublicKey == h.PublicKey {
				newScans = append(newScans, portalScan{
					Timestamp: scan.Timestamp,
					Latency:   scan.Latency,
					Success:   scan.Success,
					Error:     scan.Error,
				})
			}
		}
		slices.SortFunc(newScans, func(a, b portalScan) int { return b.Timestamp.Compare(a.Timestamp) })
		var newBenchmarks []hostdb.HostBenchmark
		for _, benchmark := range updates.Benchmarks {
			if benchmark.Network == h.Network && benchmark.PublicKey == h.PublicKey {
				newBenchmarks = append(newBenchmarks, benchmark.HostBenchmark)
			}
		}
		slices.SortFunc(newBenchmarks, func(a, b hostdb.HostBenchmark) int { return b.Timestamp.Compare(a.Timestamp) })

		if exists {
			var scans []portalScan
			var benchmarks []hostdb.HostBenchmark
			ints, ok := host.Interactions[node]
			if ok {
				scans = ints.ScanHistory
				benchmarks = ints.BenchmarkHistory
			}
			scans = append(newScans, scans...)
			slices.SortFunc(scans, func(a, b portalScan) int { return b.Timestamp.Compare(a.Timestamp) })
			if len(scans) > 24 {
				scans = scans[:24]
			}
			benchmarks = append(newBenchmarks, benchmarks...)
			slices.SortFunc(benchmarks, func(a, b hostdb.HostBenchmark) int { return b.Timestamp.Compare(a.Timestamp) })
			if len(benchmarks) > 24 {
				benchmarks = benchmarks[:24]
			}
			host.NetAddress = h.NetAddress
			host.Blocked = h.Blocked
			host.IPNets = h.IPNets
			host.LastIPChange = h.LastIPChange
			host.Settings = h.Settings
			host.PriceTable = h.PriceTable
			interactions := nodeInteractions{
				Uptime:           h.Uptime,
				Downtime:         h.Downtime,
				ScanHistory:      scans,
				BenchmarkHistory: benchmarks,
				LastSeen:         h.LastSeen,
				ActiveHosts:      h.ActiveHosts,
				Score:            calculateScore(h, scans, benchmarks),
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
				Uptime:           h.Uptime,
				Downtime:         h.Downtime,
				ScanHistory:      newScans,
				BenchmarkHistory: newBenchmarks,
				LastSeen:         h.LastSeen,
				ActiveHosts:      h.ActiveHosts,
				Score:            calculateScore(h, newScans, newBenchmarks),
				HostInteractions: hostdb.HostInteractions{
					HistoricSuccesses: h.Interactions.HistoricSuccesses,
					HistoricFailures:  h.Interactions.HistoricFailures,
					RecentSuccesses:   h.Interactions.RecentSuccesses,
					RecentFailures:    h.Interactions.RecentFailures,
					LastUpdate:        h.Interactions.LastUpdate,
				},
			}
			info, err := external.FetchIPInfo(h.NetAddress, api.token)
			if err != nil {
				api.log.Error("couldn't fetch host location", zap.String("host", h.NetAddress), zap.Error(err))
			} else {
				if (info != external.IPInfo{}) {
					err = api.saveLocation(h.PublicKey, h.Network, info)
					if err != nil {
						api.log.Error("couldn't update host location", zap.String("host", h.NetAddress), zap.Error(err))
					}
				} else {
					api.log.Debug("empty host location received", zap.String("host", h.NetAddress))
				}
			}
		}

		host.Score = calculateGlobalScore(host)
		_, err := updateScoreStmt.Exec(
			host.Score.PricesScore,
			host.Score.StorageScore,
			host.Score.CollateralScore,
			host.Score.InteractionsScore,
			host.Score.UptimeScore,
			host.Score.AgeScore,
			host.Score.VersionScore,
			host.Score.LatencyScore,
			host.Score.BenchmarksScore,
			host.Score.ContractsScore,
			host.Score.TotalScore,
			h.Network,
			h.PublicKey[:],
		)
		if err != nil {
			api.log.Error("couldn't update score", zap.String("network", h.Network), zap.Stringer("hsot", h.PublicKey), zap.Error(err))
		}
		if h.Network == "mainnet" {
			api.hosts[h.PublicKey] = host
		} else if h.Network == "zen" {
			api.hostsZen[h.PublicKey] = host
		}
	}

	var hosts, hostsZen []portalHost
	for _, host := range api.hosts {
		hosts = append(hosts, *host)
	}
	for _, host := range api.hostsZen {
		hostsZen = append(hostsZen, *host)
	}
	slices.SortStableFunc(hosts, func(a, b portalHost) int {
		if a.Score.TotalScore == b.Score.TotalScore {
			aIsOnline, bIsOnline := api.isOnline(a), api.isOnline(b)
			if aIsOnline && !bIsOnline {
				return -1
			}
			if !aIsOnline && bIsOnline {
				return 1
			}
			return a.ID - b.ID
		}
		if a.Score.TotalScore < b.Score.TotalScore {
			return 1
		} else {
			return -1
		}
	})
	slices.SortStableFunc(hostsZen, func(a, b portalHost) int {
		if a.Score.TotalScore == b.Score.TotalScore {
			aIsOnline, bIsOnline := api.isOnline(a), api.isOnline(b)
			if aIsOnline && !bIsOnline {
				return -1
			}
			if !aIsOnline && bIsOnline {
				return 1
			}
			return a.ID - b.ID
		}
		if a.Score.TotalScore < b.Score.TotalScore {
			return 1
		} else {
			return -1
		}
	})
	for i := range hosts {
		api.hosts[hosts[i].PublicKey].Rank = i + 1
	}
	for i := range hostsZen {
		api.hostsZen[hostsZen[i].PublicKey].Rank = i + 1
	}
	api.mu.Unlock()

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
	info, lastFetched, err := api.getLocation(pk, network, host.NetAddress)
	if err != nil {
		return portalHost{}, utils.AddContext(err, "couldn't get host location")
	} else if host.LastIPChange.After(lastFetched) {
		newInfo, err := external.FetchIPInfo(host.NetAddress, api.token)
		if err != nil {
			api.log.Error("couldn't fetch host location", zap.String("host", host.NetAddress), zap.Error(err))
		} else {
			if (newInfo != external.IPInfo{}) {
				info = newInfo
				err = api.saveLocation(pk, network, info)
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
func (api *portalAPI) getHosts(network string, all bool, offset, limit int, query, country string, sortBy sortType, asc bool) (hosts []portalHost, more bool, total int, err error) {
	if offset < 0 {
		offset = 0
	}

	if country != "" {
		rows, err := api.db.Query(`
			SELECT public_key
			FROM locations
			WHERE network = ?
			AND country = ?
		`, network, country)
		if err != nil {
			return nil, false, 0, utils.AddContext(err, "couldn't query public keys")
		}

		var keys []types.PublicKey
		for rows.Next() {
			pk := make([]byte, 32)
			if err := rows.Scan(&pk); err != nil {
				rows.Close()
				return nil, false, 0, utils.AddContext(err, "couldn't decode public key")
			}
			keys = append(keys, types.PublicKey(pk))
		}
		rows.Close()

		api.mu.RLock()
		for _, key := range keys {
			var host *portalHost
			if network == "mainnet" {
				host = api.hosts[key]
			} else if network == "zen" {
				host = api.hostsZen[key]
			}
			if (all || api.isOnline(*host)) && (query == "" || strings.Contains(host.NetAddress, query)) {
				hosts = append(hosts, *host)
			}
		}
		api.mu.RUnlock()
	} else {
		api.mu.RLock()
		if network == "mainnet" {
			for _, host := range api.hosts {
				if (all || api.isOnline(*host)) && (query == "" || strings.Contains(host.NetAddress, query)) {
					hosts = append(hosts, *host)
				}
			}
		} else if network == "zen" {
			for _, host := range api.hostsZen {
				if (all || api.isOnline(*host)) && (query == "" || strings.Contains(host.NetAddress, query)) {
					hosts = append(hosts, *host)
				}
			}
		}
		api.mu.RUnlock()
	}

	slices.SortStableFunc(hosts, func(a, b portalHost) int {
		switch sortBy {
		case sortByID:
			if asc {
				return a.ID - b.ID
			} else {
				return b.ID - a.ID
			}
		case sortByRank:
			if asc {
				return a.Rank - b.Rank
			} else {
				return b.Rank - a.Rank
			}
		case sortByTotalStorage:
			if a.Settings.TotalStorage == b.Settings.TotalStorage {
				return a.ID - b.ID
			}
			if a.Settings.TotalStorage > b.Settings.TotalStorage {
				if asc {
					return 1
				} else {
					return -1
				}
			} else {
				if asc {
					return -1
				} else {
					return 1
				}
			}
		case sortByUsedStorage:
			if a.Settings.TotalStorage-a.Settings.RemainingStorage == b.Settings.TotalStorage-b.Settings.RemainingStorage {
				return a.ID - b.ID
			}
			if a.Settings.TotalStorage-a.Settings.RemainingStorage > b.Settings.TotalStorage-b.Settings.RemainingStorage {
				if asc {
					return 1
				} else {
					return -1
				}
			} else {
				if asc {
					return -1
				} else {
					return 1
				}
			}
		case sortByStoragePrice:
			if a.Settings.StoragePrice.Cmp(b.Settings.StoragePrice) == 0 {
				return a.ID - b.ID
			}
			if a.Settings.StoragePrice.Cmp(b.Settings.StoragePrice) > 0 {
				if asc {
					return 1
				} else {
					return -1
				}
			} else {
				if asc {
					return -1
				} else {
					return 1
				}
			}
		case sortByUploadPrice:
			if a.Settings.UploadBandwidthPrice.Cmp(b.Settings.UploadBandwidthPrice) == 0 {
				return a.ID - b.ID
			}
			if a.Settings.UploadBandwidthPrice.Cmp(b.Settings.UploadBandwidthPrice) > 0 {
				if asc {
					return 1
				} else {
					return -1
				}
			} else {
				if asc {
					return -1
				} else {
					return 1
				}
			}
		case sortByDownloadPrice:
			if a.Settings.DownloadBandwidthPrice.Cmp(b.Settings.DownloadBandwidthPrice) == 0 {
				return a.ID - b.ID
			}
			if a.Settings.DownloadBandwidthPrice.Cmp(b.Settings.DownloadBandwidthPrice) > 0 {
				if asc {
					return 1
				} else {
					return -1
				}
			} else {
				if asc {
					return -1
				} else {
					return 1
				}
			}
		}
		return 0
	})

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

	for i := range hosts {
		info, lastFetched, err := api.getLocation(hosts[i].PublicKey, network, hosts[i].NetAddress)
		if err != nil {
			return nil, false, 0, utils.AddContext(err, "couldn't get host location")
		} else if hosts[i].LastIPChange.After(lastFetched) {
			newInfo, err := external.FetchIPInfo(hosts[i].NetAddress, api.token)
			if err != nil {
				api.log.Error("couldn't fetch host location", zap.String("host", hosts[i].NetAddress), zap.Error(err))
			} else {
				if (newInfo != external.IPInfo{}) {
					info = newInfo
					err = api.saveLocation(hosts[i].PublicKey, network, info)
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
func (api *portalAPI) getLocation(pk types.PublicKey, network, addr string) (info external.IPInfo, lastFetched time.Time, err error) {
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
		AND network = ?
	`, pk[:], network).Scan(
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
		if err := api.saveLocation(pk, network, info); err != nil {
			return external.IPInfo{}, time.Time{}, utils.AddContext(err, "couldn't save location")
		}
		return info, time.Now(), nil
	}
	lastFetched = time.Unix(lf, 0)
	return
}

// saveLocation saves the host's geolocation in the database.
func (api *portalAPI) saveLocation(pk types.PublicKey, network string, info external.IPInfo) error {
	_, err := api.db.Exec(`
		INSERT INTO locations (
			network,
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
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) AS new
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
		network,
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

	scanStmt, err := api.db.Prepare(`
		SELECT ran_at, success, latency, error, settings, price_table
		FROM scans
		WHERE network = ?
		AND node = ?
		AND public_key = ?
		AND ran_at > ?
		AND ran_at < ?
		AND (? OR success = TRUE)
		ORDER BY ran_at DESC
		LIMIT ?
	`)
	if err != nil {
		return nil, utils.AddContext(err, "couldn't prepare scan statement")
	}
	defer scanStmt.Close()

	for node := range api.clients {
		rows, err := scanStmt.Query(
			network,
			node,
			pk[:],
			from.Unix(),
			to.Unix(),
			!successful,
			num,
		)
		if err != nil {
			return nil, utils.AddContext(err, "couldn't query scan history")
		}

		for rows.Next() {
			var ra int64
			var success bool
			var latency float64
			var msg string
			var settings, pt []byte
			if err := rows.Scan(&ra, &success, &latency, &msg, &settings, &pt); err != nil {
				rows.Close()
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
					rows.Close()
					return nil, utils.AddContext(err, "couldn't decode host settings")
				}
			}
			if len(pt) > 0 {
				d := types.NewBufDecoder(pt)
				utils.DecodePriceTable(&scan.PriceTable, d)
				if err := d.Err(); err != nil {
					rows.Close()
					return nil, utils.AddContext(err, "couldn't decode host price table")
				}
			}
			scans = append(scans, scan)
		}
		rows.Close()
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

	benchmarkStmt, err := api.db.Prepare(`
		SELECT ran_at, success, upload_speed, download_speed, ttfb, error
		FROM benchmarks
		WHERE network = ?
		AND node = ?
		AND public_key = ?
		AND ran_at > ?
		AND ran_at < ?
		AND (? OR success = TRUE)
		ORDER BY ran_at DESC
		LIMIT ?
	`)
	if err != nil {
		return nil, utils.AddContext(err, "couldn't prepare benchmark statement")
	}
	defer benchmarkStmt.Close()

	for node := range api.clients {
		rows, err := benchmarkStmt.Query(
			network,
			node,
			pk[:],
			from.Unix(),
			to.Unix(),
			!successful,
			num,
		)
		if err != nil {
			return nil, utils.AddContext(err, "couldn't query benchmark history")
		}

		for rows.Next() {
			var ra int64
			var success bool
			var ul, dl, ttfb float64
			var msg string
			if err := rows.Scan(&ra, &success, &ul, &dl, &ttfb, &msg); err != nil {
				rows.Close()
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
		rows.Close()
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
			price_score,
			storage_score,
			collateral_score,
			interactions_score,
			uptime_score,
			age_score,
			version_score,
			latency_score,
			benchmarks_score,
			contracts_score,
			total_score,
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
		var ps, ss, cs, is, us, as, vs, ls, bs, cons, ts float64
		var settings, pt []byte
		if err := rows.Scan(
			&id,
			&network,
			&pk,
			&fs,
			&ks,
			&blocked,
			&netaddress,
			&ipNets,
			&lc,
			&ps,
			&ss,
			&cs,
			&is,
			&us,
			&as,
			&vs,
			&ls,
			&bs,
			&cons,
			&ts,
			&settings,
			&pt,
		); err != nil {
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
			Score: scoreBreakdown{
				PricesScore:       ps,
				StorageScore:      ss,
				CollateralScore:   cs,
				InteractionsScore: is,
				UptimeScore:       us,
				AgeScore:          as,
				VersionScore:      vs,
				LatencyScore:      ls,
				BenchmarksScore:   bs,
				ContractsScore:    cons,
				TotalScore:        ts,
			},
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

	var hosts, hostsZen []portalHost
	for _, host := range api.hosts {
		hosts = append(hosts, *host)
	}
	for _, host := range api.hostsZen {
		hostsZen = append(hostsZen, *host)
	}
	slices.SortStableFunc(hosts, func(a, b portalHost) int {
		if a.Score.TotalScore == b.Score.TotalScore {
			aIsOnline, bIsOnline := api.isOnline(a), api.isOnline(b)
			if aIsOnline && !bIsOnline {
				return -1
			}
			if !aIsOnline && bIsOnline {
				return 1
			}
			return a.ID - b.ID
		}
		if a.Score.TotalScore < b.Score.TotalScore {
			return 1
		} else {
			return -1
		}
	})
	slices.SortStableFunc(hostsZen, func(a, b portalHost) int {
		if a.Score.TotalScore == b.Score.TotalScore {
			aIsOnline, bIsOnline := api.isOnline(a), api.isOnline(b)
			if aIsOnline && !bIsOnline {
				return -1
			}
			if !aIsOnline && bIsOnline {
				return 1
			}
			return a.ID - b.ID
		}
		if a.Score.TotalScore < b.Score.TotalScore {
			return 1
		} else {
			return -1
		}
	})
	for i := range hosts {
		api.hosts[hosts[i].PublicKey].Rank = i + 1
	}
	for i := range hostsZen {
		api.hostsZen[hostsZen[i].PublicKey].Rank = i + 1
	}

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
			price_score,
			storage_score,
			collateral_score,
			interactions_score,
			uptime_score,
			age_score,
			version_score,
			latency_score,
			benchmarks_score,
			contracts_score,
			total_score,
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
			var ut, dt, lastSeen int64
			var ps, ss, cs, is, us, as, vs, ls, bs, cons, ts float64
			var hsi, hfi, rsi, rfi float64
			var ah int
			if err := rows.Scan(
				&node,
				&ut,
				&dt,
				&lastSeen,
				&ah,
				&ps,
				&ss,
				&cs,
				&is,
				&us,
				&as,
				&vs,
				&ls,
				&bs,
				&cons,
				&ts,
				&hsi,
				&hfi,
				&rsi,
				&rfi,
				&lu,
			); err != nil {
				rows.Close()
				return utils.AddContext(err, "couldn't decode interactions")
			}
			interactions := nodeInteractions{
				Uptime:      time.Duration(ut) * time.Second,
				Downtime:    time.Duration(dt) * time.Second,
				LastSeen:    time.Unix(lastSeen, 0),
				ActiveHosts: ah,
				Score: scoreBreakdown{
					PricesScore:       ps,
					StorageScore:      ss,
					CollateralScore:   cs,
					InteractionsScore: is,
					UptimeScore:       us,
					AgeScore:          as,
					VersionScore:      vs,
					LatencyScore:      ls,
					BenchmarksScore:   bs,
					ContractsScore:    cons,
					TotalScore:        ts,
				},
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

	return utils.ComposeErrors(api.loadScans(hosts, network), api.loadBenchmarks(hosts, network))
}

func (api *portalAPI) loadScans(hosts map[types.PublicKey]*portalHost, network string) error {
	scanStmt, err := api.db.Prepare(`
		SELECT
			ran_at,
			success,
			latency,
			error
		FROM scans
		WHERE network = ?
		AND node = ?
		AND public_key = ?
		ORDER BY ran_at DESC
		LIMIT 24
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
				if err := rows.Scan(&ra, &success, &latency, &msg); err != nil {
					rows.Close()
					return utils.AddContext(err, "couldn't decode scan history")
				}
				scan := portalScan{
					Timestamp: time.Unix(ra, 0),
					Success:   success,
					Latency:   time.Duration(latency) * time.Millisecond,
					Error:     msg,
				}
				int.ScanHistory = append(int.ScanHistory, scan)
			}
			rows.Close()
			host.Interactions[node] = int
		}
	}

	return nil
}

func (api *portalAPI) loadBenchmarks(hosts map[types.PublicKey]*portalHost, network string) error {
	benchmarkStmt, err := api.db.Prepare(`
		SELECT
			ran_at,
			success,
			upload_speed,
			download_speed,
			ttfb,
			error
		FROM benchmarks
		WHERE network = ?
		AND node = ?
		AND public_key = ?
		ORDER BY ran_at DESC
		LIMIT 24
	`)
	if err != nil {
		return utils.AddContext(err, "couldn't prepare benchmark statement")
	}
	defer benchmarkStmt.Close()

	for _, host := range hosts {
		for node, int := range host.Interactions {
			rows, err := benchmarkStmt.Query(network, node, host.PublicKey[:])
			if err != nil {
				return utils.AddContext(err, "couldn't query benchmarks")
			}

			for rows.Next() {
				var ra int64
				var success bool
				var ul, dl, ttfb float64
				var msg string
				if err := rows.Scan(&ra, &success, &ul, &dl, &ttfb, &msg); err != nil {
					rows.Close()
					return utils.AddContext(err, "couldn't decode benchmarks")
				}
				benchmark := hostdb.HostBenchmark{
					Timestamp:     time.Unix(ra, 0),
					Success:       success,
					UploadSpeed:   ul,
					DownloadSpeed: dl,
					TTFB:          time.Duration(ttfb) * time.Millisecond,
					Error:         msg,
				}
				int.BenchmarkHistory = append(int.BenchmarkHistory, benchmark)
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

// getHostsOnMap returns the online hosts that are located within the
// provided geo coordinates.
func (api *portalAPI) getHostsOnMap(network string, northWest, southEast string, query string) (hosts []portalHost, err error) {
	coords0 := strings.Split(northWest, ",")
	if len(coords0) != 2 {
		return nil, fmt.Errorf("wrong coordinates provided: %s", northWest)
	}
	x0, err := strconv.ParseFloat(coords0[0], 64)
	if err != nil {
		return nil, utils.AddContext(err, fmt.Sprintf("wrong coordinates provided: %s", northWest))
	}
	y0, err := strconv.ParseFloat(coords0[1], 64)
	if err != nil {
		return nil, utils.AddContext(err, fmt.Sprintf("wrong coordinates provided: %s", northWest))
	}

	coords1 := strings.Split(southEast, ",")
	if len(coords1) != 2 {
		return nil, fmt.Errorf("wrong coordinates provided: %s", southEast)
	}
	x1, err := strconv.ParseFloat(coords1[0], 64)
	if err != nil {
		return nil, utils.AddContext(err, fmt.Sprintf("wrong coordinates provided: %s", southEast))
	}
	y1, err := strconv.ParseFloat(coords1[1], 64)
	if err != nil {
		return nil, utils.AddContext(err, fmt.Sprintf("wrong coordinates provided: %s", southEast))
	}

	if x1 < x0 {
		x0, x1 = x1, x0
	}
	if y1 < y0 {
		y0, y1 = y1, y0
	}

	api.mu.RLock()
	var totalHosts []portalHost
	if network == "mainnet" {
		for _, host := range api.hosts {
			if api.isOnline(*host) && (query == "" || strings.Contains(host.NetAddress, query)) {
				totalHosts = append(totalHosts, *host)
			}
		}
	} else if network == "zen" {
		for _, host := range api.hostsZen {
			if api.isOnline(*host) && (query == "" || strings.Contains(host.NetAddress, query)) {
				totalHosts = append(totalHosts, *host)
			}
		}
	}
	api.mu.RUnlock()

	searchStmt, err := api.db.Prepare(`
		SELECT
			ip,
			host_name,
			city,
			region,
			country,
			loc,
			isp,
			zip,
			time_zone
		FROM locations
		WHERE public_key = ?
	`)
	if err != nil {
		return nil, utils.AddContext(err, "couldn't prepare search statement")
	}
	defer searchStmt.Close()

	for _, host := range totalHosts {
		var ip, name, city, region, country, loc, isp, zip, tz string
		if err := searchStmt.QueryRow(host.PublicKey[:]).Scan(
			&ip,
			&name,
			&city,
			&region,
			&country,
			&loc,
			&isp,
			&zip,
			&tz,
		); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				info, err := external.FetchIPInfo(host.NetAddress, api.token)
				if err != nil {
					continue
				}
				if err := api.saveLocation(host.PublicKey, network, info); err != nil {
					api.log.Error("couldn't save location", zap.Stringer("host", host.PublicKey), zap.Error(err))
					continue
				}
				loc = info.Location
			}
		}

		coords := strings.Split(loc, ",")
		if len(coords) != 2 {
			continue
		}
		x, err := strconv.ParseFloat(coords[0], 64)
		if err != nil {
			continue
		}
		y, err := strconv.ParseFloat(coords[1], 64)
		if err != nil {
			continue
		}

		if x > x0 && x < x1 && y > y0 && y < y1 {
			host.IPInfo = external.IPInfo{
				IP:       ip,
				HostName: name,
				City:     city,
				Region:   region,
				Country:  country,
				Location: loc,
				ISP:      isp,
				ZIP:      zip,
				TimeZone: tz,
			}
			hosts = append(hosts, host)
		}
	}

	return
}

// calculateAverages calculates the averages for the given network.
func (api *portalAPI) calculateAverages() {
	api.mu.RLock()
	var hosts, hostsZen []portalHost
	for _, host := range api.hosts {
		if api.isOnline(*host) {
			hosts = append(hosts, *host)
		}
	}
	for _, host := range api.hostsZen {
		if api.isOnline(*host) {
			hostsZen = append(hostsZen, *host)
		}
	}
	api.mu.RUnlock()

	slices.SortStableFunc(hosts, func(a, b portalHost) int {
		return a.Rank - b.Rank
	})
	slices.SortStableFunc(hostsZen, func(a, b portalHost) int {
		return a.Rank - b.Rank
	})

	api.averages = calculateTiers(hosts)
	api.averagesZen = calculateTiers(hostsZen)
}

func calculateTiers(sortedHosts []portalHost) networkAverages {
	calculateTier := func(hostSlice []portalHost) averagePrices {
		var tier averagePrices
		var count int
		for _, host := range hostSlice {
			tier.StoragePrice = tier.StoragePrice.Add(host.Settings.StoragePrice)
			tier.Collateral = tier.Collateral.Add(host.Settings.Collateral)
			tier.UploadPrice = tier.UploadPrice.Add(host.Settings.UploadBandwidthPrice)
			tier.DownloadPrice = tier.DownloadPrice.Add(host.Settings.DownloadBandwidthPrice)
			tier.ContractDuration += host.Settings.MaxDuration
			count++
		}
		if count > 0 {
			tier.StoragePrice = tier.StoragePrice.Div64(uint64(count))
			tier.Collateral = tier.Collateral.Div64(uint64(count))
			tier.UploadPrice = tier.UploadPrice.Div64(uint64(count))
			tier.DownloadPrice = tier.DownloadPrice.Div64(uint64(count))
			tier.ContractDuration /= uint64(count)
			tier.OK = true
		}
		return tier
	}

	var tier1Hosts, tier2Hosts, tier3Hosts []portalHost
	if len(sortedHosts) >= 10 {
		tier1Hosts = sortedHosts[:10]
	} else {
		tier1Hosts = sortedHosts
	}
	if len(sortedHosts) >= 100 {
		tier2Hosts = sortedHosts[10:100]
	} else {
		if len(sortedHosts) > 10 {
			tier2Hosts = sortedHosts[10:]
		}
	}
	if len(sortedHosts) > 100 {
		tier3Hosts = sortedHosts[100:]
	}

	return networkAverages{
		Tier1: calculateTier(tier1Hosts),
		Tier2: calculateTier(tier2Hosts),
		Tier3: calculateTier(tier3Hosts),
	}
}

// updateAverages makes periodical calculation of the network averages.
func (api *portalAPI) updateAverages() {
	api.calculateAverages()

	for {
		select {
		case <-api.stopChan:
			return
		case <-time.After(10 * time.Minute):
		}
		api.calculateAverages()
	}
}

// getCountries returns the list of countries the hosts in the given
// network reside in.
func (api *portalAPI) getCountries(network string) (countries []string, err error) {
	rows, err := api.db.Query(`
		SELECT DISTINCT country
		FROM locations
		WHERE country <> ''
		AND network = ?
		ORDER BY country ASC
	`, network)
	if err != nil {
		return nil, utils.AddContext(err, "couldn't query countries")
	}
	defer rows.Close()

	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, utils.AddContext(err, "couldn't decode country")
		}
		countries = append(countries, c)
	}

	return
}
