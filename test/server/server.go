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

func handleChannel(channel *bc.Channel) {
	log.Printf("Handlechannel (%q)\n", channel.Sid)

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
			channel.SendArray([]interface{}{t})
		case <-done:
			channel.Close()
		}
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
	handler.SetCrossDomainPrefix(*hostname+":"+*port, []string{"bc0", "bc1"})

	http.Handle("/channel/", logr(handler))
	http.Handle("/closure-library/",
		logr(http.StripPrefix("/closure-library/", http.FileServer(http.Dir(*closureDir)))))

	http.Handle("/", http.FileServer(http.Dir(*publicDir)))

	err := http.ListenAndServe(":"+*port, nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
