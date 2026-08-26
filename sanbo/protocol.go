package main

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
