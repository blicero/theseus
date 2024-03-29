// /home/krylon/go/src/github.com/blicero/theseus/database/db.go
// -*- mode: go; coding: utf-8; -*-
// Created on 01. 07. 2022 by Benjamin Walkenhorst
// (c) 2022 Benjamin Walkenhorst
// Time-stamp: <2022-10-01 18:39:12 krylon>

// Package database provides persistence for the application's data.
package database

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"regexp"
	"sync"
	"time"

	"github.com/blicero/krylib"
	"github.com/blicero/theseus/common"
	"github.com/blicero/theseus/database/query"
	"github.com/blicero/theseus/logdomain"
	"github.com/blicero/theseus/objects"
	"github.com/blicero/theseus/objects/repeat"

	_ "github.com/mattn/go-sqlite3" // Import the database driver
)

var (
	openLock sync.Mutex
	idCnt    int64
)

// ErrTxInProgress indicates that an attempt to initiate a transaction failed
// because there is already one in progress.
var ErrTxInProgress = errors.New("A Transaction is already in progress")

// ErrNoTxInProgress indicates that an attempt was made to finish a
// transaction when none was active.
var ErrNoTxInProgress = errors.New("There is no transaction in progress")

// ErrEmptyUpdate indicates that an update operation would not change any
// values.
var ErrEmptyUpdate = errors.New("Update operation does not change any values")

// ErrInvalidValue indicates that one or more parameters passed to a method
// had values that are invalid for that operation.
var ErrInvalidValue = errors.New("Invalid value for parameter")

// ErrObjectNotFound indicates that an Object was not found in the database.
var ErrObjectNotFound = errors.New("object was not found in database")

// ErrInvalidSavepoint is returned when a user of the Database uses an unkown
// (or expired) savepoint name.
var ErrInvalidSavepoint = errors.New("that save point does not exist")

// If a query returns an error and the error text is matched by this regex, we
// consider the error as transient and try again after a short delay.
var retryPat = regexp.MustCompile("(?i)database is (?:locked|busy)")

// worthARetry returns true if an error returned from the database
// is matched by the retryPat regex.
func worthARetry(e error) bool {
	return retryPat.MatchString(e.Error())
} // func worthARetry(e error) bool

// retryDelay is the amount of time we wait before we repeat a database
// operation that failed due to a transient error.
const retryDelay = 25 * time.Millisecond

func waitForRetry() {
	time.Sleep(retryDelay)
} // func waitForRetry()

// Database is the storage backend for managing podcasts audio books.
//
// It is not safe to share a Database instance between goroutines, however
// opening multiple connections to the same Database is safe.
type Database struct {
	id            int64
	db            *sql.DB
	tx            *sql.Tx
	log           *log.Logger
	path          string
	spNameCounter int
	spNameCache   map[string]string
	queries       map[query.ID]*sql.Stmt
}

// Open opens a Database. If the database specified by the path does not exist,
// yet, it is created and initialized.
func Open(path string) (*Database, error) {
	var (
		err      error
		dbExists bool
		db       = &Database{
			path:          path,
			spNameCounter: 1,
			spNameCache:   make(map[string]string),
			queries:       make(map[query.ID]*sql.Stmt),
		}
	)

	openLock.Lock()
	defer openLock.Unlock()
	idCnt++
	db.id = idCnt

	if db.log, err = common.GetLogger(logdomain.Database); err != nil {
		return nil, err
	} else if common.Debug {
		db.log.Printf("[DEBUG] Open database %s\n", path)
	}

	var connstring = fmt.Sprintf("%s?_locking=NORMAL&_journal=WAL&_fk=1&recursive_triggers=0",
		path)

	if dbExists, err = krylib.Fexists(path); err != nil {
		db.log.Printf("[ERROR] Failed to check if %s already exists: %s\n",
			path,
			err.Error())
		return nil, err
	} else if db.db, err = sql.Open("sqlite3", connstring); err != nil {
		db.log.Printf("[ERROR] Failed to open %s: %s\n",
			path,
			err.Error())
		return nil, err
	}

	if !dbExists {
		if err = db.initialize(); err != nil {
			var e2 error
			if e2 = db.db.Close(); e2 != nil {
				db.log.Printf("[CRITICAL] Failed to close database: %s\n",
					e2.Error())
				return nil, e2
			} else if e2 = os.Remove(path); e2 != nil {
				db.log.Printf("[CRITICAL] Failed to remove database file %s: %s\n",
					db.path,
					e2.Error())
			}
			return nil, err
		}
		db.log.Printf("[INFO] Database at %s has been initialized\n",
			path)
	}

	return db, nil
} // func Open(path string) (*Database, error)

func (db *Database) initialize() error {
	var err error
	var tx *sql.Tx

	if common.Debug {
		db.log.Printf("[DEBUG] Initialize fresh database at %s\n",
			db.path)
	}

	if tx, err = db.db.Begin(); err != nil {
		db.log.Printf("[ERROR] Cannot begin transaction: %s\n",
			err.Error())
		return err
	}

	for _, q := range initQueries {
		db.log.Printf("[TRACE] Execute init query:\n%s\n",
			q)
		if _, err = tx.Exec(q); err != nil {
			db.log.Printf("[ERROR] Cannot execute init query: %s\n%s\n",
				err.Error(),
				q)
			if rbErr := tx.Rollback(); rbErr != nil {
				db.log.Printf("[CANTHAPPEN] Cannot rollback transaction: %s\n",
					rbErr.Error())
				return rbErr
			}
			return err
		}
	}

	if err = tx.Commit(); err != nil {
		db.log.Printf("[CANTHAPPEN] Failed to commit init transaction: %s\n",
			err.Error())
		return err
	}

	return nil
} // func (db *Database) initialize() error

// Close closes the database.
// If there is a pending transaction, it is rolled back.
func (db *Database) Close() error {
	// I wonder if would make more snese to panic() if something goes wrong

	var err error

	if db.tx != nil {
		if err = db.tx.Rollback(); err != nil {
			db.log.Printf("[CRITICAL] Cannot roll back pending transaction: %s\n",
				err.Error())
			return err
		}
		db.tx = nil
	}

	for key, stmt := range db.queries {
		if err = stmt.Close(); err != nil {
			db.log.Printf("[CRITICAL] Cannot close statement handle %s: %s\n",
				key,
				err.Error())
			return err
		}
		delete(db.queries, key)
	}

	if err = db.db.Close(); err != nil {
		db.log.Printf("[CRITICAL] Cannot close database: %s\n",
			err.Error())
	}

	db.db = nil
	return nil
} // func (db *Database) Close() error

func (db *Database) getQuery(id query.ID) (*sql.Stmt, error) {
	var (
		stmt  *sql.Stmt
		found bool
		err   error
	)

	if stmt, found = db.queries[id]; found {
		return stmt, nil
	} else if _, found = dbQueries[id]; !found {
		return nil, fmt.Errorf("Unknown Query %d",
			id)
	}

	db.log.Printf("[TRACE] Prepare query %s\n", id)

PREPARE_QUERY:
	if stmt, err = db.db.Prepare(dbQueries[id]); err != nil {
		if worthARetry(err) {
			waitForRetry()
			goto PREPARE_QUERY
		}

		db.log.Printf("[ERROR] Cannor parse query %s: %s\n%s\n",
			id,
			err.Error(),
			dbQueries[id])
		return nil, err
	}

	db.queries[id] = stmt
	return stmt, nil
} // func (db *Database) getQuery(query.ID) (*sql.Stmt, error)

func (db *Database) resetSPNamespace() {
	db.spNameCounter = 1
	db.spNameCache = make(map[string]string)
} // func (db *Database) resetSPNamespace()

func (db *Database) generateSPName(name string) string {
	var spname = fmt.Sprintf("Savepoint%05d",
		db.spNameCounter)

	db.spNameCache[name] = spname
	db.spNameCounter++
	return spname
} // func (db *Database) generateSPName() string

// PerformMaintenance performs some maintenance operations on the database.
// It cannot be called while a transaction is in progress and will block
// pretty much all access to the database while it is running.
func (db *Database) PerformMaintenance() error {
	var mQueries = []string{
		"PRAGMA wal_checkpoint(TRUNCATE)",
		"VACUUM",
		"REINDEX",
		"ANALYZE",
	}
	var err error

	if db.tx != nil {
		return ErrTxInProgress
	}

	for _, q := range mQueries {
		if _, err = db.db.Exec(q); err != nil {
			db.log.Printf("[ERROR] Failed to execute %s: %s\n",
				q,
				err.Error())
		}
	}

	return nil
} // func (db *Database) PerformMaintenance() error

// Begin begins an explicit database transaction.
// Only one transaction can be in progress at once, attempting to start one,
// while another transaction is already in progress will yield ErrTxInProgress.
func (db *Database) Begin() error {
	var err error

	db.log.Printf("[DEBUG] Database#%d Begin Transaction\n",
		db.id)

	if db.tx != nil {
		return ErrTxInProgress
	}

BEGIN_TX:
	for db.tx == nil {
		if db.tx, err = db.db.Begin(); err != nil {
			if worthARetry(err) {
				waitForRetry()
				continue BEGIN_TX
			} else {
				db.log.Printf("[ERROR] Failed to start transaction: %s\n",
					err.Error())
				return err
			}
		}
	}

	db.resetSPNamespace()

	return nil
} // func (db *Database) Begin() error

// SavepointCreate creates a savepoint with the given name.
//
// Savepoints only make sense within a running transaction, and just like
// with explicit transactions, managing them is the responsibility of the
// user of the Database.
//
// Creating a savepoint without a surrounding transaction is not allowed,
// even though SQLite allows it.
//
// For details on how Savepoints work, check the excellent SQLite
// documentation, but here's a quick guide:
//
// Savepoints are kind-of-like transactions within a transaction: One
// can create a savepoint, make some changes to the database, and roll
// back to that savepoint, discarding all changes made between
// creating the savepoint and rolling back to it. Savepoints can be
// quite useful, but there are a few things to keep in mind:
//
// - Savepoints exist within a transaction. When the surrounding transaction
//   is finished, all savepoints created within that transaction cease to exist,
//   no matter if the transaction is commited or rolled back.
//
// - When the database is recovered after being interrupted during a
//   transaction, e.g. by a power outage, the entire transaction is rolled back,
//   including all savepoints that might exist.
//
// - When a savepoint is released, nothing changes in the state of the
//   surrounding transaction. That means rolling back the surrounding
//   transaction rolls back the entire transaction, regardless of any
//   savepoints within.
//
// - Savepoints do not nest. Releasing a savepoint releases it and *all*
//   existing savepoints that have been created before it. Rolling back to a
//   savepoint removes that savepoint and all savepoints created after it.
func (db *Database) SavepointCreate(name string) error {
	var err error

	db.log.Printf("[DEBUG] SavepointCreate(%s)\n",
		name)

	if db.tx == nil {
		return ErrNoTxInProgress
	}

SAVEPOINT:
	// It appears that the SAVEPOINT statement does not support placeholders.
	// But I do want to used named savepoints.
	// And I do want to use the given name so that no SQL injection
	// becomes possible.
	// It would be nice if the database package or at least the SQLite
	// driver offered a way to escape the string properly.
	// One possible solution would be to use names generated by the
	// Database instead of user-defined names.
	//
	// But then I need a way to use the Database-generated name
	// in rolling back and releasing the savepoint.
	// I *could* use the names strictly inside the Database, store them in
	// a map or something and hand out a key to that name to the user.
	// Since savepoint only exist within one transaction, I could even
	// re-use names from one transaction to the next.
	//
	// Ha! I could accept arbitrary names from the user, generate a
	// clean name, and store these in a map. That way the user can
	// still choose names that are outwardly visible, but they do
	// not touch the Database itself.
	//
	//if _, err = db.tx.Exec("SAVEPOINT ?", name); err != nil {
	// if _, err = db.tx.Exec("SAVEPOINT " + name); err != nil {
	// 	if worthARetry(err) {
	// 		waitForRetry()
	// 		goto SAVEPOINT
	// 	}

	// 	db.log.Printf("[ERROR] Failed to create savepoint %s: %s\n",
	// 		name,
	// 		err.Error())
	// }

	var internalName = db.generateSPName(name)

	var spQuery = "SAVEPOINT " + internalName

	if _, err = db.tx.Exec(spQuery); err != nil {
		if worthARetry(err) {
			waitForRetry()
			goto SAVEPOINT
		}

		db.log.Printf("[ERROR] Failed to create savepoint %s: %s\n",
			name,
			err.Error())
	}

	return err
} // func (db *Database) SavepointCreate(name string) error

// SavepointRelease releases the Savepoint with the given name, and all
// Savepoints created before the one being release.
func (db *Database) SavepointRelease(name string) error {
	var (
		err                   error
		internalName, spQuery string
		validName             bool
	)

	db.log.Printf("[DEBUG] SavepointRelease(%s)\n",
		name)

	if db.tx != nil {
		return ErrNoTxInProgress
	}

	if internalName, validName = db.spNameCache[name]; !validName {
		db.log.Printf("[ERROR] Attempt to release unknown Savepoint %q\n",
			name)
		return ErrInvalidSavepoint
	}

	db.log.Printf("[DEBUG] Release Savepoint %q (%q)",
		name,
		db.spNameCache[name])

	spQuery = "RELEASE SAVEPOINT " + internalName

SAVEPOINT:
	if _, err = db.tx.Exec(spQuery); err != nil {
		if worthARetry(err) {
			waitForRetry()
			goto SAVEPOINT
		}

		db.log.Printf("[ERROR] Failed to release savepoint %s: %s\n",
			name,
			err.Error())
	} else {
		delete(db.spNameCache, internalName)
	}

	return err
} // func (db *Database) SavepointRelease(name string) error

// SavepointRollback rolls back the running transaction to the given savepoint.
func (db *Database) SavepointRollback(name string) error {
	var (
		err                   error
		internalName, spQuery string
		validName             bool
	)

	db.log.Printf("[DEBUG] SavepointRollback(%s)\n",
		name)

	if db.tx != nil {
		return ErrNoTxInProgress
	}

	if internalName, validName = db.spNameCache[name]; !validName {
		return ErrInvalidSavepoint
	}

	spQuery = "ROLLBACK TO SAVEPOINT " + internalName

SAVEPOINT:
	if _, err = db.tx.Exec(spQuery); err != nil {
		if worthARetry(err) {
			waitForRetry()
			goto SAVEPOINT
		}

		db.log.Printf("[ERROR] Failed to create savepoint %s: %s\n",
			name,
			err.Error())
	}

	delete(db.spNameCache, name)
	return err
} // func (db *Database) SavepointRollback(name string) error

// Rollback terminates a pending transaction, undoing any changes to the
// database made during that transaction.
// If no transaction is active, it returns ErrNoTxInProgress
func (db *Database) Rollback() error {
	var err error

	db.log.Printf("[DEBUG] Database#%d Roll back Transaction\n",
		db.id)

	if db.tx == nil {
		return ErrNoTxInProgress
	} else if err = db.tx.Rollback(); err != nil {
		return fmt.Errorf("Cannot roll back database transaction: %s",
			err.Error())
	}

	db.tx = nil
	db.resetSPNamespace()

	return nil
} // func (db *Database) Rollback() error

// Commit ends the active transaction, making any changes made during that
// transaction permanent and visible to other connections.
// If no transaction is active, it returns ErrNoTxInProgress
func (db *Database) Commit() error {
	var err error

	db.log.Printf("[DEBUG] Database#%d Commit Transaction\n",
		db.id)

	if db.tx == nil {
		return ErrNoTxInProgress
	} else if err = db.tx.Commit(); err != nil {
		return fmt.Errorf("Cannot commit transaction: %s",
			err.Error())
	}

	db.resetSPNamespace()
	db.tx = nil
	return nil
} // func (db *Database) Commit() error

// ReminderAdd adds a Reminder to the Database.
func (db *Database) ReminderAdd(r *objects.Reminder) error {
	const qid query.ID = query.ReminderAdd
	var (
		err    error
		msg    string
		stmt   *sql.Stmt
		tx     *sql.Tx
		res    sql.Result
		status bool
		now    time.Time
	)

	if stmt, err = db.getQuery(qid); err != nil {
		db.log.Printf("[ERROR] Cannot prepare query %s: %s\n",
			qid.String(),
			err.Error())
		return err
	} else if db.tx != nil {
		tx = db.tx
	} else {
	BEGIN_AD_HOC:
		if tx, err = db.db.Begin(); err != nil {
			if worthARetry(err) {
				waitForRetry()
				goto BEGIN_AD_HOC
			} else {
				msg = fmt.Sprintf("Error starting transaction: %s",
					err.Error())
				db.log.Printf("[ERROR] %s\n", msg)
				return errors.New(msg)
			}

		} else {
			defer func() {
				var err2 error
				if status {
					if err2 = tx.Commit(); err2 != nil {
						db.log.Printf("[ERROR] Failed to commit ad-hoc transaction: %s\n",
							err2.Error())
					}
				} else if err2 = tx.Rollback(); err2 != nil {
					db.log.Printf("[ERROR] Rollback of ad-hoc transaction failed: %s\n",
						err2.Error())
				}
			}()
		}
	}

	stmt = tx.Stmt(stmt)
	now = time.Now()

EXEC_QUERY:
	if res, err = stmt.Exec(
		r.Title,
		r.Description,
		r.Timestamp.Unix(),
		r.Recur.Repeat,
		r.Recur.Weekdays(),
		0,
		0,
		r.UniqueID(),
		now.Unix(),
	); err != nil {
		if worthARetry(err) {
			waitForRetry()
			goto EXEC_QUERY
		} else {
			err = fmt.Errorf("Cannot add Reminder %q to database: %s",
				r.Title,
				err.Error())
			db.log.Printf("[ERROR] %s\n", err.Error())
			return err
		}
	} else {
		var programID int64

		if programID, err = res.LastInsertId(); err != nil {
			db.log.Printf("[ERROR] Cannot get ID of new Reminder %s: %s\n",
				r.Title,
				err.Error())
			return err
		}

		status = true
		r.ID = programID
		r.Changed = now
		return nil
	}
} // func (db *Database) ReminderAdd(r *objects.Reminder) error

// ReminderDelete removes a Reminder entry from the database.
func (db *Database) ReminderDelete(r *objects.Reminder) error {
	const qid query.ID = query.ReminderDelete
	var (
		err    error
		msg    string
		stmt   *sql.Stmt
		tx     *sql.Tx
		status bool
	)

	if stmt, err = db.getQuery(qid); err != nil {
		db.log.Printf("[ERROR] Cannot prepare query %s: %s\n",
			qid.String(),
			err.Error())
		return err
	} else if db.tx != nil {
		tx = db.tx
	} else {
	BEGIN_AD_HOC:
		if tx, err = db.db.Begin(); err != nil {
			if worthARetry(err) {
				waitForRetry()
				goto BEGIN_AD_HOC
			} else {
				msg = fmt.Sprintf("Error starting transaction: %s",
					err.Error())
				db.log.Printf("[ERROR] %s\n", msg)
				return errors.New(msg)
			}

		} else {
			defer func() {
				var err2 error
				if status {
					if err2 = tx.Commit(); err2 != nil {
						db.log.Printf("[ERROR] Failed to commit ad-hoc transaction: %s\n",
							err2.Error())
					}
				} else if err2 = tx.Rollback(); err2 != nil {
					db.log.Printf("[ERROR] Rollback of ad-hoc transaction failed: %s\n",
						err2.Error())
				}
			}()
		}
	}

	stmt = tx.Stmt(stmt)

EXEC_QUERY:
	if _, err = stmt.Exec(r.ID); err != nil {
		if worthARetry(err) {
			waitForRetry()
			goto EXEC_QUERY
		} else {
			err = fmt.Errorf("Cannot delete Reminder %s (%d) from database: %s",
				r.Title,
				r.ID,
				err.Error())
			db.log.Printf("[ERROR] %s\n", err.Error())
			return err
		}
	}

	status = true
	return nil
} // func (db *Database) ReminderDelete(r *objects.Reminder) error

// ReminderGetPending fetches all Reminder entries from the database
// that have not been marked as finished.
func (db *Database) ReminderGetPending(t time.Time) ([]objects.Reminder, error) {
	const qid query.ID = query.ReminderGetPending
	var (
		err  error
		stmt *sql.Stmt
	)

	if stmt, err = db.getQuery(qid); err != nil {
		db.log.Printf("[ERROR] Cannot prepare query %s: %s\n",
			qid,
			err.Error())
		return nil, err
	} else if db.tx != nil {
		stmt = db.tx.Stmt(stmt)
	}

	var rows *sql.Rows

EXEC_QUERY:
	if rows, err = stmt.Query(); err != nil {
		if worthARetry(err) {
			waitForRetry()
			goto EXEC_QUERY
		}

		db.log.Printf("[ERROR] Failed to load pending Reminders: %s\n",
			err.Error())

		return nil, err
	}

	defer rows.Close() // nolint: errcheck,gosec

	var items = make([]objects.Reminder, 0)
	var now = time.Now().Truncate(time.Minute)

	for rows.Next() {
		var (
			stamp, changed, days int64
			r                    objects.Reminder
		)

		if err = rows.Scan(
			&r.ID,
			&r.Title,
			&r.Description,
			&stamp,
			&r.Recur.Repeat,
			&days,
			&r.Recur.Counter,
			&r.Recur.Limit,
			&r.UUID,
			&changed); err != nil {
			db.log.Printf("[ERROR] Cannot scan row: %s\n", err.Error())
			return nil, err
		}

		r.Timestamp = time.Unix(stamp, 0)
		r.Changed = time.Unix(changed, 0)
		for i := 0; i < 7; i++ {
			r.Recur.Days[i] = (days & (1 << i)) != 0
		}

		r.Recur.Offset = int(stamp)

		if r.Recur.Repeat != repeat.Once || r.DueNext(&now).Before(t) {
			items = append(items, r)
		}
	}

	return items, nil
} // func (db *Database) ReminderGetPending(t time.Time) ([]objects.Reminder, error)

// ReminderGetPendingWithNotifications fetches all Reminder entries from the database
// that have not been marked as finished.
func (db *Database) ReminderGetPendingWithNotifications(t time.Time) ([]objects.Reminder, error) {
	const qid query.ID = query.ReminderGetPendingWithNotifications
	var (
		err  error
		stmt *sql.Stmt
	)

	if stmt, err = db.getQuery(qid); err != nil {
		db.log.Printf("[ERROR] Cannot prepare query %s: %s\n",
			qid,
			err.Error())
		return nil, err
	} else if db.tx != nil {
		stmt = db.tx.Stmt(stmt)
	}

	var rows *sql.Rows

EXEC_QUERY:
	if rows, err = stmt.Query(); err != nil {
		if worthARetry(err) {
			waitForRetry()
			goto EXEC_QUERY
		}

		db.log.Printf("[ERROR] Failed to load pending Reminders: %s\n",
			err.Error())

		return nil, err
	}

	defer rows.Close() // nolint: errcheck,gosec

	var items = make([]objects.Reminder, 0)
	var now = time.Now().Truncate(time.Minute)

	for rows.Next() {
		var (
			stamp, changed, days int64
			r                    objects.Reminder
		)

		if err = rows.Scan(
			&r.ID,
			&r.Title,
			&r.Description,
			&stamp,
			&r.Recur.Repeat,
			&days,
			&r.Recur.Counter,
			&r.Recur.Limit,
			&r.UUID,
			&changed); err != nil {
			db.log.Printf("[ERROR] Cannot scan row: %s\n", err.Error())
			return nil, err
		}

		r.Timestamp = time.Unix(stamp, 0)
		r.Changed = time.Unix(changed, 0)
		for i := 0; i < 7; i++ {
			r.Recur.Days[i] = (days & (1 << i)) != 0
		}

		r.Recur.Offset = int(stamp)

		if r.Recur.Repeat != repeat.Once || r.DueNext(&now).Before(t) {
			items = append(items, r)
		}
	}

	return items, nil
} // func (db *Database) ReminderGetPendingWithNotifications(t time.Time) ([]objects.Reminder, error)

// ReminderGetAll retrieves all Reminders from the database.
func (db *Database) ReminderGetAll() ([]objects.Reminder, error) {
	const qid query.ID = query.ReminderGetAll
	var (
		err  error
		stmt *sql.Stmt
	)

	if stmt, err = db.getQuery(qid); err != nil {
		db.log.Printf("[ERROR] Cannot prepare query %s: %s\n",
			qid,
			err.Error())
		return nil, err
	} else if db.tx != nil {
		stmt = db.tx.Stmt(stmt)
	}

	var rows *sql.Rows

EXEC_QUERY:
	if rows, err = stmt.Query(); err != nil {
		if worthARetry(err) {
			waitForRetry()
			goto EXEC_QUERY
		}

		db.log.Printf("[ERROR] Failed to load all Reminders: %s\n",
			err.Error())
		return nil, err
	}

	defer rows.Close() // nolint: errcheck,gosec

	var items = make([]objects.Reminder, 0)

	for rows.Next() {
		var (
			stamp, changed, days int64
			r                    objects.Reminder
		)

		if err = rows.Scan(
			&r.ID,
			&r.Title,
			&r.Description,
			&stamp,
			&r.Recur.Repeat,
			&days,
			&r.Recur.Counter,
			&r.Recur.Limit,
			&r.Finished,
			&r.UUID,
			&changed); err != nil {
			db.log.Printf("[ERROR] Cannot scan row: %s\n", err.Error())
			return nil, err
		}

		r.Timestamp = time.Unix(stamp, 0)
		r.Changed = time.Unix(changed, 0)
		r.Recur.Offset = int(stamp)
		for i := 0; i < 7; i++ {
			r.Recur.Days[i] = (days & (1 << i)) != 0
		}

		items = append(items, r)
	}

	return items, nil
} // func (db *Database) ReminderGetAll() ([]objects.Reminder, error)

// ReminderGetFinished returns all the Reminder entries that have already passed
// and been acknowledged by the user.
func (db *Database) ReminderGetFinished() ([]objects.Reminder, error) {
	const qid query.ID = query.ReminderGetFinished
	var (
		err  error
		stmt *sql.Stmt
	)

	if stmt, err = db.getQuery(qid); err != nil {
		db.log.Printf("[ERROR] Cannot prepare query %s: %s\n",
			qid,
			err.Error())
		return nil, err
	} else if db.tx != nil {
		stmt = db.tx.Stmt(stmt)
	}

	var rows *sql.Rows

EXEC_QUERY:
	if rows, err = stmt.Query(); err != nil {
		if worthARetry(err) {
			waitForRetry()
			goto EXEC_QUERY
		}

		return nil, err
	}

	defer rows.Close() // nolint: errcheck,gosec

	var items = make([]objects.Reminder, 0)

	for rows.Next() {
		var (
			stamp, changed, days int64
			r                    objects.Reminder
		)

		if err = rows.Scan(
			&r.ID,
			&r.Title,
			&r.Description,
			&stamp,
			&r.UUID,
			&changed); err != nil {
			db.log.Printf("[ERROR] Cannot scan row: %s\n", err.Error())
			return nil, err
		}

		r.Timestamp = time.Unix(stamp, 0)
		r.Changed = time.Unix(changed, 0)
		r.Recur.Offset = int(stamp)
		for i := 0; i < 7; i++ {
			r.Recur.Days[i] = (days & (1 << i)) != 0
		}
		r.Finished = true

		items = append(items, r)
	}

	return items, nil
} // func (db *Database) ReminderGetFinished() ([]objects.Reminder, error)

// ReminderGetByID looks up a Reminder by its Title
func (db *Database) ReminderGetByID(id int64) (*objects.Reminder, error) {
	const qid query.ID = query.ReminderGetByID
	var (
		err  error
		stmt *sql.Stmt
	)

	if stmt, err = db.getQuery(qid); err != nil {
		db.log.Printf("[ERROR] Cannot prepare query %s: %s\n",
			qid,
			err.Error())
		return nil, err
	} else if db.tx != nil {
		stmt = db.tx.Stmt(stmt)
	}

	var rows *sql.Rows

EXEC_QUERY:
	if rows, err = stmt.Query(id); err != nil {
		if worthARetry(err) {
			waitForRetry()
			goto EXEC_QUERY
		}

		return nil, err
	}

	defer rows.Close() // nolint: errcheck,gosec

	if rows.Next() {
		var (
			stamp, changed, days int64
			r                    = &objects.Reminder{ID: id}
		)

		if err = rows.Scan(
			&r.Title,
			&r.Description,
			&stamp,
			&r.Recur.Repeat,
			&days,
			&r.Recur.Counter,
			&r.Recur.Limit,
			&r.Finished,
			&r.UUID,
			&changed); err != nil {
			db.log.Printf("[ERROR] Cannot scan row: %s\n", err.Error())
			return nil, err
		}

		r.Timestamp = time.Unix(stamp, 0)
		r.Recur.Offset = int(stamp)
		r.Changed = time.Unix(changed, 0)
		for i := 0; i < 7; i++ {
			r.Recur.Days[i] = (days & (1 << i)) != 0
		}
		r.Finished = true

		return r, nil
	}

	return nil, nil
} // func (db *Database) ReminderGetByID(id int64) (*objects.Reminder, error)

// ReminderSetFinished sets the Finished-flag of the given Reminder entry to the
// given state.
func (db *Database) ReminderSetFinished(r *objects.Reminder, flag bool) error {
	const qid query.ID = query.ReminderSetFinished
	var (
		err    error
		msg    string
		stmt   *sql.Stmt
		tx     *sql.Tx
		status bool
		now    time.Time
	)

	if stmt, err = db.getQuery(qid); err != nil {
		db.log.Printf("[ERROR] Cannot prepare query %s: %s\n",
			qid.String(),
			err.Error())
		return err
	} else if db.tx != nil {
		tx = db.tx
	} else {
	BEGIN_AD_HOC:
		if tx, err = db.db.Begin(); err != nil {
			if worthARetry(err) {
				waitForRetry()
				goto BEGIN_AD_HOC
			} else {
				msg = fmt.Sprintf("Error starting transaction: %s",
					err.Error())
				db.log.Printf("[ERROR] %s\n", msg)
				return errors.New(msg)
			}

		} else {
			defer func() {
				var err2 error
				if status {
					if err2 = tx.Commit(); err2 != nil {
						db.log.Printf("[ERROR] Failed to commit ad-hoc transaction: %s\n",
							err2.Error())
					}
				} else if err2 = tx.Rollback(); err2 != nil {
					db.log.Printf("[ERROR] Rollback of ad-hoc transaction failed: %s\n",
						err2.Error())
				}
			}()
		}
	}

	stmt = tx.Stmt(stmt)
	now = time.Now()

EXEC_QUERY:
	if _, err = stmt.Exec(flag, now.Unix(), r.ID); err != nil {
		if worthARetry(err) {
			waitForRetry()
			goto EXEC_QUERY
		} else {
			err = fmt.Errorf("Cannot add Reminder %q to database: %s",
				r.Title,
				err.Error())
			db.log.Printf("[ERROR] %s\n", err.Error())
			return err
		}
	}

	r.Finished = flag
	r.Changed = now
	status = true
	return nil
} // func (db *Database) ReminderSetFinished(r *objects.Reminder, flag bool) error

// ReminderSetTitle sets the Finished-flag of the given Reminder entry to the
// given state.
func (db *Database) ReminderSetTitle(r *objects.Reminder, title string) error {
	const qid query.ID = query.ReminderSetTitle
	var (
		err    error
		msg    string
		stmt   *sql.Stmt
		tx     *sql.Tx
		status bool
		now    time.Time
	)

	if stmt, err = db.getQuery(qid); err != nil {
		db.log.Printf("[ERROR] Cannot prepare query %s: %s\n",
			qid.String(),
			err.Error())
		return err
	} else if db.tx != nil {
		tx = db.tx
	} else {
	BEGIN_AD_HOC:
		if tx, err = db.db.Begin(); err != nil {
			if worthARetry(err) {
				waitForRetry()
				goto BEGIN_AD_HOC
			} else {
				msg = fmt.Sprintf("Error starting transaction: %s",
					err.Error())
				db.log.Printf("[ERROR] %s\n", msg)
				return errors.New(msg)
			}

		} else {
			defer func() {
				var err2 error
				if status {
					if err2 = tx.Commit(); err2 != nil {
						db.log.Printf("[ERROR] Failed to commit ad-hoc transaction: %s\n",
							err2.Error())
					}
				} else if err2 = tx.Rollback(); err2 != nil {
					db.log.Printf("[ERROR] Rollback of ad-hoc transaction failed: %s\n",
						err2.Error())
				}
			}()
		}
	}

	stmt = tx.Stmt(stmt)
	now = time.Now()

EXEC_QUERY:
	if _, err = stmt.Exec(title, now.Unix(), r.ID); err != nil {
		if worthARetry(err) {
			waitForRetry()
			goto EXEC_QUERY
		} else {
			err = fmt.Errorf("Cannot add Reminder %q to database: %s",
				r.Title,
				err.Error())
			db.log.Printf("[ERROR] %s\n", err.Error())
			return err
		}
	}

	r.Title = title
	r.Changed = now
	status = true
	return nil
} // func (db *Database) ReminderSetTitle(r *objects.Reminder, title string) error

// ReminderSetTimestamp updates a Reminder's timestamp to the given value.
func (db *Database) ReminderSetTimestamp(r *objects.Reminder, t time.Time) error {
	const qid query.ID = query.ReminderSetTimestamp
	var (
		err    error
		msg    string
		stmt   *sql.Stmt
		tx     *sql.Tx
		status bool
		now    time.Time
	)

	if stmt, err = db.getQuery(qid); err != nil {
		db.log.Printf("[ERROR] Cannot prepare query %s: %s\n",
			qid.String(),
			err.Error())
		return err
	} else if db.tx != nil {
		tx = db.tx
	} else {
	BEGIN_AD_HOC:
		if tx, err = db.db.Begin(); err != nil {
			if worthARetry(err) {
				waitForRetry()
				goto BEGIN_AD_HOC
			} else {
				msg = fmt.Sprintf("Error starting transaction: %s",
					err.Error())
				db.log.Printf("[ERROR] %s\n", msg)
				return errors.New(msg)
			}

		} else {
			defer func() {
				var err2 error
				if status {
					if err2 = tx.Commit(); err2 != nil {
						db.log.Printf("[ERROR] Failed to commit ad-hoc transaction: %s\n",
							err2.Error())
					}
				} else if err2 = tx.Rollback(); err2 != nil {
					db.log.Printf("[ERROR] Rollback of ad-hoc transaction failed: %s\n",
						err2.Error())
				}
			}()
		}
	}

	stmt = tx.Stmt(stmt)
	now = time.Now()

EXEC_QUERY:
	if _, err = stmt.Exec(t.Unix(), now.Unix(), r.ID); err != nil {
		if worthARetry(err) {
			waitForRetry()
			goto EXEC_QUERY
		} else {
			err = fmt.Errorf("Cannot add Reminder %q to database: %s",
				r.Title,
				err.Error())
			db.log.Printf("[ERROR] %s\n", err.Error())
			return err
		}
	}

	r.Timestamp = t
	r.Changed = now
	status = true
	return nil
} // func (db *Database) ReminderSetTimestamp(r *objects.Reminder, t time.Time) error

// ReminderSetDescription updates a Reminder's Description.
func (db *Database) ReminderSetDescription(r *objects.Reminder, desc string) error {
	const qid query.ID = query.ReminderSetDescription
	var (
		err    error
		msg    string
		stmt   *sql.Stmt
		tx     *sql.Tx
		status bool
		now    time.Time
	)

	if stmt, err = db.getQuery(qid); err != nil {
		db.log.Printf("[ERROR] Cannot prepare query %s: %s\n",
			qid.String(),
			err.Error())
		return err
	} else if db.tx != nil {
		tx = db.tx
	} else {
	BEGIN_AD_HOC:
		if tx, err = db.db.Begin(); err != nil {
			if worthARetry(err) {
				waitForRetry()
				goto BEGIN_AD_HOC
			} else {
				msg = fmt.Sprintf("Error starting transaction: %s",
					err.Error())
				db.log.Printf("[ERROR] %s\n", msg)
				return errors.New(msg)
			}

		} else {
			defer func() {
				var err2 error
				if status {
					if err2 = tx.Commit(); err2 != nil {
						db.log.Printf("[ERROR] Failed to commit ad-hoc transaction: %s\n",
							err2.Error())
					}
				} else if err2 = tx.Rollback(); err2 != nil {
					db.log.Printf("[ERROR] Rollback of ad-hoc transaction failed: %s\n",
						err2.Error())
				}
			}()
		}
	}

	stmt = tx.Stmt(stmt)
	now = time.Now()

EXEC_QUERY:
	if _, err = stmt.Exec(desc, now.Unix(), r.ID); err != nil {
		if worthARetry(err) {
			waitForRetry()
			goto EXEC_QUERY
		} else {
			err = fmt.Errorf("Cannot add Reminder %q to database: %s",
				r.Title,
				err.Error())
			db.log.Printf("[ERROR] %s\n", err.Error())
			return err
		}
	}

	r.Description = desc
	r.Changed = now
	status = true
	return nil
} // func (db *Database) ReminderSetDescription(r *objects.Reminder, desc string) error

// ReminderReactivate updates a Reminder's timestamp to the given value.
func (db *Database) ReminderReactivate(r *objects.Reminder, t time.Time) error {
	const qid query.ID = query.ReminderReactivate
	var (
		err    error
		msg    string
		stmt   *sql.Stmt
		tx     *sql.Tx
		status bool
		now    time.Time
	)

	if stmt, err = db.getQuery(qid); err != nil {
		db.log.Printf("[ERROR] Cannot prepare query %s: %s\n",
			qid.String(),
			err.Error())
		return err
	} else if db.tx != nil {
		tx = db.tx
	} else {
	BEGIN_AD_HOC:
		if tx, err = db.db.Begin(); err != nil {
			if worthARetry(err) {
				waitForRetry()
				goto BEGIN_AD_HOC
			} else {
				msg = fmt.Sprintf("Error starting transaction: %s",
					err.Error())
				db.log.Printf("[ERROR] %s\n", msg)
				return errors.New(msg)
			}

		} else {
			defer func() {
				var err2 error
				if status {
					if err2 = tx.Commit(); err2 != nil {
						db.log.Printf("[ERROR] Failed to commit ad-hoc transaction: %s\n",
							err2.Error())
					}
				} else if err2 = tx.Rollback(); err2 != nil {
					db.log.Printf("[ERROR] Rollback of ad-hoc transaction failed: %s\n",
						err2.Error())
				}
			}()
		}
	}

	stmt = tx.Stmt(stmt)
	now = time.Now()

EXEC_QUERY:
	if _, err = stmt.Exec(t.Unix(), now.Unix(), r.ID); err != nil {
		if worthARetry(err) {
			waitForRetry()
			goto EXEC_QUERY
		} else {
			err = fmt.Errorf("Cannot add Reminder %q to database: %s",
				r.Title,
				err.Error())
			db.log.Printf("[ERROR] %s\n", err.Error())
			return err
		}
	}

	r.Timestamp = t
	r.Changed = now
	r.Finished = false
	status = true
	return nil
} // func (db *Database) ReminderReactivate(r *objects.Reminder, t time.Time) error

// ReminderSetChanged sets the Reminder's ctime to a specific point in time,
// used for synchronization.
func (db *Database) ReminderSetChanged(r *objects.Reminder, t time.Time) error {
	const qid query.ID = query.ReminderSetChanged
	var (
		err    error
		msg    string
		stmt   *sql.Stmt
		tx     *sql.Tx
		status bool
	)

	if stmt, err = db.getQuery(qid); err != nil {
		db.log.Printf("[ERROR] Cannot prepare query %s: %s\n",
			qid.String(),
			err.Error())
		return err
	} else if db.tx != nil {
		tx = db.tx
	} else {
	BEGIN_AD_HOC:
		if tx, err = db.db.Begin(); err != nil {
			if worthARetry(err) {
				waitForRetry()
				goto BEGIN_AD_HOC
			} else {
				msg = fmt.Sprintf("Error starting transaction: %s",
					err.Error())
				db.log.Printf("[ERROR] %s\n", msg)
				return errors.New(msg)
			}

		} else {
			defer func() {
				var err2 error
				if status {
					if err2 = tx.Commit(); err2 != nil {
						db.log.Printf("[ERROR] Failed to commit ad-hoc transaction: %s\n",
							err2.Error())
					}
				} else if err2 = tx.Rollback(); err2 != nil {
					db.log.Printf("[ERROR] Rollback of ad-hoc transaction failed: %s\n",
						err2.Error())
				}
			}()
		}
	}

	stmt = tx.Stmt(stmt)

EXEC_QUERY:
	if _, err = stmt.Exec(t.Unix(), r.ID); err != nil {
		if worthARetry(err) {
			waitForRetry()
			goto EXEC_QUERY
		} else {
			err = fmt.Errorf("Cannot add Reminder %q to database: %s",
				r.Title,
				err.Error())
			db.log.Printf("[ERROR] %s\n", err.Error())
			return err
		}
	}

	r.Changed = t
	status = true
	return nil
} // func (db *Database) ReminderSetChanged(r *objects.Reminder, t time.Time) error

// ReminderSetRepeat sets the Reminder's Repeat mode
func (db *Database) ReminderSetRepeat(r *objects.Reminder, c repeat.Repeat) error {
	const qid query.ID = query.ReminderSetRepeat
	var (
		err    error
		msg    string
		stmt   *sql.Stmt
		tx     *sql.Tx
		status bool
	)

	if stmt, err = db.getQuery(qid); err != nil {
		db.log.Printf("[ERROR] Cannot prepare query %s: %s\n",
			qid.String(),
			err.Error())
		return err
	} else if db.tx != nil {
		tx = db.tx
	} else {
	BEGIN_AD_HOC:
		if tx, err = db.db.Begin(); err != nil {
			if worthARetry(err) {
				waitForRetry()
				goto BEGIN_AD_HOC
			} else {
				msg = fmt.Sprintf("Error starting transaction: %s",
					err.Error())
				db.log.Printf("[ERROR] %s\n", msg)
				return errors.New(msg)
			}

		} else {
			defer func() {
				var err2 error
				if status {
					if err2 = tx.Commit(); err2 != nil {
						db.log.Printf("[ERROR] Failed to commit ad-hoc transaction: %s\n",
							err2.Error())
					}
				} else if err2 = tx.Rollback(); err2 != nil {
					db.log.Printf("[ERROR] Rollback of ad-hoc transaction failed: %s\n",
						err2.Error())
				}
			}()
		}
	}

	stmt = tx.Stmt(stmt)
	var now = time.Now()

EXEC_QUERY:
	if _, err = stmt.Exec(c, now.Unix(), r.ID); err != nil {
		if worthARetry(err) {
			waitForRetry()
			goto EXEC_QUERY
		} else {
			err = fmt.Errorf("Cannot add Reminder %q to database: %s",
				r.Title,
				err.Error())
			db.log.Printf("[ERROR] %s\n", err.Error())
			return err
		}
	}

	r.Recur.Repeat = c
	r.Changed = now
	status = true
	return nil
} // func (db *Database) ReminderSetRepeat(r *objects.Reminder, c objects.Recurrence) error

// ReminderSetWeekdays sets the Weekdays the Reminder is to go off, assuming it is
// set to the appropriate repeat mode.
func (db *Database) ReminderSetWeekdays(r *objects.Reminder, days objects.Weekdays) error {
	const qid query.ID = query.ReminderSetWeekdays
	var (
		err    error
		msg    string
		stmt   *sql.Stmt
		tx     *sql.Tx
		status bool
	)

	if stmt, err = db.getQuery(qid); err != nil {
		db.log.Printf("[ERROR] Cannot prepare query %s: %s\n",
			qid.String(),
			err.Error())
		return err
	} else if db.tx != nil {
		tx = db.tx
	} else {
	BEGIN_AD_HOC:
		if tx, err = db.db.Begin(); err != nil {
			if worthARetry(err) {
				waitForRetry()
				goto BEGIN_AD_HOC
			} else {
				msg = fmt.Sprintf("Error starting transaction: %s",
					err.Error())
				db.log.Printf("[ERROR] %s\n", msg)
				return errors.New(msg)
			}

		} else {
			defer func() {
				var err2 error
				if status {
					if err2 = tx.Commit(); err2 != nil {
						db.log.Printf("[ERROR] Failed to commit ad-hoc transaction: %s\n",
							err2.Error())
					}
				} else if err2 = tx.Rollback(); err2 != nil {
					db.log.Printf("[ERROR] Rollback of ad-hoc transaction failed: %s\n",
						err2.Error())
				}
			}()
		}
	}

	stmt = tx.Stmt(stmt)
	var now = time.Now()

EXEC_QUERY:
	if _, err = stmt.Exec(days.Bitfield(), now.Unix(), r.ID); err != nil {
		if worthARetry(err) {
			waitForRetry()
			goto EXEC_QUERY
		} else {
			err = fmt.Errorf("Cannot add Reminder %q to database: %s",
				r.Title,
				err.Error())
			db.log.Printf("[ERROR] %s\n", err.Error())
			return err
		}
	}

	r.Recur.Days = days
	r.Changed = now
	status = true
	return nil
} // func (db *Database) ReminderSetWeekdays(r *objects.Reminder, days objects.Weekdays) error

// ReminderSetLimit sets the repeat limit to the speficied value.
func (db *Database) ReminderSetLimit(r *objects.Reminder, limit int) error {
	const qid query.ID = query.ReminderSetLimit
	var (
		err    error
		msg    string
		stmt   *sql.Stmt
		tx     *sql.Tx
		status bool
	)

	if stmt, err = db.getQuery(qid); err != nil {
		db.log.Printf("[ERROR] Cannot prepare query %s: %s\n",
			qid.String(),
			err.Error())
		return err
	} else if db.tx != nil {
		tx = db.tx
	} else {
	BEGIN_AD_HOC:
		if tx, err = db.db.Begin(); err != nil {
			if worthARetry(err) {
				waitForRetry()
				goto BEGIN_AD_HOC
			} else {
				msg = fmt.Sprintf("Error starting transaction: %s",
					err.Error())
				db.log.Printf("[ERROR] %s\n", msg)
				return errors.New(msg)
			}

		} else {
			defer func() {
				var err2 error
				if status {
					if err2 = tx.Commit(); err2 != nil {
						db.log.Printf("[ERROR] Failed to commit ad-hoc transaction: %s\n",
							err2.Error())
					}
				} else if err2 = tx.Rollback(); err2 != nil {
					db.log.Printf("[ERROR] Rollback of ad-hoc transaction failed: %s\n",
						err2.Error())
				}
			}()
		}
	}

	stmt = tx.Stmt(stmt)
	var now = time.Now()

EXEC_QUERY:
	if _, err = stmt.Exec(limit, now.Unix(), r.ID); err != nil {
		if worthARetry(err) {
			waitForRetry()
			goto EXEC_QUERY
		} else {
			err = fmt.Errorf("Cannot add Reminder %q to database: %s",
				r.Title,
				err.Error())
			db.log.Printf("[ERROR] %s\n", err.Error())
			return err
		}
	}

	r.Recur.Limit = limit
	r.Changed = now
	status = true
	return nil
} // func (db *Database) ReminderSetLimit(r *objects.Reminder, limit int) error

// ReminderResetCounter resets a Reminder's counter to zero.
func (db *Database) ReminderResetCounter(r *objects.Reminder) error {
	const qid query.ID = query.ReminderResetCounter
	var (
		err    error
		msg    string
		stmt   *sql.Stmt
		tx     *sql.Tx
		status bool
	)

	if stmt, err = db.getQuery(qid); err != nil {
		db.log.Printf("[ERROR] Cannot prepare query %s: %s\n",
			qid.String(),
			err.Error())
		return err
	} else if db.tx != nil {
		tx = db.tx
	} else {
	BEGIN_AD_HOC:
		if tx, err = db.db.Begin(); err != nil {
			if worthARetry(err) {
				waitForRetry()
				goto BEGIN_AD_HOC
			} else {
				msg = fmt.Sprintf("Error starting transaction: %s",
					err.Error())
				db.log.Printf("[ERROR] %s\n", msg)
				return errors.New(msg)
			}

		} else {
			defer func() {
				var err2 error
				if status {
					if err2 = tx.Commit(); err2 != nil {
						db.log.Printf("[ERROR] Failed to commit ad-hoc transaction: %s\n",
							err2.Error())
					}
				} else if err2 = tx.Rollback(); err2 != nil {
					db.log.Printf("[ERROR] Rollback of ad-hoc transaction failed: %s\n",
						err2.Error())
				}
			}()
		}
	}

	stmt = tx.Stmt(stmt)
	var now = time.Now()

EXEC_QUERY:
	if _, err = stmt.Exec(now.Unix(), r.ID); err != nil {
		if worthARetry(err) {
			waitForRetry()
			goto EXEC_QUERY
		} else {
			err = fmt.Errorf("Cannot add Reminder %q to database: %s",
				r.Title,
				err.Error())
			db.log.Printf("[ERROR] %s\n", err.Error())
			return err
		}
	}

	r.Recur.Counter = 0
	r.Changed = now
	status = true
	return nil
} // func (db *Database) ReminderResetCounter(r *objects.Reminder) error

// ReminderIncCounter increments the Reminder's counter by 1.
func (db *Database) ReminderIncCounter(r *objects.Reminder) error {
	const qid query.ID = query.ReminderIncCounter
	var (
		err    error
		msg    string
		stmt   *sql.Stmt
		tx     *sql.Tx
		status bool
	)

	if stmt, err = db.getQuery(qid); err != nil {
		db.log.Printf("[ERROR] Cannot prepare query %s: %s\n",
			qid.String(),
			err.Error())
		return err
	} else if db.tx != nil {
		tx = db.tx
	} else {
	BEGIN_AD_HOC:
		if tx, err = db.db.Begin(); err != nil {
			if worthARetry(err) {
				waitForRetry()
				goto BEGIN_AD_HOC
			} else {
				msg = fmt.Sprintf("Error starting transaction: %s",
					err.Error())
				db.log.Printf("[ERROR] %s\n", msg)
				return errors.New(msg)
			}

		} else {
			defer func() {
				var err2 error
				if status {
					if err2 = tx.Commit(); err2 != nil {
						db.log.Printf("[ERROR] Failed to commit ad-hoc transaction: %s\n",
							err2.Error())
					}
				} else if err2 = tx.Rollback(); err2 != nil {
					db.log.Printf("[ERROR] Rollback of ad-hoc transaction failed: %s\n",
						err2.Error())
				}
			}()
		}
	}

	stmt = tx.Stmt(stmt)
	var (
		now  = time.Now()
		rows *sql.Rows
	)

EXEC_QUERY:
	if rows, err = stmt.Query(now.Unix(), r.ID); err != nil {
		if worthARetry(err) {
			waitForRetry()
			goto EXEC_QUERY
		} else {
			err = fmt.Errorf("Cannot add Reminder %q to database: %s",
				r.Title,
				err.Error())
			db.log.Printf("[ERROR] %s\n", err.Error())
			return err
		}
	}

	defer rows.Close() // nolint: errcheck

	rows.Next()

	if err = rows.Scan(&r.Recur.Counter); err != nil {
		db.log.Printf("[ERROR] Cannot scan Counter from Rows: %s\n",
			err.Error())
		return err
	}

	r.Changed = now
	status = true
	return nil
} // func (db *Database) ReminderIncCounter(r *objects.Reminder) error

// NotificationAdd creates a new Notification to be displayed for a recurring Reminder
// at a certain point in time.
func (db *Database) NotificationAdd(r *objects.Reminder, t time.Time) (*objects.Notification, error) {
	const qid query.ID = query.NotificationAdd
	var (
		err    error
		msg    string
		stmt   *sql.Stmt
		tx     *sql.Tx
		status bool
	)

	if stmt, err = db.getQuery(qid); err != nil {
		db.log.Printf("[ERROR] Cannot prepare query %s: %s\n",
			qid.String(),
			err.Error())
		return nil, err
	} else if db.tx != nil {
		tx = db.tx
	} else {
	BEGIN_AD_HOC:
		if tx, err = db.db.Begin(); err != nil {
			if worthARetry(err) {
				waitForRetry()
				goto BEGIN_AD_HOC
			} else {
				msg = fmt.Sprintf("Error starting transaction: %s",
					err.Error())
				db.log.Printf("[ERROR] %s\n", msg)
				return nil, errors.New(msg)
			}

		} else {
			defer func() {
				var err2 error
				if status {
					if err2 = tx.Commit(); err2 != nil {
						db.log.Printf("[ERROR] Failed to commit ad-hoc transaction: %s\n",
							err2.Error())
					}
				} else if err2 = tx.Rollback(); err2 != nil {
					db.log.Printf("[ERROR] Rollback of ad-hoc transaction failed: %s\n",
						err2.Error())
				}
			}()
		}
	}

	stmt = tx.Stmt(stmt)
	var (
		res sql.Result
		not = &objects.Notification{
			ReminderID: r.ID,
			Timestamp:  t,
		}
	)

EXEC_QUERY:
	if res, err = stmt.Exec(r.ID, not.Timestamp.Unix()); err != nil {
		if worthARetry(err) {
			waitForRetry()
			goto EXEC_QUERY
		} else {
			err = fmt.Errorf("Cannot add Notification for %q to database: %s",
				r.Title,
				err.Error())
			db.log.Printf("[ERROR] %s\n", err.Error())
			return nil, err
		}
	} else if not.ID, err = res.LastInsertId(); err != nil {
		db.log.Printf("[ERROR] Cannot get ID of newly added Notification: %s\n",
			err.Error())
		return nil, err
	}

	status = true
	return not, nil
} // func NotificationAdd(r *Reminder, t time.Time) (*objects.Notification, error)

// NotificationDisplay stores the time when a Notification has been last displayed.
func (db *Database) NotificationDisplay(n *objects.Notification, t time.Time) error {
	const qid query.ID = query.NotificationDisplay
	var (
		err    error
		msg    string
		stmt   *sql.Stmt
		tx     *sql.Tx
		status bool
	)

	if stmt, err = db.getQuery(qid); err != nil {
		db.log.Printf("[ERROR] Cannot prepare query %s: %s\n",
			qid.String(),
			err.Error())
		return err
	} else if db.tx != nil {
		tx = db.tx
	} else {
	BEGIN_AD_HOC:
		if tx, err = db.db.Begin(); err != nil {
			if worthARetry(err) {
				waitForRetry()
				goto BEGIN_AD_HOC
			} else {
				msg = fmt.Sprintf("Error starting transaction: %s",
					err.Error())
				db.log.Printf("[ERROR] %s\n", msg)
				return errors.New(msg)
			}

		} else {
			defer func() {
				var err2 error
				if status {
					if err2 = tx.Commit(); err2 != nil {
						db.log.Printf("[ERROR] Failed to commit ad-hoc transaction: %s\n",
							err2.Error())
					}
				} else if err2 = tx.Rollback(); err2 != nil {
					db.log.Printf("[ERROR] Rollback of ad-hoc transaction failed: %s\n",
						err2.Error())
				}
			}()
		}
	}

	stmt = tx.Stmt(stmt)

EXEC_QUERY:
	if _, err = stmt.Exec(t.Unix(), n.ID); err != nil {
		if worthARetry(err) {
			waitForRetry()
			goto EXEC_QUERY
		} else {
			err = fmt.Errorf("Cannot add Notification %d for Reminder %d to database: %s",
				n.ID,
				n.ReminderID,
				err.Error())
			db.log.Printf("[ERROR] %s\n", err.Error())
			return err
		}
	}

	n.Displayed = t
	status = true
	return nil
} // func (db *Database) NotificationDisplay(n *objects.Notification) error

// NotificationAcknowledge stores the time when a Notification has been
// acknowledged by the user and is thus completed.
func (db *Database) NotificationAcknowledge(n *objects.Notification, t time.Time) error {
	const qid query.ID = query.NotificationAcknowledge
	var (
		err    error
		msg    string
		stmt   *sql.Stmt
		tx     *sql.Tx
		status bool
	)

	if stmt, err = db.getQuery(qid); err != nil {
		db.log.Printf("[ERROR] Cannot prepare query %s: %s\n",
			qid.String(),
			err.Error())
		return err
	} else if db.tx != nil {
		tx = db.tx
	} else {
	BEGIN_AD_HOC:
		if tx, err = db.db.Begin(); err != nil {
			if worthARetry(err) {
				waitForRetry()
				goto BEGIN_AD_HOC
			} else {
				msg = fmt.Sprintf("Error starting transaction: %s",
					err.Error())
				db.log.Printf("[ERROR] %s\n", msg)
				return errors.New(msg)
			}

		} else {
			defer func() {
				var err2 error
				if status {
					if err2 = tx.Commit(); err2 != nil {
						db.log.Printf("[ERROR] Failed to commit ad-hoc transaction: %s\n",
							err2.Error())
					}
				} else if err2 = tx.Rollback(); err2 != nil {
					db.log.Printf("[ERROR] Rollback of ad-hoc transaction failed: %s\n",
						err2.Error())
				}
			}()
		}
	}

	stmt = tx.Stmt(stmt)

EXEC_QUERY:
	if _, err = stmt.Exec(t.Unix(), n.ID); err != nil {
		if worthARetry(err) {
			waitForRetry()
			goto EXEC_QUERY
		} else {
			err = fmt.Errorf("Cannot acknowledge Notification %d for Reminder %d: %s",
				n.ID,
				n.ReminderID,
				err.Error())
			db.log.Printf("[ERROR] %s\n", err.Error())
			return err
		}
	}

	n.Acknowledged = t
	status = true
	return nil
} // func (db *Database) NotificationAcknowledge(n *objects.Notification) error

// NotificationGetByID fetches a Notification by its database ID.
func (db *Database) NotificationGetByID(id int64) (*objects.Notification, error) {
	const qid query.ID = query.NotificationGetByID
	var (
		err  error
		stmt *sql.Stmt
	)

	if stmt, err = db.getQuery(qid); err != nil {
		db.log.Printf("[ERROR] Cannot prepare query %s: %s\n",
			qid,
			err.Error())
		return nil, err
	} else if db.tx != nil {
		stmt = db.tx.Stmt(stmt)
	}

	var rows *sql.Rows

EXEC_QUERY:
	if rows, err = stmt.Query(id); err != nil {
		if worthARetry(err) {
			waitForRetry()
			goto EXEC_QUERY
		}

		db.log.Printf("[ERROR] Failed to load all Reminders: %s\n",
			err.Error())
		return nil, err
	}

	defer rows.Close() // nolint: errcheck,gosec

	if rows.Next() {
		var (
			item           = &objects.Notification{ID: id}
			tstamp         int64
			dstamp, astamp *int64
		)

		if err = rows.Scan(&item.ReminderID, &tstamp, &dstamp, &astamp); err != nil {
			db.log.Printf("[ERROR] Cannot scan Row: %s\n",
				err.Error())
			return nil, err
		}

		item.Timestamp = time.Unix(tstamp, 0)
		if dstamp != nil {
			item.Displayed = time.Unix(*dstamp, 0)
		}
		if astamp != nil {
			item.Acknowledged = time.Unix(*astamp, 0)
		}

		return item, nil
	}

	return nil, nil
} // func (db *Database) NotificationGetByID(id int64) (n *objects.Notification, error)

// NotificationGetByReminder fetches all Notifications stored for the given
// Reminder, in reverse chronological order, up to the specified limit.
// If the limit is a negative number, all Notifications will be fetched.
func (db *Database) NotificationGetByReminder(r *objects.Reminder, max int) ([]objects.Notification, error) {
	const qid query.ID = query.NotificationGetByReminder
	var (
		err  error
		stmt *sql.Stmt
		size int
	)

	if max > 0 {
		size = max
	} else {
		size = 32
	}

	if stmt, err = db.getQuery(qid); err != nil {
		db.log.Printf("[ERROR] Cannot prepare query %s: %s\n",
			qid,
			err.Error())
		return nil, err
	} else if db.tx != nil {
		stmt = db.tx.Stmt(stmt)
	}

	var rows *sql.Rows

EXEC_QUERY:
	if rows, err = stmt.Query(r.ID, max); err != nil {
		if worthARetry(err) {
			waitForRetry()
			goto EXEC_QUERY
		}

		db.log.Printf("[ERROR] Failed to load all Reminders: %s\n",
			err.Error())
		return nil, err
	}

	defer rows.Close() // nolint: errcheck,gosec

	var items = make([]objects.Notification, 0, size)

	for rows.Next() {
		var (
			n              = objects.Notification{ReminderID: r.ID}
			tstamp         int64
			dstamp, astamp *int64
		)

		if err = rows.Scan(&n.ID, &tstamp, &dstamp, &astamp); err != nil {
			db.log.Printf("[ERROR] Cannot scan Row: %s\n",
				err.Error())
			return nil, err
		}

		n.Timestamp = time.Unix(tstamp, 0)
		if dstamp != nil {
			n.Displayed = time.Unix(*dstamp, 0)
		}
		if astamp != nil {
			n.Acknowledged = time.Unix(*astamp, 0)
		}

		items = append(items, n)
	}

	return items, nil
} // func (db *Database) NotificationGetByReminder(r *objects.Reminder) ([]objects.Notification, error)

// NotificationGetByReminderStamp fetches a Notification by the given Reminder
// and Timestamp.
func (db *Database) NotificationGetByReminderStamp(r *objects.Reminder, t time.Time) (*objects.Notification, error) {
	const qid query.ID = query.NotificationGetByReminderStamp
	var (
		err  error
		stmt *sql.Stmt
	)

	if stmt, err = db.getQuery(qid); err != nil {
		db.log.Printf("[ERROR] Cannot prepare query %s: %s\n",
			qid,
			err.Error())
		return nil, err
	} else if db.tx != nil {
		stmt = db.tx.Stmt(stmt)
	}

	var rows *sql.Rows

EXEC_QUERY:
	if rows, err = stmt.Query(r.ID, t.Unix()); err != nil {
		if worthARetry(err) {
			waitForRetry()
			goto EXEC_QUERY
		}

		db.log.Printf("[ERROR] Failed to load all Reminders: %s\n",
			err.Error())
		return nil, err
	}

	defer rows.Close() // nolint: errcheck,gosec

	if rows.Next() {
		var (
			item = &objects.Notification{
				ReminderID: r.ID,
				Timestamp:  t,
			}
			tstamp         int64
			dstamp, astamp *int64
		)

		if err = rows.Scan(&item.ID, &dstamp, &astamp); err != nil {
			db.log.Printf("[ERROR] Cannot scan Row: %s\n",
				err.Error())
			return nil, err
		}

		item.Timestamp = time.Unix(tstamp, 0)
		if dstamp != nil {
			item.Displayed = time.Unix(*dstamp, 0)
		}
		if astamp != nil {
			item.Acknowledged = time.Unix(*astamp, 0)
		}

		return item, nil
	}

	return nil, nil
} // func (db *Database) NotificationGetByReminderStamp(r *objects.Reminder, t time.Time) (*objects.Notification, error)

// NotificationGetByReminderPending fetches all Notifications for the
// given Reminder that have not been acknowledged.
func (db *Database) NotificationGetByReminderPending(r *objects.Reminder) ([]objects.Notification, error) {
	const qid query.ID = query.NotificationGetByReminderPending
	var (
		err  error
		stmt *sql.Stmt
	)

	if stmt, err = db.getQuery(qid); err != nil {
		db.log.Printf("[ERROR] Cannot prepare query %s: %s\n",
			qid,
			err.Error())
		return nil, err
	} else if db.tx != nil {
		stmt = db.tx.Stmt(stmt)
	}

	var rows *sql.Rows

EXEC_QUERY:
	if rows, err = stmt.Query(r.ID); err != nil {
		if worthARetry(err) {
			waitForRetry()
			goto EXEC_QUERY
		}

		db.log.Printf("[ERROR] Failed to load all Reminders: %s\n",
			err.Error())
		return nil, err
	}

	defer rows.Close() // nolint: errcheck,gosec

	var items = make([]objects.Notification, 0, 32)

	for rows.Next() {
		var (
			n      = objects.Notification{ReminderID: r.ID}
			tstamp int64
			dstamp *int64
		)

		if err = rows.Scan(&n.ID, &tstamp, &dstamp); err != nil {
			db.log.Printf("[ERROR] Cannot scan Row: %s\n",
				err.Error())
			return nil, err
		}

		n.Timestamp = time.Unix(tstamp, 0)
		if dstamp != nil {
			n.Displayed = time.Unix(*dstamp, 0)
		}

		items = append(items, n)
	}

	return items, nil
} // func (db *Database) NotificationGetByReminderPending(r *objects.Reminder) ([]objects.Notification, error)

// NotificationGetPending fetches all Notifications that have not been
// acknowledged, yet.
func (db *Database) NotificationGetPending() ([]objects.Notification, error) {
	const qid query.ID = query.NotificationGetPending
	var (
		err  error
		stmt *sql.Stmt
	)

	if stmt, err = db.getQuery(qid); err != nil {
		db.log.Printf("[ERROR] Cannot prepare query %s: %s\n",
			qid,
			err.Error())
		return nil, err
	} else if db.tx != nil {
		stmt = db.tx.Stmt(stmt)
	}

	var rows *sql.Rows

EXEC_QUERY:
	if rows, err = stmt.Query(); err != nil {
		if worthARetry(err) {
			waitForRetry()
			goto EXEC_QUERY
		}

		db.log.Printf("[ERROR] Failed to load all Reminders: %s\n",
			err.Error())
		return nil, err
	}

	defer rows.Close() // nolint: errcheck,gosec

	var items = make([]objects.Notification, 0, 32)

	for rows.Next() {
		var (
			n      objects.Notification
			tstamp int64
			dstamp *int64
		)

		if err = rows.Scan(&n.ID, &n.ReminderID, &tstamp, &dstamp); err != nil {
			db.log.Printf("[ERROR] Cannot scan Row: %s\n",
				err.Error())
			return nil, err
		}

		n.Timestamp = time.Unix(tstamp, 0)
		if dstamp != nil {
			n.Displayed = time.Unix(*dstamp, 0)
		}

		items = append(items, n)
	}

	return items, nil
} // func (db *Database) NotificationGetPending() ([]objects.Notification, error)

// NotificationCleanup removes stale notifications from the database
// that are more than a week old and have either been acknowledged
// at least a week ago or never displayed in the first place.
func (db *Database) NotificationCleanup() (int64, error) {
	const qid query.ID = query.NotificationCleanup
	var (
		err    error
		msg    string
		stmt   *sql.Stmt
		tx     *sql.Tx
		status bool
	)

	if stmt, err = db.getQuery(qid); err != nil {
		db.log.Printf("[ERROR] Cannot prepare query %s: %s\n",
			qid.String(),
			err.Error())
		return 0, err
	} else if db.tx != nil {
		tx = db.tx
	} else {
	BEGIN_AD_HOC:
		if tx, err = db.db.Begin(); err != nil {
			if worthARetry(err) {
				waitForRetry()
				goto BEGIN_AD_HOC
			} else {
				msg = fmt.Sprintf("Error starting transaction: %s",
					err.Error())
				db.log.Printf("[ERROR] %s\n", msg)
				return 0, errors.New(msg)
			}

		} else {
			defer func() {
				var err2 error
				if status {
					if err2 = tx.Commit(); err2 != nil {
						db.log.Printf("[ERROR] Failed to commit ad-hoc transaction: %s\n",
							err2.Error())
					}
				} else if err2 = tx.Rollback(); err2 != nil {
					db.log.Printf("[ERROR] Rollback of ad-hoc transaction failed: %s\n",
						err2.Error())
				}
			}()
		}
	}

	stmt = tx.Stmt(stmt)
	var rows *sql.Rows

EXEC_QUERY:
	if rows, err = stmt.Query(); err != nil {
		if worthARetry(err) {
			waitForRetry()
			goto EXEC_QUERY
		} else {
			err = fmt.Errorf("Cannot clean up Notifications: %s",
				err.Error())
			db.log.Printf("[ERROR] %s\n", err.Error())
			return 0, err
		}
	}

	defer rows.Close() // nolint: errcheck
	var cnt int64

	for rows.Next() {
		var n int64

		if err = rows.Scan(&n); err != nil {
			db.log.Printf("[ERROR] Cannot scan Counter from Rows: %s\n",
				err.Error())
			return 0, err
		} else if n > 0 {
			cnt += n
		}
	}

	status = true
	return cnt, nil
} // func (db *Database) NotificationCleanup() (int64, error)
