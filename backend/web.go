// /home/krylon/go/src/github.com/blicero/theseus/backend/web.go
// -*- mode: go; coding: utf-8; -*-
// Created on 04. 07. 2022 by Benjamin Walkenhorst
// (c) 2022 Benjamin Walkenhorst
// Time-stamp: <2022-10-01 19:47:15 krylon>

package backend

import (
	"bytes"
	"fmt"
	"io"
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
	d.router.HandleFunc("/reminder/{id:(?:\\d+)}/set_finished/{flag:(?i:\\w+)}", d.handleReminderSetFinished)

	d.router.HandleFunc("/peer/all", d.handlePeerListGet)
	d.router.HandleFunc("/sync/pull", d.handleReminderSyncPull)
	d.router.HandleFunc("/sync/push", d.handleReminderSyncPush)
	d.router.HandleFunc("/sync/start", d.handleReminderSyncStart)

	return nil
} // func (d *Daemon) initWebHandlers() error

func (d *Daemon) serveHTTP() {
	var err error

	defer d.log.Println("[INFO] Web server is shutting down")

	d.log.Printf("[INFO] Web interface is going online at %s\n", d.web.Addr)
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

func (d *Daemon) handleDBMaintenance(w http.ResponseWriter, r *http.Request) {
	d.log.Printf("[TRACE] Handle %s from %s\n",
		r.URL,
		r.RemoteAddr)
} // func (d *Daemon) handleDBMaintenance(w http.ResponseWriter, r *http.Request)

func (d *Daemon) handleReminderAdd(w http.ResponseWriter, r *http.Request) {
	d.log.Printf("[TRACE] Handle %s from %s\n",
		r.URL,
		r.RemoteAddr)

	var (
		err      error
		rem      objects.Reminder
		db       *database.Database
		msg      string
		response = objects.Response{ID: d.getID()}
	)

	if err = r.ParseForm(); err != nil {
		d.log.Printf("[ERROR] Cannot parse form data: %s\n",
			err.Error())
		response.Message = err.Error()
		goto SEND_RESPONSE
	} else if err = ffjson.Unmarshal([]byte(r.PostFormValue("reminder")), &rem); err != nil {
		d.log.Printf("[ERROR] Cannot parse Reminder: %s\n",
			err.Error())
		response.Message = err.Error()
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
	// d.log.Printf("[TRACE] Handle %s from %s\n",
	// 	r.URL,
	// 	r.RemoteAddr)

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
	// d.log.Printf("[TRACE] Handle %s from %s\n",
	// 	r.URL,
	// 	r.RemoteAddr)

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
		err       error
		db        *database.Database
		jstr, msg string
		remR      objects.Reminder
		remL      *objects.Reminder
		res       = objects.Response{ID: d.getID()}
		txStatus  bool
	)

	if err = r.ParseForm(); err != nil {
		msg = fmt.Sprintf("Cannot parse form data: %s", err.Error())
		d.log.Printf("[ERROR] %s\n", msg)
		res.Message = msg
		goto SEND_RESPONSE
	}

	jstr = r.FormValue("reminder")

	if err = ffjson.Unmarshal([]byte(jstr), &remR); err != nil {
		msg = fmt.Sprintf("Cannot parse Reminder: %s\n%s",
			err.Error(),
			jstr)
		res.Message = msg
		d.log.Printf("[ERROR] %s\n", msg)
		goto SEND_RESPONSE
	}

	db = d.pool.Get()
	defer d.pool.Put(db)

	if remL, err = db.ReminderGetByID(remR.ID); err != nil {
		msg = fmt.Sprintf("Failed to look up Reminder #%d: %s",
			remR.ID,
			err.Error())
		d.log.Printf("[ERROR] %s\n", msg)
		res.Message = msg
		goto SEND_RESPONSE
	} else if remL == nil {
		msg = fmt.Sprintf("Could not find Reminder #%d in database", remR.ID)
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

	if durAbs(remR.Timestamp.Sub(remL.Timestamp)) > time.Minute {
		if err = db.ReminderSetTimestamp(remL, remR.Timestamp); err != nil {
			msg = fmt.Sprintf("Error updating timestamp on Reminder %d: %s",
				remL.ID,
				err.Error())
			d.log.Printf("[ERROR] %s\n", msg)
			res.Message = msg
			goto SEND_RESPONSE
		}
	}

	if remL.Title != remR.Title {
		if err = db.ReminderSetTitle(remL, remR.Title); err != nil {
			msg = fmt.Sprintf("Failed to update Title of Reminder %d from %q to %q: %s",
				remL.ID,
				remL.Title,
				remR.Title,
				err.Error())
			d.log.Printf("[ERROR] %s\n", msg)
			res.Message = msg
			goto SEND_RESPONSE
		}
	}

	if remL.Description != remR.Description {
		if err = db.ReminderSetDescription(remL, remR.Description); err != nil {
			msg = fmt.Sprintf("Failed to update Description of Reminder %d: %s",
				remL.ID,
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
		// } else if err = db.ReminderSetFinished(rem, false); err != nil {
	} else if err = db.ReminderReactivate(rem, time.Now().Add(time.Minute*60)); err != nil {
		msg = fmt.Sprintf("Cannot clear Finished flag for Reminder %d (%q): %s",
			id,
			rem.Title,
			err.Error())
		d.log.Printf("[ERROR] %s\n", msg)
		res.Message = msg
		goto SEND_RESPONSE
	} /* else if err = db.ReminderSetTimestamp(rem, time.Now().Add(time.Second*300)); err != nil {
		msg = fmt.Sprintf("Cannot set Timestamp on Remonder %d (%q): %s",
			id,
			rem.Title,
			err.Error())
		d.log.Printf("[ERROR] %s\n", msg)
		res.Message = msg
		goto SEND_RESPONSE
	} */

	res.Status = true

SEND_RESPONSE:
	if res.Status {
		db.Commit() // nolint: errcheck
	} else {
		db.Rollback() // nolint: errcheck
	}

	d.sendResponseJSON(w, &res)
} // func (d *Daemon) handleReminderReactivate(w http.ResponseWriter, r *http.Request)

func (d *Daemon) handleReminderSetFinished(w http.ResponseWriter, r *http.Request) {
	d.log.Printf("[TRACE] Handle %s from %s\n",
		r.URL,
		r.RemoteAddr)

	var (
		err                 error
		vars                map[string]string
		idstr, msg, flagStr string
		id                  int64
		flag                bool
		db                  *database.Database
		rem                 *objects.Reminder
		res                 = objects.Response{ID: d.getID()}
	)

	vars = mux.Vars(r)

	idstr = vars["id"]
	flagStr = vars["flag"]

	if id, err = strconv.ParseInt(idstr, 10, 64); err != nil {
		msg = fmt.Sprintf("Cannot parse ID %q: %s",
			idstr,
			err.Error())
		d.log.Printf("[ERROR] %s\n", msg)
		res.Message = msg
		goto SEND_RESPONSE
	} else if flag, err = strconv.ParseBool(flagStr); err != nil {
		msg = fmt.Sprintf("Cannot parse Flag %q: %s",
			flagStr,
			err.Error())
		d.log.Printf("[ERROR] %s\n", msg)
		res.Message = msg
		goto SEND_RESPONSE
	}

	db = d.pool.Get()
	defer d.pool.Put(db)

	if err = db.Begin(); err != nil {
		msg = fmt.Sprintf("Cannot start DB transaction: %s",
			err.Error())
		d.log.Printf("[ERROR] %s\n", msg)
		goto SEND_RESPONSE
	} else if rem, err = db.ReminderGetByID(id); err != nil {
		msg = fmt.Sprintf("Cannot get Reminder #%d from DB: %s",
			id,
			err.Error())
		d.log.Printf("[ERROR] %s\n", msg)
		goto SEND_RESPONSE
	} else if rem.Finished == flag {
		msg = fmt.Sprintf("Reminder's Finished flag is already %t",
			flag)
		d.log.Printf("[ERROR] %s\n", msg)
		res.Status = true
		res.Message = msg
		goto SEND_RESPONSE
	} else if err = db.ReminderSetFinished(rem, flag); err != nil {
		msg = fmt.Sprintf("Cannot set Finished flag for Reminder %q (%d) to %t: %s\n",
			rem.Title,
			rem.ID,
			flag,
			err.Error())
		d.log.Printf("[ERROR] %s\n", msg)
		goto SEND_RESPONSE
	} else {
		res.Status = true
		res.Message = "Success"
	}

SEND_RESPONSE:
	if res.Status {
		db.Commit() // nolint: errcheck
	} else {
		db.Rollback() // nolint: errcheck
	}

	d.sendResponseJSON(w, &res)
} // func (d *Daemon) handleReminderSetFinished(w http.ResponseWriter, r *http.request)

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

func (d *Daemon) handleReminderSyncPull(w http.ResponseWriter, r *http.Request) {
	d.log.Printf("[TRACE] Handle %s from %s\n",
		r.URL,
		r.RemoteAddr)

	var (
		err       error
		msg       string
		db        *database.Database
		buf       []byte
		reminders []objects.Reminder
	)

	db = d.pool.Get()
	defer d.pool.Put(db)

	if reminders, err = db.ReminderGetAll(); err != nil {
		msg = fmt.Sprintf("Cannot get list of Reminders from database: %s",
			err.Error())
		d.log.Printf("[ERROR] %s\n",
			msg)
		goto SEND_ERROR
	} else if buf, err = ffjson.Marshal(reminders); err != nil {
		msg = fmt.Sprintf("Cannot serialize Response: %s",
			err.Error())
		d.log.Printf("[ERROR] %s\n",
			msg)
		goto SEND_ERROR
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", strconv.Itoa(len(buf)))
	w.WriteHeader(200)
	w.Write(buf) // nolint: errcheck
	return

SEND_ERROR:
	var res = objects.Response{
		ID:      d.getID(),
		Message: msg,
	}

	if buf, err = ffjson.Marshal(&res); err != nil {
		d.log.Printf("[ERROR] Cannot serizalize Response object: %s\n",
			err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", strconv.Itoa(len(buf)))
	w.WriteHeader(500)
	w.Write(buf) // nolint: errcheck
} // func (d *Daemon) handleReminderSyncPull(w http.ResponseWriter, r *http.Request)

func (d *Daemon) handleReminderSyncPush(w http.ResponseWriter, r *http.Request) {
	d.log.Printf("[TRACE] Handle %s from %s\n",
		r.URL,
		r.RemoteAddr)

	var (
		err    error
		buf    bytes.Buffer
		remote []objects.Reminder
		msg    string
		status bool
		res    objects.Response
	)

	if r.Method != "POST" {
		msg = fmt.Sprintf("Invalid HTTP method %s, POST is required",
			r.Method)
		d.log.Printf("[ERROR] %s\n", msg)
	} else if _, err = io.Copy(&buf, r.Body); err != nil {
		msg = fmt.Sprintf("Cannot read Request body: %s",
			err.Error())
		d.log.Printf("[ERROR] %s\n", msg)
	} else if err = ffjson.Unmarshal(buf.Bytes(), &remote); err != nil {
		msg = fmt.Sprintf("Cannot parse JSON data: %s",
			err.Error())
		d.log.Printf("[ERROR] %s\n", msg)
	} else if err = d.reminderMerge(remote); err != nil {
		msg = fmt.Sprintf("Failed to merge Reminders: %s",
			err.Error())
		d.log.Printf("[ERROR] %s\n", msg)
	} else {
		msg = "Success"
		status = true
	}

	res.ID = d.getID()
	res.Status = status
	res.Message = msg

	d.sendResponseJSON(w, &res)
} // func (d *Daemon) handleReminderSyncPush(w http.ResponseWriter, r *http.Request)

func (d *Daemon) handleReminderSyncStart(w http.ResponseWriter, r *http.Request) {
	d.log.Printf("[TRACE] Handle %s from %s\n",
		r.URL,
		r.RemoteAddr)

	var (
		err  error
		name string
		peer objects.Peer
		svc  service
		ok   bool
		res  = objects.Response{ID: d.getID()}
	)

	if err = r.ParseForm(); err != nil {
		res.Message = fmt.Sprintf("Cannot parse form data: %s",
			err.Error())
		d.log.Printf("[ERROR] %s\n", res.Message)
		goto SEND_RESPONSE
	}

	name = r.FormValue("host")

	d.pLock.RLock()
	if svc, ok = d.peers[name]; !ok {
		d.pLock.RUnlock()
		res.Message = fmt.Sprintf("Could not find Peer %s in cache",
			name)
		d.log.Printf("[ERROR] %s\n", res.Message)
		goto SEND_RESPONSE
	}

	d.pLock.RUnlock()
	peer = svc.mkPeer()

	if err = d.synchronize(&peer); err != nil {
		res.Message = fmt.Sprintf("Error synchronizing with Peer %s: %s",
			name,
			err.Error())
		d.log.Printf("[ERROR] %s\n", res.Message)
	} else {
		res.Status = true
		res.Message = "Success"
	}

SEND_RESPONSE:
	d.sendResponseJSON(w, &res)
} // func (d *Daemon) handleReminderSyncStart(w http.ResponseWriter, r *http.Request)

func (d *Daemon) handlePeerListGet(w http.ResponseWriter, r *http.Request) {
	d.log.Printf("[TRACE] Handle %s from %s\n",
		r.URL,
		r.RemoteAddr)

	var (
		err   error
		buf   []byte
		peers []objects.Peer
	)

	peers = make([]objects.Peer, 0)

	// d.log.Println("[TRACE] Acquire pLock")
	d.pLock.RLock()
	for _, e := range d.peers {
		d.log.Printf("[DEBUG] Peer %s - %s, TTL %d, expires %s\n",
			e.rr.Instance,
			e.timestamp.Format(common.TimestampFormat),
			e.rr.TTL,
			e.timestamp.Add(time.Duration(e.rr.TTL)*time.Second).Format(common.TimestampFormat))
		if !e.isExpired() {
			var p = objects.Peer{
				Instance: e.rr.Instance,
				Hostname: e.rr.HostName,
				Domain:   e.rr.Domain,
				Port:     e.rr.Port,
			}

			peers = append(peers, p)
		}
	}
	d.pLock.RUnlock()
	// d.log.Println("[TRACE] Released pLock")

	if buf, err = ffjson.Marshal(peers); err != nil {
		d.log.Printf("[ERROR] Cannot serialize peer list of %d members: %s\n",
			len(peers),
			err.Error())
		buf = []byte(fmt.Sprintf("Cannot serialize list of %d peers: %s",
			len(peers),
			err.Error()))
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Content-Length", strconv.Itoa(len(buf)))
		w.WriteHeader(500)
		w.Write(buf) // nolint: errcheck
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", strconv.Itoa(len(buf)))
	w.WriteHeader(200)
	w.Write(buf) // nolint: errcheck
} // func (d *Daemon) handlePeerListGet(w http.ResponseWriter, r *http.Request)

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
