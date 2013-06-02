// Copyright (c) 2013 Mathieu Turcotte
// Licensed under the MIT license.

package main

import (
	"flag"
	bc "github.com/MathieuTurcotte/go-browserchannel/browserchannel"
	"log"
	"net/http"
	"time"
)

var publicDir = flag.String("public_directory", "", "path to public directory")
var closureDir = flag.String("closure_directory", "", "path to closure directory")
var port = flag.String("port", "8080", "the port to listen on")
var hostname = flag.String("hostname", "hpenvy.local", "the server hostname")

func handleTest2(channel *bc.Channel) {
	log.Printf("handleTest2 (%q)\n", channel.Sid)

	for m := range channel.Maps() {
		if payload, ok := m["payload"]; ok {
			channel.SendArray(bc.Array{payload})
		}
	}
}

func handleTest1(channel *bc.Channel) {
	log.Printf("handleTest1 (%q)\n", channel.Sid)

	ticks := time.Tick(15 * time.Second)
	done := time.After(5 * time.Minute)

	for {
		select {
		case _, ok := <-channel.Maps():
			if !ok {
				log.Printf("%s: returned with no data, closing\n", channel.Sid)
				return
			}
		case t := <-ticks:
			channel.SendArray(bc.Array{t})
		case <-done:
			channel.Close()
		}
	}
}

func handleChannel(channel *bc.Channel) {
	log.Printf("handleChannel: %q\n", channel.Sid)

	testHandlers := map[string]bc.ChannelHandler{
		"1": handleTest1,
		"2": handleTest2,
	}

	select {
	case m, ok := <-channel.Maps():
		if ok {
			id := m["id"]
			if handler, ok := testHandlers[id]; ok {
				handler(channel)
			} else {
				log.Printf("%s: no test handler for %s\n", channel.Sid, id)
			}
		} else {
			log.Printf("%s: returned with no data, closing\n", channel.Sid)
		}
	case <-time.After(10 * time.Second):
		log.Printf("%s: timed out before selecting test case\n", channel.Sid)
		channel.Close()
	}
}

func logr(handler http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s", r.URL)
		handler.ServeHTTP(w, r)
	}
}

func main() {
	flag.Parse()

	log.Printf("closure dir: %s", *closureDir)
	log.Printf("public dir: %s", *publicDir)

	handler := bc.NewHandler(handleChannel)
	handler.SetCrossDomainPrefix(*hostname+":"+*port, []string{"bc0", "bc1", "bc2"})

	http.Handle("/channel/", logr(handler))
	http.Handle("/closure-library/",
		logr(http.StripPrefix("/closure-library/", http.FileServer(http.Dir(*closureDir)))))

	http.Handle("/", http.FileServer(http.Dir(*publicDir)))

	err := http.ListenAndServe(":"+*port, nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
