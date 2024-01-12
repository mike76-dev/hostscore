package hostdb

import (
	"database/sql"
	"errors"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/mike76-dev/hostscore/internal/utils"
	"github.com/mike76-dev/hostscore/persist"
	"go.sia.tech/core/chain"
	"go.sia.tech/core/types"
)

type hostDBStore struct {
	db      *sql.DB
	tx      *sql.Tx
	log     *persist.Logger
	network string

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
			failed_interactions,
			successful_interactions,
			last_seen,
			ip_nets,
			last_ip_change
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) AS new
		ON DUPLICATE KEY UPDATE
			first_seen = new.first_seen,
			known_since = new.known_since,
			blocked = new.blocked,
			net_address = new.net_address,
			uptime = new.uptime,
			downtime = new.downtime,
			failed_interactions = new.failed_interactions,
			successful_interactions = new.successful_interactions,
			last_seen = new.last_seen,
			ip_nets = new.ip_nets,
			last_ip_change = new.last_ip_change
	`,
		host.ID,
		host.PublicKey[:],
		host.FirstSeen.Unix(),
		host.KnownSince,
		host.Blocked,
		host.NetAddress,
		int64(host.Uptime.Seconds()),
		int64(host.Downtime.Seconds()),
		host.SuccessfulInteractions,
		host.FailedInteractions,
		host.LastSeen.Unix(),
		strings.Join(host.IPNets, ";"),
		host.LastIPChange.Unix(),
	)
	return err
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
			successful_interactions,
			failed_interactions,
			last_seen,
			ip_nets,
			last_ip_change
		FROM hdb_hosts_` + s.network,
	)
	if err != nil {
		return utils.AddContext(err, "couldn't query hosts")
	}

	for rows.Next() {
		var id int
		pk := make([]byte, 32)
		var ks uint64
		var b bool
		var na, ip string
		var ut, dt, fs, ls, lc int64
		var si, fi float64
		if err := rows.Scan(&id, &pk, &fs, &ks, &b, &na, &ut, &dt, &si, &fi, &ls, &ip, &lc); err != nil {
			rows.Close()
			return utils.AddContext(err, "couldn't scan host data")
		}
		host := &HostDBEntry{
			ID:                     id,
			FirstSeen:              time.Unix(fs, 0),
			KnownSince:             ks,
			Blocked:                b,
			NetAddress:             na,
			Uptime:                 time.Duration(ut) * time.Second,
			Downtime:               time.Duration(dt) * time.Second,
			SuccessfulInteractions: si,
			FailedInteractions:     fi,
			LastSeen:               time.Unix(ls, 0),
			IPNets:                 strings.Split(ip, ";"),
			LastIPChange:           time.Unix(lc, 0),
		}
		copy(host.PublicKey[:], pk)
		s.mu.Lock()
		if host.Blocked || s.blockedDomains.isBlocked(host.NetAddress) {
			host.Blocked = true
			s.blockedHosts[host.PublicKey] = struct{}{}
		}
		s.mu.Unlock()

		scanRows, err := s.db.Query(`
			SELECT ran_at, rhp2, rhp3, latency_rhp2, latency_rhp3, settings, price_table
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
			var rhp2, rhp3 bool
			var l2, l3 float64
			var settings, pt []byte
			if err := scanRows.Scan(&ra, &rhp2, &rhp3, &l2, &l3, &settings, &pt); err != nil {
				scanRows.Close()
				rows.Close()
				return utils.AddContext(err, "couldn't load scan history")
			}
			scan := HostDBScan{
				Timestamp:   time.Unix(ra, 0),
				RHP2:        rhp2,
				RHP3:        rhp3,
				LatencyRHP2: time.Duration(l2) * time.Millisecond,
				LatencyRHP3: time.Duration(l3) * time.Millisecond,
			}
			d := types.NewBufDecoder(settings)
			utils.DecodeSettings(&scan.Settings, d)
			if d.Err() != nil {
				scanRows.Close()
				rows.Close()
				return utils.AddContext(err, "couldn't decode host settings")
			}
			d = types.NewBufDecoder(pt)
			utils.DecodePriceTable(&scan.PriceTable, d)
			if d.Err() != nil {
				scanRows.Close()
				rows.Close()
				return utils.AddContext(err, "couldn't decode host price table")
			}
			host.ScanHistory = append(host.ScanHistory, scan)
		}
		scanRows.Close()
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
			host := s.hosts[pk]
			if host == nil {
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
			host := s.hosts[pk]
			if host == nil {
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
