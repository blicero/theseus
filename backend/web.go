// /home/krylon/go/src/github.com/blicero/theseus/backend/web.go
// -*- mode: go; coding: utf-8; -*-
// Created on 04. 07. 2022 by Benjamin Walkenhorst
// (c) 2022 Benjamin Walkenhorst
// Time-stamp: <2022-07-22 18:40:49 krylon>

package backend

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/blicero/theseus/common"
	"github.com/blicero/theseus/database"
	"github.com/blicero/theseus/objects"
	"github.com/gorilla/mux"
	"github.com/pquerna/ffjson/ffjson"
)

func (d *Daemon) initWebHandlers() error {
	d.router.HandleFunc("/reminder/add", d.handleReminderAdd)
	d.router.HandleFunc("/reminder/pending", d.handleReminderGetPending)
	d.router.HandleFunc("/reminder/all", d.handleReminderGetAll)
	d.router.HandleFunc("/reminder/edit/title", d.handleReminderSetTitle)
	d.router.HandleFunc("/reminder/edit/timestamp", d.handleReminderSetTimestamp)
	d.router.HandleFunc("/reminder/{id:(?:\\d+)}/update", d.handleReminderUpdate)
	d.router.HandleFunc("/reminder/{id:(?:\\d+)}/reactivate", d.handleReminderReactivate)
	d.router.HandleFunc("/reminder/{id:(?:\\d+)}/delete", d.handleReminderDelete)

	return nil
} // func (d *Daemon) initWebHandlers() error

func (d *Daemon) serveHTTP() {
	var err error

	defer d.log.Println("[INFO] Web server is shutting down")

	d.log.Printf("[INFO] Web frontend is going online at %s\n", d.web.Addr)
	http.Handle("/", d.router)

	if err = d.web.ListenAndServe(); err != nil {
		if err != http.ErrServerClosed {
			d.log.Printf("[ERROR] ListenAndServe returned an error: %s\n",
				err.Error())
		} else {
			d.log.Println("[INFO] HTTP Server has shut down.")
		}
	}
} // func (d *Daemon) serveHTTP()

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
		deadline  = time.Now().Add(queueTimeout)
	)

	db = d.pool.Get()
	defer d.pool.Put(db)

	if reminders, err = db.ReminderGetPending(deadline); err != nil {
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

func (d *Daemon) handleReminderGetAll(w http.ResponseWriter, r *http.Request) {
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

	if reminders, err = db.ReminderGetAll(); err != nil {
		d.log.Printf("[ERROR] Cannot load Reminders: %s\n",
			err.Error())

	} else if buf, err = ffjson.Marshal(reminders); err != nil {
		d.log.Printf("[ERROR] Cannot serialize Reminder list: %s\n",
			err.Error())

	}

	defer ffjson.Pool(buf)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	w.Write(buf) // nolint: errcheck
} // func (d *Daemon) handleReminderGetAll(w http.ResponseWriter, r *http.Request)

func (d *Daemon) handleReminderSetTitle(w http.ResponseWriter, r *http.Request) {
	d.log.Printf("[TRACE] Handle %s from %s\n",
		r.URL,
		r.RemoteAddr)

	var (
		err               error
		db                *database.Database
		idstr, title, msg string
		id                int64
		rem               *objects.Reminder
		response          = objects.Response{ID: d.getID()}
	)

	if err = r.ParseForm(); err != nil {
		msg = fmt.Sprintf("Cannot parse form data: %s",
			err.Error())
		d.log.Printf("[ERROR] %s\n", msg)
		response.Message = msg
		goto SEND_RESPONSE
	}

	idstr = r.FormValue("id")
	title = r.FormValue("title")

	if id, err = strconv.ParseInt(idstr, 10, 64); err != nil {
		msg = fmt.Sprintf("Cannot parse ID %q: %s",
			idstr,
			err.Error())
		d.log.Printf("[CANTHAPPEN] %s\n", msg)
		response.Message = msg
		goto SEND_RESPONSE
	}

	db = d.pool.Get()
	defer d.pool.Put(db)

	if rem, err = db.ReminderGetByID(id); err != nil {
		msg = fmt.Sprintf("Failed to get Reminder #%d: %s",
			id,
			err.Error())
		d.log.Printf("[ERROR] %s\n", msg)
		response.Message = msg
		goto SEND_RESPONSE
	} else if rem == nil {
		msg = fmt.Sprintf("Reminder #%d was not found in database",
			id)
		d.log.Printf("[DEBUG] %s\n", msg)
		response.Message = msg
		goto SEND_RESPONSE
	} else if err = db.ReminderSetTitle(rem, title); err != nil {
		msg = fmt.Sprintf("Cannot update Title of Reminder %d (%q): %s",
			id,
			rem.Title,
			err.Error())
		d.log.Printf("[ERROR] %s\n", err.Error())
		response.Message = msg
		goto SEND_RESPONSE
	}

	response.Message = fmt.Sprintf("Title was updated to %q", title)
	response.Status = true

SEND_RESPONSE:
	d.sendResponseJSON(w, &response)
} // func (d *Daemon) handleReminderSetTitle(w http.ResponseWriter, r *http.Request)

func (d *Daemon) handleReminderSetTimestamp(w http.ResponseWriter, r *http.Request) {
	d.log.Printf("[TRACE] Handle %s from %s\n",
		r.URL,
		r.RemoteAddr)

	var (
		err              error
		db               *database.Database
		tstr, idstr, msg string
		id               int64
		t                time.Time
		rem              *objects.Reminder
		res              = objects.Response{ID: d.getID()}
	)

	if err = r.ParseForm(); err != nil {
		msg = fmt.Sprintf("Cannot parse form data: %s", err.Error())
		d.log.Printf("[ERROR] %s\n", msg)
		res.Message = msg
		goto SEND_RESPONSE
	}

	idstr = r.FormValue("id")
	tstr = r.FormValue("timestamp")

	if id, err = strconv.ParseInt(idstr, 10, 64); err != nil {
		msg = fmt.Sprintf("Cannot parse ID %q: %s",
			idstr,
			err.Error())
		d.log.Printf("[ERROR] %s\n", msg)
		res.Message = msg
		goto SEND_RESPONSE
	} else if t, err = time.Parse(time.RFC3339, tstr); err != nil {
		msg = fmt.Sprintf("Cannot parse timestamp %q: %s",
			tstr,
			err.Error())
		d.log.Printf("[ERROR] %s\n", msg)
		res.Message = msg
		goto SEND_RESPONSE
	}

	db = d.pool.Get()
	defer d.pool.Put(db)

	if rem, err = db.ReminderGetByID(id); err != nil {
		msg = fmt.Sprintf("Failed to look up Reminder #%d: %s",
			id,
			err.Error())
		d.log.Printf("[ERROR] %s\n", msg)
		res.Message = msg
		goto SEND_RESPONSE
	} else if err = db.ReminderSetTimestamp(rem, t); err != nil {
		msg = fmt.Sprintf("Failed to update Timestamp of Reminder %d (%q) to %s\n",
			id,
			rem.Title,
			tstr)
		d.log.Printf("[ERROR] %s\n", msg)
		res.Message = msg
		goto SEND_RESPONSE
	}

	res.Status = true
	res.Message = "OK"

SEND_RESPONSE:
	d.sendResponseJSON(w, &res)
} // func (d *Daemon) handleReminderSetTimestamp(w http.ResponseWriter, r *http.Request)

func (d *Daemon) handleReminderUpdate(w http.ResponseWriter, r *http.Request) {
	d.log.Printf("[TRACE] Handle %s from %s\n",
		r.URL,
		r.RemoteAddr)

	var (
		err                                 error
		db                                  *database.Database
		id                                  int64
		t                                   time.Time
		idstr, tstr, titleStr, bodyStr, msg string
		rem                                 *objects.Reminder
		res                                 = objects.Response{ID: d.getID()}
		txStatus                            bool
	)

	vars := mux.Vars(r)

	if err = r.ParseForm(); err != nil {
		msg = fmt.Sprintf("Cannot parse form data: %s", err.Error())
		d.log.Printf("[ERROR] %s\n", msg)
		res.Message = msg
		goto SEND_RESPONSE
	}

	idstr = vars["id"]
	tstr = r.FormValue("timestamp")
	titleStr = r.FormValue("title")
	bodyStr = r.FormValue("body")

	db = d.pool.Get()
	defer d.pool.Put(db)

	if id, err = strconv.ParseInt(idstr, 10, 64); err != nil {
		msg = fmt.Sprintf("Cannot parse ID %q: %s",
			idstr,
			err.Error())
		d.log.Printf("[ERROR] %s\n", msg)
		res.Message = msg
		goto SEND_RESPONSE
	} else if t, err = time.Parse(time.RFC3339, tstr); err != nil {
		msg = fmt.Sprintf("Cannot parse timestamp %q: %s",
			tstr,
			err.Error())
		d.log.Printf("[ERROR] %s\n", msg)
		res.Message = msg
		goto SEND_RESPONSE
	}

	if rem, err = db.ReminderGetByID(id); err != nil {
		msg = fmt.Sprintf("Failed to look up Reminder #%d: %s",
			id,
			err.Error())
		d.log.Printf("[ERROR] %s\n", msg)
		res.Message = msg
		goto SEND_RESPONSE
	} else if rem == nil {
		msg = fmt.Sprintf("Could not find Reminder #%d in database", id)
		d.log.Printf("[DEBUG] %s\n", msg)
		res.Message = msg
		goto SEND_RESPONSE
	} else if err = db.Begin(); err != nil {
		msg = fmt.Sprintf("Error starting transaction: %s",
			err.Error())
		d.log.Printf("[ERROR] %s\n", msg)
		res.Message = msg
		goto SEND_RESPONSE
	}

	if durAbs(rem.Timestamp.Sub(t)) > time.Minute {
		if err = db.ReminderSetTimestamp(rem, t); err != nil {
			msg = fmt.Sprintf("Error updating timestamp on Reminder %d: %s",
				rem.ID,
				err.Error())
			d.log.Printf("[ERROR] %s\n", msg)
			res.Message = msg
			goto SEND_RESPONSE
		}
	}

	if rem.Title != titleStr {
		if err = db.ReminderSetTitle(rem, titleStr); err != nil {
			msg = fmt.Sprintf("Failed to update Title of Reminder %d from %q to %q: %s",
				rem.ID,
				rem.Title,
				titleStr,
				err.Error())
			d.log.Printf("[ERROR] %s\n", msg)
			res.Message = msg
			goto SEND_RESPONSE
		}
	}

	if rem.Description != bodyStr {
		if err = db.ReminderSetDescription(rem, bodyStr); err != nil {
			msg = fmt.Sprintf("Failed to update Description of Reminder %d: %s",
				rem.ID,
				err.Error())
			d.log.Printf("[ERROR] %s\n", msg)
			res.Message = msg
			goto SEND_RESPONSE
		}
	}

	res.Status = true
	res.Message = "OK"
	txStatus = true

SEND_RESPONSE:
	if txStatus {
		if err = db.Commit(); err != nil {
			msg = fmt.Sprintf("Error committing transaction: %s",
				err.Error())
			d.log.Printf("[ERROR] %s\n", msg)
			res.Message = msg
			res.Status = false
		}
	} else {
		if err = db.Rollback(); err != nil {
			msg = fmt.Sprintf("Failed to rollback transaction: %s",
				err.Error())
			d.log.Printf("[ERROR] %s\n", msg)
		}
	}

	d.sendResponseJSON(w, &res)
} // func (d *Daemon) handleReminderUpdate(w http.ResponseWriter, r *http.Request)

func (d *Daemon) handleReminderReactivate(w http.ResponseWriter, r *http.Request) {
	d.log.Printf("[TRACE] Handle %s from %s\n",
		r.URL,
		r.RemoteAddr)

	var (
		err        error
		vars       map[string]string
		idstr, msg string
		id         int64
		db         *database.Database
		rem        *objects.Reminder
		res        = objects.Response{ID: d.getID()}
	)

	vars = mux.Vars(r)

	idstr = vars["id"]

	if id, err = strconv.ParseInt(idstr, 10, 64); err != nil {
		msg = fmt.Sprintf("Cannot parse ID %q: %s",
			idstr,
			err.Error())
		d.log.Printf("[ERROR] %s\n", msg)
		res.Message = msg
		goto SEND_RESPONSE
	}

	db = d.pool.Get()
	defer d.pool.Put(db)

	if err = db.Begin(); err != nil {
		msg = fmt.Sprintf("Error starting transaction: %s",
			err.Error())
		d.log.Printf("[ERROR] %s\n", msg)
		res.Message = msg
		goto SEND_RESPONSE
	} else if rem, err = db.ReminderGetByID(id); err != nil {
		msg = fmt.Sprintf("Cannot lookup Reminder by ID %d: %s",
			id,
			err.Error())
		d.log.Printf("[ERROR] %s\n", msg)
		res.Message = msg
		goto SEND_RESPONSE
	} else if rem == nil {
		msg = fmt.Sprintf("Did not find Reminder %d in database", id)
		d.log.Printf("[INFO] %s\n", msg)
		res.Message = msg
		goto SEND_RESPONSE
	} else if err = db.ReminderSetFinished(rem, false); err != nil {
		msg = fmt.Sprintf("Cannot clear Finished flag for Reminder %d (%q): %s",
			id,
			rem.Title,
			err.Error())
		d.log.Printf("[ERROR] %s\n", msg)
		res.Message = msg
		goto SEND_RESPONSE
	} else if err = db.ReminderSetTimestamp(rem, time.Now().Add(time.Second*300)); err != nil {
		msg = fmt.Sprintf("Cannot set Timestamp on Remonder %d (%q): %s",
			id,
			rem.Title,
			err.Error())
		d.log.Printf("[ERROR] %s\n", msg)
		res.Message = msg
		goto SEND_RESPONSE
	}

	res.Status = true

SEND_RESPONSE:
	if res.Status {
		db.Commit() // nolint: errcheck
	} else {
		db.Rollback() // nolint: errcheck
	}

	d.sendResponseJSON(w, &res)
} // func (d *Daemon) handleReminderReactivate(w http.ResponseWriter, r *http.request)

func (d *Daemon) handleReminderDelete(w http.ResponseWriter, r *http.Request) {
	d.log.Printf("[TRACE] Handle %s from %s\n",
		r.URL,
		r.RemoteAddr)

	var (
		err        error
		vars       map[string]string
		idstr, msg string
		id         int64
		db         *database.Database
		rem        *objects.Reminder
		res        = objects.Response{ID: d.getID()}
	)

	vars = mux.Vars(r)

	idstr = vars["id"]

	if id, err = strconv.ParseInt(idstr, 10, 64); err != nil {
		msg = fmt.Sprintf("Cannot parse ID %q: %s",
			idstr,
			err.Error())
		d.log.Printf("[ERROR] %s\n", msg)
		res.Message = msg
		goto SEND_RESPONSE
	}

	db = d.pool.Get()
	defer d.pool.Put(db)

	if rem, err = db.ReminderGetByID(id); err != nil {
		msg = fmt.Sprintf("Cannot lookup Reminder by ID %d: %s",
			id,
			err.Error())
		d.log.Printf("[ERROR] %s\n", msg)
		res.Message = msg
		goto SEND_RESPONSE
	} else if rem == nil {
		msg = fmt.Sprintf("Did not find Reminder %d in database", id)
		d.log.Printf("[INFO] %s\n", msg)
		res.Message = msg
		goto SEND_RESPONSE
	} else if err = db.ReminderDelete(rem); err != nil {
		msg = fmt.Sprintf("Failed to delete Reminder %d (%q): %s",
			id,
			rem.Title,
			err.Error())
		d.log.Printf("[ERROR] %s\n", msg)
		res.Message = msg
		goto SEND_RESPONSE
	}

	res.Message = fmt.Sprintf("Reminder %d (%q) was deleted",
		id,
		rem.Title)
	res.Status = true

SEND_RESPONSE:
	d.sendResponseJSON(w, &res)
} // func (d *Daemon) handleReminderDelete(w http.ResponseWriter, r *http.Request)

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
