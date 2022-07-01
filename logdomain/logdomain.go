// /home/krylon/go/src/github.com/blicero/raconteur/logdomain/logdomain.go
// -*- mode: go; coding: utf-8; -*-
// Created on 06. 09. 2021 by Benjamin Walkenhorst
// (c) 2021 Benjamin Walkenhorst
// Time-stamp: <2022-07-01 21:22:03 krylon>

// Package logdomain provides constants for log sources.
package logdomain

//go:generate stringer -type=ID

// ID represents a log source
type ID uint8

// These constants signify the various parts of the application.
const (
	Common ID = iota
	DBPool
	Database
	GUI
	Backend
)

// AllDomains returns a slice of all the known log sources.
func AllDomains() []ID {
	return []ID{
		Common,
		DBPool,
		Database,
		GUI,
		Backend,
	}
} // func AllDomains() []ID
