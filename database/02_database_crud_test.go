// /home/krylon/go/src/github.com/blicero/theseus/database/02_database_crud_test.go
// -*- mode: go; coding: utf-8; -*-
// Created on 19. 08. 2022 by Benjamin Walkenhorst
// (c) 2022 Benjamin Walkenhorst
// Time-stamp: <2022-08-19 19:43:42 krylon>

package database

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/blicero/theseus/common"
	"github.com/blicero/theseus/objects"
)

const (
	itemCnt   = 32
	maxOffset = time.Hour * 168
)

var items []*objects.Reminder

func init() {
	items = make([]*objects.Reminder, itemCnt)

	var now = time.Now()

	for i := range items {
		var r = &objects.Reminder{
			Title: fmt.Sprintf("TEST #%03d", i),
			Description: fmt.Sprintf("This is just another test, the %dth one",
				i+1),
			Timestamp: now.Add(time.Duration(rand.Int63n(int64(maxOffset)))),
			UUID:      common.GetUUID(),
		}

		items[i] = r
	}
}

func TestReminderAdd(t *testing.T) {
	if db == nil {
		t.SkipNow()
	}

	for _, r := range items {
		var err error

		if err = db.ReminderAdd(r); err != nil {
			t.Fatalf("Cannot add Reminder %s: %s",
				r.Title,
				err.Error())
		} else if r.ID == 0 {
			t.Errorf("ID of Reminder %q is 0", r.Title)
		}
	}
} // func TestReminderAdd(t *testing.T)

func TestReminderGetAll(t *testing.T) {
	if db == nil {
		t.SkipNow()
	}

	var (
		err error
		rem []objects.Reminder
	)

	if rem, err = db.ReminderGetAll(); err != nil {
		t.Fatalf("Cannot fetch all Reminders: %s",
			err.Error())
	} else if len(rem) != len(items) {
		t.Fatalf("Unexpected number of Reminders: %d (expected %d)",
			len(rem),
			len(items))
	}
} // func TestReminderGetAll(t *testing.T)

func TestReminderFinish(t *testing.T) {
	if db == nil {
		t.SkipNow()
	}

	for _, r := range items {
		if rand.Intn(100) >= 50 {
			continue
		}

		var err error

		if err = db.ReminderSetFinished(r, true); err != nil {
			t.Errorf("Cannot set Reminder %q as finished: %s",
				r.Title,
				err.Error())
		} else if !r.Finished {
			t.Errorf("Reminder %q should be marked as finished, but it is not",
				r.Title)
		}

	}
} // func TestReminderFinish(t *testing.T)
