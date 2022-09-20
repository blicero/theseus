// /home/krylon/go/src/github.com/blicero/theseus/backend/01_backend_test.go
// -*- mode: go; coding: utf-8; -*-
// Created on 02. 07. 2022 by Benjamin Walkenhorst
// (c) 2022 Benjamin Walkenhorst
// Time-stamp: <2022-09-20 15:26:05 krylon>

package backend

import (
	"fmt"
	"testing"
	"time"

	"github.com/blicero/theseus/common"
	"github.com/blicero/theseus/database"
	"github.com/blicero/theseus/objects"
)

var back *Daemon

func TestSummon(t *testing.T) {
	var err error

	if back, err = Summon("localhost:9596"); err != nil {
		back = nil
		t.Errorf("Cannot create Daemon: %s",
			err.Error())
	}
} // func TestSummon(t *testing.T)

func TestNotify(t *testing.T) {
	if back == nil {
		t.SkipNow()
	}

	const timeout = 10000 // 10,000 ms = 10s

	var (
		err error
		msg = fmt.Sprintf("%s: Testing, Testing, 1, 2, 3!",
			time.Now().Format(common.TimestampFormat))
		db  *database.Database
		rem = &objects.Reminder{
			Title:       "Testing, one, two",
			Description: msg,
			Timestamp:   time.Now(),
			UUID:        common.GetUUID(),
		}
	)

	db = back.pool.Get()
	defer back.pool.Put(db)

	if err = db.ReminderAdd(rem); err != nil {
		t.Errorf("Cannot add Reminder to database: %s",
			err.Error())
	}

	if err = back.notify(rem, timeout); err != nil {
		t.Errorf("Cannot send notification via DBus: %s",
			err.Error())
	}
} // func TestNotify(t *testing.T)
