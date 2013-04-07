// Copyright (c) 2013 Mathieu Turcotte
// Licensed under the MIT license.

// Package browserchannel provides a server-side browser channel
// implementation. See http://goo.gl/F287G for the client-side API.
package bc

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// The browser channel protocol version implemented by this library.
const SupportedProcolVersion = 8

var (
	ErrClosed       = errors.New("handler closed")
	ErrBadMap       = errors.New("bad map")
	ErrBodyTooLarge = errors.New("body too large")
)

const (
	DefaultBindPath = "bind"
	DefaultTestPath = "test"
)

type queryType int

// Possible values for the query TYPE parameter.
const (
	queryUnset = iota
	queryTerminate
	queryXmlHttp
	queryHtml
	queryTest
)

func parseQueryType(s string) (qtype queryType) {
	switch s {
	case "html":
		qtype = queryHtml
	case "xmlhttp":
		qtype = queryXmlHttp
	case "terminate":
		qtype = queryTerminate
	case "test":
		qtype = queryTest
	}
	return
}

func parseAid(s string) (aid int, err error) {
	aid = -1
	if len(s) == 0 {
		return
	}
	aid, err = strconv.Atoi(s)
	return
}

func parseProtoVersion(s string) (version int) {
	version, err := strconv.Atoi(s)
	if err != nil {
		version = -1
	}
	return
}

type bindParams struct {
	cver    string
	sid     SessionId
	qtype   queryType
	domain  string
	rid     string
	aid     int
	chunked bool
	values  url.Values
	method  string
}

func parseBindParams(req *http.Request, values url.Values) (params *bindParams, err error) {
	cver := req.Form.Get("VER")
	qtype := parseQueryType(req.Form.Get("TYPE"))
	domain := req.Form.Get("DOMAIN")
	rid := req.Form.Get("zx")
	ci := req.Form.Get("CI") == "1"
	sid, err := parseSessionId(req.Form.Get("SID"))
	if err != nil {
		return
	}
	aid, err := parseAid(req.Form.Get("AID"))
	if err != nil {
		return
	}
	params = &bindParams{cver, sid, qtype, domain, rid, aid, ci, values, req.Method}
	return
}

type testParams struct {
	ver    int
	init   bool
	qtype  queryType
	domain string
}

func parseTestParams(req *http.Request) *testParams {
	version := parseProtoVersion(req.Form.Get("VER"))
	qtype := parseQueryType(req.Form.Get("TYPE"))
	domain := req.Form.Get("DOMAIN")
	init := req.Form.Get("MODE") == "init"
	return &testParams{version, init, qtype, domain}
}

// Set of default headers returned for each XML HTTP requests.
var xmlHttpHeaders = map[string]string{
	"Content-Type":           "text/plain",
	"Cache-Control":          "no-cache, no-store, max-age=0, must-revalidate",
	"Expires":                "Fri, 01 Jan 1990 00:00:00 GMT",
	"X-Content-Type-Options": "nosniff",
	"Transfer-Encoding":      "chunked",
	"Pragma":                 "no-cache",
}

// Headers returned to html requests (IE < 10).
var htmlHeaders = map[string]string{
	"Content-Type":           "text/html",
	"Cache-Control":          "no-cache, no-store, max-age=0, must-revalidate",
	"Expires":                "Fri, 01 Jan 1990 00:00:00 GMT",
	"X-Content-Type-Options": "nosniff",
	"Transfer-Encoding":      "chunked",
	"Pragma":                 "no-cache",
}

func getHeaders(qtype queryType) (headers *map[string]string) {
	if qtype == queryHtml {
		headers = &htmlHeaders
	} else {
		headers = &xmlHttpHeaders
	}
	return
}

type channelMap struct {
	sync.RWMutex
	m map[SessionId]*Channel
}

func (m *channelMap) get(sid SessionId) *Channel {
	m.RLock()
	defer m.RUnlock()
	return m.m[sid]
}

func (m *channelMap) set(sid SessionId, channel *Channel) {
	m.Lock()
	defer m.Unlock()
	m.m[sid] = channel
}

func (m *channelMap) del(sid SessionId) (deleted bool) {
	m.Lock()
	defer m.Unlock()
	_, deleted = m.m[sid]
	delete(m.m, sid)
	return
}

// Contains the browser channel cross domain info for a single domain.
type crossDomainInfo struct {
	hostMatcher *regexp.Regexp
	domain      string
	prefix      string
}

func getHostPrefix(info *crossDomainInfo) string {
	if info != nil {
		return info.prefix
	}
	return ""
}

type Handler struct {
	corsInfo *crossDomainInfo
	prefix   string
	channels *channelMap
	bindPath string
	testPath string
	gcChan   chan SessionId
	connChan chan *Channel
}

// Creates a new browser channel handler.
func NewHandler() (h *Handler) {
	h = new(Handler)
	h.channels = &channelMap{m: make(map[SessionId]*Channel)}
	h.bindPath = DefaultBindPath
	h.testPath = DefaultTestPath
	h.gcChan = make(chan SessionId, 10)
	h.connChan = make(chan *Channel)
	return
}

// Sets the cross domain information for this browser channel. The origin is
// used as the Access-Control-Allow-Origin header value and should respect the
// format specified in http://www.w3.org/TR/cors/. The prefix is used to set
// the hostPrefix parameter on the client side.
func (h *Handler) SetCrossDomainPrefix(domain string, prefix string) {
	h.corsInfo = &crossDomainInfo{makeOriginMatcher(domain), domain, prefix}
}

// Initializes the browser channel handler.
func (h *Handler) Init() *Handler {
	go h.removeClosedSession()
	return h
}

// Accept waits for and returns the next channel to the listener.
func (h *Handler) Accept() (channel *Channel, err error) {
	channel, ok := <-h.connChan
	if !ok {
		err = ErrClosed
	}
	return
}

// Removes closed channels from the handler's channel map.
func (h *Handler) removeClosedSession() {
	for {
		sid, ok := <-h.gcChan
		if !ok {
			break
		}

		log.Printf("removing %s from session map\n", sid)

		if !h.channels.del(sid) {
			log.Printf("missing channel for %s in session map\n", sid)
		}
	}
}

func (h *Handler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	// The CORS  spec only supports *, null or the exact domain.
	// http://www.w3.org/TR/cors/#access-control-allow-origin-response-header
	// http://tools.ietf.org/html/rfc6454#section-7.1
	origin := req.Header.Get("origin")
	if len(origin) > 0 && h.corsInfo != nil &&
		h.corsInfo.hostMatcher.MatchString(origin) {
		rw.Header().Set("Access-Control-Allow-Origin", origin)
		rw.Header().Set("Access-Control-Allow-Credentials", "true")
	}

	// The body is parsed before calling ParseForm so the values don't get
	// collapsed into a single collection.
	values, err := parseBody(req.Body)
	if err != nil {
		rw.WriteHeader(400)
		return
	}

	req.ParseForm()

	path := req.URL.Path
	if strings.HasSuffix(path, h.testPath) {
		log.Printf("test:%s\n", req.URL)
		h.handleTestRequest(rw, parseTestParams(req))
	} else if strings.HasSuffix(path, h.bindPath) {
		log.Printf("bind:%s:%s\n", req.Method, req.URL)
		params, err := parseBindParams(req, values)
		if err != nil {
			rw.WriteHeader(400)
			return
		}
		h.handleBindRequest(rw, params)
	} else {
		rw.WriteHeader(404)
	}
}

func (h *Handler) handleTestRequest(rw http.ResponseWriter, params *testParams) {
	if params.ver != SupportedProcolVersion {
		rw.Header().Set("Status", "Unsupported protocol version.")
		rw.WriteHeader(400)
		io.WriteString(rw, "Unsupported protocol version.")
	} else if params.init {
		rw.Header().Set("Status", "OK")
		setHeaders(rw, &xmlHttpHeaders)
		rw.WriteHeader(200)
		io.WriteString(rw, "[\""+getHostPrefix(h.corsInfo)+"\",\"\"]")
	} else {
		rw.Header().Set("Status", "OK")
		setHeaders(rw, getHeaders(params.qtype))
		rw.WriteHeader(200)

		if params.qtype == queryHtml {
			writeHtmlHead(rw)
			writeHtmlDomain(rw, params.domain)
			writeHtmlRpc(rw, "11111")
			writeHtmlPadding(rw)
		} else {
			io.WriteString(rw, "11111")
		}

		time.Sleep(2 * time.Second)

		if params.qtype == queryHtml {
			writeHtmlRpc(rw, "2")
			writeHtmlDone(rw)
		} else {
			io.WriteString(rw, "2")
		}
	}
}

func (h *Handler) handleBindRequest(rw http.ResponseWriter, params *bindParams) {
	var channel *Channel
	sid := params.sid

	// If the client has specified a session id, lookup the session object
	// in the sessions map. Lookup failure should be signaled to the client
	// using a 400 status code and a message containing 'Unknown SID'. See
	// goog/net/channelrequest.js for more context on how this error is
	// handled.
	if sid != nullSessionId {
		channel = h.channels.get(sid)
		if channel == nil {
			log.Printf("failed to lookup session %s\n", sid)
			rw.Header().Set("Status", "Unknown SID")
			setHeaders(rw, &xmlHttpHeaders)
			rw.WriteHeader(400)
			io.WriteString(rw, "Unknown SID")
			return
		}
	}

	if channel == nil {
		sid, _ = newSesionId()
		log.Printf("creating session %s\n", sid)
		channel = newChannel(params.cver, sid, h.gcChan, h.corsInfo)
		h.channels.set(sid, channel)
		h.connChan <- channel
		go channel.start()
	}

	if params.aid != -1 {
		channel.acknowledge(params.aid)
	}

	switch params.method {
	case "POST":
		h.handleBindPost(rw, params, channel)
	case "GET":
		h.handleBindGet(rw, params, channel)
	default:
		rw.WriteHeader(400)
	}
}

func (h *Handler) handleBindPost(rw http.ResponseWriter, params *bindParams, channel *Channel) {
	log.Printf("%s: bind parameters: %v\n", channel.Sid, params.values)

	offset, maps, err := parseIncomingMaps(params.values)
	if err != nil {
		rw.WriteHeader(400)
		return
	}

	channel.receiveMaps(offset, maps)

	if channel.state == channelInit {
		rw.Header().Set("Status", "OK")
		setHeaders(rw, &xmlHttpHeaders)
		rw.WriteHeader(200)
		rw.(http.Flusher).Flush()

		// The initial forward request is used as a back channel to send the
		// server configuration: ['c', id, host, version]. This payload has to
		// be sent immediately, so the streaming is disabled on the back
		// channel. Note that the first bind request made by IE<10 does not
		// contain a TYPE=html query parameter and therefore receives the same
		// length prefixed array reply as is sent to the XHR streaming clients.
		backChannel := newBackChannel(channel.Sid, rw, false, "", params.rid)
		channel.setBackChannel(backChannel)
		backChannel.wait()
	} else {
		// On normal forward channel request, the session status is returned
		// to the client. The session status contains 3 pieces of information:
		// does this session has a back channel, the last array id sent to the
		// client and the number of outstanding bytes in the back channel.
		b, _ := json.Marshal(channel.getState())
		rw.Header().Set("Status", "OK")
		setHeaders(rw, &xmlHttpHeaders)
		rw.WriteHeader(200)
		io.WriteString(rw, strconv.FormatInt(int64(len(b)), 10)+"\n")
		rw.Write(b)
	}
}

func (h *Handler) handleBindGet(rw http.ResponseWriter, params *bindParams, channel *Channel) {
	if params.qtype == queryTerminate {
		channel.Close()
	} else {
		rw.Header().Set("Status", "OK")
		setHeaders(rw, getHeaders(params.qtype))
		rw.WriteHeader(200)
		rw.(http.Flusher).Flush()

		isHtml := params.qtype == queryHtml
		bc := newBackChannel(channel.Sid, rw, isHtml, params.domain, params.rid)
		bc.setChunked(params.chunked)
		channel.setBackChannel(bc)
		bc.wait()
	}
}
