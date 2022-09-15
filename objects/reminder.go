// /home/krylon/go/src/github.com/blicero/theseus/objects/reminder.go
// -*- mode: go; coding: utf-8; -*-
// Created on 30. 06. 2022 by Benjamin Walkenhorst
// (c) 2022 Benjamin Walkenhorst
// Time-stamp: <2022-09-15 17:52:10 krylon>

package objects

import (
	"fmt"
	"time"

	"github.com/blicero/theseus/common"
)

//go:generate ffjson reminder.go

// Reminder is ... a reminder.
type Reminder struct {
	ID          int64
	Title       string
	Description string
	Timestamp   time.Time
	Recur       Alarmclock
	Finished    bool
	UUID        string
	Changed     time.Time
}

const tod = "15:04:05" // tod == Time Of Day

// Due returns the Reminder's due time.
// If ref is non-nil, it is used as the reference point from which
// to compute the next due time for recurring Reminders, otherwise the
// current time is used.
func (r *Reminder) Due(ref *time.Time) time.Time {
	var now time.Time

	if ref == nil {
		now = time.Now().In(time.UTC)
	} else {
		now = *ref
	}

	switch r.Recur.Repeat {
	case Once:
		return r.Timestamp
	case Daily:
		var stamp = r.Timestamp.Unix()

		if stamp < (now.Unix() % 86400) {
			return now.Truncate(time.Second * 86400).Add(time.Second * 86400).Add(time.Second * time.Duration(stamp))
		}

		return now.Truncate(time.Second * 86400).Add(time.Second * time.Duration(stamp))
	case Custom:
		var (
			offset = r.Timestamp.Unix()
			due    = now.Truncate(time.Hour * 24).Add(time.Duration(offset) * time.Second)
		)

		fmt.Printf("Reference time is %s\n",
			now.Format(common.TimestampFormatSubSecond))
		fmt.Printf("Offset: %06d | Due: %s\n",
			offset,
			due.Format(common.TimestampFormat))

		if due.Before(now) {
			fmt.Printf("The time of day (%s) is past the time stamp (%s)\n",
				now.Format(tod),
				due.Format(tod))
			due = due.Add(time.Hour * 24)
		}

		for !r.Recur.Days.On(due.Weekday()) {
			fmt.Printf("Reminder is not due on %s, skipping one day ahead.\n",
				due.Weekday())
			due = due.Add(time.Hour * 24)
		}

		fmt.Printf("Reminder IS due on %s at %s\n",
			due.Weekday(),
			due.Format(common.TimestampFormatTime))

		return due
	default:
		panic(fmt.Errorf("Invalid Recurrence type %d", r.Recur.Repeat))
	}
} // func (r *Reminder) Due() time.Time

// IsDue returns true if the Reminder's due time has passed.
func (r *Reminder) IsDue() bool {
	return r.Timestamp.Before(time.Now())
} // func (r *Reminder) IsDue() bool

// Payload returns the Reminder's Title and Description.
func (r *Reminder) Payload() (string, string) {
	return r.Title, r.Description
} // func (r *Reminder) Payload() (string, string)

// UniqueID returns an identifier that is unique across instances.
// I.e. a UUID.
func (r *Reminder) UniqueID() string {
	return r.UUID
} // func (r *Reminder) UniqueID() string

// IsNewer returns true if the receiver's Changed stamp is
// more recent than the argument's.
func (r *Reminder) IsNewer(other *Reminder) bool {
	return r.Changed.After(other.Changed)
} // func (r *Reminder) IsNewer(other *Reminder) bool
