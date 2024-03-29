// /home/krylon/go/src/github.com/blicero/theseus/backend/backend.go
// -*- mode: go; coding: utf-8; -*-
// Created on 01. 07. 2022 by Benjamin Walkenhorst
// (c) 2022 Benjamin Walkenhorst
// Time-stamp: <2022-10-01 19:04:04 krylon>

// Package backend implements the ... backend of the application,
// the part that deals with the database and dbus.
package backend

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/blicero/theseus/common"
	"github.com/blicero/theseus/database"
	"github.com/blicero/theseus/logdomain"
	"github.com/blicero/theseus/objects"
	"github.com/blicero/theseus/objects/repeat"
	"github.com/godbus/dbus/v5"
	"github.com/gorilla/mux"
	"github.com/grandcat/zeroconf"
	"github.com/pquerna/ffjson/ffjson"
)

const (
	notifyObj            = "org.freedesktop.Notifications"
	notifyIntf           = "org.freedesktop.Notifications" // nolint: deadcode,unused,varcheck
	notifyPath           = "/org/freedesktop/Notifications"
	notifyMethod         = "org.freedesktop.Notifications.Notify"
	queueDepth           = 5
	queueTimeout         = time.Second * 30
	defaultReminderDelay = time.Second * 300
)

type service struct {
	rr        *zeroconf.ServiceEntry
	timestamp time.Time
}

func (s *service) mkPeer() objects.Peer {
	return objects.Peer{
		Instance: s.rr.Instance,
		Hostname: s.rr.HostName,
		Domain:   s.rr.Domain,
		Port:     s.rr.Port,
	}
} // func (s *service) mkPeer() objects.Peer

func mkService(rr *zeroconf.ServiceEntry) service {
	return service{
		rr:        rr,
		timestamp: time.Now(),
	}
}

func (s *service) isExpired() bool {
	return s.timestamp.Add(time.Second * time.Duration(s.rr.TTL)).Before(time.Now())
} // func (s *service) IsExpired() bool

// Daemon is the centerpiece of the backend, coordinating between the database, the clients, etc.
type Daemon struct {
	log        *log.Logger
	pool       *database.Pool
	bus        *dbus.Conn
	lock       sync.RWMutex // nolint: structcheck,unused
	active     bool
	hostname   string
	Queue      chan *objects.Reminder
	web        http.Server
	router     *mux.Router
	mimeTypes  map[string]string
	listenAddr string
	idLock     sync.Mutex
	idCnt      int64
	signalQ    chan *dbus.Signal
	nLock      sync.RWMutex
	pending    map[uint32]int64
	dnssd      *zeroconf.Server
	pLock      sync.RWMutex
	peers      map[string]service
}

// Summon summons a Daemon and returns it. No sacrifice or idolatry is required.
func Summon(addr string) (*Daemon, error) {
	var (
		err error
		d   = &Daemon{
			listenAddr: addr,
			active:     true,
			Queue:      make(chan *objects.Reminder, queueDepth),
			router:     mux.NewRouter(),
			mimeTypes: map[string]string{
				".css":  "text/css",
				".map":  "application/json",
				".js":   "text/javascript",
				".png":  "image/png",
				".jpg":  "image/jpeg",
				".jpeg": "image/jpeg",
				".webp": "image/webp",
				".gif":  "image/gif",
				".json": "application/json",
				".html": "text/html",
			},
			pending: make(map[uint32]int64),
			peers:   make(map[string]service),
		}
	)

	if d.log, err = common.GetLogger(logdomain.Backend); err != nil {
		fmt.Printf("ERROR initializing Logger: %s\n",
			err.Error())
		return nil, err
	} else if d.pool, err = database.NewPool(4); err != nil {
		d.log.Printf("[ERROR] Cannot initialize database pool: %s\n",
			err.Error())
		return nil, err
	} else if d.bus, err = dbus.SessionBus(); err != nil {
		d.log.Printf("[ERROR] Failed to connect to DBus Session bus: %s\n",
			err.Error())
		return nil, err
	} else if d.hostname, err = os.Hostname(); err != nil {
		d.log.Printf("[ERROR] Cannot query system hostname: %s\n",
			err.Error())
		return nil, err
	}

	d.web.Addr = addr
	d.web.ErrorLog = d.log
	d.web.Handler = d.router

	if err = d.bus.AddMatchSignal(
		dbus.WithMatchObjectPath("/org/freedesktop/Notifications"),
		dbus.WithMatchInterface("org.freedesktop.Notifications"),
	); err != nil {
		d.log.Printf("[ERROR] Cannot register signals with DBus: %s\n",
			err.Error())
		return nil, err
	}

	if err = d.initWebHandlers(); err != nil {
		d.log.Printf("[ERROR] Failed to initialize web server: %s\n",
			err.Error())
		return nil, err
	}

	d.signalQ = make(chan *dbus.Signal, 25)
	d.bus.Signal(d.signalQ)

	// do more stuff?

	go d.notifyLoop()
	go d.dbLoop()
	go d.serveHTTP()

	if err = d.initDnsSd(); err != nil {
		d.log.Printf("[ERROR] Cannot register Service with DNS-SD: %s\n",
			err.Error())
		return nil, err
	}

	go d.findPeers()

	return d, nil
} // func Summon() (*Daemon, error)

// IsAlive returns true if the Daemon's active flag is set.
func (d *Daemon) IsAlive() bool {
	d.lock.RLock()
	var alive = d.active
	d.lock.RUnlock()

	return alive
} // func (d *Daemon) IsAlive() bool

// Banish clears the Daemon's active flag, telling components to shut down.
func (d *Daemon) Banish() error {
	var (
		err         error
		ctx, cancel = context.WithTimeout(context.Background(), time.Second*3)
	)
	defer cancel()

	d.dnssd.Shutdown()

	if err = d.web.Shutdown(ctx); err != nil {
		d.log.Printf("[ERROR] Failed to shutdown web server: %s\n",
			err.Error())
	}

	if ctx.Err() != nil {
		err = ctx.Err()
		d.log.Printf("[ERROR] Failed to gracefully shut down web server: %s\n",
			ctx.Err().Error())
		d.web.Close() // nolint: errcheck
	}

	d.lock.Lock()
	d.active = false
	d.lock.Unlock()
	return err
} // func (d *Daemon) Banish() error

func (d *Daemon) getNotificationID(id int64) (uint32, bool) {
	var (
		nid uint32
		ok  bool
	)

	d.nLock.RLock()
	//nid, ok = d.pending[id]
	for dbusID, dbID := range d.pending {
		if id == dbID {
			nid = dbusID
			ok = true
			break
		}
	}
	d.nLock.RUnlock()

	return nid, ok
} // func (d *Daemon) getNotificationID(id int64) (uint32, bool)

func (d *Daemon) removePending(id uint32) {
	d.nLock.Lock()
	delete(d.pending, id)
	d.nLock.Unlock()
} // func (d *Daemon) removePending(nid uint32)

func (d *Daemon) notifyLoop() {
	defer d.log.Println("[TRACE] Quitting notifyLoop")

	var (
		err  error
		tick = time.NewTicker(queueTimeout)
	)
	defer tick.Stop()

	for d.IsAlive() {
		select {
		case <-tick.C:
			continue
		case n := <-d.signalQ:
			d.log.Printf("[DEBUG] Received signal: %#v\n",
				n)

			switch n.Name {
			case "org.freedesktop.Notifications.NotificationClosed":
				var (
					nid uint32
					ok  bool
				)

				if nid, ok = n.Body[0].(uint32); ok {
					d.removePending(nid)
				}
			case "org.freedesktop.Notifications.ActionInvoked":
				var action = n.Body[1].(string)

				d.log.Printf("[DEBUG] User clicked %s\n",
					action)

				switch strings.ToLower(action) {
				case "delay":
					if err = d.delayNotification(n.Body[0].(uint32)); err != nil {
						d.log.Printf("[ERROR] Cannot delay Notification: %s\n",
							err.Error())
					}
				case "ok":
					if err = d.finishNotification(n.Body[0].(uint32)); err != nil {
						d.log.Printf("[ERROR] Cannot finish Notification: %s\n",
							err.Error())
					}
				default:
					d.log.Printf("[ERROR] Unknown action %q from Notification\n",
						action)
				}
			}
		case m := <-d.Queue:
			var title, body = m.Payload()
			d.log.Printf("[DEBUG] Received Notification: %s\n%s\n",
				title,
				body)

			if err = d.notify(m, 0); err != nil {
				d.log.Printf("[ERROR] Failed to post Notification %q: %s\n",
					title,
					err.Error())
			}
		}
	}
} // func (d *Daemon) notifyLoop()

func (d *Daemon) notify(r *objects.Reminder, timeout int32) error {
	var (
		err        error
		head, body string
		db         *database.Database
		due, now   time.Time
		pending    []objects.Notification
		obj        = d.bus.Object(notifyObj, notifyPath)
	)

	if obj == nil {
		err = fmt.Errorf("Did not find object %s (%s) on session bus",
			notifyObj,
			notifyPath)
		d.log.Printf("[ERROR] %s\n", err.Error())
		return err
	}

	head, body = r.Payload()
	due = r.DueNext(nil)

	d.log.Printf("[DEBUG] Check pending Notifications for Reminder %d (%q)\n",
		r.ID,
		r.Title)

	db = d.pool.Get()
	defer d.pool.Put(db)

	if pending, err = db.NotificationGetByReminderPending(r); err != nil {
		d.log.Printf("[ERROR] Cannot fetch pending Notifications for Reminder %q (%d): %s\n",
			r.Title,
			r.ID,
			err.Error())
		return err
	}

	d.log.Printf("[DEBUG] Found %d pending Notifications for Reminder %d\n",
		len(pending),
		r.ID)

	var found bool

	for _, n := range pending {
		if n.Timestamp.Truncate(time.Minute).Equal(due) {
			found = true
			break
		}
	}

	if !found {
		var n *objects.Notification

		d.log.Printf("[DEBUG] Adding Notification for Reminder %d at %s\n",
			r.ID,
			due.Format(common.TimestampFormat))

		if n, err = db.NotificationAdd(r, due); err != nil {
			d.log.Printf("[ERROR] Cannot add Notification for Reminder %d at %s: %s\n",
				r.ID,
				due.Format(common.TimestampFormat),
				err.Error())
			return err
		}

		pending = append(pending, *n)
	}

	due = r.DuePrev(nil)

	d.log.Printf("[DEBUG] Looking for most recent Notification for Reminder %d at %s\n",
		r.ID,
		due.Format(common.TimestampFormat))

	for _, n := range pending {
		if n.Timestamp.Truncate(time.Minute).Equal(due) {
			d.log.Printf("[DEBUG] Found Notification for Reminder %d at %s: %d\n",
				r.ID,
				due.Format(common.TimestampFormat),
				n.ID)
			found = true
			break
		}
	}

	// If we don't find a pending Notification for the most recent
	// Recurrence, *maybe* we should check if there is an *acknowledged*
	// Notification before trying to display one?
	if !found {
		var n *objects.Notification

		if n, err = db.NotificationGetByReminderStamp(r, due); err != nil {
			d.log.Printf("[ERROR] Cannot look up Notification for Reminder %d at %s: %s\n",
				r.ID,
				due.Format(common.TimestampFormat),
				err.Error())
			return err
		} else if n != nil && n.Acknowledged.After(common.Epoch) {
			d.log.Printf("[DEBUG] Found acknowledged Notification for Reminder %d at %s: Acknowledged at %s\n",
				r.ID,
				due.Format(common.TimestampFormat),
				n.Acknowledged.Format(common.TimestampFormat))
			goto LETS_GO
		} else if n, err = db.NotificationAdd(r, due); err != nil {
			d.log.Printf("[ERROR] Cannot add Notification for Reminder %d at %s: %s\n",
				r.ID,
				due.Format(common.TimestampFormat),
				err.Error())
			return err
		}

		d.log.Printf("[DEBUG] Added Notification for Reminder %d at %s: %d\n",
			r.ID,
			due.Format(common.TimestampFormat),
			n.ID)
		pending = append(pending, *n)
	}

LETS_GO:
	now = time.Now().Truncate(time.Minute)

	for _, n := range pending {
		if n.Timestamp.After(now.Add(queueTimeout)) {
			continue
		}

		var msg = fmt.Sprintf("%s -- %s",
			n.Timestamp.Format(common.TimestampFormatMinute),
			body)

		var res = obj.Call(
			notifyMethod,
			0,
			common.AppName,
			uint32(0),
			"",
			head,
			msg,
			[]string{
				"OK",
				"OK",
				"Delay",
				"Delay",
			},
			map[string]*dbus.Variant{},
			timeout,
		)

		if res.Err != nil {
			d.log.Printf("[ERROR] Cannot send Notification %q: %s\n",
				head,
				res.Err.Error())
			return res.Err
		}

		var ret uint32

		if err = res.Store(&ret); err != nil {
			d.log.Printf("[ERROR] Cannot store return value of %s: %s\n",
				notifyMethod,
				err.Error())
			return err
		} else if common.Debug {
			d.log.Printf("[DEBUG] RESPONSE: %d\n",
				ret)
		}

		if err = db.NotificationDisplay(&n, now); err != nil {
			d.log.Printf("[ERROR] Cannot set Display stamp for Notification %d at %s: %s\n",
				n.ID,
				now.Format(common.TimestampFormat),
				err.Error())
		} else {
			d.nLock.Lock()
			d.pending[ret] = n.ID
			d.nLock.Unlock()
		}

	}
	return nil
} // func (d *Daemon) notify(n objects.Notification, timeout int32) error

func (d *Daemon) finishNotification(notID uint32) error {
	var (
		err      error
		db       *database.Database
		rem      *objects.Reminder
		not      *objects.Notification
		nid, rid int64
		ok       bool
	)

	db = d.pool.Get()
	defer d.pool.Put(db)

	d.nLock.RLock()
	defer d.nLock.RUnlock()

	if nid, ok = d.pending[notID]; !ok {
		d.log.Printf("[INFO] Notification ID %d was not found in cache\n",
			notID)
		return nil
	}

	defer delete(d.pending, notID)

	if not, err = db.NotificationGetByID(nid); err != nil {
		d.log.Printf("[ERROR] Cannot get Notification %d: %s\n",
			nid,
			err.Error())

		return err
	} else if not == nil {
		d.log.Printf("[CANTHAPPEN] Could not find Notification %d in database\n",
			nid)
		return nil
	} else if rem, err = db.ReminderGetByID(not.ReminderID); err != nil {
		d.log.Printf("[ERROR] Cannot look up Reminder #%d: %s\n",
			rid,
			err.Error())
		return err
	} else if rem == nil {
		d.log.Printf("[DEBUG] Reminder #%d was not found in database.\n",
			rid)
		return nil
	} else if rem.Recur.Repeat != repeat.Once {
		d.log.Printf("[DEBUG] Reminder %d (%q) is recurring (%s)\n",
			rem.ID,
			rem.Title,
			rem.Recur.Repeat)
	} else if err = db.ReminderSetFinished(rem, true); err != nil {
		d.log.Printf("[ERROR] Cannot set finished-flag on Reminder %d (%q): %s\n",
			rid,
			rem.Title,
			err.Error())
		return err
	}

	if err = db.NotificationAcknowledge(not, time.Now()); err != nil {
		d.log.Printf("[ERROR] Failed to acknowledge Notification %d for Reminder %d: %s\n",
			not.ID,
			rem.ID,
			err.Error())
	}

	return nil
} // func (d *Daemon) finishNotification(notID uint32) error

// What would it mean to delay a Reminder that goes off regularly?
// In that case, we can't just update the timestamp in the database, now,
// can we?
// So what do we do in those cases?
// Basically, we'd have to create a copy of the Reminder that is set to go off
// in five minutes (or whatever). ...
func (d *Daemon) delayNotification(nID uint32) error {
	var (
		err       error
		db        *database.Database
		rem       *objects.Reminder
		not       *objects.Notification
		nid       int64
		ok        bool
		timestamp = time.Now().Add(defaultReminderDelay)
	)

	d.log.Printf("[DEBUG] Delay Notification %d until %s\n",
		nID,
		timestamp.Format(common.TimestampFormat))

	db = d.pool.Get()
	defer d.pool.Put(db)

	d.nLock.RLock()
	defer d.nLock.RUnlock()

	if nid, ok = d.pending[nID]; !ok {
		err = fmt.Errorf("Did not find database ID for Notification %d",
			nID)
		d.log.Printf("[ERROR] %s\n",
			err.Error())
		return err
	}

	defer delete(d.pending, nID)

	if not, err = db.NotificationGetByID(nid); err != nil {
		d.log.Printf("[ERROR] Failed to look up Notification %d in database: %s\n",
			nid,
			err.Error())
		return err
	} else if not == nil {
		err = fmt.Errorf("Did not find Notficiation %d in database",
			nid)
		d.log.Printf("[ERROR] %s\n",
			err.Error())
		return err
	} else if rem, err = db.ReminderGetByID(not.ReminderID); err != nil {
		d.log.Printf("[ERROR] Cannot look up Reminder #%d: %s\n",
			not.ReminderID,
			err.Error())
		return err
	} else if rem == nil {
		d.log.Printf("[DEBUG] Reminder #%d was not found in database.\n",
			not.ReminderID)
		return nil
	} else if rem.Recur.Repeat != repeat.Once {
		// What does it mean to delay a Reminder that is set to go off
		// regularly? We need to to post the notification again in a
		// few minutes, but without touching the database record.
		//
		// (Unless the Reminder is also set to go off only a limited
		// number of times, but currently we are completely ignoring
		// that part part anyway.)
		//
		// How can we do that?
		// The easiest way I can think of is to start a goroutine
		// that just waits for the delay interval and sends the Reminder
		// to the Notification queue again.
		// I *think* this should be repeatable ad nauseam, so ...
		// let's try.
		go func() {
			time.Sleep(defaultReminderDelay)
			if d.IsAlive() {
				d.Queue <- rem
			}
		}()
	} else if err = db.ReminderSetTimestamp(rem, timestamp); err != nil {
		d.log.Printf("[ERROR] Cannot delay Reminder %d (%q): %s\n",
			rem.ID,
			rem.Title,
			err.Error())
		return err
	}

	return nil
} // func (d *Daemon) delayNotification(nID uint32) error

// dbLoop periodically checks for pending Reminders in the database.
func (d *Daemon) dbLoop() {
	defer d.log.Println("[TRACE] dbLoop is shutting down")

	var ticker = time.NewTicker(queueTimeout)
	defer ticker.Stop()

	for d.IsAlive() {
		var err error
		<-ticker.C

		if err = d.dbCheck(); err != nil {
			d.log.Printf("[ERROR] Failed to get Reminders from Database: %s\n",
				err.Error())
		}
	}
} // func (d *Daemon) dbLoop()

// dbCheck does the actual interaction with the database for dbLoop.
// We do this in a separate method so we can use defer to return the
// database connection to the pool.
func (d *Daemon) dbCheck() error {
	var (
		err       error
		db        *database.Database
		reminders []objects.Reminder
		deadline  = time.Now().Add(queueTimeout)
	)

	db = d.pool.Get()
	defer d.pool.Put(db)

	if reminders, err = db.ReminderGetPendingWithNotifications(deadline); err != nil {
		d.log.Printf("[ERROR] Cannot get pending Reminders from Database: %s\n",
			err.Error())
		return err
	}

	for idx, r := range reminders {
		if _, ok := d.getNotificationID(r.ID); !ok {
			d.Queue <- &reminders[idx]
		} else if common.Debug {
			d.log.Printf("[TRACE] Notification for Reminder %q (%d) is already on display\n",
				r.Title,
				r.ID)
		}
	}

	return nil
} // func (d *Daemon) dbCheck() error

func (d *Daemon) reminderMerge(remote []objects.Reminder) error {
	var (
		err      error
		local    []objects.Reminder
		db       *database.Database
		idmap    map[string]int
		errmsg   string
		txStatus bool
	)

	if len(remote) == 0 {
		d.log.Println("[TRACE] Remote object list is empty")
		return nil
	}

	db = d.pool.Get()
	defer d.pool.Put(db)

	if err = db.Begin(); err != nil {
		errmsg = fmt.Sprintf("Failed to initialize database transaction: %s",
			err.Error())
		d.log.Printf("[ERROR] %s\n", errmsg)
		return errors.New(errmsg)
	}

	defer func() {
		if txStatus {
			if err = db.Commit(); err != nil {
				d.log.Printf("[ERROR] Failed to commit database transaction after successful merge:\n\t%s\n",
					err.Error())
			}
		} else {
			if err = db.Rollback(); err != nil {
				d.log.Printf("[ERROR] Cannot roll back transaction after failed merge:\n\t%s\n",
					err.Error())
			}
		}
	}()

	if local, err = db.ReminderGetAll(); err != nil {
		errmsg = fmt.Sprintf("Failed to load local Reminders from database: %s",
			err.Error())
		d.log.Printf("[ERROR] %s\n", errmsg)
		return errors.New(errmsg)
	}

	idmap = make(map[string]int, len(local))

	for idx, rem := range local {
		idmap[rem.UUID] = idx
	}

	// Donnerstag, 22. 09. 2022, 19:52 -- I have not looked at this code since I introduced recurring Reminders.
	// The added fields are not taken into consideration at all...
	for _, remR := range remote {
		var (
			lidx  int
			ok    bool
			remL  objects.Reminder
			ctime = remR.Changed
		)

		if lidx, ok = idmap[remR.UUID]; !ok {
			// Add Reminder to database
			if err = db.ReminderAdd(&remR); err != nil {
				errmsg = fmt.Sprintf("Failed to add Reminder %q (%s) to database: %s",
					remR.Title,
					remR.UUID,
					err.Error())
				d.log.Printf("[ERROR] %s\n", errmsg)
				return errors.New(errmsg)
			} else if err = db.ReminderSetFinished(&remR, remR.Finished); err != nil {
				errmsg = fmt.Sprintf("Failed to set finished-flag on new Reminder %q (%s): %s",
					remR.Title,
					remR.UUID,
					err.Error())
				d.log.Printf("[ERROR] %s\n", errmsg)
				return errors.New(errmsg)
			} else if err = db.ReminderSetChanged(&remR, ctime); err != nil {
				errmsg = fmt.Sprintf("Failed to set CTime on Reminder %q (%s): %s",
					remR.Title,
					remR.UUID,
					err.Error())
				d.log.Printf("[ERROR] %s\n", errmsg)
				return errors.New(errmsg)
			}
		} else if remL = local[lidx]; remR.Changed.After(remL.Changed) {
			// Update Reminder in local database
			// This is slightly more tedious, because we need to
			// check *which fields* we need to update.
			if remL.Title != remR.Title {
				if err = db.ReminderSetTitle(&remL, remR.Title); err != nil {
					errmsg = fmt.Sprintf("Failed to update title on Reminder %d (%q): %s",
						remL.ID,
						remL.UUID,
						err.Error())
					d.log.Printf("[ERROR] %s\n", errmsg)
					return errors.New(errmsg)
				}
			}

			if remL.Description != remR.Description {
				if err = db.ReminderSetDescription(&remL, remR.Description); err != nil {
					errmsg = fmt.Sprintf("Failed to update description on Reminder %d (%q): %s",
						remL.ID,
						remL.UUID,
						err.Error())
					d.log.Printf("[ERROR] %s\n", errmsg)
					return errors.New(errmsg)
				}
			}

			if !remL.Timestamp.Equal(remR.Timestamp) {
				if err = db.ReminderSetTimestamp(&remL, remR.Timestamp); err != nil {
					errmsg = fmt.Sprintf("Failed to update timestamp on Reminder %d (%q): %s",
						remL.ID,
						remL.UUID,
						err.Error())
					d.log.Printf("[ERROR] %s\n", errmsg)
					return errors.New(errmsg)
				}
			}

			if remL.Finished != remR.Finished {
				if err = db.ReminderSetFinished(&remL, remR.Finished); err != nil {
					errmsg = fmt.Sprintf("Failed to update finished-flag on Reminder %d (%q): %s",
						remL.ID,
						remL.UUID,
						err.Error())
					d.log.Printf("[ERROR] %s\n", errmsg)
					return errors.New(errmsg)
				}
			}

			// I just learned that in Go, comparing structs for
			// equality is a-okay and works just about as one
			// would expect.
			if remL.Recur != remR.Recur {
				if remL.Recur.Repeat != remR.Recur.Repeat {
					if err = db.ReminderSetRepeat(&remL, remR.Recur.Repeat); err != nil {
						errmsg = fmt.Sprintf("Cannot set repeat mode for Reminder %d from %s to %s: %s",
							remL.ID,
							remL.Recur.Repeat,
							remR.Recur.Repeat,
							err.Error())
						d.log.Printf("[ERROR] %s\n",
							errmsg)
						return errors.New(errmsg)
					}
				}

				if remL.Recur.Days != remR.Recur.Days {
					if err = db.ReminderSetWeekdays(&remL, remR.Recur.Days); err != nil {
						errmsg = fmt.Sprintf("Cannot set Weekdays for Reminder %d from %s to %s: %s",
							remL.ID,
							remL.Recur.Days,
							remR.Recur.Days,
							err.Error())
						d.log.Printf("[ERROR] %s\n",
							errmsg)
						return errors.New(errmsg)
					}
				}
			}

			if err = db.ReminderSetChanged(&remL, ctime); err != nil {
				errmsg = fmt.Sprintf("Cannot update change stamp on Reminder %d (%q): %s",
					remL.ID,
					remL.UUID,
					err.Error())
				d.log.Printf("[ERROR] %s\n", errmsg)
				return errors.New(errmsg)
			}
		}
	}

	txStatus = true
	return nil
} // func (d *Daemon) reminderMerge(remote []objects.Reminder) error

func (d *Daemon) synchronize(peer *objects.Peer) error {
	var (
		err                  error
		addr                 url.URL
		uri                  string
		buf                  bytes.Buffer
		client               http.Client
		res                  *http.Response
		answer               objects.Response
		remote, local, delta []objects.Reminder
		db                   *database.Database
	)

	addr = url.URL{
		Scheme: "http",
		Host: fmt.Sprintf("%s:%d",
			peer.Hostname,
			peer.Port),
		Path: "/sync/pull",
	}

	uri = addr.String()

	if res, err = client.Get(uri); err != nil {
		d.log.Printf("[ERROR] Cannot get Reminder list from Peer %s: %s\n",
			peer,
			err.Error())
		return err
	}

	defer res.Body.Close()
	if _, err = io.Copy(&buf, res.Body); err != nil {
		d.log.Printf("[ERROR] Cannot read HTTP response body from %s: %s\n",
			peer.Hostname,
			err.Error())
		return err
	}

	if res.StatusCode != 200 {
		d.log.Printf("[ERROR] HTTP request to %s failed: %s\n%s\n",
			peer.Hostname,
			res.Status,
			buf.Bytes())

		if err = ffjson.Unmarshal(buf.Bytes(), &answer); err != nil {
			d.log.Printf("[ERROR] Failed to decode Response structure from %s: %s\n",
				peer.Hostname,
				err.Error())
			return err
		}

		d.log.Printf("[ERROR] Response from %s: %#v\n",
			peer.Hostname,
			answer)
		return errors.New(answer.Message)
	} else if err = ffjson.Unmarshal(buf.Bytes(), &remote); err != nil {
		d.log.Printf("[ERROR] Cannot decode response from %s: %s\n%s\n",
			peer.Hostname,
			err.Error(),
			buf.Bytes())
		return err
	} else if err = d.reminderMerge(remote); err != nil {
		d.log.Printf("[ERROR] Failed to merge Reminder items from %s into local database: %s\n",
			peer.Hostname,
			err.Error())
		return err
	}

	var idmap = make(map[string]int, len(remote))

	for idx, val := range remote {
		idmap[val.UUID] = idx
	}

	db = d.pool.Get()
	defer d.pool.Put(db)

	if local, err = db.ReminderGetAll(); err != nil {
		d.log.Printf("[ERROR] Cannot get Reminders from database: %s\n",
			err.Error())
		return err
	}

	delta = make([]objects.Reminder, 0)

	for _, l := range local {
		var (
			idx int
			ok  bool
			r   objects.Reminder
		)

		if idx, ok = idmap[l.UUID]; !ok {
			delta = append(delta, l)
		} else if r = remote[idx]; l.IsNewer(&r) {
			delta = append(delta, l)
		}
	}

	if len(delta) == 0 {
		return nil
	}

	var j []byte

	if j, err = ffjson.Marshal(delta); err != nil {
		d.log.Printf("[ERROR] Cannot serialize Reminders to send to remote peer: %s\n",
			err.Error())
		return err
	}

	defer ffjson.Pool(j)

	var body = bytes.NewReader(j)

	addr.Path = "/sync/push"
	uri = addr.String()

	if res, err = client.Post(uri, "application/json", body); err != nil {
		d.log.Printf("[ERROR] Cannot get Reminder list from Peer %s: %s\n",
			peer,
			err.Error())
		return err
	}

	buf.Reset()

	defer res.Body.Close()
	if _, err = io.Copy(&buf, res.Body); err != nil {
		d.log.Printf("[ERROR] Cannot read HTTP response body from %s: %s\n",
			peer.Hostname,
			err.Error())
		return err
	}

	if err = ffjson.Unmarshal(buf.Bytes(), &answer); err != nil {
		d.log.Printf("[ERROR] Cannot parse response from %s: %s\n%s\n",
			peer.Hostname,
			err.Error(),
			buf.Bytes())
		return err
	} else if !answer.Status {
		d.log.Printf("[ERROR] Peer %s signalled a failure: %s\n",
			peer.Hostname,
			answer.Message)
		return errors.New(answer.Message)
	}

	return nil
} // func (d *Daemon) synchronize(peer string) error
