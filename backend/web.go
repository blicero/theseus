// /home/krylon/go/src/github.com/blicero/theseus/backend/web.go
// -*- mode: go; coding: utf-8; -*-
// Created on 04. 07. 2022 by Benjamin Walkenhorst
// (c) 2022 Benjamin Walkenhorst
// Time-stamp: <2022-07-05 21:27:59 krylon>

package backend

import (
	"fmt"
	"net/http"
	"time"

	"github.com/blicero/theseus/common"
	"github.com/blicero/theseus/database"
	"github.com/blicero/theseus/objects"
	"github.com/pquerna/ffjson/ffjson"
)

func (d *Daemon) initWebHandlers() error {
	d.router.HandleFunc("/reminder/add", d.handleReminderAdd)
	d.router.HandleFunc("/reminder/pending", d.handleReminderGetPending)

	return nil
} // func (d *Daemon) initWebHandlers() error

func (d *Daemon) handleReminderAdd(w http.ResponseWriter, r *http.Request) {
	d.log.Printf("[TRACE] Handle %s from %s\n",
		r.URL,
		r.RemoteAddr)

	var (
		err       error
		rem       objects.Reminder
		db        *database.Database
		tstr, msg string
		response  = objects.Response{ID: d.getID()}
	)

	if err = r.ParseForm(); err != nil {
		d.log.Printf("[ERROR] Cannot parse form data: %s\n",
			err.Error())
		response.Message = err.Error()
		goto SEND_RESPONSE
	}

	rem.Title = r.PostFormValue("title")
	rem.Description = r.PostFormValue("body")
	tstr = r.PostFormValue("time")

	if rem.Timestamp, err = time.Parse(time.RFC3339, tstr); err != nil {
		msg = fmt.Sprintf("Cannot parse time stamp %q: %s",
			tstr,
			err.Error())
		d.log.Printf("[ERROR] %s\n", msg)
		response.Message = msg
		goto SEND_RESPONSE
	}

	rem.UUID = common.GetUUID()

	db = d.pool.Get()
	defer d.pool.Put(db)

	if err = db.ReminderAdd(&rem); err != nil {
		msg = fmt.Sprintf("Cannot add Reminder %q to database: %s",
			rem.Title,
			err.Error())
		d.log.Printf("[ERROR] %s\n", msg)
		response.Message = msg
		goto SEND_RESPONSE
	}

	response.Message = rem.UUID
	response.Status = true

SEND_RESPONSE:
	d.sendResponseJSON(w, &response)
} // func (d *Daemon) handleReminderAdd(w http.ResponseWriter, r *http.Request)

func (d *Daemon) handleReminderGetPending(w http.ResponseWriter, r *http.Request) {
	d.log.Printf("[TRACE] Handle %s from %s\n",
		r.URL,
		r.RemoteAddr)

	var (
		err       error
		db        *database.Database
		reminders []objects.Reminder
		buf       []byte
	)

	db = d.pool.Get()
	defer d.pool.Put(db)

	if reminders, err = db.ReminderGetPending(); err != nil {
		d.log.Printf("[ERROR] Cannot load Reminders: %s\n",
			err.Error())
	}

	if buf, err = ffjson.Marshal(reminders); err != nil {
		d.log.Printf("[ERROR] Cannot serialize Reminder list: %s\n",
			err.Error())

	}

	defer ffjson.Pool(buf)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	w.Write(buf) // nolint: errcheck
} // func (d *Daemon) handleReminderGetPending(w http.ResponseWriter, r *http.Request)

//////////////////////////////////////////////////////////////////////////////////////////////////
/// Helpers //////////////////////////////////////////////////////////////////////////////////////
//////////////////////////////////////////////////////////////////////////////////////////////////

func (d *Daemon) sendResponseJSON(w http.ResponseWriter, res *objects.Response) {
	var (
		err error
		buf []byte
	)

	if buf, err = ffjson.Marshal(res); err != nil {
		d.log.Printf("[ERROR] Cannot serialize Response object %#v: %s\n",
			res,
			err.Error())
		return
	}

	defer ffjson.Pool(buf)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	w.Write(buf) // nolint: errcheck
} // func (d *Daemon) sendErrorMessageJSON(w http.ResponseWriter, msg string)

func (d *Daemon) getID() int64 {
	d.idLock.Lock()
	d.idCnt++
	var id = d.idCnt
	d.idLock.Unlock()
	return id
} // func (d *Daemon) getID() int64
