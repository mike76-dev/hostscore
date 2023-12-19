package database

import (
	"database/sql"
	"errors"
	"fmt"
	"log"

	"github.com/mike76-dev/hostscore/internal/utils"
	"go.sia.tech/core/chain"
)

// MySQLDB is a wrapper around the MySQL database that
// implements chain.DB interface.
type MySQLDB struct {
	db     *sql.DB
	tx     *sql.Tx
	log    *log.Logger
	dbName string
}

// NewMySQLDB returns an initialized MySQLDB object.
func NewMySQLDB(db *sql.DB, name string, logger *log.Logger) *MySQLDB {
	return &MySQLDB{
		db:     db,
		dbName: name,
		log:    logger,
	}
}

// newTx starts a new transaction if it soesn't exist yet.
func (mdb *MySQLDB) newTx() (err error) {
	if mdb.tx == nil {
		mdb.tx, err = mdb.db.Begin()
	}
	return
}

// Bucket implements chain.DB.
func (mdb *MySQLDB) Bucket(name []byte) chain.DBBucket {
	if err := mdb.newTx(); err != nil {
		panic(err)
	}

	var count int
	if err := mdb.tx.QueryRow(`
		SELECT COUNT(*)
		FROM information_schema.tables
		WHERE table_schema = ?
		AND table_name = ?
	`, mdb.dbName, string(name)).Scan(&count); err != nil {
		panic(err)
	}
	if count == 0 {
		return nil
	}

	return &MySQLBucket{
		db:    mdb,
		table: string(name),
	}
}

// CreateBucket implements chain.DB.
func (mdb *MySQLDB) CreateBucket(name []byte) (chain.DBBucket, error) {
	if err := mdb.newTx(); err != nil {
		return nil, err
	}

	rawQuery := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			table_key BLOB NOT NULL,
			value     LONGBLOB NOT NULL,
			PRIMARY KEY (table_key(32) ASC)
		)
	`, string(name))
	_, err := mdb.tx.Exec(rawQuery)
	if err != nil {
		return nil, err
	}

	return &MySQLBucket{
		db:    mdb,
		table: string(name),
	}, nil
}

// Flush implements chain.DB.
func (mdb *MySQLDB) Flush() error {
	if mdb.tx == nil {
		return nil
	}

	if err := mdb.tx.Commit(); err != nil {
		return err
	}

	mdb.tx = nil
	return nil
}

// Cancel implements chain.DB.
func (mdb *MySQLDB) Cancel() {
	if mdb.tx == nil {
		return
	}

	mdb.tx.Rollback()
	mdb.tx = nil
}

// Close closes the database.
func (mdb *MySQLDB) Close() error {
	return utils.ComposeErrors(mdb.Flush(), mdb.db.Close())
}

// MySQLBucket is a wrapper around a MySQL table that
// implements chain.DBBucket interface.
type MySQLBucket struct {
	db    *MySQLDB
	table string
}

// Get implements chain.DBBucket.
func (bucket MySQLBucket) Get(key []byte) []byte {
	if err := bucket.db.newTx(); err != nil {
		bucket.db.log.Println("ERROR: Get failed:", err)
		return nil
	}

	var value []byte
	rawQuery := fmt.Sprintf(`
		SELECT value
		FROM %s
		WHERE table_key = ?
	`, bucket.table)
	err := bucket.db.tx.QueryRow(rawQuery, key).Scan(&value)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		bucket.db.log.Println("ERROR: Get failed:", err)
	}

	return value
}

// Put implements chain.DBBucket.
func (bucket MySQLBucket) Put(key, value []byte) error {
	if err := bucket.db.newTx(); err != nil {
		bucket.db.log.Println("ERROR: Put failed:", err)
		return nil
	}

	rawQuery := fmt.Sprintf(`
		INSERT INTO %s (table_key, value)
		VALUES (?, ?) AS new
		ON DUPLICATE KEY UPDATE value = new.value
	`, bucket.table)
	_, err := bucket.db.tx.Exec(rawQuery, key, value)

	return err
}

// Delete implements chain.DBBucket.
func (bucket MySQLBucket) Delete(key []byte) error {
	if err := bucket.db.newTx(); err != nil {
		bucket.db.log.Println("ERROR: Delete failed:", err)
		return nil
	}

	rawQuery := fmt.Sprintf(`
		DELETE FROM %s
		WHERE table_key = ?
	`, bucket.table)
	_, err := bucket.db.tx.Exec(rawQuery, key)

	return err
}
