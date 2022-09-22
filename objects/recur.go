// /home/krylon/go/src/github.com/blicero/theseus/objects/recur.go
// -*- mode: go; coding: utf-8; -*-
// Created on 06. 09. 2022 by Benjamin Walkenhorst
// (c) 2022 Benjamin Walkenhorst
// Time-stamp: <2022-09-22 18:57:13 krylon>

//go:generate ffjson recur.go

package objects

import (
	"fmt"
	"strings"
	"time"

	"github.com/blicero/theseus/objects/repeat"
)

// Weekdays is a list of weekdays that a recurring Reminder can be set
// to go off on.
type Weekdays [7]bool

// Bitfield returns an unsigned integer using the least significant bits
// as flags from right to left, i.e. the least significant bit is Monday,
// the second bit from the right is Tuesday, etc. The most significant
// bit it always zero.
func (w *Weekdays) Bitfield() uint8 {
	var days uint8 = b2i(w[0]) |
		b2i(w[1])<<1 |
		b2i(w[2])<<2 |
		b2i(w[3])<<3 |
		b2i(w[4])<<4 |
		b2i(w[5])<<5 |
		b2i(w[6])<<6

	return days
}

// Count returns the number of weekdays the Reminder is set to go off.
func (w *Weekdays) Count() int {
	var cnt int

	for _, b := range w {
		if b {
			cnt++
		}
	}

	return cnt
} // func (w *Weekdays) Count() int

// On returns the flag value for the given weekday.
func (w *Weekdays) On(d time.Weekday) bool {
	return w[(d+6)%7]
} // func (w *Weekdays) On(d time.Weekday) bool

// Recurrence specifies a potentially recurring point in time
// as an offset into the day (in seconds) and a Recurrence to
// specify how the event will repeat.
type Recurrence struct {
	ID      int64
	Offset  int
	Repeat  repeat.Repeat
	Days    Weekdays
	Limit   int
	Counter int
	UUID    string
	Changed time.Time
}

// Go's time package has a type Weekday, too, can I use that somehow?
// ... Turns out it's not super useful to us because it insists on
// Sunday being the first days of the week, whereas in Europe it's
// considered the last day of the week. So no.

// Weekdays returns a uint8 with the bitwise map of the days of the week
// when a Recurrence occurs.
func (a *Recurrence) Weekdays() uint8 {
	return a.Days.Bitfield()
} // func (a *Alarmclock) Weekdays() uint8

var wDayStr = []string{
	"Mo",
	"Di",
	"Mi",
	"Do",
	"Fr",
	"Sa",
	"So",
}

func (a *Recurrence) String() string {
	var (
		offset, str string
	)

	if a == nil {
		return "(None)"
	}

	offset = fmtOffset(a.Offset)

	switch a.Repeat {
	case repeat.Once:
		fallthrough
	case repeat.Daily:
		str = fmt.Sprintf("%s(%s)",
			a.Repeat,
			offset)
	case repeat.Custom:
		var days = make([]string, 0, 7)

		for idx, v := range a.Days {
			if v {
				days = append(days, wDayStr[idx])
			}
		}

		str = fmt.Sprintf("%s(%s)",
			a.Repeat,
			strings.Join(days, ","))
	default:
		str = fmt.Sprintf("InvalidRecurrence(%d)", a.Repeat)
	}

	return str
} // func (a *Alarmclock) String() string

func fmtOffset(off int) string {
	var h, m, s int

	if off > 3600 {
		h = off / 3600
		off = off % 3600
	}

	if off > 60 {
		m = off / 60
		off = off % 60
	}

	s = off

	return fmt.Sprintf("%02d:%02d:%02d",
		h, m, s)
} // func fmtOffset(off int) string

func b2i(b bool) uint8 {
	if b {
		return 1
	}
	return 0
} // func b2i(b bool) uint8
