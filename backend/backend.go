// /home/krylon/go/src/github.com/blicero/theseus/backend/backend.go
// -*- mode: go; coding: utf-8; -*-
// Created on 01. 07. 2022 by Benjamin Walkenhorst
// (c) 2022 Benjamin Walkenhorst
// Time-stamp: <2022-07-01 22:05:27 krylon>

// Package backend implements the ... backend of the application,
// the part that deals with the database and dbus.
package backend

import (
	"fmt"
	"log"
	"sync"

	"github.com/blicero/krylib"
	"github.com/blicero/theseus/common"
	"github.com/blicero/theseus/database"
	"github.com/blicero/theseus/logdomain"
	"github.com/blicero/theseus/objects"
	"github.com/godbus/dbus"
)

// Daemon is the centerpiece of the backend, coordinating between the database, the clients, etc.
type Daemon struct {
	log  *log.Logger
	pool *database.Pool
	bus  *dbus.Conn
	lock sync.RWMutex
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

func (d *Daemon) notify(n *objects.Notification) error {
	return krylib.ErrNotImplemented
}
