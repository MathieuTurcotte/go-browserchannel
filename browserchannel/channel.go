// Copyright (c) 2013 Mathieu Turcotte
// Licensed under the MIT license.

package browserchannel

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
	channelDisposed
)

// Type of the data transmitted from the server to the client. The values
// stored in the array will be serialized to JSON.
type Array []interface{}

type outgoingArray struct {
	index    int
	elements Array
}

// All calls that are modifying the browser channel state are transformed into
// operations and then queued on the browser channel internal operation
// channel. Operations are executed by the browser channel goroutine. Because
// of that, there is no synchronization logic required on a per-operation
// basis since operations are all guarantied to be executed sequentially.
type operation interface {
	execute(*Channel)
}

// There is one Channel instance per connection. Each instance run in its own
// goroutine until it is being closed either on the client-side or on the
// server-side.
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

	gcChan    chan<- SessionId
	opChan    chan operation
	mapChan   chan *Map
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
		opChan:         make(chan operation, 100),
		mapChan:        make(chan *Map, 100),
	}
}

func (c *Channel) log(format string, v ...interface{}) {
	log.Printf("%s %s", c.Sid, fmt.Sprintf(format, v...))
}

func (c *Channel) start() {
	c.armChannelTimeout()

	// The channel is marked as disposed once it has been removed from the http
	// handler channel map. The opChan is closed immediately after the channel
	// is marked as disposed which unblock the select statement and shuts down
	// the channel permanently.
	for c.state != channelDisposed {
		select {
		case op, ok := <-c.opChan:
			// It is possible for operations to be queued on the channel even after it
			// has been closed since it is still present in the handler map at this
			// point. Operations attempted on a closed channel should simply be ignored
			// but still serviced in order to avoid to block the operation channel.
			// This has to be done on a per-operation basis since some operations might
			// require some cleanup to be performed.
			if ok {
				op.execute(c)
			}
		case <-timerChan(c.ackTimeout):
			c.log("ack timeout")
			c.clearBackChannel(false /* permanent */)
		case <-timerChan(c.channelTimeout):
			c.log("channel timeout")
			c.closeInternal()
		case <-timerChan(c.backChannelExpiration):
			c.log("back channel timeout")
			c.clearBackChannel(false /* permanent */)
		case <-tickerChan(c.backChannelHeartbeat):
			c.log("back channel heartbeat")
			c.queueArray(Array{"noop"})
			c.flush()
		}
	}

	c.log("shutted down")
}

// Sends an array on the channel.
func (c *Channel) SendArray(array Array) {
	c.opChan <- &sendArrayOp{array}
}

type sendArrayOp struct {
	array Array
}

func (op *sendArrayOp) execute(c *Channel) {
	c.queueArray(op.array)
	c.flush()
}

// Reads a map from the client. The call Will block until the there's a map
// ready or the channel is closed in which case the ok argument will be false.
func (c *Channel) ReadMap() (m *Map, ok bool) {
	m, ok = <-c.mapChan
	return
}

// Closes the channel. After a channel has been closed, read calls will
// return immediately and arrays sent on the client will be dropped.
func (c *Channel) Close() {
	c.opChan <- new(closeOp)
}

type closeOp int

func (op *closeOp) execute(c *Channel) {
	c.closeInternal()
}

func (c *Channel) closeInternal() {
	// The channel could have been closed due to a timeout right
	// before someone called the Close method on it.
	if c.state == channelClosed {
		return
	}

	c.clearBackChannel(true /* permanent */)
	c.state = channelClosed
	close(c.mapChan)
	c.gcChan <- c.Sid
}

func (c *Channel) dispose() {
	c.state = channelDisposed
	close(c.opChan)
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
	if c.state == channelReady {
		c.log("receive %v", op.maps)
		c.maps.enqueue(op.offset, op.maps)
		c.dequeueMaps()
	} else {
		c.log("drop %v; channel isn't ready", op.maps)
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
	c.log("set back channel [chunked:%t]", op.bChannel.isChunked())

	if c.state == channelClosed {
		// Make sure to discard the back channel otherwise the calling goroutine
		// will end up waiting forever on its completion.
		op.bChannel.discard()
	} else if c.state == channelInit {
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
