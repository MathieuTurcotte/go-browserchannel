// Copyright (c) 2013 Mathieu Turcotte
// Licensed under the MIT license.

package bc

import (
	"encoding/json"
	"fmt"
	"log"
	"time"
)

const (
	maxOutgoingArrays                 = 100
	channelReopenTimeoutDelay         = 20 * time.Second
	backChannelExpirationTimeoutDelay = 1 * time.Minute
	backChannelHeartbeatDelay         = 30 * time.Second
	ackTimeoutDelay                   = 1 * time.Minute
)

type channelState int

const (
	channelInit channelState = iota
	channelReady
	channelClosed
)

// Type of the data transmitted from the server to the client. The values
// stored in the array will be serialized to JSON.
type Array []interface{}

type outgoingArray struct {
	index    int
	elements Array
}

// Interface for the operations that are queued on the internal channel event
// loop.
type operation interface {
	execute(*Channel)
}

// There is one Channel instance per connection. Each instance run in its own
// goroutine until it is being closed either on the client-side or on the
// server-side due to a timeout.
type Channel struct {
	Version string
	Sid     SessionId
	state   channelState

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

	gcChan    chan<- SessionId
	opChan    chan operation
	mapChan   chan *Map
	arrayChan chan Array
}

func newChannel(clientVersion string, sid SessionId, gcChan chan<- SessionId,
	corsInfo *crossDomainInfo) *Channel {
	return &Channel{
		Version:        clientVersion,
		Sid:            sid,
		state:          channelInit,
		corsInfo:       corsInfo,
		maps:           newMapQueue(100 /* capacity */),
		outgoingArrays: []*outgoingArray{},
		gcChan:         gcChan,
		opChan:         make(chan operation, 10),
		mapChan:        make(chan *Map, 10),
		arrayChan:      make(chan Array),
	}
}

func (c *Channel) log(format string, v ...interface{}) {
	log.Printf("%s: %s", c.Sid, fmt.Sprintf(format, v...))
}

func (c *Channel) start() {
	c.armChannelTimeout()

	for c.state != channelClosed {
		select {
		case op := <-c.opChan:
			op.execute(c)
		case <-timerChan(c.ackTimeout):
			c.log("ack timeout")
			c.clearBackChannel(false /* permanent */)
		case <-timerChan(c.channelTimeout):
			c.log("channel timeout")
			c.Close()
		case <-timerChan(c.backChannelExpiration):
			c.log("back channel timeout")
			c.clearBackChannel(false /* permanent */)
		case <-tickerChan(c.backChannelHeartbeat):
			c.log("back channel heartbeat")
			c.queueArray(Array{"noop"})
			c.flush()
		case array := <-c.arrayChan:
			c.queueArray(array)
			c.flush()
		}
	}
}

func (c *Channel) SendArray(array []interface{}) {
	c.arrayChan <- array
}

func (c *Channel) ReadMap() (m *Map, ok bool) {
	m, ok = <-c.mapChan
	return
}

func (c *Channel) Close() {
	if c.state == channelClosed {
		return
	}

	c.clearBackChannel(true /* permanent */)
	c.state = channelClosed
	c.gcChan <- c.Sid
}

func (c *Channel) getState() []int {
	op := &getStateOp{make(chan []int)}
	c.opChan <- op
	return <-op.c
}

type getStateOp struct {
	c chan []int
}

func (op *getStateOp) execute(c *Channel) {
	outstanding := 0
	if len(c.outgoingArrays) > 0 {
		outstanding = 15
	}
	backChannel := 0
	if c.backChannel != nil {
		backChannel = 1
	}
	op.c <- []int{backChannel, c.lastSentArrayId, outstanding}
}

func (c *Channel) receiveMaps(offset int, maps []Map) {
	c.opChan <- &receiveMapsOp{offset, maps}
}

type receiveMapsOp struct {
	offset int
	maps   []Map
}

func (op *receiveMapsOp) execute(c *Channel) {
	c.log("%v", op.maps)
	c.maps.enqueue(op.offset, op.maps)
	c.dequeueMaps()
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

func (c *Channel) queueArray(a Array) {
	c.lastArrayId++
	outgoingArray := &outgoingArray{c.lastArrayId, a}
	c.outgoingArrays = append(c.outgoingArrays, outgoingArray)
}

func (c *Channel) acknowledge(aid int) {
	c.opChan <- &acknowledgeOp{aid}
}

type acknowledgeOp struct {
	aid int
}

func (op *acknowledgeOp) execute(c *Channel) {
	c.log("acknowledge %d", op.aid)

	for len(c.outgoingArrays) > 0 && c.outgoingArrays[0].index <= op.aid {
		c.outgoingArrays = c.outgoingArrays[1:]
	}

	// Make sure to release the reference to the underlying array by copying
	// the unacknowledged arrays into a new slice.
	out := make([]*outgoingArray, len(c.outgoingArrays))
	copy(out, c.outgoingArrays)
	c.outgoingArrays = out

	c.clearAckTimeout()
}

func (c *Channel) setBackChannel(bc backChannel) {
	c.opChan <- &setBackChannelOp{bc}
}

type setBackChannelOp struct {
	bChannel backChannel
}

func (op *setBackChannelOp) execute(c *Channel) {
	c.log("set back channel (chunked:%s)", op.bChannel.isChunked())

	if c.state == channelInit {
		hostPrefix := getHostPrefix(c.corsInfo)
		c.queueArray(Array{"c", c.Sid.String(), hostPrefix, 8})
		c.state = channelReady
	} else {
		c.queueArray(Array{"noop"})
	}

	if c.backChannel != nil {
		c.log("dropping back channel to set new one")
		c.clearBackChannel(false /* permanent */)
	}

	c.clearChannelTimeout()
	c.armBackChannelTimeouts()
	c.backChannel = op.bChannel

	// Special care is needed to account for the fact that the old back
	// channel may have died uncleanly. To make sure that all arrays are
	// effectively received by the client, the last sent array id is reset
	// to the last unacknowledged array id.
	if len(c.outgoingArrays) > 0 {
		c.lastSentArrayId = c.outgoingArrays[0].index - 1
	}

	c.flush()
}

func (c *Channel) flush() {
	numUnsentArrays := c.lastArrayId - c.lastSentArrayId

	if c.backChannel == nil || numUnsentArrays < 1 {
		return
	}

	next := len(c.outgoingArrays) - numUnsentArrays
	data, _ := marshalOutgoingArrays(c.outgoingArrays[next:])
	c.backChannel.send(data)
	c.lastSentArrayId = c.lastArrayId
	c.resetAckTimeout()

	// If the number of buffered outgoing arrays is greater than a given
	// threshold, force a back channel change to get acknowledgments so
	// we can free some of them later.
	if !c.canReuseBackChannel() {
		c.log("discarding back channel")
		c.clearBackChannel(false /* permanent */)
	}
}

func (c *Channel) canReuseBackChannel() bool {
	return c.backChannel.isReusable() &&
		len(c.outgoingArrays) < 100 &&
		c.state != channelClosed
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
	c.ackTimeout = time.NewTimer(ackTimeoutDelay)
}

func (c *Channel) armBackChannelTimeouts() {
	c.backChannelExpiration = time.NewTimer(backChannelExpirationTimeoutDelay)
	c.backChannelHeartbeat = time.NewTicker(backChannelHeartbeatDelay)
}

func (c *Channel) clearBackChannelTimeouts() {
	c.backChannelExpiration.Stop()
	c.backChannelExpiration = nil
	c.backChannelHeartbeat.Stop()
	c.backChannelHeartbeat = nil
}

func (c *Channel) armChannelTimeout() {
	c.channelTimeout = time.NewTimer(channelReopenTimeoutDelay)
}

func (c *Channel) clearChannelTimeout() {
	c.channelTimeout.Stop()
	c.channelTimeout = nil
}

func marshalOutgoingArrays(arrays []*outgoingArray) (data []byte, err error) {
	carrays := []interface{}{}
	for _, a := range arrays {
		carrays = append(carrays, []interface{}{a.index, a.elements})
	}
	data, err = json.Marshal(carrays)
	return
}
