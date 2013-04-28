// Copyright (c) 2013 Mathieu Turcotte
// Licensed under the MIT license.

package browserchannel

import (
	"encoding/hex"
	"errors"
	"io"
)

const bytesPerSessionId = 16

type SessionId [bytesPerSessionId]byte

var (
	nullSessionId       = SessionId{}
	errInvalidSessionId = errors.New("invalid session id string")
)

// Generates a SessionId using the given reader as byte source.
func generateSesionId(source io.Reader) (sid SessionId, err error) {
	sid = SessionId{}
	_, err = io.ReadFull(source, sid[:])
	return
}

// Parses the hexadecimal string into a SessionId instance.
//
// If the hexadecimal string is empty, nullSessionId is returned without an
// error code. Otherwise, errInvalidSessionId is returned.
func parseSessionId(repr string) (sid SessionId, err error) {
	sid = nullSessionId

	if len(repr) == 0 {
		return
	}

	decoded, err := hex.DecodeString(repr)

	if err != nil || len(decoded) != bytesPerSessionId {
		err = errInvalidSessionId
		return
	}

	copy(sid[:], decoded)
	return
}

// Gets the hexadecimal representation of the SessionId.
func (s SessionId) String() string {
	return hex.EncodeToString(s[:])
}
