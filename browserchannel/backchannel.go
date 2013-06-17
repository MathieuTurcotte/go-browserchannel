// Copyright (c) 2013 Mathieu Turcotte
// Licensed under the MIT license.

package browserchannel

import (
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
)

const dataChannelCapacity = 128

// The back channel interface shared between the XHR and HTML implementations.
type backChannel interface {
	getRequestId() string
	isReusable() bool
	setChunked(bool)
	isChunked() bool
	send(data []byte) error
	discard()
	wait()
}

// Common bookeeping information shared between the chunked XHR and HTML
// variants.
type backChannelBase struct {
	sid       SessionId
	rid       string
	w         http.ResponseWriter
	chunked   bool
	bytesSent int
	dataChan  chan []byte
	err       error
}

func (b *backChannelBase) getRequestId() string {
	return b.rid
}

func (b *backChannelBase) isReusable() bool {
	return b.chunked && b.bytesSent < 10*1024 && b.err == nil
}

func (b *backChannelBase) setChunked(chunked bool) {
	b.chunked = chunked
}

func (b *backChannelBase) send(data []byte) (err error) {
	select {
	case b.dataChan <- data:
		b.bytesSent += len(data)
		// Optimistically assumes that all the data was written successfully.
		// If an error occurs, the protocol will recover by its own means.
	default:
		log.Printf("%s[%s] back channel full\n", b.sid, b.rid)
		err = errors.New("data channel full")
		b.err = err
	}

	return
}

func (b *backChannelBase) isChunked() bool {
	return b.chunked
}

// The chunked XHR back channel implementation.
type xhrBackChannel struct {
	backChannelBase
}

func (b *xhrBackChannel) wait() {
	for data := range b.dataChan {
		log.Printf("%s[%s] xhr back channel send: %s\n", b.sid, b.rid, data)
		str := strconv.FormatInt(int64(len(data)), 10) + "\n" + string(data)
		io.WriteString(b.w, str)
		b.w.(http.Flusher).Flush()
	}

	log.Printf("%s[%s] bind wait done\n", b.sid, b.rid)
}

func (b *xhrBackChannel) discard() {
	log.Printf("%s[%s] xhr back channel close\n", b.sid, b.rid)
	close(b.dataChan)
}

// The chunked HTML back channel implementation used for IE9 and older.
type htmlBackChannel struct {
	backChannelBase
	paddingSent bool
	domain      string
}

func (b *htmlBackChannel) wait() {
	for data := range b.dataChan {
		log.Printf("%s[%s] html back channel send: %s\n", b.sid, b.rid, data)

		if !b.paddingSent {
			writeHtmlHead(b.w)
			writeHtmlDomain(b.w, b.domain)
		}

		writeHtmlRpc(b.w, string(data))

		if !b.paddingSent {
			writeHtmlPadding(b.w)
			b.paddingSent = true
		}

		b.w.(http.Flusher).Flush()
	}

	writeHtmlDone(b.w)

	log.Printf("%s[%s] bind wait done\n", b.sid, b.rid)
}

func (b *htmlBackChannel) discard() {
	log.Printf("%s %s html back channel close\n", b.sid, b.rid)
	close(b.dataChan)
}

func newBackChannel(sid SessionId, w http.ResponseWriter, html bool,
	domain string, rid string) (bc backChannel) {
	base := backChannelBase{
		sid:      sid,
		rid:      rid,
		w:        w,
		dataChan: make(chan []byte, dataChannelCapacity)}

	if html {
		bc = &htmlBackChannel{backChannelBase: base, domain: domain}
	} else {
		bc = &xhrBackChannel{base}
	}
	return
}
