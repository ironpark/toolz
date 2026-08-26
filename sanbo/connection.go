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
	if len([]byte(serverID)) > maximumRouteIDBytes {
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
		if len([]byte(connectionID)) > maximumRouteIDBytes {
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
