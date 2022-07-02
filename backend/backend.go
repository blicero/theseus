// /home/krylon/go/src/github.com/blicero/theseus/backend/backend.go
// -*- mode: go; coding: utf-8; -*-
// Created on 01. 07. 2022 by Benjamin Walkenhorst
// (c) 2022 Benjamin Walkenhorst
// Time-stamp: <2022-07-02 19:02:49 krylon>

// Package backend implements the ... backend of the application,
// the part that deals with the database and dbus.
package backend

import (
	"fmt"
	"log"
	"sync"

	"github.com/blicero/theseus/common"
	"github.com/blicero/theseus/database"
	"github.com/blicero/theseus/logdomain"
	"github.com/blicero/theseus/objects"
	"github.com/godbus/dbus"
)

const (
	notifyObj    = "org.freedesktop.Notifications"
	notifyIntf   = "org.freedesktop.Notifications" // nolint: deadcode,unused,varcheck
	notifyPath   = "/org/freedesktop/Notifications"
	notifyMethod = "org.freedesktop.Notifications.Notify"
)

// Daemon is the centerpiece of the backend, coordinating between the database, the clients, etc.
type Daemon struct {
	log  *log.Logger
	pool *database.Pool
	bus  *dbus.Conn
	lock sync.RWMutex // nolint: structcheck,unused
}

// Summon summons a Daemon and returns it. No sacrifice or idolatry is required.
func Summon() (*Daemon, error) {
	var (
		err error
		d   *Daemon
	)

	d = new(Daemon)

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

	// do more stuff?

	return d, nil
} // func Summon() (*Daemon, error)

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
