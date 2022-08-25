// /home/krylon/go/src/github.com/blicero/theseus/ui/ui.go
// -*- mode: go; coding: utf-8; -*-
// Created on 06. 07. 2022 by Benjamin Walkenhorst
// (c) 2022 Benjamin Walkenhorst
// Time-stamp: <2022-08-25 23:53:01 krylon>

package ui

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/blicero/krylib"
	"github.com/blicero/theseus/common"
	"github.com/blicero/theseus/logdomain"
	"github.com/blicero/theseus/objects"
	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
	"github.com/pquerna/ffjson/ffjson"
)

const (
	defaultBufSize        = 65536 // 64 KiB
	msgID                 = 666
	uriGetAll             = "/reminder/all"
	uriReminderAdd        = "/reminder/add"
	uriReminderDelete     = "/reminder/%d/delete"
	uriReminderEdit       = "/reminder/%d/update"
	uriReminderReactivate = "/reminder/%d/reactivate"
	uriPeerListGet        = "/peer/all"
)

type column struct {
	colType glib.Type
	title   string
	display bool
	edit    bool
}

var cols = []column{
	column{
		colType: glib.TYPE_INT,
		title:   "ID",
		display: true,
	},
	column{
		colType: glib.TYPE_STRING,
		title:   "Title",
		display: true,
		edit:    true,
	},
	column{
		colType: glib.TYPE_STRING,
		title:   "Time",
		display: true,
		edit:    true,
	},
	column{
		colType: glib.TYPE_BOOLEAN,
		title:   "Finished",
		display: true,
		edit:    true,
	},
	column{
		colType: glib.TYPE_STRING,
		title:   "UUID",
		display: false,
		edit:    false,
	},
}

func createCol(title string, id int) (*gtk.TreeViewColumn, *gtk.CellRendererText, error) {
	renderer, err := gtk.CellRendererTextNew()
	if err != nil {
		return nil, nil, err
	}

	col, err := gtk.TreeViewColumnNewWithAttribute(title, renderer, "text", id)
	if err != nil {
		return nil, nil, err
	}

	col.SetResizable(true)

	return col, renderer, nil
} // func createCol(title string, id int) (*gtk.TreeViewColumn, *gtk.CellRendererText, error)

var gtkInit sync.Once

// GUI wraps the components of the graphical user interface (hence the name),
// along with the bits and pieces needed to talk to the backend.
type GUI struct {
	srv          string
	log          *log.Logger
	lock         sync.RWMutex // nolint: unused,structcheck
	win          *gtk.Window
	mainBox      *gtk.Box
	store        *gtk.ListStore
	filter       *gtk.TreeModelFilter
	view         *gtk.TreeView
	scr          *gtk.ScrolledWindow
	menuBar      *gtk.MenuBar
	statusbar    *gtk.Statusbar
	fMenu        *gtk.Menu // nolint: unused,structcheck
	web          http.Client
	reminders    map[int64]objects.Reminder
	hideFinished bool
}

// Create creates a new GUI instance ready to be used. Call the Run() method
// to display the Window and execute the gtk event loop.
func Create(srv string) (*GUI, error) {
	var (
		err error
		win = &GUI{
			srv:       srv,
			reminders: make(map[int64]objects.Reminder),
		}
	)

	gtkInit.Do(func() { gtk.Init(nil) })

	if win.log, err = common.GetLogger(logdomain.GUI); err != nil {
		fmt.Fprintf(
			os.Stderr,
			"Cannot create Logger for GUI: %s\n",
			err.Error())
		return nil, err
	} else if win.win, err = gtk.WindowNew(gtk.WINDOW_TOPLEVEL); err != nil {
		win.log.Printf("[ERROR] Cannot create Window: %s\n",
			err.Error())
		return nil, err
	} else if win.mainBox, err = gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 1); err != nil {
		win.log.Printf("[ERROR] Cannot create gtk.Box: %s\n",
			err.Error())
		return nil, err
	} else if win.scr, err = gtk.ScrolledWindowNew(nil, nil); err != nil {
		win.log.Printf("[ERROR] Cannot create ScrolledWindow: %s\n",
			err.Error())
		return nil, err
	} else if win.menuBar, err = gtk.MenuBarNew(); err != nil {
		win.log.Printf("[ERROR] Cannot create MenuBar: %s\n",
			err.Error())
		return nil, err
	} else if win.statusbar, err = gtk.StatusbarNew(); err != nil {
		win.log.Printf("[ERROR] Cannot create Status bar: %s\n",
			err.Error())
		return nil, err
	}

	// Initialize data store and TreeView
	var typeList = make([]glib.Type, len(cols))

	for i, c := range cols {
		typeList[i] = c.colType
	}

	if win.store, err = gtk.ListStoreNew(typeList...); err != nil {
		win.log.Printf("[ERROR] Cannot create TreeStore: %s\n",
			err.Error())
		return nil, err
		//} else if win.filter, err = win.store.FilterNew(
	} else if win.view, err = gtk.TreeViewNewWithModel(win.store); err != nil {
		win.log.Printf("[ERROR] Cannot create TreeView: %s\n",
			err.Error())
		return nil, err
	}

	win.store.SetDefaultSortFunc(win.reminderCmpFunc)
	win.store.SetSortFunc(2, win.reminderCmpFunc)
	win.store.SetSortFunc(3, win.reminderCmpFunc)
	win.store.SetSortColumnId(2, gtk.SORT_ASCENDING)
	//win.view.SetReorderable(true)

	for i, c := range cols {
		var (
			col      *gtk.TreeViewColumn
			renderer *gtk.CellRendererText
		)

		if !c.display {
			continue
		}

		if col, renderer, err = createCol(c.title, i); err != nil {
			win.log.Printf("[ERROR] Cannot create TreeViewColumn %q: %s\n",
				c.title,
				err.Error())
			return nil, err
		}

		renderer.Set("editable", c.edit)     // nolint: errcheck
		renderer.Set("editable-set", c.edit) // nolint: errcheck

		win.view.AppendColumn(col)
	}

	win.win.Add(win.mainBox)
	win.scr.Add(win.view)
	win.mainBox.PackStart(win.menuBar, false, false, 1)
	win.mainBox.PackStart(win.scr, true, true, 1)
	win.mainBox.PackStart(win.statusbar, false, false, 1)

	win.win.Connect("destroy", gtk.MainQuit)

	if err = win.initTree(); err != nil {
		win.log.Printf("[ERROR] Failed to initialize TreeView: %s\n",
			err.Error())
		return nil, err
	} else if err = win.initMenu(); err != nil {
		win.log.Printf("[ERROR] Failed to intialize MenuBar: %s\n",
			err.Error())
		return nil, err
	}

	win.win.ShowAll()
	win.win.SetSizeRequest(960, 540)
	win.win.SetTitle(fmt.Sprintf("%s %s",
		common.AppName,
		common.Version))

	glib.TimeoutAdd(uint(5000), win.fetchReminders)
	glib.TimeoutAdd(uint(5000), win.fetchPeers)
	glib.IdleAdd(func() bool {
		win.fetchReminders()
		return false
	})

	return win, nil
} // func Create(srv string) (*GUI, error)

// Run executes gtk's main event loop.
func (g *GUI) Run() {
	gtk.Main()
} // func (w *RWin) Run()

func (g *GUI) initMenu() error {
	var (
		err                                  error
		fMenu, rMenu                         *gtk.Menu
		srvItem, quitItem, addItem, editItem *gtk.MenuItem
		fItem, rItem, rrItem, delItem        *gtk.MenuItem
		hideFinItem                          *gtk.CheckMenuItem
	)

	if fMenu, err = gtk.MenuNew(); err != nil {
		g.log.Printf("[ERROR] Cannot create Menu File: %s\n",
			err.Error())
		return err
	} else if rMenu, err = gtk.MenuNew(); err != nil {
		g.log.Printf("[ERROR] Cannot create Menu Reminder: %s\n",
			err.Error())
		return err
	} else if srvItem, err = gtk.MenuItemNewWithMnemonic("Choose _Server"); err != nil {
		g.log.Printf("[ERROR] Cannot create menu item SRV: %s\n",
			err.Error())
		return err
	} else if quitItem, err = gtk.MenuItemNewWithMnemonic("_Quit"); err != nil {
		g.log.Printf("[ERROR] Cannot create menu item QUIT: %s\n",
			err.Error())
		return err
	} else if addItem, err = gtk.MenuItemNewWithMnemonic("_Add Reminder"); err != nil {
		g.log.Printf("[ERROR] Cannot create menu item itemAdd: %s\n",
			err.Error())
		return err
	} else if editItem, err = gtk.MenuItemNewWithMnemonic("_Edit Reminder"); err != nil {
		g.log.Printf("[ERROR] Cannot create menu item EDIT: %s\n",
			err.Error())
		return err
	} else if rrItem, err = gtk.MenuItemNewWithMnemonic("_Reactivate Reminder"); err != nil {
		g.log.Printf("[ERROR] Cannot create menu item REACTIVATE: %s\n",
			err.Error())
		return err
	} else if fItem, err = gtk.MenuItemNewWithMnemonic("_File"); err != nil {
		g.log.Printf("[ERROR] Cannot create menu item FILE: %s\n",
			err.Error())
		return err
	} else if rItem, err = gtk.MenuItemNewWithMnemonic("_Reminder"); err != nil {
		g.log.Printf("[ERROR] Cannot create menu item REMINDER: %s\n",
			err.Error())
		return err
	} else if delItem, err = gtk.MenuItemNewWithMnemonic("_Delete Reminder"); err != nil {
		g.log.Printf("[ERROR] Cannot create menu Item DELETE: %s\n",
			err.Error())
		return err
	} else if hideFinItem, err = gtk.CheckMenuItemNewWithMnemonic("_Hide Finished"); err != nil {
		g.log.Printf("[ERROR] Cannot create menu item HIDE_FINISHED: %s\n",
			err.Error())
		return err
	}

	quitItem.Connect("activate", gtk.MainQuit)
	srvItem.Connect("activate", g.setServer)
	addItem.Connect("activate", g.reminderAdd)
	editItem.Connect("activate", g.reminderEdit)
	rrItem.Connect("activate", g.reminderReactivate)
	delItem.Connect("activate", g.reminderDelete)
	hideFinItem.Connect("activate", g.reminderHideFinished)

	fMenu.Append(srvItem)
	fMenu.Append(quitItem)
	rMenu.Append(addItem)
	rMenu.Append(editItem)
	rMenu.Append(rrItem)
	rMenu.Append(delItem)
	rMenu.Append(hideFinItem)

	g.menuBar.Append(fItem)
	g.menuBar.Append(rItem)

	fItem.SetSubmenu(fMenu)
	rItem.SetSubmenu(rMenu)

	return nil
} // func (g *GUI) initMenu() error

func (g *GUI) initTree() error {
	var (
		err error
		sel *gtk.TreeSelection
	)

	if g.filter, err = g.store.FilterNew(nil); err != nil {
		g.log.Printf("[ERROR] Cannot create TreeModelFilter: %s\n",
			err.Error())
		return err
	}

	g.filter.SetVisibleFunc(g.reminderFilterFn)
	g.view.SetModel(g.filter)

	if sel, err = g.view.GetSelection(); err != nil {
		g.log.Printf("[ERROR] Cannot get TreeSelection: %s\n",
			err.Error())
		return err
	}

	sel.SetMode(gtk.SELECTION_SINGLE)

	return nil
} // func (g *GUI) initializeTree() error

func (g *GUI) fetchReminders() bool {
	var (
		err         error
		rawURL, msg string
		res         *http.Response
	)

	krylib.Trace()

	rawURL = fmt.Sprintf("http://%s%s",
		g.srv,
		uriGetAll)

	if _, err = url.Parse(rawURL); err != nil {
		msg = fmt.Sprintf("Invalid URL %q: %s",
			rawURL,
			err.Error())
		g.log.Printf("[ERROR] %s\n", msg)
		g.pushMsg(msg)
		return true
	} else if res, err = g.web.Get(rawURL); err != nil {
		g.log.Printf("[ERROR] Failed Request to backend for %q: %s\n",
			rawURL,
			err.Error())

		return true
	} else if res.StatusCode != 200 {
		err = fmt.Errorf("Unexpected HTTP status from backend: %s",
			res.Status)
		g.log.Printf("[ERROR] %s\n",
			err.Error())
		return true
	}

	defer res.Body.Close() // nolint: errcheck

	var (
		rsize int
		body  []byte
	)

	if res.ContentLength == -1 {
		body = make([]byte, defaultBufSize)
	} else {
		body = make([]byte, res.ContentLength)
	}

	if rsize, err = res.Body.Read(body); err != nil && err != io.EOF {
		g.log.Printf("[ERROR] Failed to read HTTP response body: %s\n",
			err.Error())
		return true
	}

	var list = make([]objects.Reminder, 0, 64)

	if err = ffjson.Unmarshal(body[:rsize], &list); err != nil {
		g.log.Printf(
			"[ERROR] Cannot de-serialize response from Backend: %s\n%s\n",
			err.Error(),
			body,
		)
		return true
	}

	g.lock.Lock()
	defer g.lock.Unlock()

	var idList = make(map[int64]bool, len(list))

	for _, r := range list {
		var (
			ok   bool
			iter *gtk.TreeIter
			tstr = r.Timestamp.Format(common.TimestampFormat)
		)

		idList[r.ID] = true

		if _, ok = g.reminders[r.ID]; !ok {
			g.reminders[r.ID] = r
			iter = g.store.Append()

			g.store.Set( // nolint: errcheck
				iter,
				[]int{0, 1, 2, 3, 4},
				[]any{r.ID, r.Title, tstr, r.Finished, r.UUID},
			)
		} else if iter, err = g.getIter(r.ID); err != nil || iter == nil {
			g.log.Printf("{ERROR] Could not get TreeIter for Reminder #%d\n",
				r.ID)
			continue
		} else {
			g.store.Set( // nolint: errcheck
				iter,
				[]int{0, 1, 2, 3, 4},
				[]any{r.ID, r.Title, tstr, r.Finished, r.UUID},
			)
		}
	}

	for iter, _ := g.store.GetIterFirst(); g.store.IterNext(iter); {
		var (
			val  *glib.Value
			gval any
			id   int
			ok   bool
		)

		if val, err = g.store.GetValue(iter, 0); err != nil {
			g.log.Printf("[ERROR] Cannot get value from TreeModel: %s\n",
				err.Error())
			continue
		}
		gval, _ = val.GoValue()

		if id, ok = gval.(int); ok {
			if !idList[int64(id)] {
				g.store.Remove(iter)
			}
		}
	}

	return true
} // func (g *GUI) fetchReminders() bool

func (g *GUI) fetchPeers() bool {
	var (
		err         error
		rawURL, msg string
		res         *http.Response
	)

	krylib.Trace()

	rawURL = fmt.Sprintf("http://%s%s",
		g.srv,
		uriPeerListGet)

	if _, err = url.Parse(rawURL); err != nil {
		msg = fmt.Sprintf("Invalid URL %q: %s",
			rawURL,
			err.Error())
		g.log.Printf("[ERROR] %s\n", msg)
		g.pushMsg(msg)
		return true
	} else if res, err = g.web.Get(rawURL); err != nil {
		g.log.Printf("[ERROR] Failed Request to backend for %q: %s\n",
			rawURL,
			err.Error())

		return true
	} else if res.StatusCode != 200 {
		err = fmt.Errorf("Unexpected HTTP status from backend: %s",
			res.Status)
		g.log.Printf("[ERROR] %s\n",
			err.Error())
		return true
	}

	defer res.Body.Close() // nolint: errcheck

	var (
		rsize int
		body  []byte
		peers []objects.Peer
	)

	if res.ContentLength == -1 {
		body = make([]byte, defaultBufSize)
	} else {
		body = make([]byte, res.ContentLength)
	}

	if rsize, err = res.Body.Read(body); err != nil && err != io.EOF {
		g.log.Printf("[ERROR] Failed to read HTTP response body: %s\n",
			err.Error())
		return true
	}

	peers = make([]objects.Peer, 0, 8)

	if err = ffjson.Unmarshal(body[:rsize], &peers); err != nil {
		g.log.Printf("[ERROR] Cannot parse HTTP response body: %s\n\n%s\n",
			err.Error(),
			body[:rsize])
		return true
	}

	g.log.Printf("[TRACE] Received %d peers from Backend\n", len(peers))

	for i, p := range peers {
		g.log.Printf("[DEBUG] Got Peer %d/%d: %s\n",
			i+1,
			len(peers),
			&p)
		g.pushMsg(p.String())
	}

	return true
} // func (g *GUI) fetchPeers() bool

func (g *GUI) setServer() {
	var (
		err   error
		dlg   *gtk.Dialog
		entry *gtk.Entry
		lbl   *gtk.Label
		dbox  *gtk.Box
		grid  *gtk.Grid
	)

	if dlg, err = gtk.DialogNewWithButtons(
		"Choose Server",
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
		g.log.Printf("[ERROR] Cannot add button OK to dialog: %s\n",
			err.Error())
		return
	} else if grid, err = gtk.GridNew(); err != nil {
		g.log.Printf("[ERROR] Cannot create gtk.Grid: %s\n",
			err.Error())
		return
	} else if lbl, err = gtk.LabelNew("Server:"); err != nil {
		g.log.Printf("[ERROR] Cannot create gtk.Label: %s\n",
			err.Error())
		return
	} else if entry, err = gtk.EntryNew(); err != nil {
		g.log.Printf("[ERROR] Cannot create gtk.Entry: %s\n",
			err.Error())
		return
	} else if dbox, err = dlg.GetContentArea(); err != nil {
		g.log.Printf("[ERROR] Cannot get ContentArea of Dialog: %s\n",
			err.Error())
		return
	}

	grid.InsertColumn(0)
	grid.InsertColumn(1)
	grid.InsertRow(0)

	grid.Attach(lbl, 0, 0, 1, 1)
	grid.Attach(entry, 0, 1, 1, 1)

	dbox.PackStart(grid, true, true, 0)
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
		g.log.Println("[DEBUG] User changed their mind about adding a Program. Fine with me.")
		return
	case gtk.RESPONSE_OK:
		// 's ist los, Hund?
	default:
		g.log.Printf("[CANTHAPPEN] Well, I did NOT see this coming: %d\n",
			res)
		return
	}

	var srv string

	if srv, err = entry.GetText(); err != nil {
		g.log.Printf("[ERROR] Cannot get Text from Entry: %s\n",
			err.Error())
		return
	}

	g.srv = srv
} // func (g *GUI) setServer()

func (g *GUI) reminderAdd() {
	var (
		err                                error
		msg                                string
		dlg                                *gtk.Dialog
		dbox                               *gtk.Box
		grid                               *gtk.Grid
		cal                                *gtk.Calendar
		titleEntry, bodyEntry              *gtk.Entry
		hourInput, minuteInput             *gtk.SpinButton
		timeLbl, sepLbl, titleLbl, bodyLbl *gtk.Label
		now                                time.Time
	)

	if dlg, err = gtk.DialogNewWithButtons(
		"Choose Server",
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

	grid.Attach(cal, 0, 0, 4, 1)
	grid.Attach(timeLbl, 0, 1, 1, 1)
	grid.Attach(hourInput, 1, 1, 1, 1)
	grid.Attach(sepLbl, 2, 1, 1, 1)
	grid.Attach(minuteInput, 3, 1, 1, 1)
	grid.Attach(titleLbl, 0, 2, 1, 1)
	grid.Attach(titleEntry, 1, 2, 3, 1)
	grid.Attach(bodyLbl, 0, 3, 1, 1)
	grid.Attach(bodyEntry, 1, 3, 3, 1)

	dbox.PackStart(grid, true, true, 0)
	dlg.ShowAll()

BEGIN:
	now = time.Now()

	cal.SelectMonth(uint(now.Month())-1, uint(now.Year()))
	cal.SelectDay(uint(now.Day()))

	hourInput.SetValue(float64(now.Hour()))
	minuteInput.SetValue(float64(now.Minute()) + 10)

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
		g.log.Println("[TRACE] Input was successful")
	default:
		g.log.Printf("[CANTHAPPEN] Well, I did NOT see this coming: %d\n",
			res)
		return
	}

	var (
		year, month, day uint
		hour, min        int
		r                objects.Reminder
	)

	year, month, day = cal.GetDate()
	hour = hourInput.GetValueAsInt()
	min = minuteInput.GetValueAsInt()

	r.Timestamp = time.Date(
		int(year),
		time.Month(month+1),
		int(day),
		hour,
		min,
		0,
		0,
		time.Local)
	r.Title, _ = titleEntry.GetText()
	r.Description, _ = bodyEntry.GetText()

	g.log.Printf("[DEBUG] Reminder: %#v\n",
		&r)

	if r.Timestamp.Before(time.Now()) {
		msg = fmt.Sprintf("The time you selected is in the past: %s",
			r.Timestamp.Format(common.TimestampFormat))
		g.displayMsg(msg)
		g.log.Printf("[ERROR] %s\n", msg)
		goto BEGIN
	} else if r.Title == "" {
		msg = "You did not enter a title"
		g.displayMsg(msg)
		g.log.Printf("[ERROR] %s\n", msg)
		goto BEGIN
	}

	var (
		reply    *http.Response
		response objects.Response
		buf      bytes.Buffer
		j        []byte
		addr     = fmt.Sprintf("http://%s%s",
			g.srv,
			uriReminderAdd)
		payload = make(url.Values)
	)

	if j, err = ffjson.Marshal(r); err != nil {
		msg = fmt.Sprintf("Failed to convert Reminder to JSON: %s",
			err.Error())
		g.log.Printf("[ERROR] %s\n", msg)
		g.displayMsg(msg)
		return
	}

	payload["reminder"] = []string{string(j)}

	if reply, err = g.web.PostForm(addr, payload); err != nil {
		g.log.Printf("[ERROR] Failed to submit new Reminder to Backend: %s\n",
			err.Error())
		return
	} else if reply.StatusCode != 200 {
		g.log.Printf("[ERROR] Backend responds with status %s\n",
			reply.Status)
		return
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
		var iter = g.store.Append()

		g.store.Set( // nolint: errcheck
			iter,
			[]int{0, 1, 2, 3},
			[]any{r.ID, r.Title, r.Timestamp.Format(common.TimestampFormat), r.Finished},
		)

		g.reminders[r.ID] = r
	}
} // func (g *GUI) reminderAdd()

// Just an idea - if I did a little bit of caching, I wouldn't really need
// to painfully pull all that data out of the TreeModel.

func (g *GUI) reminderEdit() {
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

	finishedCB.SetActive(r.Finished)

	dbox.PackStart(grid, true, true, 0)
	dlg.ShowAll()

BEGIN:
	cal.SelectMonth(uint(r.Timestamp.Month())-1, uint(r.Timestamp.Year()))
	cal.SelectDay(uint(r.Timestamp.Day()))

	hourInput.SetValue(float64(r.Timestamp.Hour()))
	minuteInput.SetValue(float64(r.Timestamp.Minute()) + 10)

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
		g.log.Println("[TRACE] Input was successful")
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

	r.Timestamp = time.Date(
		int(year),
		time.Month(month+1),
		int(day),
		hour,
		min,
		0,
		0,
		time.Local)
	r.Title, _ = titleEntry.GetText()
	r.Description, _ = bodyEntry.GetText()
	r.Finished = finishedCB.GetActive()

	g.log.Printf("[DEBUG] Reminder: %#v\n",
		&r)

	if r.Timestamp.Before(time.Now()) {
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

	// FIXME Wouldn't it be easier to just serialize the entire
	//       Reminder to JSON and transmit that? I've already been
	//       bitten twice by first forgetting a field, and then
	//       using different names for a field on both ends.

	var (
		reply    *http.Response
		response objects.Response
		buf      bytes.Buffer
		addr     = fmt.Sprintf("http://%s%s",
			g.srv,
			fmt.Sprintf(uriReminderEdit, id))
		payload = make(url.Values)
	)

	payload["title"] = []string{r.Title}
	payload["body"] = []string{r.Description}
	payload["timestamp"] = []string{r.Timestamp.Format(time.RFC3339)}

	if reply, err = g.web.PostForm(addr, payload); err != nil {
		g.log.Printf("[ERROR] Failed to submit new Reminder to Backend: %s\n",
			err.Error())
		return
	} else if reply.StatusCode != 200 {
		g.log.Printf("[ERROR] Backend responds with status %s\n",
			reply.Status)
		return
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

	// Errrr, I cannot append another entry, I need to update the one
	// we edited in the first place.
	if response.Status {
		g.reminders[r.ID] = r

		g.store.Set( // nolint: errcheck
			iter,
			[]int{0, 1, 2, 3},
			[]any{r.ID, r.Title, r.Timestamp.Format(common.TimestampFormat), r.Finished},
		)
	}
} // func (g *GUI) reminderEdit()

func (g *GUI) reminderReactivate() {
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
			fmt.Sprintf(uriReminderReactivate, id))
	)

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
		g.store.Set( // nolint: errcheck
			iter,
			[]int{3},
			[]any{true},
		)

		var r = g.reminders[id]
		r.Finished = false
		g.reminders[id] = r
	}
} // func (g *GUI) reminderReactivate()

func (g *GUI) reminderDelete() {
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
		delete(g.reminders, id)
		g.store.Remove(iter)
	}
} // func (g *GUI) reminderDelete()

func (g *GUI) reminderHideFinished() {
	g.hideFinished = !g.hideFinished
	g.filter.Refilter()
} // func (g *GUI) reminderHideFinished()

func (g *GUI) reminderFilterFn(model *gtk.TreeModel, iter *gtk.TreeIter) bool {
	if !g.hideFinished {
		return true
	}

	var (
		err          error
		val          *glib.Value
		gval         any
		finished, ok bool
	)

	if val, err = model.GetValue(iter, 3); err != nil {
		g.log.Printf("[ERROR] Cannot get Value from Model: %s\n",
			err.Error())
		return true
	} else if gval, err = val.GoValue(); err != nil {
		g.log.Printf("[ERROR] Cannot get GoValue from glib.Value: %s\n",
			err.Error())
		return true
	} else if finished, ok = gval.(bool); !ok {
		g.log.Printf("[ERROR] Cannot get bool from GoValue %T\n",
			gval)
		return true
	}

	return !finished
} // func (g *GUI) reminderFilterFn(model *gtk.TreeModel, iter *gtk.TreeIter) bool

// nolint: unused
func (g *GUI) reminderCmpFunc(model *gtk.TreeModel, a, b *gtk.TreeIter) int {
	var (
		err      error
		id1, id2 int
		val      *glib.Value
		gval     any
		ok       bool
	)

	if val, err = model.GetValue(a, 0); err != nil {
		g.log.Printf("[ERROR] Cannot get glib.Value from TreeIter a: %s\n",
			err.Error())
		return 0
	} else if gval, err = val.GoValue(); err != nil {
		g.log.Printf("[ERROR] Cannot get GoValue from glib.Value a: %s\n",
			err.Error())
		return 0
	} else if id1, ok = gval.(int); !ok {
		g.log.Printf("[ERROR] Invalid type for Reminder ID: %T\n",
			gval)
		return 0
	}

	if val, err = model.GetValue(b, 0); err != nil {
		g.log.Printf("[ERROR] Cannot get glib.Value from TreeIter b: %s\n",
			err.Error())
		return 0
	} else if gval, err = val.GoValue(); err != nil {
		g.log.Printf("[ERROR] Cannot get GoValue from glib.Value b: %s\n",
			err.Error())
		return 0
	} else if id2, ok = gval.(int); !ok {
		g.log.Printf("[ERROR] Invalid type for Reminder ID: %T\n",
			gval)
		return 0
	}

	var r1, r2 objects.Reminder

	r1 = g.reminders[int64(id1)]
	r2 = g.reminders[int64(id2)]

	if r1.Finished != r2.Finished {
		if r1.Finished {
			return 1
		} else {
			return -1
		}
	} else if !r1.Timestamp.Equal(r2.Timestamp) {
		if r1.Timestamp.Before(r2.Timestamp) {
			return -1
		} else {
			return 1
		}
	}

	return strings.Compare(r1.Title, r2.Title)

	// if r1.Timestamp.Before(r2.Timestamp) {
	// 	return -1
	// } else if r1.Timestamp.After(r2.Timestamp) {
	// 	return 1
	// } else {
	// 	return strings.Compare(r1.Title, r2.Title)
	// }
} // func (g *GUI) reminderCmpFunc(a, b *gtk.TreeIter) int
