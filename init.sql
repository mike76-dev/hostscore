/* wallet */
DROP TABLE IF EXISTS wt_tip;
DROP TABLE IF EXISTS wt_sces;
DROP TABLE IF EXISTS wt_sfes;
DROP TABLE IF EXISTS wt_locked;

CREATE TABLE wt_tip (
	id      INT NOT NULL,
	network VARCHAR(8) NOT NULL,
	height  BIGINT UNSIGNED NOT NULL,
	bid     BINARY(32) NOT NULL,
	PRIMARY KEY (id)
);

CREATE TABLE wt_sces (
	scoid   BINARY(32) NOT NULL,
	network VARCHAR(8) NOT NULL,
	bytes   BLOB NOT NULL,
	PRIMARY KEY (scoid)
);

CREATE TABLE wt_sfes (
	sfoid   BINARY(32) NOT NULL,
	network VARCHAR(8) NOT NULL,
	bytes   BLOB NOT NULL,
	PRIMARY KEY (sfoid)
);

CREATE TABLE wt_locked (
	id    BINARY(32) NOT NULL,
	until BIGINT NOT NULL,
	PRIMARY KEY (id)
);

/* hostdb */
DROP TABLE IF EXISTS hdb_domains;
DROP TABLE IF EXISTS hdb_tip;
DROP TABLE IF EXISTS hdb_scans_mainnet;
DROP TABLE IF EXISTS hdb_benchmarks_mainnet;
DROP TABLE IF EXISTS hdb_hosts_mainnet;
DROP TABLE IF EXISTS hdb_scans_zen;
DROP TABLE IF EXISTS hdb_benchmarks_zen;
DROP TABLE IF EXISTS hdb_hosts_zen;

CREATE TABLE hdb_hosts_mainnet (
	id             INT NOT NULL AUTO_INCREMENT,
	public_key     BINARY(32) NOT NULL UNIQUE,
	first_seen     BIGINT NOT NULL,
	known_since    BIGINT UNSIGNED NOT NULL,
	blocked        BOOL NOT NULL,
	net_address    VARCHAR(255) NOT NULL,
	uptime         BIGINT NOT NULL,
	downtime       BIGINT NOT NULL,
	last_seen      BIGINT NOT NULL,
	ip_nets        TEXT NOT NULL,
	last_ip_change BIGINT NOT NULL,
	historic_successful_interactions DOUBLE NOT NULL,
	historic_failed_interactions     DOUBLE NOT NULL,
	recent_successful_interactions   DOUBLE NOT NULL,
	recent_failed_interactions       DOUBLE NOT NULL,
	last_update                      BIGINT UNSIGNED NOT NULL,
	revision       BLOB,
	settings       BLOB,
	price_table    BLOB,
	PRIMARY KEY (id)
);

CREATE TABLE hdb_scans_mainnet (
	id           BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
	public_key   BINARY(32) NOT NULL,
	ran_at       BIGINT NOT NULL,
	success      BOOL NOT NULL,
	latency      DOUBLE NOT NULL,
	error        TEXT NOT NULL,
	settings     BLOB,
	price_table  BLOB,
	PRIMARY KEY (id),
	FOREIGN KEY (public_key) REFERENCES hdb_hosts_mainnet(public_key)
);

CREATE TABLE hdb_benchmarks_mainnet (
	id             BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
	public_key     BINARY(32) NOT NULL,
	ran_at         BIGINT NOT NULL,
	success        BOOL NOT NULL,
	upload_speed   DOUBLE NOT NULL,
	download_speed DOUBLE NOT NULL,
	ttfb           DOUBLE NOT NULL,
	error          TEXT NOT NULL,
	PRIMARY KEY (id),
	FOREIGN KEY (public_key) REFERENCES hdb_hosts_mainnet(public_key)
);

CREATE TABLE hdb_hosts_zen (
	id             INT NOT NULL AUTO_INCREMENT,
	public_key     BINARY(32) NOT NULL UNIQUE,
	first_seen     BIGINT NOT NULL,
	known_since    BIGINT UNSIGNED NOT NULL,
	blocked        BOOL NOT NULL,
	net_address    VARCHAR(255) NOT NULL,
	uptime         BIGINT NOT NULL,
	downtime       BIGINT NOT NULL,
	last_seen      BIGINT NOT NULL,
	ip_nets        TEXT NOT NULL,
	last_ip_change BIGINT NOT NULL,
	historic_successful_interactions DOUBLE NOT NULL,
	historic_failed_interactions     DOUBLE NOT NULL,
	recent_successful_interactions   DOUBLE NOT NULL,
	recent_failed_interactions       DOUBLE NOT NULL,
	last_update                      BIGINT UNSIGNED NOT NULL,
	revision       BLOB,
	settings       BLOB,
	price_table    BLOB,
	PRIMARY KEY (id)
);

CREATE TABLE hdb_scans_zen (
	id           BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
	public_key   BINARY(32) NOT NULL,
	ran_at       BIGINT NOT NULL,
	success      BOOL NOT NULL,
	latency      DOUBLE NOT NULL,
	error        TEXT NOT NULL,
	settings     BLOB,
	price_table  BLOB,
	PRIMARY KEY (id),
	FOREIGN KEY (public_key) REFERENCES hdb_hosts_zen(public_key)
);

CREATE TABLE hdb_benchmarks_zen (
	id             BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
	public_key     BINARY(32) NOT NULL,
	ran_at         BIGINT NOT NULL,
	success        BOOL NOT NULL,
	upload_speed   DOUBLE NOT NULL,
	download_speed DOUBLE NOT NULL,
	ttfb           DOUBLE NOT NULL,
	error          TEXT NOT NULL,
	PRIMARY KEY (id),
	FOREIGN KEY (public_key) REFERENCES hdb_hosts_zen(public_key)
);

CREATE TABLE hdb_tip (
	id               INT NOT NULL,
	network VARCHAR(8) NOT NULL,
	height           BIGINT UNSIGNED NOT NULL,
	bid              BINARY(32) NOT NULL,
	PRIMARY KEY (id)
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
