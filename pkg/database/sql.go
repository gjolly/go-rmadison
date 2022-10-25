package database

import (
	"database/sql"
	"strings"

	"github.com/gjolly/go-rmadison/pkg/debianpkg"
	"github.com/pkg/errors"
)

// DB is a package databse
type DB struct {
	*sql.DB

	tableName   string
	transaction *sql.Tx
}

// NewConn initialize a connection to the DB
func NewConn(driver, path string) (*DB, error) {
	rawdb, err := sql.Open(driver, path)
	if err != nil {
		return nil, err
	}
	db := &DB{
		rawdb,
		"packages",
		nil,
	}

	err = db.setupDB(driver)

	err = db.createTableIfNeeded()
	if err != nil {
		return nil, err
	}

	return db, nil
}

func (db *DB) setupDB(driver string) error {
	if driver == "sqlite3" {
		// don't block reads
		// see https://www.sqlite.org/wal.html
		_, err := db.Exec("PRAGMA journal_mode=WAL")
		return err
	}

	return nil
}

func (db *DB) createTableIfNeeded() error {
	res, err := db.Query("SELECT COUNT(name) FROM sqlite_master WHERE type='table' AND name=?", db.tableName)
	if err != nil {
		return errors.Wrap(err, "failed to get tables from DB")
	}
	res.Next()
	var n int
	res.Scan(&n)
	res.Close()

	if n != 0 {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}

	_, err = tx.Exec(`CREATE TABLE packages (
		'name' VARCHAR(64) NOT NULL,
		'version' VARCHAR(64) NOT NULL,
		'component' VARCHAR(64) NOT NULL,
		'suite' VARCHAR(64) NOT NULL,
		'pocket' VARCHAR(64) NOT NULL,
		'architecture' VARCHAR(10) NOT NULL,
		'source' VARCHAR(64) NULL,
		'section' VARCHAR(64) NULL,
		'maintainer_name' VARCHAR(64) NULL,
		'maintainer_email' VARCHAR(64) NULL,
		'sha256' VARCHAR(65) NOT NULL,
		'size' INTEGER NOT NULL,
		'install_size' VARCHAR(64) NULL,
		'file_name' VARCHAR(64) NOT NULL,
		'replace' VARCHAR(64) NULL,
		'conflicts' VARCHAR(64) NULL,
		'suggests' VARCHAR(64) NULL,
		'description' VARCHAR(64) NULL,
		PRIMARY KEY ('name', 'component', 'suite', 'pocket', 'architecture')
	)`)
	if err != nil {
		tx.Rollback()
		return errors.Wrap(err, "Failed to create tables in DB")
	}

	_, err = tx.Exec("CREATE INDEX idx_name ON packages (name)")
	if err != nil {
		tx.Rollback()
		return errors.Wrap(err, "failed to create index")
	}

	return tx.Commit()
}

// GetPackage from the db
func (db *DB) GetPackage(pkgName string) ([]*debianpkg.PackageInfo, error) {
	rows, err := db.Query("SELECT * FROM packages WHERE name=?", pkgName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	pkgInfo := make([]*debianpkg.PackageInfo, 0)

	for rows.Next() {
		info := new(debianpkg.PackageInfo)
		info.Maintainer = new(debianpkg.PackageMaintainer)

		var (
			replaces  string
			conflicts string
			suggests  string
		)

		err = rows.Scan(
			&info.Name,
			&info.Version,
			&info.Component,
			&info.Suite,
			&info.Pocket,
			&info.Architecture,
			&info.Source,
			&info.Section,
			&info.Maintainer.Name,
			&info.Maintainer.Email,
			&info.SHA256,
			&info.Size,
			&info.InstalledSize,
			&info.FileName,
			&replaces,
			&conflicts,
			&suggests,
			&info.Description,
		)
		if err != nil {
			return nil, err
		}

		info.Suggests = strings.Split(suggests, ", ")
		info.Replaces = strings.Split(replaces, ", ")
		info.Conflicts = strings.Split(conflicts, ", ")

		pkgInfo = append(pkgInfo, info)
	}

	return pkgInfo, rows.Err()
}

// PrepareInsertPackage add a statement in the prepared list
// but do not commit anything to the db
func (db *DB) PrepareInsertPackage(pkgInfo *debianpkg.PackageInfo) error {
	var err error

	if db.transaction == nil {
		db.transaction, err = db.Begin()
		if err != nil {
			return errors.Wrap(err, "cannot start transaction, something is bad")
		}
	}

	var (
		maintainerName  string
		maintainerEmail string
	)

	if pkgInfo.Maintainer != nil {
		maintainerName = pkgInfo.Maintainer.Name
		maintainerEmail = pkgInfo.Maintainer.Email
	}

	_, err = db.transaction.Exec("INSERT OR REPLACE INTO packages VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		pkgInfo.Name,
		pkgInfo.Version,
		pkgInfo.Component,
		pkgInfo.Suite,
		pkgInfo.Pocket,
		pkgInfo.Architecture,
		pkgInfo.Source,
		pkgInfo.Section,
		maintainerName,
		maintainerEmail,
		pkgInfo.SHA256,
		pkgInfo.Size,
		pkgInfo.InstalledSize,
		pkgInfo.FileName,
		strings.Join(pkgInfo.Replaces, ", "),
		strings.Join(pkgInfo.Conflicts, ", "),
		strings.Join(pkgInfo.Suggests, ", "),
		pkgInfo.Description,
	)

	return err
}

// InsertPrepared commit the current transaction
func (db *DB) InsertPrepared() error {
	if db.transaction == nil {
		return errors.New("no transaction in progress")
	}

	err := db.transaction.Commit()
	if err != nil {
		db.transaction.Rollback()
		return err
	}

	db.transaction = nil
	return nil
}

// InsertPackage to the db
func (db *DB) InsertPackage(pkgInfo *debianpkg.PackageInfo) error {
	t, err := db.Begin()
	if err != nil {
		return err
	}

	res, err := t.Exec(
		"UPDATE packages SET name = ?, version = ?, component = ?, suite = ?, pocket = ?, architecture = ?, source = ?, section = ?, maintainer_name = ?, maintainer_email = ?, sha256 = ?, size = ?, install_size = ?, file_name = ?, replace = ?, conflicts = ?, suggests = ?, description = ? where name = ? AND component = ? AND suite = ? AND pocket = ? AND architecture = ?",
		// update
		pkgInfo.Name,
		pkgInfo.Version,
		pkgInfo.Component,
		pkgInfo.Suite,
		pkgInfo.Pocket,
		pkgInfo.Architecture,
		pkgInfo.Source,
		pkgInfo.Section,
		pkgInfo.Maintainer.Name,
		pkgInfo.Maintainer.Email,
		pkgInfo.SHA256,
		pkgInfo.Size,
		pkgInfo.InstalledSize,
		pkgInfo.FileName,
		strings.Join(pkgInfo.Replaces, ", "),
		strings.Join(pkgInfo.Conflicts, ", "),
		strings.Join(pkgInfo.Suggests, ", "),
		pkgInfo.Description,
		// where
		pkgInfo.Name,
		pkgInfo.Component,
		pkgInfo.Suite,
		pkgInfo.Pocket,
		pkgInfo.Architecture,
	)
	if err != nil {
		t.Rollback()
		return err
	}

	n, err := res.RowsAffected()
	if err != nil {
		t.Rollback()
		return err
	}
	if n != 0 {
		t.Commit()
		return nil
	}

	// no row updated, we need to insert new data
	res, err = t.Exec(
		"INSERT INTO packages VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		pkgInfo.Name,
		pkgInfo.Version,
		pkgInfo.Component,
		pkgInfo.Suite,
		pkgInfo.Pocket,
		pkgInfo.Architecture,
		pkgInfo.Source,
		pkgInfo.Section,
		pkgInfo.Maintainer.Name,
		pkgInfo.Maintainer.Email,
		pkgInfo.SHA256,
		pkgInfo.Size,
		pkgInfo.InstalledSize,
		pkgInfo.FileName,
		strings.Join(pkgInfo.Replaces, ", "),
		strings.Join(pkgInfo.Conflicts, ", "),
		strings.Join(pkgInfo.Suggests, ", "),
		pkgInfo.Description,
	)
	if err != nil {
		t.Rollback()
		return err
	}

	err = t.Commit()

	return err
}
