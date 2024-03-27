package hostdb

import (
	"bytes"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/mike76-dev/hostscore/internal/utils"
	rhpv2 "go.sia.tech/core/rhp/v2"
	rhpv3 "go.sia.tech/core/rhp/v3"
	"go.sia.tech/core/types"
	"go.sia.tech/coreutils/chain"
	"go.uber.org/zap"
)

type hostDBStore struct {
	db      *sql.DB
	tx      *sql.Tx
	log     *zap.Logger
	network string
	hdb     *HostDB

	hosts        map[types.PublicKey]*HostDBEntry
	blockedHosts map[types.PublicKey]struct{}

	activeHostsCache map[types.PublicKey][]string

	mu sync.Mutex

	tip           types.ChainIndex
	lastCommitted time.Time

	lastUpdate HostUpdates
}

func newHostDBStore(db *sql.DB, logger *zap.Logger, network string, domains *blockedDomains) (*hostDBStore, types.ChainIndex, error) {
	s := &hostDBStore{
		db:               db,
		log:              logger,
		network:          network,
		hosts:            make(map[types.PublicKey]*HostDBEntry),
		blockedHosts:     make(map[types.PublicKey]struct{}),
		activeHostsCache: make(map[types.PublicKey][]string),
	}
	err := s.load(domains)
	if err != nil {
		s.log.Error("couldn't load hosts", zap.String("network", s.network), zap.Error(err))
		return nil, types.ChainIndex{}, err
	}
	return s, s.tip, nil
}

// update updates the host entry in the database.
// NOTE: a lock must be acquired before calling update.
func (s *hostDBStore) update(host *HostDBEntry) error {
	if host.Network != s.network {
		panic("networks don't match")
	}
	if s.tx == nil {
		return errors.New("there is no transaction")
	}
	if host.Blocked || s.hdb.blockedDomains.isBlocked(host.NetAddress) {
		host.Blocked = true
		s.blockedHosts[host.PublicKey] = struct{}{}
	} else {
		delete(s.blockedHosts, host.PublicKey)
	}
	s.hosts[host.PublicKey] = host
	var rev, settings, pt bytes.Buffer
	e := types.NewEncoder(&rev)
	if (host.Revision.ParentID != types.FileContractID{}) {
		host.Revision.EncodeTo(e)
		e.Flush()
	}
	e = types.NewEncoder(&settings)
	if (host.Settings != rhpv2.HostSettings{}) {
		utils.EncodeSettings(&host.Settings, e)
		e.Flush()
	}
	e = types.NewEncoder(&pt)
	if (host.PriceTable != rhpv3.HostPriceTable{}) {
		utils.EncodePriceTable(&host.PriceTable, e)
		e.Flush()
	}
	_, err := s.tx.Exec(`
		INSERT INTO hdb_hosts_`+s.network+` (
			id,
			public_key,
			first_seen,
			known_since,
			blocked,
			net_address,
			uptime,
			downtime,
			last_seen,
			ip_nets,
			last_ip_change,
			historic_successful_interactions,
			historic_failed_interactions,
			recent_successful_interactions,
			recent_failed_interactions,
			last_update,
			revision,
			settings,
			price_table,
			modified,
			fetched
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) AS new
		ON DUPLICATE KEY UPDATE
			first_seen = new.first_seen,
			known_since = new.known_since,
			blocked = new.blocked,
			net_address = new.net_address,
			uptime = new.uptime,
			downtime = new.downtime,
			last_seen = new.last_seen,
			ip_nets = new.ip_nets,
			last_ip_change = new.last_ip_change,
			historic_successful_interactions = new.historic_successful_interactions,
			historic_failed_interactions = new.historic_failed_interactions,
			recent_successful_interactions = new.recent_successful_interactions,
			recent_failed_interactions = new.recent_failed_interactions,
			last_update = new.last_update,
			revision = new.revision,
			settings = new.settings,
			price_table = new.price_table,
			modified = new.modified
	`,
		host.ID,
		host.PublicKey[:],
		host.FirstSeen.Unix(),
		host.KnownSince,
		host.Blocked,
		host.NetAddress,
		int64(host.Uptime.Seconds()),
		int64(host.Downtime.Seconds()),
		host.LastSeen.Unix(),
		strings.Join(host.IPNets, ";"),
		host.LastIPChange.Unix(),
		host.Interactions.HistoricSuccesses,
		host.Interactions.HistoricFailures,
		host.Interactions.RecentSuccesses,
		host.Interactions.RecentFailures,
		host.Interactions.LastUpdate,
		rev.Bytes(),
		settings.Bytes(),
		pt.Bytes(),
		time.Now().Unix(),
		0,
	)
	if err != nil {
		return err
	}

	if err := s.tx.Commit(); err != nil {
		return err
	}

	s.tx, err = s.db.Begin()
	return err
}

// updateScanHistory adds a new scan to the host's scan history.
func (s *hostDBStore) updateScanHistory(host *HostDBEntry, scan HostScan) error {
	if host.Network != s.network {
		panic("networks don't match")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tx == nil {
		return errors.New("there is no transaction")
	}

	if scan.Success {
		host.LastSeen = scan.Timestamp
		if len(host.ScanHistory) > 0 {
			host.Uptime += scan.Timestamp.Sub(host.ScanHistory[len(host.ScanHistory)-1].Timestamp)
		}
	} else {
		if len(host.ScanHistory) > 0 {
			host.Downtime += scan.Timestamp.Sub(host.ScanHistory[len(host.ScanHistory)-1].Timestamp)
		}
	}

	// Limit the in-memory history to two most recent scans.
	host.ScanHistory = append(host.ScanHistory, scan)
	if len(host.ScanHistory) > 2 {
		host.ScanHistory = host.ScanHistory[1:]
	}

	var settings, pt bytes.Buffer
	if (scan.Settings != rhpv2.HostSettings{}) {
		host.Settings = scan.Settings
		e := types.NewEncoder(&settings)
		utils.EncodeSettings(&scan.Settings, e)
		e.Flush()
	}
	if (scan.PriceTable != rhpv3.HostPriceTable{}) {
		host.PriceTable = scan.PriceTable
		e := types.NewEncoder(&pt)
		utils.EncodePriceTable(&scan.PriceTable, e)
		e.Flush()
	}

	_, err := s.tx.Exec(`
		INSERT INTO hdb_scans_`+s.network+` (
			public_key,
			ran_at,
			success,
			latency,
			error,
			settings,
			price_table,
			modified,
			fetched
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		host.PublicKey[:],
		scan.Timestamp.Unix(),
		scan.Success,
		scan.Latency.Milliseconds(),
		scan.Error,
		settings.Bytes(),
		pt.Bytes(),
		time.Now().Unix(),
		0,
	)
	if err != nil {
		return utils.AddContext(err, "couldn't update scan history")
	}

	err = s.update(host)
	if err != nil {
		return utils.AddContext(err, "couldn't update host")
	}

	if (len(host.ScanHistory) > 0 && host.ScanHistory[len(host.ScanHistory)-1].Success) && (len(host.ScanHistory) > 1 && host.ScanHistory[len(host.ScanHistory)-2].Success || len(host.ScanHistory) == 1) {
		s.activeHostsCache[host.PublicKey] = host.IPNets
	} else {
		delete(s.activeHostsCache, host.PublicKey)
	}

	return nil
}

// updateBenchmarks adds a new benchmark to the host's benchmark history.
func (s *hostDBStore) updateBenchmarks(host *HostDBEntry, benchmark HostBenchmark) error {
	if host.Network != s.network {
		panic("networks don't match")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tx == nil {
		return errors.New("there is no transaction")
	}

	host.LastBenchmark = benchmark
	_, err := s.tx.Exec(`
		INSERT INTO hdb_benchmarks_`+s.network+` (
			public_key,
			ran_at,
			success,
			upload_speed,
			download_speed,
			ttfb,
			error,
			modified,
			fetched
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		host.PublicKey[:],
		benchmark.Timestamp.Unix(),
		benchmark.Success,
		benchmark.UploadSpeed,
		benchmark.DownloadSpeed,
		benchmark.TTFB.Milliseconds(),
		benchmark.Error,
		time.Now().Unix(),
		0,
	)
	if err != nil {
		return utils.AddContext(err, "couldn't update benchmarks")
	}

	err = s.update(host)
	if err != nil {
		return utils.AddContext(err, "couldn't update host")
	}

	return nil
}

// lastFailedScans returns the number of scans failed in a row.
// NOTE: a lock must be acquired before calling this function.
func (s *hostDBStore) lastFailedScans(host *HostDBEntry) int {
	if host.Network != s.network {
		panic("networks don't match")
	}
	if s.tx == nil {
		s.log.Error("there is no transaction", zap.String("network", s.network))
		return 0
	}

	var count int
	err := s.tx.QueryRow(`
		SELECT COUNT(*)
		FROM hdb_scans_`+s.network+` AS a
		WHERE a.public_key = ?
		AND a.success = FALSE
		AND (
			a.ran_at > (
				SELECT b.ran_at
				FROM hdb_scans_`+s.network+` AS b
				WHERE b.public_key = a.public_key
				AND b.success = TRUE
				ORDER BY b.ran_at DESC
				LIMIT 1
			)
			OR (
				SELECT COUNT(*)
				FROM hdb_scans_`+s.network+` AS c
				WHERE c.public_key = a.public_key
				AND c.success = TRUE
			) = 0
		)
	`, host.PublicKey[:]).Scan(&count)
	if err != nil {
		s.log.Error("couldn't query scans", zap.String("network", s.network), zap.Error(err))
		return 0
	}

	return count
}

// lastFailedBenchmarks returns the number of benchmarks failed in a row.
// NOTE: a lock must be acquired before calling this function.
func (s *hostDBStore) lastFailedBenchmarks(host *HostDBEntry) int {
	if host.Network != s.network {
		panic("networks don't match")
	}
	if s.tx == nil {
		s.log.Error("there is no transaction", zap.String("network", s.network))
		return 0
	}

	var count int
	err := s.tx.QueryRow(`
		SELECT COUNT(*)
		FROM hdb_benchmarks_`+s.network+` AS a
		WHERE a.public_key = ?
		AND a.success = FALSE
		AND (
			a.ran_at > (
				SELECT b.ran_at
				FROM hdb_benchmarks_`+s.network+` AS b
				WHERE b.public_key = a.public_key
				AND b.success = TRUE
				ORDER BY b.ran_at DESC
				LIMIT 1
			)
			OR (
				SELECT COUNT(*)
				FROM hdb_benchmarks_`+s.network+` AS c
				WHERE c.public_key = a.public_key
				AND c.success = TRUE
			) = 0
		)
	`, host.PublicKey[:]).Scan(&count)
	if err != nil {
		s.log.Error("couldn't query benchmarks", zap.String("network", s.network), zap.Error(err))
		return 0
	}

	return count
}

func (s *hostDBStore) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tx != nil {
		if err := s.tx.Commit(); err != nil {
			s.log.Error("couldn't commit transaction", zap.String("network", s.network), zap.Error(err))
		}
	}
}

func (s *hostDBStore) load(domains *blockedDomains) error {
	row := 1
	if s.network == "zen" {
		row = 2
	}

	var height uint64
	id := make([]byte, 32)
	err := s.db.QueryRow(`
		SELECT height, bid
		FROM hdb_tip
		WHERE id = ?
	`, row).Scan(&height, &id)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return utils.AddContext(err, "couldn't load tip")
	}
	s.tip.Height = height
	copy(s.tip.ID[:], id)

	scanStmt, err := s.db.Prepare(`
		SELECT ran_at, success, latency, error, settings, price_table
		FROM hdb_scans_` + s.network + `
		WHERE public_key = ?
		ORDER BY ran_at DESC
		LIMIT 2
	`)
	if err != nil {
		return utils.AddContext(err, "couldn't prepare scan statement")
	}
	defer scanStmt.Close()

	settingsStmt, err := s.db.Prepare(`
		SELECT settings
		FROM hdb_scans_` + s.network + `
		WHERE public_key = ?
		AND settings IS NOT NULL
		ORDER BY ran_at DESC
		LIMIT 1
	`)
	if err != nil {
		return utils.AddContext(err, "couldn't prepare settings statement")
	}
	defer settingsStmt.Close()

	priceTableStmt, err := s.db.Prepare(`
		SELECT price_table
		FROM hdb_scans_` + s.network + `
		WHERE public_key = ?
		AND price_table IS NOT NULL
		ORDER BY ran_at DESC
		LIMIT 1
	`)
	if err != nil {
		return utils.AddContext(err, "couldn't prepare price table statement")
	}
	defer priceTableStmt.Close()

	benchmarkStmt, err := s.db.Prepare(`
		SELECT ran_at, success, upload_speed, download_speed, ttfb, error
		FROM hdb_benchmarks_` + s.network + `
		WHERE public_key = ?
		ORDER BY ran_at DESC
		LIMIT 1
	`)
	if err != nil {
		return utils.AddContext(err, "couldn't prepare benchmark statement")
	}
	defer benchmarkStmt.Close()

	rows, err := s.db.Query(`
		SELECT
			id,
			public_key,
			first_seen,
			known_since,
			blocked,
			net_address,
			uptime,
			downtime,
			last_seen,
			ip_nets,
			last_ip_change,
			historic_successful_interactions,
			historic_failed_interactions,
			recent_successful_interactions,
			recent_failed_interactions,
			last_update,
			revision,
			settings,
			price_table
		FROM hdb_hosts_` + s.network,
	)
	if err != nil {
		return utils.AddContext(err, "couldn't query hosts")
	}

	for rows.Next() {
		var id int
		pk := make([]byte, 32)
		var ks, lu uint64
		var b bool
		var na, ip string
		var ut, dt, fs, ls, lc int64
		var hsi, hfi, rsi, rfi float64
		var rev, settings, pt []byte
		if err := rows.Scan(&id, &pk, &fs, &ks, &b, &na, &ut, &dt, &ls, &ip, &lc, &hsi, &hfi, &rsi, &rfi, &lu, &rev, &settings, &pt); err != nil {
			rows.Close()
			return utils.AddContext(err, "couldn't scan host data")
		}
		host := &HostDBEntry{
			ID:           id,
			PublicKey:    types.PublicKey(pk),
			Network:      s.network,
			FirstSeen:    time.Unix(fs, 0),
			KnownSince:   ks,
			Blocked:      b,
			NetAddress:   na,
			Uptime:       time.Duration(ut) * time.Second,
			Downtime:     time.Duration(dt) * time.Second,
			LastSeen:     time.Unix(ls, 0),
			IPNets:       strings.Split(ip, ";"),
			LastIPChange: time.Unix(lc, 0),
			Interactions: HostInteractions{
				HistoricSuccesses: hsi,
				HistoricFailures:  hfi,
				RecentSuccesses:   rsi,
				RecentFailures:    rfi,
				LastUpdate:        lu,
			},
		}
		if len(rev) > 0 {
			d := types.NewBufDecoder(rev)
			host.Revision.DecodeFrom(d)
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
		if host.Blocked || domains.isBlocked(host.NetAddress) {
			host.Blocked = true
			s.blockedHosts[host.PublicKey] = struct{}{}
		}

		scanRows, err := scanStmt.Query(pk)
		if err != nil {
			rows.Close()
			return utils.AddContext(err, "couldn't query scans")
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
				return utils.AddContext(err, "couldn't load scan history")
			}
			scan := HostScan{
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
					return utils.AddContext(err, "couldn't decode host settings")
				}
			}
			if len(pt) > 0 {
				d := types.NewBufDecoder(pt)
				utils.DecodePriceTable(&scan.PriceTable, d)
				if err := d.Err(); err != nil {
					scanRows.Close()
					rows.Close()
					return utils.AddContext(err, "couldn't decode host price table")
				}
			}
			host.ScanHistory = append([]HostScan{scan}, host.ScanHistory...)
		}
		scanRows.Close()

		if len(settings) == 0 {
			err = settingsStmt.QueryRow(pk).Scan(&settings)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				rows.Close()
				return utils.AddContext(err, "couldn't load host settings")
			}
			if len(settings) > 0 {
				d := types.NewBufDecoder(settings)
				utils.DecodeSettings(&host.Settings, d)
				if err := d.Err(); err != nil {
					rows.Close()
					return utils.AddContext(err, "couldn't decode host settings")
				}
			}
		}

		if len(pt) == 0 {
			err = priceTableStmt.QueryRow(pk).Scan(&pt)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				rows.Close()
				return utils.AddContext(err, "couldn't load host price table")
			}
			if len(pt) > 0 {
				d := types.NewBufDecoder(pt)
				utils.DecodePriceTable(&host.PriceTable, d)
				if err := d.Err(); err != nil {
					rows.Close()
					return utils.AddContext(err, "couldn't decode host price table")
				}
			}
		}

		var ra int64
		var success bool
		var ul, dl, ttfb float64
		var msg string
		err = benchmarkStmt.QueryRow(pk).Scan(&ra, &success, &ul, &dl, &ttfb, &msg)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			rows.Close()
			return utils.AddContext(err, "couldn't load benchmarks")
		}
		if err == nil {
			host.LastBenchmark = HostBenchmark{
				Timestamp:     time.Unix(ra, 0),
				Success:       success,
				UploadSpeed:   ul,
				DownloadSpeed: dl,
				TTFB:          time.Duration(ttfb) * time.Millisecond,
				Error:         msg,
			}
		}
		s.hosts[host.PublicKey] = host
		if (len(host.ScanHistory) > 0 && host.ScanHistory[len(host.ScanHistory)-1].Success) && (len(host.ScanHistory) > 1 && host.ScanHistory[len(host.ScanHistory)-2].Success || len(host.ScanHistory) == 1) {
			s.activeHostsCache[host.PublicKey] = host.IPNets
		}
	}
	rows.Close()

	s.log.Info("loading complete", zap.String("network", s.network))

	s.tx, err = s.db.Begin()
	return err
}

func (s *hostDBStore) isSynced() bool {
	if s.network == "zen" {
		return isSynced(s.hdb.syncerZen)
	}
	return isSynced(s.hdb.syncer)
}

// ProcessChainApplyUpdate implements chain.Subscriber.
func (s *hostDBStore) ProcessChainApplyUpdate(cau *chain.ApplyUpdate, mayCommit bool) (err error) {
	// Check if the update is for the right network.
	if cau.State.Network.Name != s.network {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.tip = cau.State.Index
	row := 1
	if s.network == "zen" {
		row = 2
	}
	_, err = s.tx.Exec(`
		REPLACE INTO hdb_tip (id, network, height, bid)
		VALUES (?, ?, ?, ?)
	`, row, s.network, s.tip.Height, s.tip.ID[:])
	if err != nil {
		s.log.Error("couldn't update tip", zap.String("network", s.network), zap.Error(err))
		return err
	}

	for _, txn := range cau.Block.Transactions {
		for _, ad := range txn.ArbitraryData {
			addr, pk, err := decodeAnnouncement(ad)
			if err != nil {
				// Not a valid host announcement.
				continue
			}
			if err := utils.IsValid(addr); err != nil {
				// Invalid netaddress.
				continue
			}
			if utils.IsLocal(addr) {
				// Local netaddress.
				continue
			}
			host, exists := s.hosts[pk]
			if !exists {
				host = &HostDBEntry{
					ID:         len(s.hosts) + 1,
					Network:    s.network,
					PublicKey:  pk,
					FirstSeen:  cau.Block.Timestamp,
					KnownSince: cau.State.Index.Height,
				}
			}
			host.NetAddress = addr
			ipNets, err := utils.LookupIPNets(addr)
			if err == nil && !utils.EqualIPNets(ipNets, host.IPNets) {
				host.IPNets = ipNets
				host.LastIPChange = cau.Block.Timestamp
			}
			err = s.update(host)
			if err != nil {
				s.log.Error("couldn't update host", zap.String("network", s.network), zap.Error(err))
				return err
			}
			if (!exists || s.isSynced()) && !host.Blocked {
				s.hdb.queueScan(host)
			}
		}
	}

	for _, txn := range cau.Block.V2Transactions() {
		for _, at := range txn.Attestations {
			addr, pk, err := decodeV2Announcement(at)
			if err != nil {
				// Not a valid host announcement.
				continue
			}
			if err := utils.IsValid(addr); err != nil {
				// Invalid netaddress.
				continue
			}
			if utils.IsLocal(addr) {
				// Local netaddress.
				continue
			}
			host, exists := s.hosts[pk]
			if !exists {
				host = &HostDBEntry{
					ID:         len(s.hosts) + 1,
					Network:    s.network,
					PublicKey:  pk,
					FirstSeen:  cau.Block.Timestamp,
					KnownSince: cau.State.Index.Height,
				}
			}
			host.NetAddress = addr
			ipNets, err := utils.LookupIPNets(addr)
			if err == nil && !utils.EqualIPNets(ipNets, host.IPNets) {
				host.IPNets = ipNets
				host.LastIPChange = cau.Block.Timestamp
			}
			err = s.update(host)
			if err != nil {
				s.log.Error("couldn't update host", zap.String("network", s.network), zap.Error(err))
				return err
			}
			if (!exists || s.isSynced()) && !host.Blocked {
				s.hdb.queueScan(host)
			}
		}
	}

	if mayCommit || time.Since(s.lastCommitted) >= 3*time.Second {
		err = s.tx.Commit()
		if err != nil {
			return utils.AddContext(err, "couldn't commit transaction")
		}
		s.lastCommitted = time.Now()
		s.tx, err = s.db.Begin()
	}

	return err
}

// ProcessChainRevertUpdate implements chain.Subscriber.
func (s *hostDBStore) ProcessChainRevertUpdate(_ *chain.RevertUpdate) error {
	return nil
}

func (s *hostDBStore) activeHostsInSubnet(ipNets []string) int {
	var count int
	subnets := make(map[string]struct{})
	for _, ip := range ipNets {
		subnets[ip] = struct{}{}
	}
outer:
	for _, entry := range s.activeHostsCache {
		for _, ip := range entry {
			if _, exists := subnets[ip]; exists {
				count++
				continue outer
			}
		}
	}
	return count
}

// checkSubnets calculates the total number of active hosts sharing
// the same subnet(s).
func (s *hostDBStore) checkSubnets(ipNets []string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.activeHostsInSubnet(ipNets)
}

// getRecentUpdates returns the most recently updated database records
// since the last retrieval.
// The batch size is limited to avoid sending too large responses.
func (s *hostDBStore) getRecentUpdates(id UpdateID) (updates HostUpdates, err error) {
	if s.tx == nil {
		s.log.Error("there is no transaction", zap.String("network", s.network))
		return HostUpdates{}, errors.New("no database transaction")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.tx.Query(`
		SELECT public_key
		FROM hdb_hosts_` + s.network + `
		WHERE modified > fetched
		ORDER BY id ASC
		LIMIT 1000
	`)
	if err != nil {
		return HostUpdates{}, utils.AddContext(err, "couldn't query hosts")
	}

	for rows.Next() {
		pk := make([]byte, 32)
		if err := rows.Scan(&pk); err != nil {
			rows.Close()
			return HostUpdates{}, utils.AddContext(err, "couldn't decode host data")
		}
		host := s.hosts[types.PublicKey(pk)]
		host.ActiveHosts = s.activeHostsInSubnet(host.IPNets)
		updates.Hosts = append(updates.Hosts, *host)
	}
	rows.Close()

	rows, err = s.tx.Query(`
		SELECT id, public_key, ran_at, success, latency, error, settings, price_table
		FROM hdb_scans_`+s.network+`
		WHERE modified > fetched
		ORDER BY id ASC
		LIMIT ?
	`, 1000-len(updates.Hosts))
	if err != nil {
		return HostUpdates{}, utils.AddContext(err, "couldn't query scans")
	}

	for rows.Next() {
		var id, ra int64
		var success bool
		var latency float64
		var msg string
		var settings, pt []byte
		pk := make([]byte, 32)
		if err := rows.Scan(&id, &pk, &ra, &success, &latency, &msg, &settings, &pt); err != nil {
			rows.Close()
			return HostUpdates{}, utils.AddContext(err, "couldn't decode scans")
		}
		scan := ScanHistory{
			HostScan: HostScan{
				ID:        id,
				Timestamp: time.Unix(ra, 0),
				Success:   success,
				Latency:   time.Duration(latency) * time.Millisecond,
				Error:     msg,
			},
			PublicKey: types.PublicKey(pk),
			Network:   s.network,
		}
		if len(settings) > 0 {
			d := types.NewBufDecoder(settings)
			utils.DecodeSettings(&scan.Settings, d)
			if err := d.Err(); err != nil {
				rows.Close()
				return HostUpdates{}, utils.AddContext(err, "couldn't decode host settings")
			}
		}
		if len(pt) > 0 {
			d := types.NewBufDecoder(pt)
			utils.DecodePriceTable(&scan.PriceTable, d)
			if err := d.Err(); err != nil {
				rows.Close()
				return HostUpdates{}, utils.AddContext(err, "couldn't decode host price table")
			}
		}
		updates.Scans = append(updates.Scans, scan)
	}
	rows.Close()

	rows, err = s.tx.Query(`
		SELECT id, public_key, ran_at, success, upload_speed, download_speed, ttfb, error
		FROM hdb_benchmarks_`+s.network+`
		WHERE modified > fetched
		ORDER BY id ASC
		LIMIT ?
	`, 1000-len(updates.Hosts)-len(updates.Scans))
	if err != nil {
		return HostUpdates{}, utils.AddContext(err, "couldn't query benchmarks")
	}

	for rows.Next() {
		var id, ra int64
		var success bool
		var ul, dl, ttfb float64
		var msg string
		pk := make([]byte, 32)
		if err := rows.Scan(&id, &pk, &ra, &success, &ul, &dl, &ttfb, &msg); err != nil {
			rows.Close()
			return HostUpdates{}, utils.AddContext(err, "couldn't decode benchmarks")
		}
		benchmark := BenchmarkHistory{
			HostBenchmark: HostBenchmark{
				ID:            id,
				Timestamp:     time.Unix(ra, 0),
				Success:       success,
				UploadSpeed:   ul,
				DownloadSpeed: dl,
				TTFB:          time.Duration(ttfb) * time.Millisecond,
				Error:         msg,
			},
			PublicKey: types.PublicKey(pk),
			Network:   s.network,
		}
		updates.Benchmarks = append(updates.Benchmarks, benchmark)
	}
	rows.Close()

	updates.ID = id
	s.lastUpdate = updates

	return
}

// finalizeUpdates updates the timestamps after the client confirms the data receipt.
func (s *hostDBStore) finalizeUpdates(id UpdateID) error {
	if id != s.lastUpdate.ID {
		return nil
	}

	if s.tx == nil {
		s.log.Error("there is no transaction", zap.String("network", s.network))
		return errors.New("no database transaction")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	hostStsmt, err := s.tx.Prepare(`
		UPDATE hdb_hosts_` + s.network + `
		SET fetched = ?
		WHERE id = ?
	`)
	if err != nil {
		return utils.AddContext(err, "couldn't prepare host statement")
	}

	for _, host := range s.lastUpdate.Hosts {
		if host.Network != s.network {
			continue
		}
		_, err := hostStsmt.Exec(time.Now().Unix(), host.ID)
		if err != nil {
			hostStsmt.Close()
			return utils.AddContext(err, "couldn't update timestamp in hosts table")
		}
	}
	hostStsmt.Close()

	scanStmt, err := s.tx.Prepare(`
		UPDATE hdb_scans_` + s.network + `
		SET fetched = ?
		WHERE id = ?
	`)
	if err != nil {
		return utils.AddContext(err, "couldn't prepare scan statement")
	}

	for _, scan := range s.lastUpdate.Scans {
		if scan.Network != s.network {
			continue
		}
		_, err := scanStmt.Exec(time.Now().Unix(), scan.ID)
		if err != nil {
			scanStmt.Close()
			return utils.AddContext(err, "couldn't update timestamp in scans table")
		}
	}
	scanStmt.Close()

	benchmarkStmt, err := s.tx.Prepare(`
		UPDATE hdb_benchmarks_` + s.network + `
		SET fetched = ?
		WHERE id = ?
	`)
	if err != nil {
		return utils.AddContext(err, "couldn't prepare benchmark statement")
	}

	for _, benchmark := range s.lastUpdate.Benchmarks {
		if benchmark.Network != s.network {
			continue
		}
		_, err := benchmarkStmt.Exec(time.Now().Unix(), benchmark.ID)
		if err != nil {
			benchmarkStmt.Close()
			return utils.AddContext(err, "couldn't update timestamp in benchmarks table")
		}
	}
	benchmarkStmt.Close()

	s.lastUpdate = HostUpdates{}

	if err := s.tx.Commit(); err != nil {
		return utils.AddContext(err, "couldn't commit transaction")
	}

	s.tx, err = s.db.Begin()
	return err
}

func (s *hostDBStore) getHostsForScan() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, host := range s.hosts {
		if host.Blocked {
			continue
		}
		if len(host.ScanHistory) == 0 || time.Since(host.ScanHistory[len(host.ScanHistory)-1].Timestamp) >= s.calculateScanInterval(host) {
			s.hdb.queueScan(host)
			continue
		}
		t := host.LastBenchmark.Timestamp
		if t.IsZero() || time.Since(t) >= s.calculateBenchmarkInterval(host) {
			s.hdb.queueScan(host)
		}
	}
}

func (s *hostDBStore) pruneOldRecords() error {
	if s.tx == nil {
		return errors.New("no database transaction")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.tx.Exec(`
		DELETE FROM hdb_scans_`+s.network+`
		WHERE ran_at < ?
	`, time.Now().AddDate(0, 0, -7).Unix())
	if err != nil {
		return utils.AddContext(err, "couldn't delete old scans")
	}

	_, err = s.tx.Exec(`
		DELETE FROM hdb_benchmarks_`+s.network+`
		WHERE ran_at < ?
	`, time.Now().AddDate(0, 0, -28).Unix())
	if err != nil {
		return utils.AddContext(err, "couldn't delete old benchmarks")
	}

	if err := s.tx.Commit(); err != nil {
		return utils.AddContext(err, "couldn't commit transaction")
	}

	s.tx, err = s.db.Begin()
	return err
}
