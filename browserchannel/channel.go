// Copyright (c) 2013 Mathieu Turcotte
// Licensed under the MIT license.

package browserchannel

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"
)

var (
	ErrClosed = errors.New("channel closed")
)

const (
	maxOutgoingArrays          = 100
	channelReopenTimeoutDelay  = 20 * time.Second
	backChannelExpirationDelay = 1 * time.Minute
	backChannelHeartbeatDelay  = 30 * time.Second
	ackTimeoutDelay            = 1 * time.Minute
)

type channelState int

const (
	channelInit channelState = iota
	channelReady
	channelWriteClosed
	channelClosed
)

// Type of the data transmitted from the server to the client. The values
// stored in the array will be serialized to JSON.
type Array []interface{}

type outgoingArray struct {
	index    int
	elements Array
}

var (
	noopArray = Array{"noop"}
	stopArray = Array{"stop"}
)

func marshalOutgoingArrays(arrays []*outgoingArray) (data []byte, err error) {
	carrays := []interface{}{}
	for _, a := range arrays {
		carrays = append(carrays, []interface{}{a.index, a.elements})
	}
	data, err = json.Marshal(carrays)
	return
}

type Channel struct {
	// The client specific version string.
	Version string
	// The channel session id.
	Sid   SessionId
	state channelState

	backChannel backChannel
	corsInfo    *crossDomainInfo

	maps           *mapQueue
	outgoingArrays []*outgoingArray

	lastArrayId     int
	lastSentArrayId int

	ackTimeout            *time.Timer
	channelTimeout        *time.Timer
	backChannelExpiration *time.Timer
	backChannelHeartbeat  *time.Ticker

	heartbeatStop chan bool
	gcChan        chan<- SessionId
	mapChan       chan *Map

	lock sync.Mutex
}

func newChannel(clientVersion string, sid SessionId, gcChan chan<- SessionId,
	corsInfo *crossDomainInfo) (c *Channel) {
	return &Channel{
		Version:              clientVersion,
		Sid:                  sid,
		state:                channelInit,
		corsInfo:             corsInfo,
		maps:                 newMapQueue(100 /* capacity */),
		outgoingArrays:       []*outgoingArray{},
		backChannelHeartbeat: time.NewTicker(backChannelHeartbeatDelay),
		heartbeatStop:        make(chan bool, 1),
		mapChan:              make(chan *Map, 100),
		gcChan:               gcChan,
	}
}

func (c *Channel) log(format string, v ...interface{}) {
	log.Printf("%s: %s", c.Sid, fmt.Sprintf(format, v...))
}

// Sends an array on the channel. Will return an error if the channel isn't
// ready, i.e. initializing or closed.
func (c *Channel) SendArray(array Array) (err error) {
	c.lock.Lock()
	defer c.lock.Unlock()

	if c.state != channelReady {
		err = ErrClosed
		return
	}

	c.queueArray(array)
	c.flush()
	return
}

func (c *Channel) queueArray(a Array) {
	c.lastArrayId++
	outgoingArray := &outgoingArray{c.lastArrayId, a}
	c.outgoingArrays = append(c.outgoingArrays, outgoingArray)
}

func (c *Channel) flush() {
	numUnsentArrays := c.lastArrayId - c.lastSentArrayId

	if c.backChannel == nil || numUnsentArrays == 0 {
		return
	}

	next := len(c.outgoingArrays) - numUnsentArrays
	data, _ := marshalOutgoingArrays(c.outgoingArrays[next:])
	c.backChannel.send(data)
	c.lastSentArrayId = c.lastArrayId
	c.resetAckTimeout()

	// If the channel is in the write closed state, i.e. was closed from the
	// server side, then permanently shutdown the channel. The client won't
	// send acknowledgments once the stop signal has been sent.
	if c.state == channelWriteClosed {
		c.terminateInternal()
		return
	}

	// If the number of buffered outgoing arrays is greater than a given
	// threshold, force a back channel change to get acknowledgments so
	// we can free some of them later.
	if !c.backChannel.isReusable() || len(c.outgoingArrays) > 100 {
		c.log("discarding back channel")
		c.clearBackChannel(false /* permanent */)
	}
}

// Reads a map from the client. The call will block until there is a map
// ready or the channel is closed in which case the ok will be false.
func (c *Channel) ReadMap() (m *Map, ok bool) {
	m, ok = <-c.mapChan
	return
}

// Close the channel from the server side. Outgoing arrays will be delivered to
// the client before shutting down the channel permanently. SendArray calls will
// return an error after the channel has been closed.
func (c *Channel) Close() {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.state = channelWriteClosed
	c.queueArray(stopArray)
	c.flush()
	close(c.mapChan)
}

// Close the channel after a query terminate was received from the client.
// Intended to be called from the channel handler.
func (c *Channel) terminate() {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.terminateInternal()
}

func (c *Channel) terminateInternal() {
	c.clearBackChannel(true /* permanent */)
	c.state = channelClosed
	c.gcChan <- c.Sid

	if c.state == channelInit || c.state == channelReady {
		close(c.mapChan)
	}
}

func (c *Channel) getState() []int {
	c.lock.Lock()
	defer c.lock.Unlock()

	outstanding := 0
	if len(c.outgoingArrays) > 0 {
		outstanding = 15
	}

	backChannel := 0
	if c.backChannel != nil {
		backChannel = 1
	}

	return []int{backChannel, c.lastSentArrayId, outstanding}
}

func (c *Channel) receiveMaps(offset int, maps []Map) {
	c.lock.Lock()
	defer c.lock.Unlock()

	if len(maps) == 0 {
		return
	}

	if c.state == channelReady {
		c.log("receive %v", maps)
		c.maps.enqueue(offset, maps)
		c.dequeueMaps()
	} else {
		c.log("drop %v; channel isn't ready", maps)
	}
}

func (c *Channel) dequeueMaps() {
	for {
		if m, ok := c.maps.dequeue(); ok {
			c.mapChan <- &m
		} else {
			break
		}
	}
}

func (c *Channel) acknowledgeArrays(aid int) {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.log("acknowledge %d", aid)

	for len(c.outgoingArrays) > 0 && c.outgoingArrays[0].index <= aid {
		c.outgoingArrays = c.outgoingArrays[1:]
	}

	// Make sure to release the reference to the underlying array by copying
	// the unacknowledged arrays into a new slice.
	out := make([]*outgoingArray, len(c.outgoingArrays))
	copy(out, c.outgoingArrays)
	c.outgoingArrays = out

	c.clearAckTimeout()
}

// Sets or replaces the back channel.
func (c *Channel) setBackChannel(bc backChannel) {
	c.lock.Lock()
	defer c.lock.Unlock()

	if c.state == channelClosed {
		bc.discard()
		return
	}

	c.log("set back channel [rid:%s,chunked:%t]", bc.getRequestId(), bc.isChunked())

	if c.state == channelInit {
		hostPrefix := getHostPrefix(c.corsInfo)
		c.queueArray(Array{"c", c.Sid.String(), hostPrefix, 8})
		c.state = channelReady
	}

	if c.backChannel != nil {
		c.log("dropping back channel to set new one")
		c.clearBackChannel(false /* permanent */)
	}

	c.backChannel = bc
	c.clearChannelTimeout()
	c.armBackChannelTimeouts()

	// Special care is needed to account for the fact that the old back
	// channel may have died uncleanly. To make sure that all arrays are
	// effectively received by the client, the last sent array id is reset
	// to the last unacknowledged array id.
	if len(c.outgoingArrays) > 0 {
		c.lastSentArrayId = c.outgoingArrays[0].index - 1
	}

	c.flush()
}

// Clear back channel and starts the channel session timeout if the permanent
// argument is false.
func (c *Channel) clearBackChannel(permanent bool) {
	if c.backChannel == nil {
		return
	}

	c.log("clear back channel %s", c.backChannel.getRequestId())

	c.clearAckTimeout()
	c.clearBackChannelTimeouts()

	c.backChannel.discard()
	c.backChannel = nil

	// When the back channel is cleared permanently, this means that the channel
	// is being shut down. The channel timeout shouldn't be arm in this case.
	if !permanent {
		c.armChannelTimeout()
	}
}

func (c *Channel) clearAckTimeout() {
	if c.ackTimeout != nil {
		c.ackTimeout.Stop()
		c.ackTimeout = nil
	}
}

func (c *Channel) resetAckTimeout() {
	c.clearAckTimeout()
	c.ackTimeout = time.AfterFunc(ackTimeoutDelay, func() {
		c.lock.Lock()
		defer c.lock.Unlock()

		c.log("ack timeout")
		c.clearBackChannel(false /* permanent */)
	})
}

func (c *Channel) armBackChannelTimeouts() {
	c.backChannelExpiration = time.AfterFunc(backChannelExpirationDelay, func() {
		c.lock.Lock()
		defer c.lock.Unlock()

		c.log("back channel expired")
		c.clearBackChannel(false /* permanent */)
	})

	go heartbeat(c, c.backChannelHeartbeat.C, c.heartbeatStop)
}

func heartbeat(c *Channel, ticks <-chan time.Time, stops <-chan bool) {
	c.log("start heartbeats")

	for {
		select {
		case <-ticks:
			c.log("heartbeat")
			c.SendArray(noopArray)
		case <-stops:
			c.log("stop heartbeats")
			return
		}
	}
}

func (c *Channel) clearBackChannelTimeouts() {
	c.backChannelExpiration.Stop()
	c.backChannelExpiration = nil
	c.heartbeatStop <- true
}

func (c *Channel) armChannelTimeout() {
	c.channelTimeout = time.AfterFunc(channelReopenTimeoutDelay, func() {
		c.log("channel timeout")
		c.terminate()
	})
}

func (c *Channel) clearChannelTimeout() {
	c.channelTimeout.Stop()
	c.channelTimeout = nil
}
