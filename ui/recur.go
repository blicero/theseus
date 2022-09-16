// /home/krylon/go/src/github.com/blicero/theseus/ui/recur.go
// -*- mode: go; coding: utf-8; -*-
// Created on 10. 09. 2022 by Benjamin Walkenhorst
// (c) 2022 Benjamin Walkenhorst
// Time-stamp: <2022-09-16 18:57:39 krylon>

package ui

import (
	"errors"
	"fmt"
	"log"
	"math"

	"github.com/blicero/theseus/objects"
	"github.com/gotk3/gotk3/gtk"
)

var dayName = [7]string{
	"Mo",
	"Di",
	"Mi",
	"Do",
	"Fr",
	"Sa",
	"So",
}

// RecurEditor is a sorta-kinda custom widget for editing Recurrences
// of Reminders.
// I don't think I can create "real" custom widgets in Go, but I'll
// try to create something reusable.
type RecurEditor struct {
	log                        *log.Logger
	rec                        *objects.Alarmclock
	box                        *gtk.Box
	oBox, tBox, cntBox, dayBox *gtk.Box
	rtCombo                    *gtk.ComboBoxText
	offMin, offHour            *gtk.SpinButton
	cntEdit                    *gtk.SpinButton
	weekdays                   [7]*gtk.CheckButton
}

// NewRecurEditor creates and returns a fresh Editor for Recurrences.
func NewRecurEditor(r *objects.Alarmclock, l *log.Logger) (*RecurEditor, error) {
	var (
		err error
		e   = &RecurEditor{
			log: l,
			rec: r,
		}
	)

	if r == nil {
		e.rec = new(objects.Alarmclock)
	} else {
		e.rec = r
	}

	if e.box, err = gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 1); err != nil {
		e.log.Printf("[ERROR] Cannot create gtk.Box: %s\n",
			err.Error())
		return nil, err
	} else if e.tBox, err = gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 1); err != nil {
		e.log.Printf("[ERROR] Cannot create gtk.Box: %s\n",
			err.Error())
		return nil, err
	} else if e.oBox, err = gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 1); err != nil {
		e.log.Printf("[ERROR] Cannot create gtk.Box: %s\n",
			err.Error())
		return nil, err
	} else if e.cntBox, err = gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 1); err != nil {
		e.log.Printf("[ERROR] Cannot create gtk.Box: %s\n",
			err.Error())
		return nil, err
	} else if e.dayBox, err = gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 1); err != nil {
		e.log.Printf("[ERROR] Cannot create gtk.Box: %s\n",
			err.Error())
		return nil, err
	} else if e.rtCombo, err = gtk.ComboBoxTextNew(); err != nil {
		e.log.Printf("[ERROR] Cannot create gtk.ComboBoxText: %s\n",
			err.Error())
		return nil, err
	} else if e.cntEdit, err = gtk.SpinButtonNewWithRange(0, math.Inf(1), 1); err != nil {
		e.log.Printf("[ERROR] Cannot create gtk.SpinButton: %s\n",
			err.Error())
		return nil, err
	} else if e.offMin, err = gtk.SpinButtonNewWithRange(0, 59, 1); err != nil {
		e.log.Printf("[ERROR] Cannot create gtk.SpinButton: %s\n",
			err.Error())
		return nil, err
	} else if e.offHour, err = gtk.SpinButtonNewWithRange(0, 23, 1); err != nil {
		e.log.Printf("[ERROR] Cannot create gtk.SpinButton: %s\n",
			err.Error())
		return nil, err
	}

	e.rtCombo.AppendText(objects.Once.String())
	e.rtCombo.AppendText(objects.Daily.String())
	e.rtCombo.AppendText(objects.Custom.String())

	for i := range e.weekdays {
		if e.weekdays[i], err = gtk.CheckButtonNewWithLabel(dayName[i]); err != nil {
			e.log.Printf("[ERROR] Cannot create gtk.CheckButton: %s\n",
				err.Error())
			return nil, err
		}

	}

	e.box.PackStart(e.oBox, true, true, 0)
	e.box.PackStart(e.tBox, true, true, 0)
	e.box.PackStart(e.cntBox, true, true, 0)
	e.box.PackStart(e.dayBox, true, true, 0)

	e.oBox.PackStart(e.offHour, true, true, 0)
	e.oBox.PackStart(e.offMin, true, true, 0)

	e.rtCombo.SetActive(0)
	e.rtCombo.Connect("changed", e.handleTypeChange)

	e.tBox.PackStart(e.rtCombo, true, true, 0)
	e.cntBox.PackStart(e.cntEdit, true, true, 0)
	for _, v := range e.weekdays {
		e.dayBox.PackStart(v, true, true, 0)
	}

	var min, hour int

	switch e.rec.Repeat {
	case objects.Once:
		// Nothing to do here, move along
	case objects.Custom:
		for i, f := range e.rec.Days {
			e.weekdays[i].SetActive(f)
		}
		fallthrough
	case objects.Daily:
		hour = e.rec.Offset / 3600
		min = (e.rec.Offset % 3600) / 60

		e.offHour.SetValue(float64(hour))
		e.offMin.SetValue(float64(min))
	default:
		e.log.Printf("[CANTHAPPEN] Invalid recurrence type: %d\n",
			r.Repeat)
	}

	return e, nil
} // func NewRecurEditor(r *objects.Alarmclock, l *log.Logger) (*RecurEditor, error)

func (e *RecurEditor) handleTypeChange() {
	switch txt := e.rtCombo.GetActiveText(); txt {
	case objects.Once.String():
		e.cntBox.Hide()
		e.dayBox.Hide()
		e.offMin.SetSensitive(false)
		e.offHour.SetSensitive(false)
		// e.offMin.SetProperty("editable", false)
		// e.offHour.SetProperty("editable", false)
	case objects.Daily.String():
		e.cntBox.ShowAll()
		e.dayBox.Hide()
		e.offMin.SetSensitive(true)
		e.offHour.SetSensitive(true)
		// e.offMin.SetProperty("editable", true)
		// e.offHour.SetProperty("editable", true)
	case objects.Custom.String():
		e.cntBox.ShowAll()
		e.dayBox.ShowAll()
		e.offMin.SetSensitive(true)
		e.offHour.SetSensitive(true)
		// e.offMin.SetProperty("editable", true)
		// e.offHour.SetProperty("editable", true)
	default:
		e.log.Printf("[CANTHAPPEN] %q is not a valid recurrence type!\n",
			txt)
	}
}

func (e *RecurEditor) GetRecurrence() objects.Alarmclock {
	switch txt := e.rtCombo.GetActiveText(); txt {
	case objects.Once.String():
		e.rec.Repeat = objects.Once
	case objects.Daily.String():
		e.rec.Repeat = objects.Daily
	case objects.Custom.String():
		e.rec.Repeat = objects.Custom
	default:
		var msg = fmt.Sprintf("%q is not a valid recurrence type!",
			txt)
		e.log.Printf("[CANTHAPPEN] %s\n", msg)
		panic(errors.New(msg))
	}

	e.rec.Offset = int(e.offMin.GetValue())*60 + int(e.offHour.GetValue())*3600

	for i, b := range e.weekdays {
		e.rec.Days[i] = b.GetActive()
	}

	return *e.rec
} // func (e *RecurEditor) GetRecurrence() objects.Alarmclock
