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
