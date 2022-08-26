// /home/krylon/go/src/github.com/blicero/theseus/backend/dnssd.go
// -*- mode: go; coding: utf-8; -*-
// Created on 24. 08. 2022 by Benjamin Walkenhorst
// (c) 2022 Benjamin Walkenhorst
// Time-stamp: <2022-08-26 21:08:27 krylon>

package backend

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/blicero/theseus/common"
	"github.com/grandcat/zeroconf"
)

const (
	srvName    = "Theseus"
	srvService = "_http._tcp"
	srvDomain  = "local."
)

var (
	addrPat = regexp.MustCompile(`:(\d+)$`)
)

func (d *Daemon) initDNSSd() error {
	var (
		err   error
		match []string
		port  int64
		srv   *zeroconf.Server
	)

	match = addrPat.FindStringSubmatch(d.web.Addr)

	if port, err = strconv.ParseInt(match[1], 10, 16); err != nil {
		d.log.Printf("[ERROR] Cannot parse HTTP port from server address %q: %s\n",
			d.web.Addr,
			err.Error())
		return err
	}

	var txt = []string{"txtv=0", "lo=1", "la=2"}

	var instanceName = fmt.Sprintf("%s@%s",
		srvName,
		d.hostname)

	if srv, err = zeroconf.Register(instanceName, srvService, srvDomain, int(port), txt, nil); err != nil {
		d.log.Printf("[ERROR] Cannot register service with DNS-SD: %s\n",
			err.Error())
		return err
	}

	d.dnssd = srv
	return nil
} // func (d *Daemon) initDnsSd() error

func (d *Daemon) findPeers() {
	var (
		err      error
		resolver *zeroconf.Resolver
		entries  chan *zeroconf.ServiceEntry
	)

	if resolver, err = zeroconf.NewResolver(nil); err != nil {
		d.log.Printf("[ERROR] Cannot create DNS-SD Resolver: %s\n",
			err.Error())
		return
	}

	entries = make(chan *zeroconf.ServiceEntry)

	go d.processServiceEntries(entries)

	// ctx, _ := context.WithCancel(context.Background())
	// defer cancel()

	if err = resolver.Browse(context.TODO(), srvService, srvDomain, entries); err != nil {
		d.log.Printf("{ERROR] Failed to browse for %s: %s\n",
			srvService,
			err.Error())
	}
} // func (d *Daemon) findPeers()

func (d *Daemon) processServiceEntries(queue <-chan *zeroconf.ServiceEntry) {
	defer d.log.Println("[INFO] DNS-SD Listener is quitting.")
	var peerPat = regexp.MustCompile(fmt.Sprintf("%s\\\\@(\\w+)", common.AppName))

	for entry := range queue {
		var str = rrStr(entry)
		d.log.Printf("[DEBUG] Received one ServiceEntry: %s\n",
			str)

		if strings.HasPrefix(entry.HostName, d.hostname) {
			continue
		} else if !peerPat.MatchString(entry.Instance) {
			continue
		}

		d.log.Println("[TRACE] Acquire pLock")
		d.pLock.Lock()
		d.peers[str] = entry
		d.pLock.Unlock()
		d.log.Println("[TRACE] Released pLock")
	}
} // func (d *Daemon) processServiceEntries(queue <- chan *zeroconf.ServiceEntry)
