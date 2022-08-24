// /home/krylon/go/src/github.com/blicero/theseus/backend/dnssd.go
// -*- mode: go; coding: utf-8; -*-
// Created on 24. 08. 2022 by Benjamin Walkenhorst
// (c) 2022 Benjamin Walkenhorst
// Time-stamp: <2022-08-24 22:22:31 krylon>

package backend

import (
	"regexp"
	"strconv"

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

	if srv, err = zeroconf.Register(srvName, srvService, srvDomain, int(port), txt, nil); err != nil {
		d.log.Printf("[ERROR] Cannot register service with DNS-SD: %s\n",
			err.Error())
		return err
	}

	d.dnssd = srv
	return nil
} // func (d *Daemon) initDnsSd() error

func (d *Daemon) findPeers() error {
	var (
		err      error
		resolver *zeroconf.Resolver
		entries  chan *zeroconf.ServiceEntry
	)

	if resolver, err = zeroconf.NewResolver(nil); err != nil {
		d.log.Printf("[ERROR] Cannot create DNS-SD Resolver: %s\n",
			err.Error())
		return err
	}

	entries = make(chan *zeroconf.ServiceEntry)

	go d.processServiceEntries(entries)

	return nil
} // func (d *Daemon) findPeers() error

func (d *Daemon) processServiceEntries(queue <-chan *zeroconf.ServiceEntry) {
	defer d.log.Println("[INFO] DNS-SD Listener is quitting.")

	for entry := range queue {
		d.log.Printf("[DEBUG] Received one ServiceEntry: %s\n",
			entry)
	}
} // func (d *Daemon) processServiceEntries(queue <- chan *zeroconf.ServiceEntry)
