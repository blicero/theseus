// /home/krylon/go/src/github.com/blicero/theseus/database/query/query.go
// -*- mode: go; coding: utf-8; -*-
// Created on 30. 06. 2022 by Benjamin Walkenhorst
// (c) 2022 Benjamin Walkenhorst
// Time-stamp: <2022-09-06 21:41:00 krylon>

// Package query provides symbolic constants for identifying SQL queries.
package query

//go:generate stringer -type=ID

type ID uint8

const (
	ReminderAdd ID = iota
	ReminderDelete
	ReminderSetFinished
	ReminderSetTitle
	ReminderSetDescription
	ReminderSetTimestamp
	ReminderSetChanged
	ReminderReactivate
	ReminderGetPending
	ReminderGetFinished
	ReminderGetByID
	ReminderGetAll
	RecurrenceAdd
	RecurrenceDelete
	RecurrenceSetOffset
	RecurrenceSetMax
	RecurrenceIsMax
	RecurrenceIncCount
	RecurrenceHasMax
)
