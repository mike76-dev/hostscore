DROP TABLE IF EXISTS locations;
DROP TABLE IF EXISTS scans;
DROP TABLE IF EXISTS benchmarks;
DROP TABLE IF EXISTS hosts;

CREATE TABLE hosts (
	id             INT NOT NULL,
	network        VARCHAR(8) NOT NULL,
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
	settings       BLOB,
	price_table    BLOB,
	PRIMARY KEY (id)
);

CREATE TABLE scans (
	id           BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
	network      VARCHAR(8) NOT NULL,
	public_key   BINARY(32) NOT NULL UNIQUE,
	ran_at       BIGINT NOT NULL,
	success      BOOL NOT NULL,
	latency      DOUBLE NOT NULL,
	error        TEXT NOT NULL,
	settings     BLOB,
	price_table  BLOB,
	PRIMARY KEY (id)
);

CREATE TABLE benchmarks (
	id             BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
	network        VARCHAR(8) NOT NULL,
	public_key     BINARY(32) NOT NULL UNIQUE,
	ran_at         BIGINT NOT NULL,
	success        BOOL NOT NULL,
	upload_speed   DOUBLE NOT NULL,
	download_speed DOUBLE NOT NULL,
	ttfb           DOUBLE NOT NULL,
	error          TEXT NOT NULL,
	PRIMARY KEY (id)
);

CREATE TABLE locations (
	public_key BINARY(32) NOT NULL,
	ip         TEXT NOT NULL,
	host_name  TEXT NOT NULL,
	city       TEXT NOT NULL,
	region     TEXT NOT NULL,
	country    TEXT NOT NULL,
	loc        TEXT NOT NULL,
	isp        TEXT NOT NULL,
	zip        TEXT NOT NULL,
	time_zone  TEXT NOT NULL,
	fetched_at BIGINT NOT NULL,
	PRIMARY KEY (public_key)
);