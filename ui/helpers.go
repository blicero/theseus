// /home/krylon/go/src/github.com/blicero/theseus/ui/helpers.go
// -*- mode: go; coding: utf-8; -*-
// Created on 22. 07. 2022 by Benjamin Walkenhorst
// (c) 2022 Benjamin Walkenhorst
// Time-stamp: <2022-08-22 20:17:50 krylon>

package ui

import (
	"errors"
	"fmt"

	"github.com/blicero/krylib"
	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
)

////////////////////////////////////////////////////////////////////////////////
///// General Utilities ////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////

func (g *GUI) displayMsg(msg string) {
	krylib.Trace()
	defer g.log.Printf("[TRACE] EXIT %s\n",
		krylib.TraceInfo())

	var (
		err error
		dlg *gtk.Dialog
		lbl *gtk.Label
		box *gtk.Box
	)

	if dlg, err = gtk.DialogNewWithButtons(
		"Message",
		g.win,
		gtk.DIALOG_MODAL,
		[]interface{}{
			"Okay",
			gtk.RESPONSE_OK,
		},
	); err != nil {
		g.log.Printf("[ERROR] Cannot create dialog to display message: %s\nMesage would've been %q\n",
			err.Error(),
			msg)
		return
	}

	defer dlg.Close()

	if lbl, err = gtk.LabelNew(msg); err != nil {
		g.log.Printf("[ERROR] Cannot create label to display message: %s\nMessage would've been: %q\n",
			err.Error(),
			msg)
		return
	} else if box, err = dlg.GetContentArea(); err != nil {
		g.log.Printf("[ERROR] Cannot get ContentArea of Dialog to display message: %s\nMessage would've been %q\n",
			err.Error(),
			msg)
		return
	}

	box.PackStart(lbl, true, true, 0)
	dlg.ShowAll()
	dlg.Run()
} // func (g *GUI) displayMsg(msg string)

func (g *GUI) yesOrNo(title, question string) (bool, error) {
	var (
		err    error
		dlg    *gtk.Dialog
		lbl    *gtk.Label
		box    *gtk.Box
		answer bool
	)

	if dlg, err = gtk.DialogNewWithButtons(
		title,
		g.win,
		gtk.DIALOG_MODAL,
		[]any{
			"_Yes",
			gtk.RESPONSE_YES,
			"_No",
			gtk.RESPONSE_NO,
		},
	); err != nil {
		g.log.Printf("[ERROR] Cannot create Dialog: %s\n",
			err.Error())
		return false, err
	}

	defer dlg.Close()

	if lbl, err = gtk.LabelNew(question); err != nil {
		g.log.Printf("[ERROR] Cannot create Label for Dialog: %s\n",
			err.Error())
		return false, err
	} else if box, err = dlg.GetContentArea(); err != nil {
		g.log.Printf("[ERROR] Cannot get ContentArea of Dialog to display question: %s\n",
			err.Error())
		return false, err
	}

	box.PackStart(lbl, true, true, 0)
	dlg.ShowAll()
	var res = dlg.Run()

	switch res {
	case gtk.RESPONSE_NONE:
		fallthrough
	case gtk.RESPONSE_DELETE_EVENT:
		fallthrough
	case gtk.RESPONSE_CLOSE:
		fallthrough
	case gtk.RESPONSE_CANCEL:
		// Deal with it.
		g.log.Println("[ERROR] User did not answer question")
		return false, errors.New("User did not answer question")
	case gtk.RESPONSE_YES:
		answer = true
	case gtk.RESPONSE_NO:
		answer = false
	default:
		g.log.Printf("[ERROR] Unexpected response from user: %s\n",
			responseTypeStr(res))
		return false, fmt.Errorf("Unexpected response from user: %s",
			responseTypeStr(res))
	}

	return answer, nil
} // func (g *GUI) yesOrNo(question string) (bool, error)

func (g *GUI) pushMsg(msg string) {
	g.statusbar.Push(msgID, msg)
} // func (g *GUI) pushMsg(msg string)

// getIter attempts to look up the TreeIter corresponding to the Reminder
// item with the given id.
func (g *GUI) getIter(id int64) (*gtk.TreeIter, error) {
	var (
		iter *gtk.TreeIter
	)

	iter, _ = g.store.GetIterFirst()

	for {
		var (
			err  error
			val  *glib.Value
			gval any
			rid  int64
		)

		if val, err = g.store.GetValue(iter, 0); err != nil {
			g.log.Printf("[ERROR] Cannot get value from TreeModel: %s\n",
				err.Error())
			return nil, err
		} else if gval, err = val.GoValue(); err != nil {
			g.log.Printf("[ERROR] Error converting glib.Value to GoValue: %s\n",
				err.Error())
			return nil, err
		}

		rid = int64(gval.(int))
		if rid == id {
			return iter, nil
		} else if !g.store.IterNext(iter) {
			return nil, nil
		}
	}

	// return nil, errors.New("CANTHAPPEN - Unreachable code!")
} // func (g *GUI) getIter(id int64) *gtk.TreeIter

// I usually do not like doing this manually, but the authors of gotk,
// in their wisdom, chose not to supply a String method for
// gtk.ResponseType.
// As these values are unlikely to change, we do it manually.
//
//
// const (
//     RESPONSE_NONE         ResponseType = C.GTK_RESPONSE_NONE
//     RESPONSE_REJECT       ResponseType = C.GTK_RESPONSE_REJECT
//     RESPONSE_ACCEPT       ResponseType = C.GTK_RESPONSE_ACCEPT
//     RESPONSE_DELETE_EVENT ResponseType = C.GTK_RESPONSE_DELETE_EVENT
//     RESPONSE_OK           ResponseType = C.GTK_RESPONSE_OK
//     RESPONSE_CANCEL       ResponseType = C.GTK_RESPONSE_CANCEL
//     RESPONSE_CLOSE        ResponseType = C.GTK_RESPONSE_CLOSE
//     RESPONSE_YES          ResponseType = C.GTK_RESPONSE_YES
//     RESPONSE_NO           ResponseType = C.GTK_RESPONSE_NO
//     RESPONSE_APPLY        ResponseType = C.GTK_RESPONSE_APPLY
//     RESPONSE_HELP         ResponseType = C.GTK_RESPONSE_HELP
// )

func responseTypeStr(t gtk.ResponseType) string {
	switch t {
	case gtk.RESPONSE_NONE:
		return "None"
	case gtk.RESPONSE_REJECT:
		return "Reject"
	case gtk.RESPONSE_ACCEPT:
		return "Accept"
	case gtk.RESPONSE_DELETE_EVENT:
		return "DeleteEvent"
	case gtk.RESPONSE_OK:
		return "OK"
	case gtk.RESPONSE_CANCEL:
		return "Cancel"
	case gtk.RESPONSE_CLOSE:
		return "Close"
	case gtk.RESPONSE_YES:
		return "Yes"
	case gtk.RESPONSE_NO:
		return "No"
	case gtk.RESPONSE_APPLY:
		return "Apply"
	case gtk.RESPONSE_HELP:
		return "Help"
	default:
		return fmt.Sprintf("Unknown ResponseType %d",
			t)
	}
} // func responseTypeStr(t gtk.ResponseType) string
