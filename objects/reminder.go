// /home/krylon/go/src/github.com/blicero/theseus/objects/reminder.go
// -*- mode: go; coding: utf-8; -*-
// Created on 30. 06. 2022 by Benjamin Walkenhorst
// (c) 2022 Benjamin Walkenhorst
// Time-stamp: <2022-07-05 19:45:57 krylon>

package objects

import "time"

//go:generate ffjson reminder.go

// Reminder is ... a reminder.
type Reminder struct {
	ID          int64
	Title       string
	Description string
	Timestamp   time.Time
	Finished    bool
	UUID        string
}

// Due returns the Reminder's due time
func (r *Reminder) Due() time.Time {
	return r.Timestamp
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
