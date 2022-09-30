// /home/krylon/go/src/github.com/blicero/theseus/ui/contextmenu.go
// -*- mode: go; coding: utf-8; -*-
// Created on 28. 09. 2022 by Benjamin Walkenhorst
// (c) 2022 Benjamin Walkenhorst
// Time-stamp: <2022-09-30 18:24:30 krylon>

package ui

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/blicero/theseus/common"
	"github.com/blicero/theseus/objects"
	"github.com/blicero/theseus/objects/repeat"
	"github.com/gotk3/gotk3/gdk"
	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
	"github.com/pquerna/ffjson/ffjson"
)

func (g *GUI) handleReminderClick(view *gtk.TreeView, evt *gdk.Event) {
	var be = gdk.EventButtonNewFromEvent(evt)

	if be.Button() != gdk.BUTTON_SECONDARY {
		return
	}

	var (
		err    error
		msg    string
		exists bool
		x, y   float64
		path   *gtk.TreePath
		col    *gtk.TreeViewColumn
		model  *gtk.TreeModel
		imodel gtk.ITreeModel
		iter   *gtk.TreeIter
	)

	x = be.X()
	y = be.Y()

	path, col, _, _, exists = view.GetPathAtPos(int(x), int(y))

	if !exists {
		g.log.Printf("[DEBUG] There is no item at %f/%f\n",
			x,
			y)
		return
	}

	g.log.Printf("[DEBUG] Handle Click at %f/%f -> Path %s\n",
		x,
		y,
		path)

	if imodel, err = view.GetModel(); err != nil {
		g.log.Printf("[ERROR] Cannot get Model from View: %s\n",
			err.Error())
		return
	}

	model = imodel.ToTreeModel()

	if iter, err = model.GetIter(path); err != nil {
		g.log.Printf("[ERROR] Cannot get Iter from TreePath %s: %s\n",
			path,
			err.Error())
		return
	}

	var title string = col.GetTitle()
	g.log.Printf("[DEBUG] Column %s was clicked\n",
		title)

	var (
		val *glib.Value
		gv  interface{}
		id  int64
	)

	if val, err = model.GetValue(iter, 0); err != nil {
		g.log.Printf("[ERROR] Cannot get value for column 0: %s\n",
			err.Error())
		return
	} else if gv, err = val.GoValue(); err != nil {
		g.log.Printf("[ERROR] Cannot get Go value from GLib value: %s\n",
			err.Error())
	}

	switch v := gv.(type) {
	case int:
		id = int64(v)
	case int64:
		id = v
	default:
		g.log.Printf("[ERROR] Unexpected type for ID column: %T\n",
			v)
	}

	g.log.Printf("[DEBUG] ID of clicked-on row is %d\n",
		id)

	var (
		r    objects.Reminder
		menu *gtk.Menu
	)

	if r, exists = g.reminders[id]; !exists {
		msg = fmt.Sprintf("Cannot find Reminder #%d",
			id)
		g.log.Printf("[ERROR] %s\n",
			msg)
		g.pushMsg(msg)
		return
	} else if menu, err = g.mkReminderContextMenu(path, &r); err != nil {
		msg = fmt.Sprintf("Cannot create context menu for Reminder %q: %s",
			r.Title,
			err.Error())
		g.log.Printf("[ERROR] %s\n",
			msg)
		g.pushMsg(msg)
		return
	}

	menu.ShowAll()
	menu.PopupAtPointer(evt)
} // func (g *GUI) handleReminderClick(view *gtk.TreeView, evt *gdk.Event)

func (g *GUI) mkReminderContextMenu(path *gtk.TreePath, r *objects.Reminder) (*gtk.Menu, error) {
	var (
		err               error
		msg               string
		editItem, delItem *gtk.MenuItem
		toggleItem        *gtk.CheckMenuItem
		menu              *gtk.Menu
	)

	if menu, err = gtk.MenuNew(); err != nil {
		msg = fmt.Sprintf("Cannot create Menu: %s",
			err.Error())
		goto ERROR
	} else if editItem, err = gtk.MenuItemNewWithMnemonic("_Edit"); err != nil {
		msg = fmt.Sprintf("Cannot create menu item Edit: %s",
			err.Error())
		goto ERROR
	} else if delItem, err = gtk.MenuItemNewWithMnemonic("_Delete"); err != nil {
		msg = fmt.Sprintf("Cannot create menu item Delete: %s",
			err.Error())
		goto ERROR
	} else if toggleItem, err = gtk.CheckMenuItemNewWithMnemonic("_Active"); err != nil {
		msg = fmt.Sprintf("Cannot create menu item Active: %s",
			err.Error())
		goto ERROR
	}

	toggleItem.SetActive(!r.Finished)

	// Set up signal handlers!
	editItem.Connect("activate", g.handleReminderClickEdit)
	delItem.Connect("activate", g.handleReminderClickDelete)
	toggleItem.Connect("toggled", g.handleReminderClickToggleActive)

	menu.Append(editItem)
	menu.Append(delItem)
	menu.Append(toggleItem)

	return menu, nil
ERROR:
	g.log.Printf("[ERROR] %s\n", msg)
	g.pushMsg(msg)
	g.displayMsg(msg)
	return nil, err
} // func (g *GUI) mkReminderContextMenu(path *gtk.TreePath, r *objects.Reminder) (*gtk.Menu, error)

func (g *GUI) handleReminderClickEdit() {
	var (
		err                                error
		r                                  objects.Reminder
		msg                                string
		sel                                *gtk.TreeSelection
		iter                               *gtk.TreeIter
		imodel                             gtk.ITreeModel
		model                              *gtk.TreeModel
		ok                                 bool
		dlg                                *gtk.Dialog
		dbox                               *gtk.Box
		grid                               *gtk.Grid
		cal                                *gtk.Calendar
		titleEntry, bodyEntry              *gtk.Entry
		hourInput, minuteInput             *gtk.SpinButton
		timeLbl, sepLbl, titleLbl, bodyLbl *gtk.Label
		finishedCB                         *gtk.CheckButton
		recEdit                            *RecurEditor
		id                                 int64
		gval                               *glib.Value
		rval                               any
	)

	if sel, err = g.view.GetSelection(); err != nil {
		msg = fmt.Sprintf("Failed to get Selection from TreeView: %s",
			err.Error())
		g.displayMsg(msg)
		g.log.Printf("[ERROR] %s\n", msg)
		return
	} else if imodel, iter, ok = sel.GetSelected(); !ok || iter == nil {
		g.log.Println("[ERROR] Could not get TreeIter from TreeSelection")
		return
	}

	model = imodel.ToTreeModel()
	// iter = g.filter.ConvertIterToChildIter(iter)

	if gval, err = model.GetValue(iter, 0); err != nil {
		msg = fmt.Sprintf("Error getting Column ID: %s",
			err.Error())
		g.log.Printf("[ERROR] %s\n", msg)
		g.displayMsg(msg)
		return
	} else if rval, err = gval.GoValue(); err != nil {
		msg = fmt.Sprintf("Error getting GoValue from glib.Value: %s",
			err.Error())
		g.log.Printf("[ERROR] %s\n", msg)
		g.displayMsg(msg)
		return
	}

	id = int64(rval.(int))

	if r, ok = g.reminders[id]; !ok {
		msg = fmt.Sprintf("Reminder with ID %d was not found", id)
		g.log.Printf("[ERROR] %s\n", msg)
		g.displayMsg(msg)
		return
	} else if dlg, err = gtk.DialogNewWithButtons(
		fmt.Sprintf("Edit %q", r.Title),
		g.win,
		gtk.DIALOG_MODAL,
		[]any{
			"_Cancel",
			gtk.RESPONSE_CANCEL,
			"_OK",
			gtk.RESPONSE_OK,
		},
	); err != nil {
		g.log.Printf("[ERROR] Failed to create Dialog: %s\n",
			err.Error())
		return
	}

	defer dlg.Close()

	if _, err = dlg.AddButton("_OK", gtk.RESPONSE_OK); err != nil {
		g.log.Printf("[ERROR] Cannot add OK button to dialog: %s\n",
			err.Error())
		return
	} else if grid, err = gtk.GridNew(); err != nil {
		g.log.Printf("[ERROR] Cannot create gtk.Grid: %s\n",
			err.Error())
		return
	} else if cal, err = gtk.CalendarNew(); err != nil {
		g.log.Printf("[ERROR] Cannot create gtk.Calendar: %s\n",
			err.Error())
	} else if hourInput, err = gtk.SpinButtonNewWithRange(0, 23, 1); err != nil {
		g.log.Printf("[ERROR] Cannot greate hourInput: %s\n",
			err.Error())
		return
	} else if minuteInput, err = gtk.SpinButtonNewWithRange(0, 59, 1); err != nil {
		g.log.Printf("[ERROR] Cannot greate minuteInput: %s\n",
			err.Error())
		return
	} else if timeLbl, err = gtk.LabelNew("Time"); err != nil {
		g.log.Printf("[ERROR] Cannot create time label: %s\n",
			err.Error())
		return
	} else if sepLbl, err = gtk.LabelNew(":"); err != nil {
		g.log.Printf("[ERROR] Cannot create separator label: %s\n",
			err.Error())
		return
	} else if titleLbl, err = gtk.LabelNew("Title:"); err != nil {
		g.log.Printf("[ERROR] Cannot create Title label: %s\n",
			err.Error())
		return
	} else if bodyLbl, err = gtk.LabelNew("Description:"); err != nil {
		g.log.Printf("[ERROR] Cannot create Body Label: %s\n",
			err.Error())
		return
	} else if titleEntry, err = gtk.EntryNew(); err != nil {
		g.log.Printf("[ERROR] Cannot create Entry for title: %s\n",
			err.Error())
		return
	} else if bodyEntry, err = gtk.EntryNew(); err != nil {
		g.log.Printf("[ERROR] Cannot create Entry for Body: %s\n",
			err.Error())
		return
	} else if finishedCB, err = gtk.CheckButtonNewWithLabel("Finished?"); err != nil {
		g.log.Printf("[ERROR] Cannot create CheckButton: %s\n",
			err.Error())
		return
	} else if recEdit, err = NewRecurEditor(&r.Recur, g.log); err != nil {
		g.log.Printf("[ERROR] Cannot create Recurrence Editor: %s\n",
			err.Error())
		return
	} else if dbox, err = dlg.GetContentArea(); err != nil {
		g.log.Printf("[ERROR] Cannot get ContentArea of Dialog: %s\n",
			err.Error())
		return
	}

	grid.InsertColumn(0)
	grid.InsertColumn(1)
	grid.InsertColumn(2)
	grid.InsertColumn(3)
	grid.InsertRow(0)
	grid.InsertRow(1)
	grid.InsertRow(2)
	grid.InsertRow(3)
	grid.InsertRow(4)
	grid.InsertRow(5)

	grid.Attach(cal, 0, 0, 4, 1)
	grid.Attach(timeLbl, 0, 1, 1, 1)
	grid.Attach(hourInput, 1, 1, 1, 1)
	grid.Attach(sepLbl, 2, 1, 1, 1)
	grid.Attach(minuteInput, 3, 1, 1, 1)
	grid.Attach(titleLbl, 0, 2, 1, 1)
	grid.Attach(titleEntry, 1, 2, 3, 1)
	grid.Attach(bodyLbl, 0, 3, 1, 1)
	grid.Attach(bodyEntry, 1, 3, 3, 1)
	grid.Attach(finishedCB, 0, 4, 3, 1)
	grid.Attach(recEdit.box, 0, 5, 3, 1)

	// Does it make any sense, like, at all, to edit a finished
	// Reminder and save it as "finished"?
	// Not Really, eh?
	finishedCB.SetActive(false)

	recEdit.rtCombo.Connect("changed",
		func() {
			switch txt := recEdit.rtCombo.GetActiveText(); txt {
			case repeat.Once.String():
				cal.SetSensitive(true)
				hourInput.SetSensitive(true)
				minuteInput.SetSensitive(true)
			case repeat.Daily.String():
				fallthrough
			case repeat.Custom.String():
				cal.SetSensitive(false)
				hourInput.SetSensitive(false)
				minuteInput.SetSensitive(false)
			}
		})
	recEdit.rtCombo.SetActive(int(r.Recur.Repeat))

	dbox.PackStart(grid, true, true, 0)
	dlg.ShowAll()

BEGIN:
	if r.Recur.Repeat == repeat.Once {
		cal.SelectMonth(uint(r.Timestamp.Month())-1, uint(r.Timestamp.Year()))
		cal.SelectDay(uint(r.Timestamp.Day()))

		hourInput.SetValue(float64(r.Timestamp.Hour()))
		minuteInput.SetValue(float64(r.Timestamp.Minute()) + 10)
	} else {
		var min, hour int

		hour = r.Recur.Offset / 3600
		min = (r.Recur.Offset % 3600) / 60
		hourInput.SetValue(float64(hour))
		minuteInput.SetValue(float64(min))
	}

	titleEntry.SetText(r.Title)
	bodyEntry.SetText(r.Description)

	var res = dlg.Run()

	switch res {
	case gtk.RESPONSE_NONE:
		fallthrough
	case gtk.RESPONSE_DELETE_EVENT:
		fallthrough
	case gtk.RESPONSE_CLOSE:
		fallthrough
	case gtk.RESPONSE_CANCEL:
		g.log.Println("[DEBUG] User changed their mind about adding a Program. Fine with me.")
		return
	case gtk.RESPONSE_OK:
		// 's ist los, Hund?
	default:
		g.log.Printf("[CANTHAPPEN] Well, I did NOT see this coming: %d\n",
			res)
		return
	}

	var (
		year, month, day uint
		hour, min        int
	)

	year, month, day = cal.GetDate()
	hour = hourInput.GetValueAsInt()
	min = minuteInput.GetValueAsInt()

	if r.Recur.Repeat == repeat.Once {
		r.Timestamp = time.Date(
			int(year),
			time.Month(month+1),
			int(day),
			hour,
			min,
			0,
			0,
			time.Local)
	} else {
		r.Recur = recEdit.GetRecurrence()
		r.Timestamp = time.Unix(int64(r.Recur.Offset), 0).In(time.UTC)
	}

	r.Title, _ = titleEntry.GetText()
	r.Description, _ = bodyEntry.GetText()
	r.Finished = finishedCB.GetActive()

	g.log.Printf("[DEBUG] Reminder: %#v\n",
		&r)

	if r.Recur.Repeat == repeat.Once && r.Timestamp.Before(time.Now()) {
		var msg = fmt.Sprintf("The time you selected is in the past: %s",
			r.Timestamp.Format(common.TimestampFormat))
		g.displayMsg(msg)
		g.log.Printf("[ERROR] %s\n", msg)
		goto BEGIN
	} else if r.Title == "" {
		var msg = "You did not enter a title"
		g.displayMsg(msg)
		g.log.Printf("[ERROR] %s\n", msg)
		goto BEGIN
	}

	var (
		reply    *http.Response
		response objects.Response
		rcvBuf   bytes.Buffer
		sndBuf   []byte
		addr     = fmt.Sprintf("http://%s%s",
			g.srv,
			fmt.Sprintf(uriReminderEdit, id))
		payload = make(url.Values)
	)

	if sndBuf, err = ffjson.Marshal(r); err != nil {
		g.log.Printf("[ERROR] Cannot serialize Reminder: %s\n",
			err.Error())
		return
	}

	payload["reminder"] = []string{string(sndBuf)}

	if reply, err = g.web.PostForm(addr, payload); err != nil {
		g.log.Printf("[ERROR] Failed to submit new Reminder to Backend: %s\n",
			err.Error())
		return
	} else if reply.StatusCode != 200 {
		g.log.Printf("[ERROR] Backend responds with status %s\n",
			reply.Status)
		return
	}

	defer reply.Body.Close() // nolint: errcheck

	if _, err = io.Copy(&rcvBuf, reply.Body); err != nil {
		g.log.Printf("[ERROR] Cannot read HTTP reply from backend: %s\n",
			err.Error())
		return
	} else if err = ffjson.Unmarshal(rcvBuf.Bytes(), &response); err != nil {
		g.log.Printf("[ERROR] Cannot de-serialize Response from JSON: %s\n",
			err.Error())
		return
	}

	g.log.Printf("[DEBUG] Got response from backend: %#v\n",
		response)

	if response.Status {
		g.reminders[r.ID] = r
		var tstr, cstr string

		tstr = r.Timestamp.Format(common.TimestampFormat)
		cstr = r.Changed.Format(common.TimestampFormat)

		if !g.store.IterIsValid(iter) {
			g.log.Printf("[ERROR] TreeIter for Reminder %q (%d) is no longer valid\n",
				r.Title,
				r.ID)
			if iter, err = g.getIter(r.ID); err != nil {
				g.log.Printf("[ERROR] Could not find TreeIter for Reminder %q (%d): %s\n",
					r.Title,
					r.ID,
					err.Error())
				return
			} else if iter == nil {
				g.log.Printf("[ERROR] Could not find TreeIter for Reminder %q (%d)\n",
					r.Title,
					r.ID)
				return
			}
		}

		g.store.Set( // nolint: errcheck
			iter,
			[]int{0, 1, 2, 3, 5, 6},
			[]any{
				r.ID,
				r.Title,
				tstr,
				r.Recur.String(),
				r.Finished,
				cstr,
			},
		)
	} else {
		g.log.Printf("[ERROR] Failed to update Reminder %q in backend: %s\n",
			r.Title,
			response.Message)
		g.pushMsg(response.Message)
		g.displayMsg(response.Message)
	}
} // func (g *GUI) handleReminderClickEdit()

func (g *GUI) handleReminderClickDelete() {
	var (
		err  error
		msg  string
		ok   bool
		sel  *gtk.TreeSelection
		iter *gtk.TreeIter
		id   int64
		gval *glib.Value
		rval any
	)

	if sel, err = g.view.GetSelection(); err != nil {
		msg = fmt.Sprintf("Failed to get Selection from TreeView: %s",
			err.Error())
		g.displayMsg(msg)
		g.log.Printf("[ERROR] %s\n", msg)
		return
	} else if _, iter, ok = sel.GetSelected(); !ok || iter == nil {
		g.log.Println("[ERROR] Could not get TreeIter from TreeSelection")
		return
	}

	iter = g.filter.ConvertIterToChildIter(iter)

	if gval, err = g.store.GetValue(iter, 0); err != nil {
		msg = fmt.Sprintf("Could not get glib.Value from TreeIter: %s",
			err.Error())
		g.log.Printf("[ERROR] %s\n", msg)
		g.pushMsg(msg)
		return
	} else if rval, err = gval.GoValue(); err != nil {
		msg = fmt.Sprintf("Cannot get Go value from glib.Value: %s", err.Error())
		g.log.Printf("[ERROR] %s\n", msg)
		g.pushMsg(msg)
		return
	}

	id = int64(rval.(int))

	var (
		reply    *http.Response
		response objects.Response
		buf      bytes.Buffer
		addr     = fmt.Sprintf("http://%s%s",
			g.srv,
			fmt.Sprintf(uriReminderDelete, id))
		rem = g.reminders[id]
	)

	if ok, err = g.yesOrNo("Are you sure?", fmt.Sprintf("Do you want to delete Reminder #%d (%q)?", rem.ID, rem.Title)); err != nil {
		msg = fmt.Sprintf("Failed to ask user for confirmation: %s",
			err.Error())
		g.log.Printf("[ERROR] %s\n", msg)
		g.pushMsg(msg)
		return
	} else if !ok {
		g.log.Printf("[INFO] User did not agree to delete Reminder %d (%q)\n",
			rem.ID,
			rem.Title)
		return
	} else if reply, err = g.web.Get(addr); err != nil {
		msg = fmt.Sprintf("Failed to GET %s: %s",
			addr,
			err.Error())
		g.log.Printf("[ERROR] %s\n", msg)
		g.pushMsg(msg)
	} else if reply.StatusCode != 200 {
		msg = fmt.Sprintf("Unexpected HTTP status from server: %s",
			reply.Status)
		g.log.Printf("[ERROR] %s\n", msg)
		g.pushMsg(msg)
	} else if reply.Close {
		g.log.Printf("[DEBUG] I will close the Body I/O object for %s\n",
			reply.Request.URL)
		defer reply.Body.Close() // nolint: errcheck
	}

	if _, err = io.Copy(&buf, reply.Body); err != nil {
		g.log.Printf("[ERROR] Cannot read HTTP reply from backend: %s\n",
			err.Error())
		return
	} else if err = ffjson.Unmarshal(buf.Bytes(), &response); err != nil {
		g.log.Printf("[ERROR] Cannot de-serialize Response from JSON: %s\n",
			err.Error())
		return
	}

	g.log.Printf("[DEBUG] Got response from backend: %#v\n",
		response)

	if response.Status {
		iter, _ = g.getIter(id)
		delete(g.reminders, id)
		g.store.Remove(iter)
	} else {
		g.log.Printf("[ERROR] Failed to delete Reminder %q in backend: %s\n",
			id,
			response.Message)
		g.pushMsg(response.Message)
		g.displayMsg(response.Message)
	}
} // func (g *GUI) handleReminderClickDelete()

func (g *GUI) handleReminderClickToggleActive() {
	var (
		err  error
		msg  string
		ok   bool
		sel  *gtk.TreeSelection
		iter *gtk.TreeIter
		id   int64
		gval *glib.Value
		rval any
		r    objects.Reminder
	)

	if sel, err = g.view.GetSelection(); err != nil {
		msg = fmt.Sprintf("Failed to get Selection from TreeView: %s",
			err.Error())
		g.displayMsg(msg)
		g.log.Printf("[ERROR] %s\n", msg)
		return
	} else if _, iter, ok = sel.GetSelected(); !ok || iter == nil {
		g.log.Println("[ERROR] Could not get TreeIter from TreeSelection")
		return
	}

	iter = g.filter.ConvertIterToChildIter(iter)

	if gval, err = g.store.GetValue(iter, 0); err != nil {
		msg = fmt.Sprintf("Could not get glib.Value from TreeIter: %s",
			err.Error())
		g.log.Printf("[ERROR] %s\n", msg)
		g.pushMsg(msg)
		return
	} else if rval, err = gval.GoValue(); err != nil {
		msg = fmt.Sprintf("Cannot get Go value from glib.Value: %s", err.Error())
		g.log.Printf("[ERROR] %s\n", msg)
		g.pushMsg(msg)
		return
	}

	id = int64(rval.(int))

	if r, ok = g.reminders[id]; !ok {
		msg = fmt.Sprintf("Cannot find Reminder #%d in cache", id)
		g.log.Printf("[ERROR] %s\n", msg)
		g.pushMsg(msg)
		return
	}

	var (
		reply    *http.Response
		response objects.Response
		buf      bytes.Buffer
		addr     = fmt.Sprintf("http://%s%s",
			g.srv,
			fmt.Sprintf(uriReminderSetFinished, id, !r.Finished))
	)

	g.log.Printf("[TRACE] GET %s\n", addr)

	if reply, err = g.web.Get(addr); err != nil {
		msg = fmt.Sprintf("Failed to GET %s: %s",
			addr,
			err.Error())
		g.log.Printf("[ERROR] %s\n", msg)
		g.pushMsg(msg)
	} else if reply.StatusCode != 200 {
		msg = fmt.Sprintf("Unexpected HTTP status from server: %s",
			reply.Status)
		g.log.Printf("[ERROR] %s\n", msg)
		g.pushMsg(msg)
	} else if reply.Close {
		g.log.Printf("[DEBUG] I will close the Body I/O object for %s\n",
			reply.Request.URL)
		defer reply.Body.Close() // nolint: errcheck
	}

	if _, err = io.Copy(&buf, reply.Body); err != nil {
		g.log.Printf("[ERROR] Cannot read HTTP reply from backend: %s\n",
			err.Error())
		return
	} else if err = ffjson.Unmarshal(buf.Bytes(), &response); err != nil {
		g.log.Printf("[ERROR] Cannot de-serialize Response from JSON: %s\n",
			err.Error())
		return
	}

	g.log.Printf("[DEBUG] Got response from backend: %#v\n",
		response)

	if response.Status {
		var r = g.reminders[id]
		r.Finished = false
		r.Changed = time.Now()
		g.reminders[id] = r
		g.store.Set( // nolint: errcheck
			iter,
			[]int{3, 5},
			[]any{true, r.Changed.Format(common.TimestampFormat)},
		)
	} else {
		g.log.Printf("[ERROR] Failed to reactivate Reminder %d in backend: %s\n",
			id,
			response.Message)
		g.pushMsg(response.Message)
		g.displayMsg(response.Message)
	}
} // func (g *GUI) handleReminderClickToggleActive()
