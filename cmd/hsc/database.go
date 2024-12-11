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
	"github.com/mike76-dev/hostscore/internal/build"
	"github.com/mike76-dev/hostscore/internal/utils"
	rhpv2 "go.sia.tech/core/rhp/v2"
	rhpv3 "go.sia.tech/core/rhp/v3"
	"go.sia.tech/core/types"
	"go.uber.org/zap"
)

// scanPruneThreshold determines how old a scan record needs to be to get pruned.
const scanPruneThreshold = 14 * 24 * time.Hour

// scanPruneInterval determines how often old scan records get pruned.
const scanPruneInterval = time.Hour

// errHostNotFound is returned when the specified host couldn't be found.
var errHostNotFound = errors.New("host not found")

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
			error
		)
		VALUES (?, ?, ?, ?, ?, ?, ?)
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
	}

	for _, scan := range updates.Scans {
		_, err := scanStmt.Exec(
			scan.Network,
			node,
			scan.PublicKey[:],
			scan.Timestamp.Unix(),
			scan.Success,
			scan.Latency.Milliseconds(),
			scan.Error,
		)
		if err != nil {
			api.log.Warn("couldn't insert scan record", zap.Stringer("host", scan.PublicKey), zap.String("network", scan.Network), zap.String("node", scan.Node), zap.Error(err))
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
			api.log.Warn("couldn't insert benchmark record", zap.Stringer("host", benchmark.PublicKey), zap.String("network", benchmark.Network), zap.String("node", benchmark.Node), zap.Error(err))
		}
	}

	api.mu.Lock()
	for _, h := range updates.Hosts {
		var host *portalHost
		var exists bool
		hosts, ok := api.hosts[h.Network]
		if ok {
			host, exists = hosts[h.PublicKey]
		}
		var count int
		if err := priceChangeCountStmt.QueryRow(h.PublicKey[:]).Scan(&count); err != nil {
			tx.Rollback()
			api.mu.Unlock()
			return utils.AddContext(err, "couldn't count price changes")
		}
		if exists && (count == 0 || pricesChanged(h.Settings, host.Settings)) {
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
				api.log.Warn("couldn't update price change", zap.Stringer("host", h.PublicKey), zap.String("network", h.Network), zap.String("node", node), zap.Error(err))
			}
		}

		if exists {
			host.NetAddress = h.NetAddress
			host.Blocked = h.Blocked
			host.IPNets = h.IPNets
			host.LastIPChange = h.LastIPChange
			host.Settings = h.Settings
			host.PriceTable = h.PriceTable
			interactions := host.Interactions[node]
			interactions.Uptime = h.Uptime
			interactions.Downtime = h.Downtime
			interactions.LastSeen = h.LastSeen
			interactions.ActiveHosts = h.ActiveHosts
			interactions.HostInteractions = hostdb.HostInteractions{
				HistoricSuccesses: h.Interactions.HistoricSuccesses,
				HistoricFailures:  h.Interactions.HistoricFailures,
				RecentSuccesses:   h.Interactions.RecentSuccesses,
				RecentFailures:    h.Interactions.RecentFailures,
				LastUpdate:        h.Interactions.LastUpdate,
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
			tx.Rollback()
			api.mu.Unlock()
			return utils.AddContext(err, "couldn't update score")
		}
		api.hosts[h.Network][h.PublicKey] = host
	}

	toUpdate := make(map[string]map[types.PublicKey]struct{})
	toUpdate["mainnet"] = make(map[types.PublicKey]struct{})
	toUpdate["zen"] = make(map[types.PublicKey]struct{})

	newScans := make(map[string]map[types.PublicKey][]portalScan)
	newScans["mainnet"] = make(map[types.PublicKey][]portalScan)
	newScans["zen"] = make(map[types.PublicKey][]portalScan)
	for _, scan := range updates.Scans {
		toUpdate[scan.Network][scan.PublicKey] = struct{}{}
		newScans[scan.Network][scan.PublicKey] = append(newScans[scan.Network][scan.PublicKey], portalScan{
			Timestamp: scan.Timestamp,
			Latency:   scan.Latency,
			Success:   scan.Success,
			Error:     scan.Error,
		})
	}

	newBenchmarks := make(map[string]map[types.PublicKey][]hostdb.HostBenchmark)
	newBenchmarks["mainnet"] = make(map[types.PublicKey][]hostdb.HostBenchmark)
	newBenchmarks["zen"] = make(map[types.PublicKey][]hostdb.HostBenchmark)
	for _, benchmark := range updates.Benchmarks {
		toUpdate[benchmark.Network][benchmark.PublicKey] = struct{}{}
		newBenchmarks[benchmark.Network][benchmark.PublicKey] = append(newBenchmarks[benchmark.Network][benchmark.PublicKey], benchmark.HostBenchmark)
	}

	for network, keys := range toUpdate {
		for pk := range keys {
			hosts := api.hosts[network]
			host, exists := hosts[pk]
			if !exists {
				api.log.Warn("orphaned scan or benchmark found", zap.String("network", network), zap.Stringer("host", pk))
				continue
			}

			interactions := host.Interactions[node]
			interactions.ScanHistory = append(interactions.ScanHistory, newScans[network][pk]...)
			slices.SortFunc(interactions.ScanHistory, func(a, b portalScan) int { return b.Timestamp.Compare(a.Timestamp) })
			if len(interactions.ScanHistory) > 48 {
				interactions.ScanHistory = interactions.ScanHistory[:48]
			}
			interactions.BenchmarkHistory = append(interactions.BenchmarkHistory, newBenchmarks[network][pk]...)
			slices.SortFunc(interactions.BenchmarkHistory, func(a, b hostdb.HostBenchmark) int { return b.Timestamp.Compare(a.Timestamp) })
			if len(interactions.BenchmarkHistory) > 12 {
				interactions.BenchmarkHistory = interactions.BenchmarkHistory[:12]
			}
			interactions.Score = calculateScore(*host, node, interactions.ScanHistory, interactions.BenchmarkHistory)
			host.Interactions[node] = interactions

			_, err = interactionsStmt.Exec(
				network,
				node,
				pk[:],
				int64(interactions.Uptime.Seconds()),
				int64(interactions.Downtime.Seconds()),
				interactions.LastSeen.Unix(),
				interactions.ActiveHosts,
				interactions.Score.PricesScore,
				interactions.Score.StorageScore,
				interactions.Score.CollateralScore,
				interactions.Score.InteractionsScore,
				interactions.Score.UptimeScore,
				interactions.Score.AgeScore,
				interactions.Score.VersionScore,
				interactions.Score.LatencyScore,
				interactions.Score.BenchmarksScore,
				interactions.Score.ContractsScore,
				interactions.Score.TotalScore,
				interactions.HistoricSuccesses,
				interactions.HistoricFailures,
				interactions.RecentSuccesses,
				interactions.RecentFailures,
				interactions.LastUpdate,
			)
			if err != nil {
				api.log.Warn("couldn't update host interactions", zap.Stringer("host", host.PublicKey), zap.String("network", network), zap.String("node", node), zap.Error(err))
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
				network,
				pk[:],
			)
			if err != nil {
				tx.Rollback()
				api.mu.Unlock()
				return utils.AddContext(err, "couldn't update score")
			}
		}
	}

	var hosts, hostsZen []portalHost
	for _, host := range api.hosts["mainnet"] {
		hosts = append(hosts, *host)
	}
	for _, host := range api.hosts["zen"] {
		hostsZen = append(hostsZen, *host)
	}
	slices.SortStableFunc(hosts, func(a, b portalHost) int {
		if a.Score.TotalScore == b.Score.TotalScore {
			aIsOnline, bIsOnline := isOnline(a), isOnline(b)
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
			aIsOnline, bIsOnline := isOnline(a), isOnline(b)
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
		api.hosts["mainnet"][hosts[i].PublicKey].Rank = i + 1
	}
	for i := range hostsZen {
		api.hosts["zen"][hostsZen[i].PublicKey].Rank = i + 1
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
func isOnline(host portalHost) bool {
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
	api.mu.RLock()
	hosts := api.hosts[network]
	h, exists := hosts[pk]
	api.mu.RUnlock()
	if !exists {
		return portalHost{}, errHostNotFound
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
		allHosts := api.hosts[network]
		for _, key := range keys {
			host := allHosts[key]
			if (all || isOnline(*host)) && (query == "" || strings.Contains(host.NetAddress, query)) {
				hosts = append(hosts, *host)
			}
		}
		api.mu.RUnlock()
	} else {
		api.mu.RLock()
		allHosts := api.hosts[network]
		for _, host := range allHosts {
			if (all || isOnline(*host)) && (query == "" || strings.Contains(host.NetAddress, query)) {
				hosts = append(hosts, *host)
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
func (api *portalAPI) getScans(network, node string, pk types.PublicKey, all bool, from, to time.Time, limit int64) (scans []scanHistory, err error) {
	f := int64(0)
	t := time.Now().Unix()
	if from.Unix() != (time.Time{}).Unix() {
		f = from.Unix()
	}
	if to.Unix() != (time.Time{}).Unix() {
		t = to.Unix()
	}
	if limit < 0 {
		limit = math.MaxInt64
	}

	api.mu.RLock()
	hosts := api.hosts[network]
	_, ok := hosts[pk]
	api.mu.RUnlock()

	if !ok {
		return nil, errHostNotFound
	}

	rows, err := api.db.Query(`
		SELECT node, ran_at, success, latency, error
		FROM scans
		WHERE network = ?
		AND (? OR node = ?)
		AND public_key = ?
		AND ran_at >= ?
		AND ran_at <= ?
		AND (? OR success = TRUE)
		ORDER BY ran_at DESC
		LIMIT ?
	`,
		network,
		node == "global",
		node,
		pk[:],
		f,
		t,
		all,
		limit,
	)
	if err != nil {
		return nil, utils.AddContext(err, "couldn't query scan history")
	}
	defer rows.Close()

	for rows.Next() {
		var ra int64
		var success bool
		var latency float64
		var n, msg string
		if err := rows.Scan(&n, &ra, &success, &latency, &msg); err != nil {
			return nil, utils.AddContext(err, "couldn't decode scan history")
		}
		scan := scanHistory{
			Timestamp: time.Unix(ra, 0),
			Success:   success,
			Latency:   time.Duration(latency) * time.Millisecond,
			Error:     msg,
			PublicKey: pk,
			Network:   network,
			Node:      n,
		}
		scans = append(scans, scan)
	}

	return
}

// getBenchmarks returns the benchmark history according to the criteria provided.
func (api *portalAPI) getBenchmarks(network, node string, pk types.PublicKey, all bool, from, to time.Time, limit int64) (benchmarks []hostdb.BenchmarkHistory, err error) {
	f := int64(0)
	t := time.Now().Unix()
	if from.Unix() != (time.Time{}).Unix() {
		f = from.Unix()
	}
	if to.Unix() != (time.Time{}).Unix() {
		t = to.Unix()
	}
	if limit < 0 {
		limit = math.MaxInt64
	}

	api.mu.RLock()
	hosts := api.hosts[network]
	_, ok := hosts[pk]
	api.mu.RUnlock()

	if !ok {
		return nil, errHostNotFound
	}

	rows, err := api.db.Query(`
		SELECT node, ran_at, success, upload_speed, download_speed, ttfb, error
		FROM benchmarks
		WHERE network = ?
		AND (? OR node = ?)
		AND public_key = ?
		AND ran_at >= ?
		AND ran_at <= ?
		AND (? OR success = TRUE)
		ORDER BY ran_at DESC
		LIMIT ?
	`,
		network,
		node == "global",
		node,
		pk[:],
		f,
		t,
		all,
		limit,
	)
	if err != nil {
		return nil, utils.AddContext(err, "couldn't query benchmark history")
	}
	defer rows.Close()

	for rows.Next() {
		var ra int64
		var success bool
		var ul, dl, ttfb float64
		var n, msg string
		if err := rows.Scan(&n, &ra, &success, &ul, &dl, &ttfb, &msg); err != nil {
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
			Node:      n,
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

		api.hosts[network][host.PublicKey] = host
	}
	rows.Close()

	var hosts, hostsZen []portalHost
	for _, host := range api.hosts["mainnet"] {
		hosts = append(hosts, *host)
	}
	for _, host := range api.hosts["zen"] {
		hostsZen = append(hostsZen, *host)
	}
	slices.SortStableFunc(hosts, func(a, b portalHost) int {
		if a.Score.TotalScore == b.Score.TotalScore {
			aIsOnline, bIsOnline := isOnline(a), isOnline(b)
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
			aIsOnline, bIsOnline := isOnline(a), isOnline(b)
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
		api.hosts["mainnet"][hosts[i].PublicKey].Rank = i + 1
	}
	for i := range hostsZen {
		api.hosts["zen"][hostsZen[i].PublicKey].Rank = i + 1
	}

	if err := api.loadInteractions("mainnet"); err != nil {
		return utils.AddContext(err, "couldn't load mainnet interactions")
	}

	if err := api.loadInteractions("zen"); err != nil {
		return utils.AddContext(err, "couldn't load zen interactions")
	}

	return nil
}

func (api *portalAPI) loadInteractions(network string) error {
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

	hosts := api.hosts[network]
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

	return utils.ComposeErrors(api.loadScans(network), api.loadBenchmarks(network))
}

func (api *portalAPI) loadScans(network string) error {
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
		LIMIT 48
	`)
	if err != nil {
		return utils.AddContext(err, "couldn't prepare scan statement")
	}
	defer scanStmt.Close()

	hosts := api.hosts[network]
	for _, host := range hosts {
		for node, interactions := range host.Interactions {
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
				interactions.ScanHistory = append(interactions.ScanHistory, scan)
			}
			rows.Close()
			host.Interactions[node] = interactions
		}
	}

	return nil
}

func (api *portalAPI) loadBenchmarks(network string) error {
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
		LIMIT 12
	`)
	if err != nil {
		return utils.AddContext(err, "couldn't prepare benchmark statement")
	}
	defer benchmarkStmt.Close()

	hosts := api.hosts[network]
	for _, host := range hosts {
		for node, interactions := range host.Interactions {
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
				interactions.BenchmarkHistory = append(interactions.BenchmarkHistory, benchmark)
			}
			rows.Close()
			host.Interactions[node] = interactions
		}
	}

	return nil
}

// getPriceChanges retrieves the historic price changes of the given host.
func (api *portalAPI) getPriceChanges(network string, pk types.PublicKey, from, to time.Time, limit int64) (pcs []priceChange, err error) {
	f := int64(0)
	t := time.Now().Unix()
	if from.Unix() != (time.Time{}).Unix() {
		f = from.Unix()
	}
	if to.Unix() != (time.Time{}).Unix() {
		t = to.Unix()
	}
	if limit < 0 {
		limit = math.MaxInt64
	}

	api.mu.RLock()
	hosts := api.hosts[network]
	_, ok := hosts[pk]
	api.mu.RUnlock()

	if !ok {
		return nil, errHostNotFound
	}

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
		AND changed_at <= ?
		ORDER BY changed_at DESC
		LIMIT ?
	`, network, pk[:], f, t, limit)
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

	// Sort in ascending order.
	slices.Reverse(pcs)

	return
}

// calculateAverages calculates the averages for the given network.
func (api *portalAPI) calculateAverages() {
	var hosts, hostsZen []portalHost
	api.mu.RLock()
	for _, host := range api.hosts["mainnet"] {
		if isOnline(*host) {
			hosts = append(hosts, *host)
		}
	}
	for _, host := range api.hosts["zen"] {
		if isOnline(*host) {
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

	api.mu.Lock()
	api.averages["mainnet"] = calculateTiers(hosts)
	api.averages["zen"] = calculateTiers(hostsZen)
	api.mu.Unlock()
}

func calculateTiers(sortedHosts []portalHost) map[string]networkAverages {
	calculateTier := func(hostSlice []portalHost) networkAverages {
		var tier networkAverages
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
			tier.Available = true
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

	result := make(map[string]networkAverages)
	result["tier1"] = calculateTier(tier1Hosts)
	result["tier2"] = calculateTier(tier2Hosts)
	result["tier3"] = calculateTier(tier3Hosts)

	return result
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
func (api *portalAPI) getCountries(network string, all bool) (countries []string, _ error) {
	if all {
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

		for rows.Next() {
			var c string
			if err := rows.Scan(&c); err != nil {
				rows.Close()
				return nil, utils.AddContext(err, "couldn't decode country")
			}
			countries = append(countries, c)
		}

		rows.Close()
		return countries, nil
	}

	stmt, err := api.db.Prepare(`
		SELECT country
		FROM locations
		WHERE network = ?
		AND public_key = ?
	`)
	if err != nil {
		return nil, utils.AddContext(err, "couldn't prepare statement")
	}
	defer stmt.Close()

	api.mu.RLock()
	allCountries := make(map[string]struct{})
	hosts := api.hosts[network]
	for pk, host := range hosts {
		if !isOnline(*host) {
			continue
		}
		var c string
		if err := stmt.QueryRow(network, pk[:]).Scan(&c); err != nil {
			continue
		}
		allCountries[c] = struct{}{}
	}
	api.mu.RUnlock()

	for c := range allCountries {
		countries = append(countries, c)
	}

	return countries, nil
}

// getHostKeys returns a list of host public keys according to certain criteria.
func (api *portalAPI) getHostKeys(
	network string,
	node string,
	maxStoragePrice types.Currency,
	maxUploadPrice types.Currency,
	maxDownloadPrice types.Currency,
	maxContractPrice types.Currency,
	minContractDuration uint64,
	maxBaseRPCPrice types.Currency,
	maxSectorAccessPrice types.Currency,
	minAvailableStorage uint64,
	minVersion string,
	maxLatency time.Duration,
	minUploadSpeed float64,
	minDownloadSpeed float64,
	countries []string,
	limit int,
) (keys []types.PublicKey, err error) {
	stmt, err := api.db.Prepare(`
		SELECT country
		FROM locations
		WHERE network = ?
		AND public_key = ?
	`)
	if err != nil {
		return nil, utils.AddContext(err, "couldn't prepare statement")
	}
	defer stmt.Close()

	allCountries := make(map[string]struct{})
	for _, c := range countries {
		allCountries[strings.ToLower(c)] = struct{}{}
	}

	api.mu.RLock()
	hosts := api.hosts[network]
	var selectedHosts []portalHost

outer:
	for _, host := range hosts {
		if !isOnline(*host) {
			continue
		}

		if !host.Settings.AcceptingContracts {
			continue
		}

		if host.Settings.StoragePrice.Cmp(maxStoragePrice) > 0 {
			continue
		}

		if host.Settings.UploadBandwidthPrice.Cmp(maxUploadPrice) > 0 {
			continue
		}

		if host.Settings.DownloadBandwidthPrice.Cmp(maxDownloadPrice) > 0 {
			continue
		}

		if host.Settings.ContractPrice.Cmp(maxContractPrice) > 0 {
			continue
		}

		if host.Settings.MaxDuration < minContractDuration {
			continue
		}

		if host.Settings.BaseRPCPrice.Cmp(maxBaseRPCPrice) > 0 {
			continue
		}

		if host.Settings.SectorAccessPrice.Cmp(maxSectorAccessPrice) > 0 {
			continue
		}

		if host.Settings.RemainingStorage < minAvailableStorage {
			continue
		}

		if minVersion != "" && build.VersionCmp(host.Settings.Version, minVersion) < 0 {
			continue
		}

		if maxLatency > 0 || minUploadSpeed > 0 || minDownloadSpeed > 0 {
			if node == "global" {
				for _, interactions := range host.Interactions {
					lat, ul, dl := getSpeeds(interactions)
					if maxLatency > 0 && lat > maxLatency {
						continue outer
					}
					if minUploadSpeed > 0 && ul < minUploadSpeed {
						continue outer
					}
					if minDownloadSpeed > 0 && dl < minDownloadSpeed {
						continue outer
					}
				}
			} else {
				interactions := host.Interactions[node]
				lat, ul, dl := getSpeeds(interactions)
				if maxLatency > 0 && lat > maxLatency {
					continue
				}
				if minUploadSpeed > 0 && ul < minUploadSpeed {
					continue
				}
				if minDownloadSpeed > 0 && dl < minDownloadSpeed {
					continue
				}
			}
		}

		if len(countries) > 0 {
			var c string
			if err := stmt.QueryRow(network, host.PublicKey[:]).Scan(&c); err != nil {
				api.mu.RUnlock()
				return nil, utils.AddContext(err, "couldn't retrieve country")
			}
			if _, ok := allCountries[strings.ToLower(c)]; !ok {
				continue
			}
		}

		selectedHosts = append(selectedHosts, *host)
	}
	api.mu.RUnlock()

	slices.SortStableFunc(selectedHosts, func(a, b portalHost) int { return a.Rank - b.Rank })

	if limit < 0 || limit > len(selectedHosts) {
		limit = len(selectedHosts)
	}

	for _, sh := range selectedHosts[:limit] {
		keys = append(keys, sh.PublicKey)
	}

	return
}

func getSpeeds(interactions nodeInteractions) (lat time.Duration, ul, dl float64) {
	var scans, benchmarks int
	for _, scan := range interactions.ScanHistory {
		if scan.Success {
			lat += scan.Latency
			scans++
		}
	}

	for _, benchmark := range interactions.BenchmarkHistory {
		if benchmark.Success {
			ul += benchmark.UploadSpeed
			dl += benchmark.DownloadSpeed
			benchmarks++
		}
	}

	if scans > 0 {
		lat /= time.Duration(scans)
	}

	if benchmarks > 0 {
		ul /= float64(benchmarks)
		dl /= float64(benchmarks)
	}

	return
}

func (api *portalAPI) pruneOldScans() {
	for {
		select {
		case <-api.stopChan:
			return
		case <-time.After(scanPruneInterval):
		}

		_, err := api.db.Exec(`
			DELETE FROM scans
			WHERE ran_at < ?
			LIMIT 100000
		`, time.Now().Unix()-int64(scanPruneThreshold.Seconds()))
		if err != nil {
			api.log.Error("unable to prune old scans", zap.Error(err))
		}
	}
}
