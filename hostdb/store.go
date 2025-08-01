package hostdb

import (
	"bytes"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/mike76-dev/hostscore/internal/utils"
	rhpv4 "go.sia.tech/core/rhp/v4"
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

	v2Contracts map[types.FileContractID]types.V2FileContractElement

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
		v2Contracts:      make(map[types.FileContractID]types.V2FileContractElement),
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
	var rev, settings bytes.Buffer
	e := types.NewEncoder(&rev)
	if (host.V2Revision.Parent.ID != types.FileContractID{}) {
		host.V2Revision.EncodeTo(e)
		e.Flush()
		s.v2Contracts[host.V2Revision.Parent.ID] = host.V2Revision.Parent
	}
	e = types.NewEncoder(&settings)
	if (host.V2Settings != rhpv4.HostSettings{}) {
		host.V2Settings.EncodeTo(e)
		e.Flush()
	}

	_, err := s.tx.Exec(`
		INSERT INTO hdb_hosts (
			id,
			network,
			public_key,
			first_seen,
			known_since,
			blocked,
			v2,
			net_address,
			uptime,
			downtime,
			last_seen,
			ip_nets,
			last_ip_change,
			successes,
			failures,
			last_update,
			revision,
			settings,
			siamux_addresses,
			modified,
			fetched
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) AS new
		ON DUPLICATE KEY UPDATE
			first_seen = new.first_seen,
			known_since = new.known_since,
			blocked = new.blocked,
			v2 = new.v2,
			net_address = new.net_address,
			uptime = new.uptime,
			downtime = new.downtime,
			last_seen = new.last_seen,
			ip_nets = new.ip_nets,
			last_ip_change = new.last_ip_change,
			successes = new.successes,
			failures = new.failures,
			last_update = new.last_update,
			revision = new.revision,
			settings = new.settings,
			siamux_addresses = new.siamux_addresses,
			modified = new.modified
	`,
		host.ID,
		host.Network,
		host.PublicKey[:],
		host.FirstSeen.Unix(),
		host.KnownSince,
		host.Blocked,
		host.V2,
		host.NetAddress,
		int64(host.Uptime.Seconds()),
		int64(host.Downtime.Seconds()),
		host.LastSeen.Unix(),
		strings.Join(host.IPNets, ";"),
		host.LastIPChange.Unix(),
		host.Interactions.Successes,
		host.Interactions.Failures,
		host.Interactions.LastUpdate,
		rev.Bytes(),
		settings.Bytes(),
		strings.Join(host.SiamuxAddresses, ","),
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

	var settings bytes.Buffer
	if (scan.V2Settings != rhpv4.HostSettings{}) {
		host.V2Settings = scan.V2Settings
		e := types.NewEncoder(&settings)
		scan.V2Settings.EncodeTo(e)
		e.Flush()
	}

	_, err := s.tx.Exec(`
		INSERT INTO hdb_scans (
			network,
			public_key,
			ran_at,
			success,
			latency,
			error,
			settings,
			modified,
			fetched
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		host.Network,
		host.PublicKey[:],
		scan.Timestamp.Unix(),
		scan.Success,
		scan.Latency.Milliseconds(),
		scan.Error,
		settings.Bytes(),
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
		INSERT INTO hdb_benchmarks (
			network,
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
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		host.Network,
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
		FROM hdb_scans AS a
		WHERE a.public_key = ?
		AND a.network = ?
		AND a.success = FALSE
		AND (
			a.ran_at > (
				SELECT b.ran_at
				FROM hdb_scans AS b
				WHERE b.public_key = a.public_key
				AND b.network = a.network
				AND b.success = TRUE
				ORDER BY b.ran_at DESC
				LIMIT 1
			)
			OR (
				SELECT COUNT(*)
				FROM hdb_scans AS c
				WHERE c.public_key = a.public_key
				AND c.network = a.network
				AND c.success = TRUE
			) = 0
		)
	`, host.PublicKey[:], host.Network).Scan(&count)
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
		FROM hdb_benchmarks AS a
		WHERE a.public_key = ?
		AND a.network = ?
		AND a.success = FALSE
		AND (
			a.ran_at > (
				SELECT b.ran_at
				FROM hdb_benchmarks AS b
				WHERE b.public_key = a.public_key
				AND b.network = a.network
				AND b.success = TRUE
				ORDER BY b.ran_at DESC
				LIMIT 1
			)
			OR (
				SELECT COUNT(*)
				FROM hdb_benchmarks AS c
				WHERE c.public_key = a.public_key
				AND c.network = a.network
				AND c.success = TRUE
			) = 0
		)
	`, host.PublicKey[:], host.Network).Scan(&count)
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
	var height uint64
	id := make([]byte, 32)
	err := s.db.QueryRow(`
		SELECT height, bid
		FROM hdb_tip
		WHERE network = ?
	`, s.network).Scan(&height, &id)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return utils.AddContext(err, "couldn't load tip")
	}
	s.tip.Height = height
	copy(s.tip.ID[:], id)

	rows, err := s.db.Query(`
		SELECT
			id,
			public_key,
			first_seen,
			known_since,
			blocked,
			v2,
			net_address,
			uptime,
			downtime,
			last_seen,
			ip_nets,
			last_ip_change,
			successes,
			failures,
			last_update,
			revision,
			settings,
			siamux_addresses
		FROM hdb_hosts
		WHERE network = ?
	`, s.network,
	)
	if err != nil {
		return utils.AddContext(err, "couldn't query hosts")
	}

	for rows.Next() {
		var id int
		pk := make([]byte, 32)
		var ks, lu uint64
		var b, v2 bool
		var na, ip, smux string
		var ut, dt, fs, ls, lc int64
		var si, fi float64
		var rev, settings []byte
		if err := rows.Scan(&id, &pk, &fs, &ks, &b, &v2, &na, &ut, &dt, &ls, &ip, &lc, &si, &fi, &lu, &rev, &settings, &smux); err != nil {
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
			V2:           v2,
			NetAddress:   na,
			Uptime:       time.Duration(ut) * time.Second,
			Downtime:     time.Duration(dt) * time.Second,
			LastSeen:     time.Unix(ls, 0),
			IPNets:       strings.Split(ip, ";"),
			LastIPChange: time.Unix(lc, 0),
			Interactions: HostInteractions{
				Successes:  si,
				Failures:   fi,
				LastUpdate: lu,
			},
			SiamuxAddresses: strings.Split(smux, ","),
		}
		if v2 && len(rev) > 0 {
			d := types.NewBufDecoder(rev)
			host.V2Revision.DecodeFrom(d)
			if err := d.Err(); err != nil {
				rows.Close()
				return utils.AddContext(err, "couldn't decode revision")
			}
			if (host.V2Revision.Parent.ID != types.FileContractID{}) {
				s.v2Contracts[host.V2Revision.Parent.ID] = host.V2Revision.Parent
			}
		}
		if v2 && len(settings) > 0 {
			d := types.NewBufDecoder(settings)
			host.V2Settings.DecodeFrom(d)
			if err := d.Err(); err != nil {
				rows.Close()
				return utils.AddContext(err, "couldn't decode host settings")
			}
		}
		if host.Blocked || domains.isBlocked(host.NetAddress) {
			host.Blocked = true
			s.blockedHosts[host.PublicKey] = struct{}{}
		}
		s.hosts[host.PublicKey] = host
	}
	rows.Close()

	scanStmt, err := s.db.Prepare(`
		SELECT ran_at, success, latency, error, settings
		FROM hdb_scans
		WHERE public_key = ?
		AND network = ?
		ORDER BY ran_at DESC
		LIMIT 2
	`)
	if err != nil {
		return utils.AddContext(err, "couldn't prepare scan statement")
	}
	defer scanStmt.Close()

	settingsStmt, err := s.db.Prepare(`
		SELECT settings
		FROM hdb_scans
		WHERE public_key = ?
		AND network = ?
		AND settings IS NOT NULL
		ORDER BY ran_at DESC
		LIMIT 1
	`)
	if err != nil {
		return utils.AddContext(err, "couldn't prepare settings statement")
	}
	defer settingsStmt.Close()

	benchmarkStmt, err := s.db.Prepare(`
		SELECT ran_at, success, upload_speed, download_speed, ttfb, error
		FROM hdb_benchmarks
		WHERE public_key = ?
		AND network = ?
		ORDER BY ran_at DESC
		LIMIT 1
	`)
	if err != nil {
		return utils.AddContext(err, "couldn't prepare benchmark statement")
	}
	defer benchmarkStmt.Close()

	for _, host := range s.hosts {
		rows, err := scanStmt.Query(host.PublicKey[:], host.Network)
		if err != nil {
			return utils.AddContext(err, "couldn't query scans")
		}
		for rows.Next() {
			var ra int64
			var success bool
			var latency float64
			var msg string
			var settings []byte
			if err := rows.Scan(&ra, &success, &latency, &msg, &settings); err != nil {
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
				scan.V2Settings.DecodeFrom(d)
				if err := d.Err(); err != nil {
					rows.Close()
					return utils.AddContext(err, "couldn't decode host settings")
				}
			}
			host.ScanHistory = append([]HostScan{scan}, host.ScanHistory...)
		}
		rows.Close()

		if host.V2 {
			if (host.V2Settings == rhpv4.HostSettings{}) {
				var settings []byte
				err = settingsStmt.QueryRow(host.PublicKey[:], host.Network).Scan(&settings)
				if err != nil && !errors.Is(err, sql.ErrNoRows) {
					return utils.AddContext(err, "couldn't load host settings")
				}
				if len(settings) > 0 {
					d := types.NewBufDecoder(settings)
					host.V2Settings.DecodeFrom(d)
					if err := d.Err(); err != nil {
						return utils.AddContext(err, "couldn't decode host settings")
					}
				}
			}
		}

		var ra int64
		var success bool
		var ul, dl, ttfb float64
		var msg string
		err = benchmarkStmt.QueryRow(host.PublicKey[:], host.Network).Scan(&ra, &success, &ul, &dl, &ttfb, &msg)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
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
		if (len(host.ScanHistory) > 0 && host.ScanHistory[len(host.ScanHistory)-1].Success) && (len(host.ScanHistory) > 1 && host.ScanHistory[len(host.ScanHistory)-2].Success || len(host.ScanHistory) == 1) {
			s.activeHostsCache[host.PublicKey] = host.IPNets
		}
	}

	s.log.Info("loading complete", zap.String("network", s.network))

	s.tx, err = s.db.Begin()
	return err
}

func (s *hostDBStore) isSynced() bool {
	return isSynced(s.hdb.nodes.Syncer(s.network))
}

// updateChainState applies the chain manager updates.
func (s *hostDBStore) updateChainState(applied []chain.ApplyUpdate, mayCommit bool) error {
	extractSiamuxAddresses := func(addrs []chain.NetAddress) []string {
		var res []string
		for _, na := range addrs {
			if na.Protocol == "siamux" {
				res = append(res, na.Address)
			}
		}
		return res
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, cau := range applied {
		if s.network != cau.State.Network.Name {
			continue
		}

		s.tip = cau.State.Index
		_, err := s.tx.Exec(`
			REPLACE INTO hdb_tip (network, height, bid)
			VALUES (?, ?, ?)
		`, s.network, s.tip.Height, s.tip.ID[:])
		if err != nil {
			s.log.Error("couldn't update tip", zap.String("network", s.network), zap.Error(err))
			return err
		}

		for _, txn := range cau.Block.Transactions {
			for _, ad := range txn.ArbitraryData {
				var ha chain.HostAnnouncement
				if !ha.FromArbitraryData(ad) {
					// Not a valid host announcement.
					continue
				}
				if err := utils.IsValid(ha.NetAddress); err != nil {
					// Invalid netaddress.
					continue
				}
				if utils.IsLocal(ha.NetAddress) {
					// Local netaddress.
					continue
				}
				host, exists := s.hosts[ha.PublicKey]
				if !exists {
					host = &HostDBEntry{
						ID:         len(s.hosts) + 1,
						Network:    s.network,
						PublicKey:  ha.PublicKey,
						FirstSeen:  cau.Block.Timestamp,
						KnownSince: cau.State.Index.Height,
						V2:         false,
					}
				}
				host.NetAddress = ha.NetAddress
				ipNets, err := utils.LookupIPNets(ha.NetAddress)
				if err == nil && !utils.EqualIPNets(ipNets, host.IPNets) {
					host.IPNets = ipNets
					host.LastIPChange = cau.Block.Timestamp
				}
				err = s.update(host)
				if err != nil {
					s.log.Error("couldn't update host", zap.String("network", s.network), zap.Error(err))
					return err
				}
			}
		}

		for _, txn := range cau.Block.V2Transactions() {
			for _, at := range txn.Attestations {
				var ha chain.V2HostAnnouncement
				if err := ha.FromAttestation(at); err != nil {
					// Not a valid host announcement.
					continue
				}
				if len(ha) == 0 {
					// Empty announcement.
					continue
				}
				addrs := extractSiamuxAddresses(ha)
				if err := utils.IsValid(addrs[0]); err != nil {
					// Invalid netaddress.
					continue
				}
				if utils.IsLocal(addrs[0]) {
					// Local netaddress.
					continue
				}
				host, exists := s.hosts[at.PublicKey]
				if !exists {
					host = &HostDBEntry{
						ID:         len(s.hosts) + 1,
						Network:    s.network,
						PublicKey:  at.PublicKey,
						FirstSeen:  cau.Block.Timestamp,
						KnownSince: cau.State.Index.Height,
					}
				}
				host.V2 = true
				host.SiamuxAddresses = addrs
				host.NetAddress = addrs[0]
				ipNets, err := utils.LookupIPNets(addrs[0])
				if err == nil && !utils.EqualIPNets(ipNets, host.IPNets) {
					host.IPNets = ipNets
					host.LastIPChange = cau.Block.Timestamp
				}
				err = s.update(host)
				if err != nil {
					s.log.Error("couldn't update host", zap.String("network", s.network), zap.Error(err))
					return err
				}
				if s.isSynced() && !host.Blocked {
					s.hdb.queueScan(host)
				}
			}
		}
	}

	if mayCommit || time.Since(s.lastCommitted) >= 3*time.Second {
		err := s.tx.Commit()
		if err != nil {
			return utils.AddContext(err, "couldn't commit transaction")
		}
		s.lastCommitted = time.Now()
		s.tx, err = s.db.Begin()
		if err != nil {
			return utils.AddContext(err, "couldn't start transaction")
		}
	}

	// Submit contract expirations, if needed.
	if mayCommit {
		var res []types.V2FileContractResolution
		for id, fce := range s.v2Contracts {
			if fce.V2FileContract.ExpirationHeight <= s.tip.Height {
				res = append(res, types.V2FileContractResolution{
					Parent:     fce,
					Resolution: &types.V2FileContractExpiration{},
				})
				delete(s.v2Contracts, id)
			}
		}

		if len(res) > 0 {
			txn := types.V2Transaction{FileContractResolutions: res}
			s.hdb.nodes.Syncer(s.network).BroadcastV2TransactionSet(s.tip, []types.V2Transaction{txn})
		}
	}

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
		FROM hdb_hosts
		WHERE network = ?
		AND modified > fetched
		ORDER BY id ASC
		LIMIT 1000
	`, s.network)
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
		SELECT s.id, s.public_key, s.ran_at, s.success, s.latency, s.error, s.settings
		FROM hdb_scans s
		JOIN hdb_hosts h
		ON s.public_key = h.public_key
		AND s.network = h.network
		WHERE s.network = ?
		AND s.modified > s.fetched
		AND h.modified <= h.fetched
		ORDER BY s.id ASC
		LIMIT 1000
	`, s.network)
	if err != nil {
		return HostUpdates{}, utils.AddContext(err, "couldn't query scans")
	}

	for rows.Next() {
		var id, ra int64
		var success bool
		var latency float64
		var msg string
		var settings []byte
		pk := make([]byte, 32)
		if err := rows.Scan(&id, &pk, &ra, &success, &latency, &msg, &settings); err != nil {
			rows.Close()
			return HostUpdates{}, utils.AddContext(err, "couldn't decode scans")
		}
		scan := ScanHistoryEntry{
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
			scan.V2Settings.DecodeFrom(d)
			if err := d.Err(); err != nil {
				rows.Close()
				return HostUpdates{}, utils.AddContext(err, "couldn't decode host settings")
			}
		}
		updates.Scans = append(updates.Scans, scan)
	}
	rows.Close()

	rows, err = s.tx.Query(`
		SELECT b.id, b.public_key, b.ran_at, b.success, b.upload_speed, b.download_speed, b.ttfb, b.error
		FROM hdb_benchmarks b
		JOIN hdb_hosts h
		ON b.public_key = h.public_key
		AND b.network = h.network
		WHERE b.network = ?
		AND b.modified > b.fetched
		AND h.modified <= h.fetched
		ORDER BY b.id ASC
		LIMIT 1000
	`, s.network)
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
		benchmark := BenchmarkHistoryEntry{
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
		UPDATE hdb_hosts
		SET fetched = ?
		WHERE id = ?
		AND network = ?
	`)
	if err != nil {
		return utils.AddContext(err, "couldn't prepare host statement")
	}

	for _, host := range s.lastUpdate.Hosts {
		if host.Network != s.network {
			continue
		}
		_, err := hostStsmt.Exec(time.Now().Unix(), host.ID, s.network)
		if err != nil {
			hostStsmt.Close()
			return utils.AddContext(err, "couldn't update timestamp in hosts table")
		}
	}
	hostStsmt.Close()

	scanStmt, err := s.tx.Prepare(`
		UPDATE hdb_scans
		SET fetched = ?
		WHERE id = ?
		AND network = ?
	`)
	if err != nil {
		return utils.AddContext(err, "couldn't prepare scan statement")
	}

	for _, scan := range s.lastUpdate.Scans {
		if scan.Network != s.network {
			continue
		}
		_, err := scanStmt.Exec(time.Now().Unix(), scan.ID, s.network)
		if err != nil {
			scanStmt.Close()
			return utils.AddContext(err, "couldn't update timestamp in scans table")
		}
	}
	scanStmt.Close()

	benchmarkStmt, err := s.tx.Prepare(`
		UPDATE hdb_benchmarks
		SET fetched = ?
		WHERE id = ?
		AND network = ?
	`)
	if err != nil {
		return utils.AddContext(err, "couldn't prepare benchmark statement")
	}

	for _, benchmark := range s.lastUpdate.Benchmarks {
		if benchmark.Network != s.network {
			continue
		}
		_, err := benchmarkStmt.Exec(time.Now().Unix(), benchmark.ID, s.network)
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
		if (t.IsZero() || time.Since(t) >= s.calculateBenchmarkInterval(host)) &&
			(len(host.ScanHistory) > 0 && host.ScanHistory[len(host.ScanHistory)-1].Success) {
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
		DELETE FROM hdb_scans
		WHERE ran_at < ?
		AND network = ?
	`, time.Now().AddDate(0, 0, -7).Unix(), s.network)
	if err != nil {
		return utils.AddContext(err, "couldn't delete old scans")
	}

	_, err = s.tx.Exec(`
		DELETE FROM hdb_benchmarks_`+s.network+`
		WHERE ran_at < ?
		AND network = ?
	`, time.Now().AddDate(0, 0, -28).Unix(), s.network)
	if err != nil {
		return utils.AddContext(err, "couldn't delete old benchmarks")
	}

	if err := s.tx.Commit(); err != nil {
		return utils.AddContext(err, "couldn't commit transaction")
	}

	s.tx, err = s.db.Begin()
	return err
}
