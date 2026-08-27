package main

import (
	"encoding/json"
	"strconv"
	"time"
)

const (
	// MaximumFrameWireBytes is the compatible masked WebSocket frame ceiling.
	MaximumFrameWireBytes = 32 * 1024 * 1024
	// MaximumClientFrameHeaderBytes is the largest client-to-server frame header.
	MaximumClientFrameHeaderBytes = 14
	// MaximumMessagePayloadBytes leaves room for the largest legal client frame header.
	MaximumMessagePayloadBytes = MaximumFrameWireBytes - MaximumClientFrameHeaderBytes
	// MaximumControlPayloadBytes is the v2 control socket message ceiling.
	MaximumControlPayloadBytes = 64 * 1024
)

// The control-frame constructors below build their JSON by concatenation
// rather than json.Marshal: the reference emits these fields in this order
// with no escaping, and the parity tests compare the bytes.

// connectedFrame announces a client joining a v2 route to the control socket.
func connectedFrame(connectionID string) []byte {
	return []byte(`{"type":"connected","connectionId":"` + connectionID + `"}`)
}

// disconnectedFrame announces a v2 route losing its last client.
func disconnectedFrame(connectionID string) []byte {
	return []byte(`{"type":"disconnected","connectionId":"` + connectionID + `"}`)
}

// pongFrame answers a control-socket ping, stamped with the send time.
func pongFrame(at time.Time) []byte {
	return []byte(`{"type":"pong","ts":` + strconv.FormatInt(at.UnixMilli(), 10) + `}`)
}

// syncFrame publishes the client roster of a session to its control socket.
func syncFrame(ids []string) []byte {
	// An absent roster encodes as an empty list, never null: the reference
	// always sends a JSON array.
	if ids == nil {
		ids = []string{}
	}
	b, _ := json.Marshal(struct {
		Type          string   `json:"type"`
		ConnectionIDs []string `json:"connectionIds"`
	}{Type: "sync", ConnectionIDs: ids})
	return b
}

// controlPing reports whether a control-socket text frame is a ping.
func controlPing(payload []byte) bool {
	var frame struct {
		Type string `json:"type"`
	}
	return json.Unmarshal(payload, &frame) == nil && frame.Type == "ping"
}

// handshakeFrame is the subset of a client frame handshake validation reads.
type handshakeFrame struct {
	Type string          `json:"type"`
	Key  json.RawMessage `json:"key"`
}

// payloadLimit is the largest inbound payload this connection may send. The
// control socket carries roster notifications rather than user traffic and is
// held to a much smaller ceiling.
func payloadLimit(c Connection) int {
	if c.isControl() {
		return MaximumControlPayloadBytes
	}
	return MaximumMessagePayloadBytes
}

// readLimit is the websocket read limit for a connection: one byte past its
// payload ceiling for the control socket, so an oversized frame is read and
// answered with a close rather than dropped by the transport.
func readLimit(c Connection) int64 {
	if c.isControl() {
		return MaximumControlPayloadBytes + 1
	}
	return MaximumFrameWireBytes
}
