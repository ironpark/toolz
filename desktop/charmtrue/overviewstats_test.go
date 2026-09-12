package main

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestLatestInterfaceTraffic(t *testing.T) {
	for _, tc := range []struct {
		name, payload string
		available     bool
		rx, tx        float64
	}{
		{"reordered columns and negative sent", `{"identifier":"eth0","legend":["sent","time","received"],"data":[[-100,990,200],[-120,995,240],[-80,991,160]]}`, true, 240, 120},
		{"zero is a valid measurement", `{"legend":["time","received","sent"],"data":[[999,0,0]]}`, true, 0, 0},
		{"stale", `{"legend":["time","received","sent"],"data":[[950,1,2]]}`, false, 0, 0},
		{"null values", `{"legend":["time","received","sent"],"data":[[999,null,2]]}`, false, 0, 0},
		{"missing column", `{"legend":["time","received"],"data":[[999,1]]}`, false, 0, 0},
		{"short row", `{"legend":["time","received","sent"],"data":[[999]]}`, false, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var graph interfaceTrafficGraph
			if err := json.Unmarshal([]byte(tc.payload), &graph); err != nil {
				t.Fatal(err)
			}
			got := latestInterfaceTraffic(graph, 1000)
			if tc.available {
				if got.ReceivedKbps == nil || got.SentKbps == nil || *got.ReceivedKbps != tc.rx || *got.SentKbps != tc.tx {
					t.Fatalf("unexpected traffic: %+v", got)
				}
			} else if got.ReceivedKbps != nil || got.SentKbps != nil {
				t.Fatalf("unavailable data reported as traffic: %+v", got)
			}
		})
	}
}

type overviewStatsCaller struct{ t *testing.T }

func (c overviewStatsCaller) Call(_ context.Context, method string, params []any, result any) error {
	switch method {
	case "system.info":
		return json.Unmarshal([]byte(`{"uptime_seconds":123.75}`), result)
	case "network.general.summary":
		return json.Unmarshal([]byte(`{"ips":{"eth0":{"IPV4":["192.168.1.10/24"],"IPV6":["fd00::1/64"]}},"default_routes":["192.168.1.1"],"nameservers":["192.168.1.1"]}`), result)
	case "reporting.netdata_graph":
		if len(params) != 2 || params[0] != "interface" {
			c.t.Errorf("traffic parameters = %#v", params)
		}
		return errors.New("reporting access denied")
	}
	return errors.New("unexpected call")
}

func TestOverviewStatsPartialFailure(t *testing.T) {
	result := readOverviewStats(context.Background(), overviewStatsCaller{t})
	if result.UptimeSeconds == nil || *result.UptimeSeconds != 123.75 || result.UptimeError != "" || result.UptimeSampledAt < time.Now().Add(-time.Second).UnixMilli() {
		t.Fatalf("uptime = %+v", result)
	}
	if result.TrafficError == "" || len(result.Traffic) != 0 {
		t.Fatalf("traffic error missing: %+v", result)
	}
	if result.NetworkError != "" || len(result.Network.IPs["eth0"].IPv4) != 1 {
		t.Fatalf("IP information missing: %+v", result.Network)
	}
	if len(result.Network.DefaultRoutes) != 1 || len(result.Network.NameServers) != 1 {
		t.Fatalf("network configuration missing: %+v", result.Network)
	}
}
