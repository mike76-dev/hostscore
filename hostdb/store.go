package hostdb

import (
	"bytes"
	"database/sql"
	"errors"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/mike76-dev/hostscore/internal/utils"
	"github.com/mike76-dev/hostscore/persist"
	rhpv2 "go.sia.tech/core/rhp/v2"
	rhpv3 "go.sia.tech/core/rhp/v3"
	"go.sia.tech/core/types"
	"go.sia.tech/coreutils/chain"
)

type hostDBStore struct {
	db      *sql.DB
	tx      *sql.Tx
	log     *persist.Logger
	network string
	hdb     *HostDB

	hosts          map[types.PublicKey]*HostDBEntry
	blockedHosts   map[types.PublicKey]struct{}
	blockedDomains *blockedDomains

	mu sync.Mutex

	tip types.ChainIndex
}

func newHostDBStore(db *sql.DB, logger *persist.Logger, network string) (*hostDBStore, types.ChainIndex, error) {
	s := &hostDBStore{
		db:           db,
		log:          logger,
		network:      network,
		hosts:        make(map[types.PublicKey]*HostDBEntry),
		blockedHosts: make(map[types.PublicKey]struct{}),
	}
	err := s.load()
	if err != nil {
		s.log.Println("[ERROR] couldn't load hosts:", err)
		return nil, types.ChainIndex{}, err
	}
	return s, s.tip, nil
}

// update updates the host entry in the database.
// NOTE: a lock must be acquired before calling update.
func (s *hostDBStore) update(host *HostDBEntry) error {
	if s.tx == nil {
		return errors.New("there is no transaction")
	}
	if host.Blocked || s.blockedDomains.isBlocked(host.NetAddress) {
		host.Blocked = true
		s.blockedHosts[host.PublicKey] = struct{}{}
	} else {
		delete(s.blockedHosts, host.PublicKey)
	}
	s.hosts[host.PublicKey] = host
	var buf bytes.Buffer
	e := types.NewEncoder(&buf)
	if (host.Revision.ParentID != types.FileContractID{}) {
		host.Revision.EncodeTo(e)
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
			revision
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) AS new
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
			revision = new.revision
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
		buf.Bytes(),
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
func (s *hostDBStore) updateScanHistory(host HostDBEntry, scan HostScan) error {
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
			price_table
		)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`,
		host.PublicKey[:],
		scan.Timestamp.Unix(),
		scan.Success,
		scan.Latency.Milliseconds(),
		scan.Error,
		settings.Bytes(),
		pt.Bytes(),
	)
	if err != nil {
		return utils.AddContext(err, "couldn't update scan history")
	}

	err = s.update(&host)
	if err != nil {
		return utils.AddContext(err, "couldn't update host")
	}

	return nil
}

// updateBenchmarks adds a new benchmark to the host's benchmark history.
func (s *hostDBStore) updateBenchmarks(host HostDBEntry, benchmark HostBenchmark) error {
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
			error
		)
		VALUES (?, ?, ?, ?, ?, ?)
	`,
		host.PublicKey[:],
		benchmark.Timestamp.Unix(),
		benchmark.Success,
		benchmark.UploadSpeed,
		benchmark.DownloadSpeed,
		benchmark.Error,
	)
	if err != nil {
		return utils.AddContext(err, "couldn't update benchmarks")
	}

	err = s.update(&host)
	if err != nil {
		return utils.AddContext(err, "couldn't update host")
	}

	return nil
}

// lastFailedBenchmarks returns the number of benchmarks failed in a row.
// NOTE: a lock must be acquired before calling this function.
func (s *hostDBStore) lastFailedBenchmarks(host *HostDBEntry) int {
	if s.tx == nil {
		s.log.Println("[ERROR] there is no transaction")
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
		s.log.Println("[ERROR] couldn't query benchmarks:", err)
		return 0
	}

	return count
}

func (s *hostDBStore) close() {
	if s.tx != nil {
		if err := s.tx.Commit(); err != nil {
			s.log.Println("[ERROR] couldn't commit transaction:", err)
		}
	}
	s.log.Close()
}

func (s *hostDBStore) load() error {
	var height uint64
	id := make([]byte, 32)
	err := s.db.QueryRow(`
		SELECT height, bid
		FROM hdb_tip_`+s.network+`
		WHERE id = 1
	`).Scan(&height, &id)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return utils.AddContext(err, "couldn't load tip")
	}
	s.tip.Height = height
	copy(s.tip.ID[:], id)

	var domains []string
	rows, err := s.db.Query("SELECT dom FROM hdb_domains_" + s.network)
	if err != nil {
		return utils.AddContext(err, "couldn't query blocked domains")
	}
	for rows.Next() {
		var domain string
		if err := rows.Scan(&domain); err != nil {
			rows.Close()
			return utils.AddContext(err, "couldn't scan filtered domain")
		}
		domains = append(domains, domain)
	}
	rows.Close()
	s.blockedDomains = newBlockedDomains(domains)

	rows, err = s.db.Query(`
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
			revision
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
		var rev []byte
		if err := rows.Scan(&id, &pk, &fs, &ks, &b, &na, &ut, &dt, &ls, &ip, &lc, &hsi, &hfi, &rsi, &rfi, &lu, &rev); err != nil {
			rows.Close()
			return utils.AddContext(err, "couldn't scan host data")
		}
		host := &HostDBEntry{
			ID:           id,
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
		copy(host.PublicKey[:], pk)
		if len(rev) > 0 {
			d := types.NewBufDecoder(rev)
			host.Revision.DecodeFrom(d)
		}
		s.mu.Lock()
		if host.Blocked || s.blockedDomains.isBlocked(host.NetAddress) {
			host.Blocked = true
			s.blockedHosts[host.PublicKey] = struct{}{}
		}
		s.mu.Unlock()

		scanRows, err := s.db.Query(`
			SELECT ran_at, success, latency, error, settings, price_table
			FROM hdb_scans_`+s.network+`
			WHERE public_key = ?
			ORDER BY ran_at DESC
			LIMIT 2
		`, pk)
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
			host.ScanHistory = append(host.ScanHistory, scan)
		}
		scanRows.Close()

		var settings []byte
		err = s.db.QueryRow(`
			SELECT settings
			FROM hdb_scans_`+s.network+`
			WHERE public_key = ?
			AND settings IS NOT NULL
			ORDER BY ran_at DESC
			LIMIT 1
		`, pk).Scan(&settings)
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

		var pt []byte
		err = s.db.QueryRow(`
			SELECT price_table
			FROM hdb_scans_`+s.network+`
			WHERE public_key = ?
			AND price_table IS NOT NULL
			ORDER BY ran_at DESC
			LIMIT 1
		`, pk).Scan(&pt)
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

		var ra int64
		var success bool
		var ul, dl float64
		var msg string
		err = s.db.QueryRow(`
			SELECT ran_at, success, upload_speed, download_speed, error
			FROM hdb_benchmarks_`+s.network+`
			WHERE public_key = ?
			ORDER BY ran_at DESC
			LIMIT 1
		`, pk).Scan(&ra, &success, &ul, &dl, &msg)
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
				Error:         msg,
			}
		}
		s.mu.Lock()
		s.hosts[host.PublicKey] = host
		s.mu.Unlock()
	}
	rows.Close()
	s.log.Println("[INFO] loading complete")

	s.tx, err = s.db.Begin()
	return err
}

// ProcessChainApplyUpdate implements chain.Subscriber.
func (s *hostDBStore) ProcessChainApplyUpdate(cau *chain.ApplyUpdate, mayCommit bool) (err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.tip = cau.State.Index
	_, err = s.tx.Exec(`
		REPLACE INTO hdb_tip_`+s.network+` (id, height, bid)
		VALUES (1, ?, ?)
	`, s.tip.Height, s.tip.ID[:])
	if err != nil {
		s.log.Println("[ERROR] couldn't update tip:", err)
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
				s.log.Println("[ERROR] couldn't update host:", err)
				return err
			}
			if (!exists || s.hdb.syncer.Synced()) && !host.Blocked {
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
					PublicKey:  pk,
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
				s.log.Println("[ERROR] couldn't update host:", err)
				return err
			}
			if (!exists || s.hdb.syncer.Synced()) && !host.Blocked {
				s.hdb.queueScan(host)
			}
		}
	}

	if mayCommit {
		err = s.tx.Commit()
		if err != nil {
			return utils.AddContext(err, "couldn't commit transaction")
		}
		s.tx, err = s.db.Begin()
	}

	return err
}

// ProcessChainRevertUpdate implements chain.Subscriber.
func (s *hostDBStore) ProcessChainRevertUpdate(_ *chain.RevertUpdate) error {
	return nil
}

func (s *hostDBStore) getHosts(offset, limit int) (hosts []HostDBEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit == -1 {
		limit = len(s.hosts)
	}
	if offset > len(s.hosts) {
		offset = len(s.hosts)
	}
	if offset+limit > len(s.hosts) {
		limit = len(s.hosts) - offset
	}
	for _, host := range s.hosts {
		if host.ID > offset && host.ID <= offset+limit {
			hosts = append(hosts, *host)
		}
	}
	slices.SortFunc(hosts, func(a, b HostDBEntry) int { return a.ID - b.ID })
	return
}

func (s *hostDBStore) getHostsForScan() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, host := range s.hosts {
		if host.Blocked {
			continue
		}
		if len(host.ScanHistory) == 0 || time.Since(host.ScanHistory[len(host.ScanHistory)-1].Timestamp) >= calculateScanInterval(host) {
			s.hdb.queueScan(host)
			continue
		}
		t := host.LastBenchmark.Timestamp
		if t.IsZero() || time.Since(t) >= s.calculateBenchmarkInterval(host) {
			s.hdb.queueScan(host)
		}
	}
}
