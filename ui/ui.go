// /home/krylon/go/src/github.com/blicero/theseus/ui/ui.go
// -*- mode: go; coding: utf-8; -*-
// Created on 06. 07. 2022 by Benjamin Walkenhorst
// (c) 2022 Benjamin Walkenhorst
// Time-stamp: <2022-07-09 17:20:39 krylon>

package ui

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
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
	defaultBufSize = 65536 // 64 KiB
	uriGetPending  = "/reminder/pending"
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
}

func createCol(title string, id int) (*gtk.TreeViewColumn, *gtk.CellRendererText, error) {
	krylib.Trace()
	defer fmt.Printf("[TRACE] EXIT %s\n",
		krylib.TraceInfo())

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
	srv       string
	log       *log.Logger
	lock      sync.RWMutex // nolint: unused,structcheck
	win       *gtk.Window
	mainBox   *gtk.Box
	store     *gtk.ListStore
	view      *gtk.TreeView
	scr       *gtk.ScrolledWindow
	menuBar   *gtk.MenuBar
	statusbar *gtk.Statusbar
	fMenu     *gtk.Menu // nolint: unused,structcheck
	web       http.Client
	reminders map[int64]objects.Reminder
}

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
	} else if win.view, err = gtk.TreeViewNewWithModel(win.store); err != nil {
		win.log.Printf("[ERROR] Cannot create TreeView: %s\n",
			err.Error())
		return nil, err
	}

	for i, c := range cols {
		var (
			col      *gtk.TreeViewColumn
			renderer *gtk.CellRendererText
		)

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

	if err = win.initializeTree(); err != nil {
		win.log.Printf("[ERROR] Failed to initialize TreeView: %s\n",
			err.Error())
		return nil, err
	}

	win.win.ShowAll()
	win.win.SetSizeRequest(960, 540)
	win.win.SetTitle(fmt.Sprintf("%s %s",
		common.AppName,
		common.Version))

	glib.TimeoutAdd(uint(10000), win.fetchReminders)

	return win, nil
} // func Create(srv string) (*GUI, error)

// Run executes gtk's main event loop.
func (g *GUI) Run() {
	go func() {
		var cnt = 0
		for {
			time.Sleep(time.Second)
			cnt++
			var msg = fmt.Sprintf("%s: Tick #%d",
				time.Now().Format(common.TimestampFormat),
				cnt)
			g.statusbar.Push(666, msg)
		}
	}()

	gtk.Main()
} // func (w *RWin) Run()

func (g *GUI) initializeTree() error {
	var (
		err error
		sel *gtk.TreeSelection
	)

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
		err    error
		rawURL string
		res    *http.Response
	)

	krylib.Trace()

	rawURL = fmt.Sprintf("http://%s%s",
		g.srv,
		uriGetPending)

	if _, err = url.Parse(rawURL); err != nil {
		g.log.Printf("[ERROR] Invalid URL %q: %s\n",
			rawURL,
			err.Error())
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

	g.store.Clear()

	for _, r := range list {
		g.reminders[r.ID] = r

		var iter = g.store.Append()

		g.store.Set( // nolint: errcheck
			iter,
			[]int{0, 1, 2, 3},
			[]any{r.ID, r.Title, r.Timestamp.Format(common.TimestampFormat), r.Finished},
		)
	}

	return true
} // func (g *GUI) fetchReminders() bool
