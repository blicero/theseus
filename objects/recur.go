// /home/krylon/go/src/github.com/blicero/theseus/objects/recur.go
// -*- mode: go; coding: utf-8; -*-
// Created on 06. 09. 2022 by Benjamin Walkenhorst
// (c) 2022 Benjamin Walkenhorst
// Time-stamp: <2022-09-09 20:07:04 krylon>

//go:generate stringer -type=Recurrence

//go:generate ffjson recur.go

package objects

import "time"

// Recurrence describes how a Reminder get triggered repeatedly and regularly.
type Recurrence uint8

// Once means a Reminder goes off only once.
// Daily means it is repeated every day.
// Custom means that the user specifies on which weekdays the Reminder should
// go off (e.g. "only on workdays", or "only on weekends")
const (
	Once Recurrence = iota
	Daily
	Custom
)

// Alarmclock specifies a potentially recurring point in time
// as an offset into the day (in seconds) and a Recurrence to
// specify how the event will repeat.
type Alarmclock struct {
	ID         int64
	ReminderID int64
	Offset     int
	Repeat     Recurrence
	Days       [7]bool
	Limit      int
	Counter    int
	UUID       string
	Changed    time.Time
}

// Go's time package has a type Weekday, too, can I use that somehow?

// Weekdays returns a uint8 with the bitwise map of the days of the week
// when a Recurrence occurs.
func (a *Alarmclock) Weekdays() uint8 {
	var days uint8 = b2i(a.Days[0]) |
		b2i(a.Days[1])<<1 |
		b2i(a.Days[2])<<2 |
		b2i(a.Days[3])<<3 |
		b2i(a.Days[4])<<4 |
		b2i(a.Days[5])<<5 |
		b2i(a.Days[6])<<6

	return days
} // func (a *Alarmclock) Weekdays() uint8

func b2i(b bool) uint8 {
	if b {
		return 1
	}
	return 0
} // func b2i(b bool) uint8
