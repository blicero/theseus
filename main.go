// /home/krylon/go/src/github.com/blicero/theseus/main.go
// -*- mode: go; coding: utf-8; -*-
// Created on 01. 07. 2022 by Benjamin Walkenhorst
// (c) 2022 Benjamin Walkenhorst
// Time-stamp: <2022-07-06 22:46:26 krylon>

package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/blicero/theseus/backend"
	"github.com/blicero/theseus/common"
)

func main() {
	fmt.Printf("%s %s (built %s)\n",
		common.AppName,
		common.Version,
		common.BuildStamp)

	var (
		err                error
		daemon             *backend.Daemon
		appDir, mode, addr string
	)

	flag.StringVar(
		&appDir,
		"appdir",
		common.BaseDir,
		"The directory where application-specific files live")

	flag.StringVar(
		&mode,
		"mode",
		"backend",
		"Whether to run the *backend* or the *frontend*",
	)

	flag.StringVar(
		&addr,
		"address",
		fmt.Sprintf("localhost:%d", common.DefaultPort),
		"Address to either listen on (backend) or connect to (frontend)",
	)

	flag.Parse()

	if mode == "backend" {
		if daemon, err = backend.Summon(addr); err != nil {
			fmt.Fprintf(
				os.Stderr,
				"Failed to initialize backend: %s\n",
				err.Error())
			os.Exit(1)
		}

		var sigQ = make(chan os.Signal, 1)
		var ticker = time.NewTicker(time.Second * 2)

		signal.Notify(sigQ, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM)

		for daemon.IsAlive() {
			select {
			case sig := <-sigQ:
				fmt.Printf("Quitting on signal %s\n", sig)
			case <-ticker.C:
				continue
			}
		}
	} else if mode == "frontend" {
		fmt.Println("You probably want to BUILD the frontend first!")
	}
}
