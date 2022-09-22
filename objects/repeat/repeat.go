// /home/krylon/go/src/github.com/blicero/theseus/objects/repeat/repeat.go
// -*- mode: go; coding: utf-8; -*-
// Created on 22. 09. 2022 by Benjamin Walkenhorst
// (c) 2022 Benjamin Walkenhorst
// Time-stamp: <2022-09-22 18:25:02 krylon>

//go:generate stringer -type=Repeat

// Package repeat contains symbolic constants
// to specify at what intervals Notifications
// for a Reminder should happen.
package repeat

// Repeat describes how a Reminder get triggered repeatedly and regularly.
type Repeat uint8

// Once means a Reminder goes off only once.
// Daily means it is repeated every day.
// Custom means that the user specifies on which weekdays the Reminder should
// go off (e.g. "only on workdays", or "only on weekends")
const (
	Once Repeat = iota
	Daily
	Custom
)
