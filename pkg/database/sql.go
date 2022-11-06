package database

import (
	"database/sql"
	"strings"
	"time"

	"github.com/gjolly/go-rmadison/pkg/debianpkg"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

// DB is a package databse
type DB struct {
	*sql.DB

	tableName       string
	transaction     *sql.Tx
	packageInfoChan chan *debianpkg.PackageInfo
	log             *zap.SugaredLogger
}

// NewConn initialize a connection to the DB
func NewConn(driver, path string, log *zap.SugaredLogger) (*DB, error) {
	rawdb, err := sql.Open(driver, path)
	if err != nil {
		return nil, err
	}
	db := &DB{
		rawdb,
		"packages",
		nil,
		make(chan *debianpkg.PackageInfo, 10000),
		log,
	}

	err = db.setupDB(driver)
	if err != nil {
		return nil, err
	}

	err = db.createTableIfNeeded()
	if err != nil {
		return nil, err
	}

	go db.writer(5 * time.Second)

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
		db.log.Info("Table already exist, skipping creation")
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
		'depends' VARCHAR(200) NULL,
		'pre_depends' VARCHAR(200) NULL,
		'replace' VARCHAR(200) NULL,
		'conflicts' VARCHAR(200) NULL,
		'suggests' VARCHAR(200) NULL,
		'description' VARCHAR(64) NULL,
		'archive_url' VARCHAR(64) NOT NULL,
		PRIMARY KEY ('name', 'component', 'suite', 'pocket', 'architecture', 'archive_url')
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

	db.log.Infof("Table '%v' created", db.tableName)

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
			depends    string
			predepends string
			replaces   string
			conflicts  string
			suggests   string
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
			&depends,
			&predepends,
			&replaces,
			&conflicts,
			&suggests,
			&info.Description,
			&info.ArchiveURL,
		)
		if err != nil {
			return nil, err
		}

		info.Depends = strings.Split(depends, ", ")
		info.PreDepends = strings.Split(predepends, ", ")
		info.Suggests = strings.Split(suggests, ", ")
		info.Replaces = strings.Split(replaces, ", ")
		info.Conflicts = strings.Split(conflicts, ", ")

		pkgInfo = append(pkgInfo, info)
	}

	return pkgInfo, rows.Err()
}

// InsertPackage inserts a package into the DB
func (db *DB) InsertPackage(pkgInfo *debianpkg.PackageInfo) {
	db.packageInfoChan <- pkgInfo
}

func (db *DB) writer(forceFlushDuration time.Duration) {
	nbPkg := 0
	t := time.NewTicker(forceFlushDuration)
	for {
		select {
		case pkgInfo := <-db.packageInfoChan:
			if pkgInfo == nil {
				return
			}

			if nbPkg == 10000 {
				err := db.flush()
				if err != nil {
					db.log.Error(err)
				}
				nbPkg = 0
				continue
			}

			err := db.insetPkgInTransaction(pkgInfo)
			if err != nil {
				db.log.Error(err)
			}
		case <-t.C:
			err := db.flush()
			if err != nil {
				db.log.Error(err)
			}
		}
	}
}

// insetPkgInTransaction add a statement in the prepared list
// but do not commit anything to the db
func (db *DB) insetPkgInTransaction(pkgInfo *debianpkg.PackageInfo) error {
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

	_, err = db.transaction.Exec("INSERT OR REPLACE INTO packages VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
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
		strings.Join(pkgInfo.Depends, ", "),
		strings.Join(pkgInfo.PreDepends, ", "),
		strings.Join(pkgInfo.Replaces, ", "),
		strings.Join(pkgInfo.Conflicts, ", "),
		strings.Join(pkgInfo.Suggests, ", "),
		pkgInfo.Description,
		pkgInfo.ArchiveURL,
	)

	return err
}

// flush commits the current transaction
func (db *DB) flush() error {
	if db.transaction == nil {
		db.log.Info("No transaction in progress, doing nothing")
		return nil
	}

	err := db.transaction.Commit()
	if err != nil {
		db.transaction.Rollback()
		return err
	}

	db.transaction = nil
	return nil
}
