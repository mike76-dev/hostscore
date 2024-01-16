/* wallet */
DROP TABLE IF EXISTS wt_tip_mainnet;
DROP TABLE IF EXISTS wt_sces_mainnet;
DROP TABLE IF EXISTS wt_sfes_mainnet;
DROP TABLE IF EXISTS wt_tip_zen;
DROP TABLE IF EXISTS wt_sces_zen;
DROP TABLE IF EXISTS wt_sfes_zen;
DROP TABLE IF EXISTS wt_tip_anagami;
DROP TABLE IF EXISTS wt_sces_anagami;
DROP TABLE IF EXISTS wt_sfes_anagami;

CREATE TABLE wt_tip_mainnet (
	id     INT NOT NULL AUTO_INCREMENT,
	height BIGINT UNSIGNED NOT NULL,
	bid    BINARY(32) NOT NULL,
	PRIMARY KEY (id)
);

CREATE TABLE wt_sces_mainnet (
	scoid BINARY(32) NOT NULL,
	bytes BLOB NOT NULL,
	PRIMARY KEY (scoid)
);

CREATE TABLE wt_sfes_mainnet (
	sfoid BINARY(32) NOT NULL,
	bytes BLOB NOT NULL,
	PRIMARY KEY (sfoid)
);

CREATE TABLE wt_tip_zen (
	id     INT NOT NULL AUTO_INCREMENT,
	height BIGINT UNSIGNED NOT NULL,
	bid    BINARY(32) NOT NULL,
	PRIMARY KEY (id)
);

CREATE TABLE wt_sces_zen (
	scoid BINARY(32) NOT NULL,
	bytes BLOB NOT NULL,
	PRIMARY KEY (scoid)
);

CREATE TABLE wt_sfes_zen (
	sfoid BINARY(32) NOT NULL,
	bytes BLOB NOT NULL,
	PRIMARY KEY (sfoid)
);

CREATE TABLE wt_tip_anagami (
	id     INT NOT NULL AUTO_INCREMENT,
	height BIGINT UNSIGNED NOT NULL,
	bid    BINARY(32) NOT NULL,
	PRIMARY KEY (id)
);

CREATE TABLE wt_sces_anagami (
	scoid BINARY(32) NOT NULL,
	bytes BLOB NOT NULL,
	PRIMARY KEY (scoid)
);

CREATE TABLE wt_sfes_anagami (
	sfoid BINARY(32) NOT NULL,
	bytes BLOB NOT NULL,
	PRIMARY KEY (sfoid)
);

/* hostdb */
DROP TABLE IF EXISTS hdb_scans_mainnet;
DROP TABLE IF EXISTS hdb_hosts_mainnet;
DROP TABLE IF EXISTS hdb_domains_mainnet;
DROP TABLE IF EXISTS hdb_tip_mainnet;
DROP TABLE IF EXISTS hdb_scans_zen;
DROP TABLE IF EXISTS hdb_hosts_zen;
DROP TABLE IF EXISTS hdb_domains_zen;
DROP TABLE IF EXISTS hdb_tip_zen;
DROP TABLE IF EXISTS hdb_scans_anagami;
DROP TABLE IF EXISTS hdb_hosts_anagami;
DROP TABLE IF EXISTS hdb_domains_anagami;
DROP TABLE IF EXISTS hdb_tip_anagami;

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

CREATE TABLE hdb_domains_mainnet (
	dom VARCHAR(255) NOT NULL
);

CREATE TABLE hdb_tip_mainnet (
	id               INT NOT NULL AUTO_INCREMENT,
	height           BIGINT UNSIGNED NOT NULL,
	bid              BINARY(32) NOT NULL,
	PRIMARY KEY (id)
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

CREATE TABLE hdb_domains_zen (
	dom VARCHAR(255) NOT NULL
);

CREATE TABLE hdb_tip_zen (
	id               INT NOT NULL AUTO_INCREMENT,
	height           BIGINT UNSIGNED NOT NULL,
	bid              BINARY(32) NOT NULL,
	PRIMARY KEY (id)
);

CREATE TABLE hdb_hosts_anagami (
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
	PRIMARY KEY (id)
);

CREATE TABLE hdb_scans_anagami (
	id           BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
	public_key   BINARY(32) NOT NULL,
	ran_at       BIGINT NOT NULL,
	success      BOOL NOT NULL,
	latency      DOUBLE NOT NULL,
	error        TEXT NOT NULL,
	settings     BLOB,
	price_table  BLOB,
	PRIMARY KEY (id),
	FOREIGN KEY (public_key) REFERENCES hdb_hosts_anagami(public_key)
);

CREATE TABLE hdb_domains_anagami (
	dom VARCHAR(255) NOT NULL
);

CREATE TABLE hdb_tip_anagami (
	id               INT NOT NULL AUTO_INCREMENT,
	height           BIGINT UNSIGNED NOT NULL,
	bid              BINARY(32) NOT NULL,
	PRIMARY KEY (id)
);
