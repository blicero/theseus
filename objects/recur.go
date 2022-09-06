// /home/krylon/go/src/github.com/blicero/theseus/objects/recur.go
// -*- mode: go; coding: utf-8; -*-
// Created on 06. 09. 2022 by Benjamin Walkenhorst
// (c) 2022 Benjamin Walkenhorst
// Time-stamp: <2022-09-06 19:47:28 krylon>

//go:generate stringer -type=Recurrence

//go:generate ffjson recur.go

package objects

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
	ID      int64
	Offset  int
	Repeat  Recurrence
	Days    [7]bool
	Limit   int
	Counter int
}
