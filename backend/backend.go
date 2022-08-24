// /home/krylon/go/src/github.com/blicero/theseus/backend/backend.go
// -*- mode: go; coding: utf-8; -*-
// Created on 01. 07. 2022 by Benjamin Walkenhorst
// (c) 2022 Benjamin Walkenhorst
// Time-stamp: <2022-08-24 21:25:40 krylon>

// Package backend implements the ... backend of the application,
// the part that deals with the database and dbus.
package backend

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/blicero/theseus/common"
	"github.com/blicero/theseus/database"
	"github.com/blicero/theseus/logdomain"
	"github.com/blicero/theseus/objects"
	"github.com/godbus/dbus/v5"
	"github.com/gorilla/mux"
	"github.com/grandcat/zeroconf"
)

const (
	notifyObj            = "org.freedesktop.Notifications"
	notifyIntf           = "org.freedesktop.Notifications" // nolint: deadcode,unused,varcheck
	notifyPath           = "/org/freedesktop/Notifications"
	notifyMethod         = "org.freedesktop.Notifications.Notify"
	queueDepth           = 5
	queueTimeout         = time.Second * 2
	defaultReminderDelay = time.Second * 300
)

// Daemon is the centerpiece of the backend, coordinating between the database, the clients, etc.
type Daemon struct {
	log        *log.Logger
	pool       *database.Pool
	bus        *dbus.Conn
	lock       sync.RWMutex // nolint: structcheck,unused
	active     bool
	Queue      chan objects.Notification
	web        http.Server
	router     *mux.Router
	mimeTypes  map[string]string
	listenAddr string
	idLock     sync.Mutex
	idCnt      int64
	signalQ    chan *dbus.Signal
	nLock      sync.RWMutex
	pending    map[int64]uint32
	dnssd      *zeroconf.Server
}

// Summon summons a Daemon and returns it. No sacrifice or idolatry is required.
func Summon(addr string) (*Daemon, error) {
	var (
		err error
		d   = &Daemon{
			listenAddr: addr,
			active:     true,
			Queue:      make(chan objects.Notification, queueDepth),
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
			pending: make(map[int64]uint32),
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

	if err = d.initDNSSd(); err != nil {
		d.log.Printf("[ERROR] Cannot register Service with DNS-SD: %s\n",
			err.Error())
		return nil, err
	}

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
	nid, ok = d.pending[id]
	d.nLock.RUnlock()

	return nid, ok
} // func (d *Daemon) getNotificationID(id int64) (uint32, bool)

func (d *Daemon) removePending(id uint32) {
	d.nLock.Lock()
	defer d.nLock.Unlock()

	var remID int64

	for rid, nid := range d.pending {
		if nid == id {
			remID = rid
			break
		}
	}

	if remID != 0 {
		delete(d.pending, remID)
	}
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

			if n.Name == "org.freedesktop.Notifications.NotificationClosed" {
				var (
					nid uint32
					ok  bool
				)

				if nid, ok = n.Body[0].(uint32); ok {
					d.removePending(nid)
				}
			} else if n.Name == "org.freedesktop.Notifications.ActionInvoked" {
				var action = n.Body[1].(string)
				// ... We'll have to deal with it in some way. ;-|
				d.log.Printf("User clicked %s\n",
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

func (d *Daemon) notify(n objects.Notification, timeout int32) error {
	var (
		err        error
		obj        = d.bus.Object(notifyObj, notifyPath)
		head, body string
	)

	if obj == nil {
		err = fmt.Errorf("Did not find object %s (%s) on session bus",
			notifyObj,
			notifyPath)
		d.log.Printf("[ERROR] %s\n", err.Error())
		return err
	}

	head, body = n.Payload()

	var res = obj.Call(
		notifyMethod,
		0,
		common.AppName,
		uint32(0),
		"",
		head,
		body,
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

	if r, ok := n.(*objects.Reminder); ok {
		d.nLock.Lock()
		d.pending[r.ID] = ret
		d.nLock.Unlock()
	}

	return nil
} // func (d *Daemon) notify(n objects.Notification) error

func (d *Daemon) finishNotification(notID uint32) error {
	var (
		err error
		db  *database.Database
		rem *objects.Reminder
		rid int64
	)

	db = d.pool.Get()
	defer d.pool.Put(db)

	d.nLock.RLock()
	defer d.nLock.RUnlock()

	for nid, did := range d.pending {
		if did == notID {
			rid = nid
			break
		}
	}

	if rid == 0 {
		return nil
	} else if rem, err = db.ReminderGetByID(rid); err != nil {
		d.log.Printf("[ERROR] Cannot look up Reminder #%d: %s\n",
			rid,
			err.Error())
		return err
	} else if rem == nil {
		d.log.Printf("[DEBUG] Reminder #%d was not found in database.\n",
			rid)
		return nil
	} else if err = db.ReminderSetFinished(rem, true); err != nil {
		d.log.Printf("[ERROR] Cannot set finished-flag on Reminder %d (%q): %s\n",
			rid,
			rem.Title,
			err.Error())
		return err
	}

	return nil
} // func (d *Daemon) finishNotification(notID uint32) error

func (d *Daemon) delayNotification(nID uint32) error {
	var (
		err       error
		db        *database.Database
		rem       *objects.Reminder
		rid       int64
		timestamp = time.Now().Add(defaultReminderDelay)
	)

	d.log.Printf("[DEBUG] Delay Notification %d until %s\n",
		nID,
		timestamp.Format(common.TimestampFormat))

	db = d.pool.Get()
	defer d.pool.Put(db)

	d.nLock.RLock()
	defer d.nLock.RUnlock()

	for nid, did := range d.pending {
		if did == nID {
			rid = nid
			break
		}
	}

	if rid == 0 {
		d.log.Printf("[INFO] Did not find database ID for Notification ID %d\n",
			nID)
		return nil
	} else if rem, err = db.ReminderGetByID(rid); err != nil {
		d.log.Printf("[ERROR] Cannot look up Reminder #%d: %s\n",
			rid,
			err.Error())
		return err
	} else if rem == nil {
		d.log.Printf("[DEBUG] Reminder #%d was not found in database.\n",
			rid)
		return nil
	} else if err = db.ReminderSetTimestamp(rem, timestamp); err != nil {
		d.log.Printf("[ERROR] Cannot delay Reminder %d (%q): %s\n",
			rid,
			rem.Title,
			err.Error())
		return err
	}

	return nil
} // func (d *Daemon) delayNotification(nID uint32) error

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

func (d *Daemon) dbCheck() error {
	var (
		err       error
		db        *database.Database
		reminders []objects.Reminder
		deadline  = time.Now().Add(queueTimeout)
	)

	db = d.pool.Get()
	defer d.pool.Put(db)

	if reminders, err = db.ReminderGetPending(deadline); err != nil {
		d.log.Printf("[ERROR] Cannot get pending Reminders from Database: %s\n",
			err.Error())
		return err
	}

	for idx := range reminders {
		if _, ok := d.getNotificationID(reminders[idx].ID); !ok {
			d.Queue <- &reminders[idx]
		}
	}

	return nil
} // func (d *Daemon) dbCheck() error
