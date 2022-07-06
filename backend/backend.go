// /home/krylon/go/src/github.com/blicero/theseus/backend/backend.go
// -*- mode: go; coding: utf-8; -*-
// Created on 01. 07. 2022 by Benjamin Walkenhorst
// (c) 2022 Benjamin Walkenhorst
// Time-stamp: <2022-07-06 20:37:20 krylon>

// Package backend implements the ... backend of the application,
// the part that deals with the database and dbus.
package backend

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/blicero/theseus/common"
	"github.com/blicero/theseus/database"
	"github.com/blicero/theseus/logdomain"
	"github.com/blicero/theseus/objects"
	"github.com/godbus/dbus"
	"github.com/gorilla/mux"
)

const (
	notifyObj    = "org.freedesktop.Notifications"
	notifyIntf   = "org.freedesktop.Notifications" // nolint: deadcode,unused,varcheck
	notifyPath   = "/org/freedesktop/Notifications"
	notifyMethod = "org.freedesktop.Notifications.Notify"
	queueDepth   = 5
	queueTimeout = time.Second * 2
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

	if err = d.initWebHandlers(); err != nil {
		d.log.Printf("[ERROR] Failed to initialize web server: %s\n",
			err.Error())
		return nil, err
	}

	// do more stuff?

	go d.notifyLoop()
	go d.dbLoop()
	go d.serveHTTP()

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
		case m := <-d.Queue:
			var title, body = m.Payload()
			d.log.Printf("[DEBUG] Received Notification: %s\n%s\n",
				title,
				body)

			if err = d.notify(m); err != nil {
				d.log.Printf("[ERROR] Failed to post Notification %q: %s\n",
					title,
					err.Error())
			}
		}
	}
} // func (d *Daemon) notifyLoop()

func (d *Daemon) notify(n objects.Notification) error {
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
		[]string{},
		map[string]*dbus.Variant{},
		0,
	)

	if res.Err != nil {
		d.log.Printf("[ERROR] Cannot send Notification %q: %s\n",
			head,
			res.Err.Error())
		return res.Err
	}

	return nil
} // func (d *Daemon) notify(n objects.Notification) error

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
	)

	db = d.pool.Get()
	defer d.pool.Put(db)

	if reminders, err = db.ReminderGetPending(); err != nil {
		d.log.Printf("[ERROR] Cannot get pending Reminders from Database: %s\n",
			err.Error())
		return err
	}

	for idx := range reminders {
		d.Queue <- &reminders[idx]
	}

	return nil
} // func (d *Daemon) dbCheck() error
