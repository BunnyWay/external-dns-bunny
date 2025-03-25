// Copyright (c) BunnyWay d.o.o.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"github.com/bunnyway/external-dns-bunny/internal/dnsprovider"
	"github.com/sirupsen/logrus"
	"net/http"
	"os"
	"sigs.k8s.io/external-dns/provider/webhook/api"
	"sync"
	"time"
)

func main() {
	listenAddr := os.Getenv("LISTEN_ADDR")
	if listenAddr == "" {
		listenAddr = "localhost:8888"
	}

	probeAddr := os.Getenv("PROBE_ADDR")
	if probeAddr == "" {
		probeAddr = "localhost:8080"
	}

	logrus.WithFields(logrus.Fields{
		"addr":  listenAddr,
		"probe": probeAddr,
	}).Info("external-dns-bunny: starting server")

	healthHandler := http.NewServeMux()
	healthHandler.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	var wg sync.WaitGroup

	p := dnsprovider.NewProvider()
	startedChan := make(chan struct{})

	wg.Add(1)
	go func() {
		defer wg.Done()
		api.StartHTTPApi(
			p,
			startedChan,
			time.Second*2,
			time.Second*2,
			listenAddr,
		)
	}()

	<-startedChan

	wg.Add(1)
	go func() {
		defer wg.Done()
		err := http.ListenAndServe(probeAddr, healthHandler)
		logrus.Error(err)
	}()

	// @TODO handle signals

	wg.Wait()
	logrus.Error("external-dns-bunny: stopping server")
}
