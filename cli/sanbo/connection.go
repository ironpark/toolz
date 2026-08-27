package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
)

const maximumRouteIDBytes = 256

// Role identifies the side of the relay a WebSocket belongs to.
type Role string

const (
	RoleServer Role = "server"
	RoleClient Role = "client"
)

// Connection describes the parsed /ws query contract.
type Connection struct {
	ServerID     string
	Role         Role
	Version      int
	ConnectionID string
}

// isControl reports whether the connection is a v2 server control socket, which
// carries the session roster rather than client traffic.
func (c Connection) isControl() bool {
	return c.Version == 2 && c.Role == RoleServer && c.ConnectionID == ""
}

// ParseConnectionQuery validates the public Paseo relay query parameters.
func ParseConnectionQuery(query map[string]string) (Connection, error) {
	role, ok := query["role"]
	if !ok || (role != string(RoleServer) && role != string(RoleClient)) {
		return Connection{}, fmt.Errorf("Missing or invalid role parameter")
	}

	serverID := query["serverId"]
	if serverID == "" {
		return Connection{}, fmt.Errorf("Missing serverId parameter")
	}
	if len(serverID) > maximumRouteIDBytes {
		return Connection{}, fmt.Errorf("serverId is too long")
	}

	version := 1
	if value, ok := query["v"]; ok {
		switch strings.TrimSpace(value) {
		case "", "1":
			version = 1
		case "2":
			version = 2
		default:
			return Connection{}, fmt.Errorf("Invalid v parameter (expected 1 or 2)")
		}
	}

	connectionID := ""
	if version == 2 {
		connectionID = strings.TrimSpace(query["connectionId"])
		if len(connectionID) > maximumRouteIDBytes {
			return Connection{}, fmt.Errorf("connectionId is too long")
		}
		if role == string(RoleClient) && connectionID == "" {
			var randomID [8]byte
			if _, err := rand.Read(randomID[:]); err != nil {
				return Connection{}, fmt.Errorf("generate connection id: %w", err)
			}
			connectionID = "conn_" + hex.EncodeToString(randomID[:])
		}
	}

	return Connection{
		ServerID:     serverID,
		Role:         Role(role),
		Version:      version,
		ConnectionID: connectionID,
	}, nil
}

// peerKind is the session topology a socket belongs to. Attach, routing and
// teardown each dispatch on it, so the classification lives in one place
// rather than being re-derived from Connection fields at every site.
type peerKind int

const (
	// peerV1Server and peerV1Client are the two single-socket v1 roles.
	peerV1Server peerKind = iota
	peerV1Client
	// peerControl is the v2 server socket carrying the session roster.
	peerControl
	// peerV2Client is a client on a v2 route; a route fans out to all of them.
	peerV2Client
	// peerV2Data is the daemon-side socket serving one v2 route.
	peerV2Data
)

// kind classifies the connection into its session topology.
func (c Connection) kind() peerKind {
	if c.Version == 1 {
		if c.Role == RoleServer {
			return peerV1Server
		}
		return peerV1Client
	}
	if c.isControl() {
		return peerControl
	}
	if c.Role == RoleClient {
		return peerV2Client
	}
	return peerV2Data
}
