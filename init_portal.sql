DROP TABLE IF EXISTS locations;

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