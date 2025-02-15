DROP TABLE IF EXISTS locations;
DROP TABLE IF EXISTS scans;
DROP TABLE IF EXISTS benchmarks;
DROP TABLE IF EXISTS interactions;
DROP TABLE IF EXISTS price_changes;
DROP TABLE IF EXISTS hosts;

CREATE TABLE hosts (
	id                 INT NOT NULL,
	network            VARCHAR(8) NOT NULL,
	public_key         BINARY(32) NOT NULL UNIQUE,
	first_seen         BIGINT NOT NULL,
	known_since        BIGINT UNSIGNED NOT NULL,
	blocked            BOOL NOT NULL,
	v2                 BOOL NOT NULL,
	net_address        VARCHAR(255) NOT NULL,
	ip_nets            TEXT NOT NULL,
	last_ip_change     BIGINT NOT NULL,
    price_score        DOUBLE NOT NULL,
    storage_score      DOUBLE NOT NULL,
    collateral_score   DOUBLE NOT NULL,
    interactions_score DOUBLE NOT NULL,
    uptime_score       DOUBLE NOT NULL,
    age_score          DOUBLE NOT NULL,
    version_score      DOUBLE NOT NULL,
    latency_score      DOUBLE NOT NULL,
    benchmarks_score   DOUBLE NOT NULL,
    contracts_score    DOUBLE NOT NULL,
    total_score        DOUBLE NOT NULL,
	settings           BLOB,
	price_table        BLOB,
	PRIMARY KEY (id, network)
);

CREATE TABLE interactions (
	network            VARCHAR(8) NOT NULL,
	node               VARCHAR(8) NOT NULL,
	public_key         BINARY(32) NOT NULL,
	uptime             BIGINT NOT NULL,
	downtime           BIGINT NOT NULL,
	last_seen          BIGINT NOT NULL,
    active_hosts       INT NOT NULL,
    price_score        DOUBLE NOT NULL,
    storage_score      DOUBLE NOT NULL,
    collateral_score   DOUBLE NOT NULL,
    interactions_score DOUBLE NOT NULL,
    uptime_score       DOUBLE NOT NULL,
    age_score          DOUBLE NOT NULL,
    version_score      DOUBLE NOT NULL,
    latency_score      DOUBLE NOT NULL,
    benchmarks_score   DOUBLE NOT NULL,
    contracts_score    DOUBLE NOT NULL,
    total_score        DOUBLE NOT NULL,
	successes          DOUBLE NOT NULL,
	failures           DOUBLE NOT NULL,
	last_update        BIGINT UNSIGNED NOT NULL,
	PRIMARY KEY (network, node, public_key),
    FOREIGN KEY (public_key) REFERENCES hosts(public_key),
    INDEX idx_interactions (network, public_key)
);

CREATE TABLE scans (
	id         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
	network    VARCHAR(8) NOT NULL,
	node       VARCHAR(8) NOT NULL,
	public_key BINARY(32) NOT NULL,
	ran_at     BIGINT NOT NULL,
	success    BOOL NOT NULL,
	latency    DOUBLE NOT NULL,
	error      TEXT NOT NULL,
	PRIMARY KEY (id),
    FOREIGN KEY (public_key) REFERENCES hosts(public_key),
    INDEX idx_scans (network, node, public_key, ran_at)
);

CREATE TABLE benchmarks (
	id             BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
	network        VARCHAR(8) NOT NULL,
	node           VARCHAR(8) NOT NULL,
	public_key     BINARY(32) NOT NULL,
	ran_at         BIGINT NOT NULL,
	success        BOOL NOT NULL,
	upload_speed   DOUBLE NOT NULL,
	download_speed DOUBLE NOT NULL,
	ttfb           DOUBLE NOT NULL,
	error          TEXT NOT NULL,
	PRIMARY KEY (id),
    FOREIGN KEY (public_key) REFERENCES hosts(public_key)
);

CREATE TABLE price_changes (
    id                BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    network           VARCHAR(8) NOT NULL,
    public_key        BINARY(32) NOT NULL,
    changed_at        BIGINT NOT NULL,
    remaining_storage BIGINT UNSIGNED NOT NULL,
    total_storage     BIGINT UNSIGNED NOT NULL,
    collateral        TINYBLOB NOT NULL,
    storage_price     TINYBLOB NOT NULL,
    upload_price      TINYBLOB NOT NULL,
    download_price    TINYBLOB NOT NULL,
    PRIMARY KEY (id),
    FOREIGN KEY (public_key) REFERENCES hosts(public_key)
);

CREATE TABLE locations (
    network    VARCHAR(8) NOT NULL,
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
	PRIMARY KEY (network, public_key)
);
