package main

import (
	"context"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/ironpark/toolz/desktop/charmtrue/internal/truenas"
)

type OverviewLiveStats struct {
	UptimeSeconds   *float64           `json:"uptimeSeconds"`
	UptimeSampledAt int64              `json:"uptimeSampledAt"`
	UptimeError     string             `json:"uptimeError"`
	Traffic         []InterfaceTraffic `json:"traffic"`
	TrafficError    string             `json:"trafficError"`
	Network         NetworkSummary     `json:"network"`
	NetworkError    string             `json:"networkError"`
}

type InterfaceTraffic struct {
	Name         string   `json:"name"`
	ReceivedKbps *float64 `json:"receivedKbps"`
	SentKbps     *float64 `json:"sentKbps"`
	SampledAt    int64    `json:"sampledAt"`
}

type interfaceTrafficGraph struct {
	Identifier string       `json:"identifier"`
	Legend     []string     `json:"legend"`
	Data       [][]*float64 `json:"data"`
}

// OverviewStats keeps failures independent so a missing reporting permission
// does not hide uptime or the addresses reported by the server.
func (s *TrueNASService) OverviewStats() (OverviewLiveStats, error) {
	client, err := s.connectedClient()
	if err != nil {
		return OverviewLiveStats{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), connectionTimeout)
	defer cancel()
	return readOverviewStats(ctx, client), nil
}

func readOverviewStats(ctx context.Context, client apiCaller) OverviewLiveStats {
	result := OverviewLiveStats{Traffic: []InterfaceTraffic{}}
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		var info struct {
			UptimeSeconds *float64 `json:"uptime_seconds"`
		}
		if err := client.Call(ctx, "system.info", nil, &info); err != nil {
			result.UptimeError = err.Error()
			return
		}
		if info.UptimeSeconds == nil || *info.UptimeSeconds < 0 {
			result.UptimeError = "가동 시간 정보가 없습니다"
			return
		}
		result.UptimeSeconds = info.UptimeSeconds
		result.UptimeSampledAt = time.Now().UnixMilli()
	}()
	go func() {
		defer wg.Done()
		var summary truenas.NetworkGeneralSummary
		if err := client.Call(ctx, "network.general.summary", nil, &summary); err != nil {
			result.NetworkError = err.Error()
			return
		}
		result.Network = NetworkSummary{IPs: make(map[string]NetworkSummaryIPInfo, len(summary.IPs)), DefaultRoutes: summary.DefaultRoutes, NameServers: summary.NameServers}
		for name, addresses := range summary.IPs {
			result.Network.IPs[name] = NetworkSummaryIPInfo{IPv4: addresses.IPv4, IPv6: addresses.IPv6}
		}
	}()
	go func() {
		defer wg.Done()
		now := time.Now().Unix()
		var graphs []interfaceTrafficGraph
		if err := client.Call(ctx, "reporting.netdata_graph", []any{"interface", map[string]any{"start": now - 60, "end": now, "aggregate": false}}, &graphs); err != nil {
			result.TrafficError = err.Error()
			return
		}
		for _, graph := range graphs {
			result.Traffic = append(result.Traffic, latestInterfaceTraffic(graph, now))
		}
		sort.Slice(result.Traffic, func(i, j int) bool { return result.Traffic[i].Name < result.Traffic[j].Name })
	}()
	wg.Wait()
	return result
}

// TrueNAS's interface graph reports kilobits/second, with time/received/sent
// columns described by legend. Missing or stale samples must not become zero.
func latestInterfaceTraffic(graph interfaceTrafficGraph, now int64) InterfaceTraffic {
	result := InterfaceTraffic{Name: graph.Identifier}
	columns := map[string]int{}
	for i, name := range graph.Legend {
		columns[name] = i
	}
	timeColumn, hasTime := columns["time"]
	rxColumn, hasRX := columns["received"]
	txColumn, hasTX := columns["sent"]
	if !hasTime || !hasRX || !hasTX {
		return result
	}
	for _, row := range graph.Data {
		if timeColumn >= len(row) || rxColumn >= len(row) || txColumn >= len(row) || row[timeColumn] == nil || row[rxColumn] == nil || row[txColumn] == nil {
			continue
		}
		timestamp := int64(*row[timeColumn])
		if timestamp < now-30 || timestamp > now+5 || timestamp <= result.SampledAt {
			continue
		}
		rx, tx := math.Abs(*row[rxColumn]), math.Abs(*row[txColumn])
		if math.IsNaN(rx) || math.IsNaN(tx) || math.IsInf(rx, 0) || math.IsInf(tx, 0) {
			continue
		}
		result.ReceivedKbps, result.SentKbps, result.SampledAt = &rx, &tx, timestamp
	}
	return result
}
