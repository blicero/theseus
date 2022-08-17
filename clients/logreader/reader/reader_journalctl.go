// /home/krylon/go/src/github.com/blicero/theseus/clients/logreader/reader/reader_syslog.go
// -*- mode: go; coding: utf-8; -*-
// Created on 16. 08. 2022 by Benjamin Walkenhorst
// (c) 2022 Benjamin Walkenhorst
// Time-stamp: <2022-08-17 19:32:17 krylon>

// +build linux

package reader

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"time"

	"github.com/blicero/theseus/clients/clientlib"
	"github.com/pquerna/ffjson/ffjson"
)

const (
	interval = time.Second * 30
)

//go:generate ffjson -noencoder reader_journalctl.go

type Record struct {
	Priority           int    `json:"PRIORITY,string"`
	GID                int    `json:"_GID,string"`
	Executable         string `json:"_EXE"`
	UserSlice          string `json:"_SYSTEMD_USER_SLICE"`
	Identifier         string `json:"SYSLOG_IDENTIFIER"`
	SystemdSlice       string `json:"_SYSTEMD_SLICE"`
	Cursor             string `json:"__CURSOR"`
	CGroup             string `json:"_SYSTEMD_CGROUP"`
	SeLinuxContext     string `json:"_SELINUX_CONTEXT"`
	Message            string `json:"MESSAGE"`
	Transport          string `json:"_TRANSPORT"`
	Hostname           string `json:"_HOSTNAME"`
	AuditLoginUid      int    `json:"_AUDIT_LOGINUID,string"`
	RealtimeStamp      int64  `json:"__REALTIME_TIMESTAMP,string"`
	Session            int    `json:"_SYSTEMD_SESSION,string"`
	MonotonicTimestamp int64  `json:"__MONOTONIC_TIMESTAMP,string"`
	Uid                int    `json:"_UID,string"`
	Comm               string `json:"_COMM"`
	AuditSession       int    `json:"_AUDIT_SESSION,string"`
}

type ReaderLinux struct {
	client  *clientlib.Client
	logRoot string
	files   []string
}

func NewReader(srv string, logRoot string, files ...string) (*ReaderLinux, error) {
	var (
		err error
		rdr = &ReaderLinux{
			logRoot: logRoot,
			files:   files,
		}
	)

	if rdr.client, err = clientlib.NewClient(srv); err != nil {
		fmt.Fprintf(
			os.Stderr,
			"Cannot create Theseus API client: %s\n",
			err.Error())
		return nil, err
	}

	return rdr, nil
} // func NewReader(srv string, logRoot string, files... string) (*ReaderLinux, error)

func (r *ReaderLinux) GetLogger() *log.Logger {
	return r.client.GetLogger()
}

func (r *ReaderLinux) ReadLog(maxAge time.Duration) ([]LogRecord, error) {
	var (
		err     error
		proc    *exec.Cmd
		output  []byte
		lines   [][]byte
		records []Record
	)

	proc = exec.Command(
		"/usr/bin/journalctl",
		"--output=json",
		"--quiet",
		"--lines=2000",
	)

	if output, err = proc.Output(); err != nil {
		r.GetLogger().Printf("[ERROR] Failed to run journalctl(1): %s\n",
			err.Error())
		return nil, err
	} // else if err = ffjson.Unmarshal(output, records); err != nil {
	// 	r.GetLogger().Printf("[ERROR] Cannot parse JSON output from journalctl(1): %s\n",
	// 		err.Error())
	// 	return nil, err
	// }

	lines = bytes.Split(output, []byte{'\n'})

	var result = make([]LogRecord, 0, len(records))

	for _, line := range lines {
		if len(line) <= 1 {
			continue
		}

		var rec Record
		if err = ffjson.Unmarshal(line, &rec); err != nil {
			r.GetLogger().Printf("[ERROR] Cannot parse JSON output from journalctl(1): %s\n%s\n\n",
				err.Error(),
				line)
			return nil, err
		}

		var l = LogRecord{
			Timestamp: time.Unix(rec.MonotonicTimestamp, 0),
			Source:    rec.Executable,
			Message:   rec.Message, // strings.Join(rec.Message, " "),
			Hash:      "#Hashtag",  // We'll cover this one later.
		}

		result = append(result, l)
	}

	return result, nil
} // func (r *ReaderLinux) ReadLog(maxAge time.Duration) ([]LogRecord, error)

func (r *ReaderLinux) GetRecords(start time.Time) ([]LogRecord, error) {
	var (
		err             error
		records, result []LogRecord
	)

	if records, err = r.ReadLog(time.Minute * 40320); err != nil {
		r.GetLogger().Printf("[ERROR] Failed to read journal: %s\n",
			err.Error())
		return nil, err
	}

	result = make([]LogRecord, 0, len(records))

	for _, r := range records {
		if r.Timestamp.After(start) {
			result = append(result, r)
		}
	}

	return result, nil
} // func (r *ReaderLinux) GetRecords(start time.Time) ([]LogRecord, error)
