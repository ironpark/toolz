package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
)

const multiNodeHelperEnvironment = "SANBO_MULTI_NODE_HELPER"

// TestMultiNodeRelayProcess is the subprocess entry point. Parent tests launch
// the current test binary so every node has an independent address space;
// process-local registries cannot satisfy the multi-node assertions.
func TestMultiNodeRelayProcess(t *testing.T) {
	if os.Getenv(multiNodeHelperEnvironment) != "1" {
		return
	}
	config, err := LoadConfigFromOS()
	if err != nil {
		t.Fatal(err)
	}
	if err := mustNewRelay(t, config).Start(); err != nil {
		t.Fatal(err)
	}
}

type multiNodeProcess struct {
	name   string
	port   int
	target string
	cmd    *exec.Cmd
	output bytes.Buffer
	once   sync.Once
}

func startMultiNodeProcess(t *testing.T, name, cluster, cookie string, minimumClusterSize int) *multiNodeProcess {
	t.Helper()
	port := reserveMultiNodePort(t)
	target := "opaque-" + name
	node := &multiNodeProcess{name: name, port: port, target: target}
	node.cmd = exec.Command(os.Args[0], "-test.run=^TestMultiNodeRelayProcess$", "-test.v")
	node.cmd.Env = append(os.Environ(),
		multiNodeHelperEnvironment+"=1",
		"PASEO_RELAY_HOST=127.0.0.1",
		"PASEO_RELAY_PORT="+strconv.Itoa(port),
		"PASEO_RELAY_OWNERSHIP_TARGET="+target,
		"PASEO_RELAY_CLUSTER_QUERY="+cluster,
		"PASEO_RELAY_MIN_CLUSTER_SIZE="+strconv.Itoa(minimumClusterSize),
		"RELEASE_NODE="+name+"@127.0.0.1",
		"RELEASE_COOKIE="+cookie,
	)
	node.cmd.Stdout = &node.output
	node.cmd.Stderr = &node.output
	if err := node.cmd.Start(); err != nil {
		t.Fatalf("start node %s: %v", name, err)
	}
	t.Cleanup(node.stop)
	if !waitScenario(func() bool {
		status, _ := node.get("/health")
		return status == http.StatusOK
	}, 3*time.Second) {
		node.stop()
		t.Fatalf("node %s did not become healthy:\n%s", name, node.output.String())
	}
	return node
}

func (n *multiNodeProcess) stop() {
	n.once.Do(func() {
		if n.cmd.Process != nil {
			_ = n.cmd.Process.Kill()
		}
		_ = n.cmd.Wait()
	})
}

func (n *multiNodeProcess) baseURL() string {
	return "http://127.0.0.1:" + strconv.Itoa(n.port)
}

func (n *multiNodeProcess) get(path string) (int, string) {
	client := &http.Client{Timeout: 200 * time.Millisecond}
	response, err := client.Get(n.baseURL() + path)
	if err != nil {
		return 0, ""
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	return response.StatusCode, string(body)
}

func (n *multiNodeProcess) dial(serverID string, role Role, version int, connectionID string) (*websocket.Conn, *http.Response, error) {
	endpoint := "ws://127.0.0.1:" + strconv.Itoa(n.port) + "/ws?" +
		relayWebSocketQuery(serverID, role, version, connectionID).Encode()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	return websocket.Dial(ctx, endpoint, nil)
}

func reserveMultiNodePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return port
}

func multiNodeClusterName(t *testing.T) string {
	replacer := strings.NewReplacer("/", "-", "_", "-")
	return "_sanbo-" + strings.ToLower(replacer.Replace(t.Name())) + "._tcp.test"
}

func requireMultiNodeOwner(t *testing.T, node *multiNodeProcess, serverID string) *websocket.Conn {
	t.Helper()
	conn, response, err := node.dial(serverID, RoleServer, 1, "")
	if err != nil {
		status := 0
		if response != nil {
			status = response.StatusCode
		}
		t.Fatalf("claim %s on %s: status=%d err=%v", serverID, node.name, status, err)
	}
	t.Cleanup(func() { _ = conn.CloseNow() })
	return conn
}

func requireMultiNodeReroute(t *testing.T, node *multiNodeProcess, serverID, target string) {
	t.Helper()
	conn, response, err := node.dial(serverID, RoleClient, 2, "remote-client")
	if conn != nil {
		_ = conn.CloseNow()
		t.Fatalf("node %s upgraded a route owned by %s", node.name, target)
	}
	if err == nil || response == nil || response.StatusCode != http.StatusConflict {
		t.Fatalf("node %s reroute response=%v err=%v", node.name, response, err)
	}
	defer response.Body.Close()
	if got := response.Header.Get("x-reroute-target"); got != target {
		t.Fatalf("node %s reroute target=%q, want %q", node.name, got, target)
	}
}

func TestMultiNodeReadinessTracksClusterJoinAndLeave(t *testing.T) {
	cluster := multiNodeClusterName(t)
	cookie := "multi-node-readiness"
	first := startMultiNodeProcess(t, "readiness-a", cluster, cookie, 2)
	if status, _ := first.get("/ready"); status != http.StatusServiceUnavailable {
		t.Fatalf("single node readiness=%d, want 503", status)
	}
	second := startMultiNodeProcess(t, "readiness-b", cluster, cookie, 2)
	if !waitScenario(func() bool {
		firstStatus, _ := first.get("/ready")
		secondStatus, _ := second.get("/ready")
		return firstStatus == http.StatusOK && secondStatus == http.StatusOK
	}, 3*time.Second) {
		firstStatus, _ := first.get("/ready")
		secondStatus, _ := second.get("/ready")
		t.Fatalf("joined readiness=%d/%d, want 200/200", firstStatus, secondStatus)
	}
	second.stop()
	if !waitScenario(func() bool {
		status, _ := first.get("/ready")
		return status == http.StatusServiceUnavailable
	}, 3*time.Second) {
		status, _ := first.get("/ready")
		t.Fatalf("readiness after peer exit=%d, want 503", status)
	}
}

func TestMultiNodeRemoteOwnerReturnsOpaqueReroute(t *testing.T) {
	cluster := multiNodeClusterName(t)
	first := startMultiNodeProcess(t, "reroute-a", cluster, "reroute-cookie", 1)
	second := startMultiNodeProcess(t, "reroute-b", cluster, "reroute-cookie", 1)
	serverID := "remote-owner"
	_ = requireMultiNodeOwner(t, first, serverID)
	requireMultiNodeReroute(t, second, serverID, first.target)
}

func TestMultiNodeConcurrentClaimsProduceOneOwner(t *testing.T) {
	cluster := multiNodeClusterName(t)
	first := startMultiNodeProcess(t, "claim-a", cluster, "claim-cookie", 1)
	second := startMultiNodeProcess(t, "claim-b", cluster, "claim-cookie", 1)
	serverID := "concurrent-owner"
	type result struct {
		node     *multiNodeProcess
		conn     *websocket.Conn
		response *http.Response
		err      error
	}
	start := make(chan struct{})
	results := make(chan result, 2)
	for _, node := range []*multiNodeProcess{first, second} {
		go func(node *multiNodeProcess) {
			<-start
			conn, response, err := node.dial(serverID, RoleServer, 1, "")
			results <- result{node: node, conn: conn, response: response, err: err}
		}(node)
	}
	close(start)
	claimed, rerouted := 0, 0
	for range 2 {
		result := <-results
		if result.conn != nil {
			claimed++
			t.Cleanup(func() { _ = result.conn.CloseNow() })
			continue
		}
		if result.err != nil && result.response != nil && result.response.StatusCode == http.StatusConflict {
			rerouted++
		}
		if result.response != nil {
			_ = result.response.Body.Close()
		}
	}
	if claimed != 1 || rerouted != 1 {
		t.Fatalf("concurrent claims: upgraded=%d rerouted=%d, want 1/1", claimed, rerouted)
	}
}

func TestMultiNodeDifferentCookiesRemainIsolated(t *testing.T) {
	cluster := multiNodeClusterName(t)
	first := startMultiNodeProcess(t, "cookie-a", cluster, "cookie-one", 1)
	second := startMultiNodeProcess(t, "cookie-b", cluster, "cookie-two", 1)
	serverID := "cookie-isolation"
	_ = requireMultiNodeOwner(t, first, serverID)
	_ = requireMultiNodeOwner(t, second, serverID)
}

func TestMultiNodeDifferentClusterQueriesRemainIsolated(t *testing.T) {
	first := startMultiNodeProcess(t, "query-a", multiNodeClusterName(t)+"-a", "query-cookie", 1)
	second := startMultiNodeProcess(t, "query-b", multiNodeClusterName(t)+"-b", "query-cookie", 1)
	serverID := "query-isolation"
	_ = requireMultiNodeOwner(t, first, serverID)
	_ = requireMultiNodeOwner(t, second, serverID)
}

func TestMultiNodeOwnerDeathAllowsRemoteReclaim(t *testing.T) {
	cluster := multiNodeClusterName(t)
	first := startMultiNodeProcess(t, "failover-a", cluster, "failover-cookie", 1)
	second := startMultiNodeProcess(t, "failover-b", cluster, "failover-cookie", 1)
	serverID := "owner-failover"
	_ = requireMultiNodeOwner(t, first, serverID)
	requireMultiNodeReroute(t, second, serverID, first.target)
	first.stop()
	if !waitScenario(func() bool {
		conn, _, err := second.dial(serverID, RoleServer, 1, "")
		if err != nil {
			return false
		}
		_ = conn.CloseNow()
		return true
	}, 3*time.Second) {
		t.Fatal("surviving node did not reclaim ownership after owner exit")
	}
}

func TestMultiNodeConflictingOwnersConvergeAndCloseLoser(t *testing.T) {
	cluster := multiNodeClusterName(t)
	first := startMultiNodeProcess(t, "heal-a", cluster, "heal-cookie", 1)
	second := startMultiNodeProcess(t, "heal-b", cluster, "heal-cookie", 1)
	serverID := "partition-heal"
	firstConn, firstResponse, firstErr := first.dial(serverID, RoleServer, 1, "")
	secondConn, secondResponse, secondErr := second.dial(serverID, RoleServer, 1, "")
	if firstConn != nil {
		defer firstConn.CloseNow()
	}
	if secondConn != nil {
		defer secondConn.CloseNow()
	}
	if firstConn == nil || secondConn == nil {
		responses := []*http.Response{firstResponse, secondResponse}
		errors := []error{firstErr, secondErr}
		reroutes := 0
		for i := range responses {
			if errors[i] != nil && responses[i] != nil && responses[i].StatusCode == http.StatusConflict {
				reroutes++
			}
		}
		if reroutes != 1 {
			t.Fatalf("initial conflict did not select one owner: %v / %v", firstErr, secondErr)
		}
		return
	}

	closed := make(chan websocket.StatusCode, 2)
	for _, conn := range []*websocket.Conn{firstConn, secondConn} {
		go func(conn *websocket.Conn) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_, _, err := conn.Read(ctx)
			closed <- websocket.CloseStatus(err)
		}(conn)
	}
	firstClose, secondClose := <-closed, <-closed
	serviceRestarts := 0
	for _, code := range []websocket.StatusCode{firstClose, secondClose} {
		if code == websocket.StatusServiceRestart {
			serviceRestarts++
		}
	}
	if serviceRestarts != 1 {
		t.Fatalf("conflicting owners did not close exactly one loser with 1012: %d/%d", firstClose, secondClose)
	}
}

func TestMultiNodeOwnershipSurgeIsVisibleAcrossThreeNodes(t *testing.T) {
	cluster := multiNodeClusterName(t)
	cookie := "surge-cookie"
	nodes := []*multiNodeProcess{
		startMultiNodeProcess(t, "surge-a", cluster, cookie, 1),
		startMultiNodeProcess(t, "surge-b", cluster, cookie, 1),
		startMultiNodeProcess(t, "surge-c", cluster, cookie, 1),
	}
	const servers = 12
	for i := 0; i < servers; i++ {
		ownerIndex := i % len(nodes)
		observerIndex := (ownerIndex + 1) % len(nodes)
		serverID := fmt.Sprintf("surge-server-%d", i)
		_ = requireMultiNodeOwner(t, nodes[ownerIndex], serverID)
		requireMultiNodeReroute(t, nodes[observerIndex], serverID, nodes[ownerIndex].target)
	}
}
