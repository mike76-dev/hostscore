/* wallet */
DROP TABLE IF EXISTS wt_tip;
DROP TABLE IF EXISTS wt_sces;

CREATE TABLE wt_tip (
	network VARCHAR(8) NOT NULL,
	height  BIGINT UNSIGNED NOT NULL,
	bid     BINARY(32) NOT NULL,
	PRIMARY KEY (network)
);

CREATE TABLE wt_sces (
	scoid   BINARY(32) NOT NULL,
	network VARCHAR(8) NOT NULL,
	bytes   BLOB NOT NULL,
	PRIMARY KEY (scoid)
);

/* hostdb */
DROP TABLE IF EXISTS hdb_domains;
DROP TABLE IF EXISTS hdb_tip;
DROP TABLE IF EXISTS hdb_scans;
DROP TABLE IF EXISTS hdb_benchmarks;
DROP TABLE IF EXISTS hdb_hosts;

CREATE TABLE hdb_hosts (
	id               INT NOT NULL,
	network          VARCHAR(8) NOT NULL,
	public_key       BINARY(32) NOT NULL UNIQUE,
	first_seen       BIGINT NOT NULL,
	known_since      BIGINT UNSIGNED NOT NULL,
	blocked          BOOL NOT NULL,
	v2               BOOL NOT NULL,
	net_address      VARCHAR(255) NOT NULL,
	uptime           BIGINT NOT NULL,
	downtime         BIGINT NOT NULL,
	last_seen        BIGINT NOT NULL,
	ip_nets          TEXT NOT NULL,
	last_ip_change   BIGINT NOT NULL,
	successes        DOUBLE NOT NULL,
	failures         DOUBLE NOT NULL,
	last_update      BIGINT UNSIGNED NOT NULL,
	revision         BLOB,
	settings         BLOB,
	price_table      BLOB,
	siamux_addresses TEXT NOT NULL,
	modified         BIGINT NOT NULL,
	fetched          BIGINT NOT NULL,
	PRIMARY KEY (id, network),
	INDEX idx_hosts (network, public_key)
);

CREATE TABLE hdb_scans (
	id           BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
	network      VARCHAR(8) NOT NULL,
	public_key   BINARY(32) NOT NULL,
	ran_at       BIGINT NOT NULL,
	success      BOOL NOT NULL,
	latency      DOUBLE NOT NULL,
	error        TEXT NOT NULL,
	v2           BOOL NOT NULL,
	settings     BLOB,
	price_table  BLOB,
	modified     BIGINT NOT NULL,
	fetched      BIGINT NOT NULL,
	PRIMARY KEY (id),
	FOREIGN KEY (network, public_key) REFERENCES hdb_hosts(network, public_key)
);

CREATE TABLE hdb_benchmarks (
	id             BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
	network        VARCHAR(8) NOT NULL,
	public_key     BINARY(32) NOT NULL,
	ran_at         BIGINT NOT NULL,
	success        BOOL NOT NULL,
	upload_speed   DOUBLE NOT NULL,
	download_speed DOUBLE NOT NULL,
	ttfb           DOUBLE NOT NULL,
	error          TEXT NOT NULL,
	modified       BIGINT NOT NULL,
	fetched        BIGINT NOT NULL,
	PRIMARY KEY (id),
	FOREIGN KEY (network, public_key) REFERENCES hdb_hosts(network, public_key)
);

CREATE TABLE hdb_tip (
	network VARCHAR(8) NOT NULL,
	height  BIGINT UNSIGNED NOT NULL,
	bid     BINARY(32) NOT NULL,
	PRIMARY KEY (network)
);

CREATE TABLE hdb_domains (
	dom VARCHAR(255) NOT NULL
);

INSERT INTO hdb_domains (dom)
VALUES
	('45.148.30.56'),
	('51.158.108.244'),
	('siacentral.ddnsfree.com'),
	('siacentral.mooo.com');
