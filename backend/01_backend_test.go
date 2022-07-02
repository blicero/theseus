// /home/krylon/go/src/github.com/blicero/theseus/backend/01_backend_test.go
// -*- mode: go; coding: utf-8; -*-
// Created on 02. 07. 2022 by Benjamin Walkenhorst
// (c) 2022 Benjamin Walkenhorst
// Time-stamp: <2022-07-02 18:42:57 krylon>

package backend

import (
	"testing"
	"time"

	"github.com/blicero/theseus/common"
	"github.com/blicero/theseus/objects"
)

var back *Daemon

func TestSummon(t *testing.T) {
	var err error

	if back, err = Summon(); err != nil {
		back = nil
		t.Errorf("Cannot create Daemon: %s",
			err.Error())
	}
} // func TestSummon(t *testing.T)

func TestNotify(t *testing.T) {
	if back == nil {
		t.SkipNow()
	}

	var (
		err error
		rem = &objects.Reminder{
			ID:          42,
			Title:       "Testing, one, two",
			Description: "This is just a simple test, nothing to see here.",
			Timestamp:   time.Now(),
			UUID:        common.GetUUID(),
		}
	)

	if err = back.notify(rem); err != nil {
		t.Errorf("Cannot send notification via DBus: %s",
			err.Error())
	}
} // func TestNotify(t *testing.T)
