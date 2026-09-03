package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/coder/websocket"
)

func TestTrueNASServiceConnect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("websocket.Accept() error = %v", err)
			return
		}
		defer conn.CloseNow()

		auth := readServiceRequest(t, conn)
		if auth.Method != "auth.login_ex" {
			t.Errorf("authentication method = %q", auth.Method)
		}
		if len(auth.Params) != 1 {
			t.Fatalf("authentication params = %#v", auth.Params)
		}
		login, ok := auth.Params[0].(map[string]any)
		if !ok || login["mechanism"] != "PASSWORD_PLAIN" || login["username"] != "admin" || login["password"] != "test-password" {
			t.Errorf("authentication params = %#v", auth.Params)
		}
		writeServiceResponse(t, conn, auth.ID, map[string]any{"response_type": "SUCCESS", "user_info": nil})

		info := readServiceRequest(t, conn)
		if info.Method != "system.info" {
			t.Errorf("information method = %q", info.Method)
		}
		writeServiceResponse(t, conn, info.ID, map[string]any{
			"hostname":       "vault",
			"version":        "TrueNAS-25.10.5",
			"model":          "Test CPU",
			"cores":          8,
			"physmem":        16 * 1024 * 1024 * 1024,
			"uptime":         "2 days",
			"uptime_seconds": 172800,
		})

		for range 7 {
			req := readServiceRequest(t, conn)
			switch req.Method {
			case "pool.query":
				writeServiceResponse(t, conn, req.ID, []any{map[string]any{"id": 1, "name": "tank", "status": "ONLINE", "healthy": true, "size": 1000, "allocated": 400, "free": 600}})
			case "disk.query":
				writeServiceResponse(t, conn, req.ID, []any{map[string]any{"name": "sda", "identifier": "disk-1", "model": "Test Disk", "serial": "ABC", "type": "SSD", "size": 1000, "pool": "tank"}})
			case "pool.dataset.query":
				writeServiceResponse(t, conn, req.ID, []any{map[string]any{"id": "tank/data", "name": "tank/data", "pool": "tank", "type": "FILESYSTEM", "mountpoint": "/mnt/tank/data", "used": map[string]any{"parsed": 400}, "available": map[string]any{"parsed": 600}}})
			case "pool.snapshot.query":
				writeServiceResponse(t, conn, req.ID, 2)
			case "sharing.smb.query":
				writeServiceResponse(t, conn, req.ID, []any{map[string]any{
					"id": 10, "name": "media", "path": "/mnt/tank/media", "purpose": "DEFAULT_SHARE",
					"comment": "Media library", "enabled": true, "readonly": true, "browsable": true,
					"audit":   map[string]any{"enable": true, "watch_list": []string{"editors"}, "ignore_list": []string{}},
					"options": map[string]any{"purpose": "DEFAULT_SHARE", "aapl_name_mangling": true, "hostsallow": []string{"10.0.0.10"}, "hostsdeny": []string{}},
				}})
			case "sharing.nfs.query":
				writeServiceResponse(t, conn, req.ID, []any{map[string]any{
					"id": 11, "paths": []string{"/mnt/tank/data"}, "enabled": true, "ro": true,
					"networks": []string{"10.0.0.0/24"}, "security": []string{"SYS"}, "maproot_user": "root",
				}})
			case "rsynctask.query":
				writeServiceResponse(t, conn, req.ID, []any{map[string]any{
					"id": 12, "path": "/mnt/tank/data", "user": "backup", "mode": "MODULE",
					"remotehost": "backup", "remoteport": 873, "remotemodule": "archive", "direction": "PUSH",
					"desc": "Nightly", "schedule": map[string]any{"minute": "15", "hour": "2", "dom": "*", "month": "*", "dow": "*"},
					"recursive": true, "enabled": true,
				}})
			default:
				t.Errorf("unexpected method %q", req.Method)
			}
		}
	}))
	defer server.Close()

	service := &TrueNASService{profilesPath: t.TempDir() + "/servers.json"}
	connection, err := service.Connect(strings.Replace(server.URL, "http://", "ws://", 1), "admin", "test-password", "password", false, true, false)
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	if !connection.Connected || connection.System.Hostname != "vault" {
		t.Fatalf("Connect() = %#v", connection)
	}
	if connection.System.UptimeSeconds != 172800 {
		t.Fatalf("Connect().System.UptimeSeconds = %v", connection.System.UptimeSeconds)
	}
	if service.AppInfo().Status != "Connected" {
		t.Fatalf("AppInfo().Status = %q", service.AppInfo().Status)
	}
	if !service.CurrentConnection().Connected {
		t.Fatal("CurrentConnection().Connected = false")
	}
	savedServers, err := service.SavedServers()
	if err != nil {
		t.Fatalf("SavedServers() error = %v", err)
	}
	if len(savedServers) != 1 || savedServers[0].Name != "vault" || savedServers[0].Username != "admin" || savedServers[0].AuthenticationMethod != "password" {
		t.Fatalf("SavedServers() = %#v", savedServers)
	}
	storage, err := service.StorageOverview()
	if err != nil {
		t.Fatalf("StorageOverview() error = %v", err)
	}
	if len(storage.Pools) != 1 || storage.Pools[0].Name != "tank" || len(storage.Disks) != 1 || len(storage.Datasets) != 1 || storage.SnapshotCount != 2 {
		t.Fatalf("StorageOverview() = %#v", storage)
	}
	if storage.TotalSize != 1000 || storage.TotalAllocated != 400 || storage.TotalFree != 600 {
		t.Fatalf("StorageOverview() capacity = (%d, %d, %d)", storage.TotalSize, storage.TotalAllocated, storage.TotalFree)
	}
	sharing, err := service.SharingOverview()
	if err != nil {
		t.Fatalf("SharingOverview() error = %v", err)
	}
	if sharing.SMBCount != 1 || sharing.NFSCount != 1 || len(sharing.Shares) != 2 || len(sharing.RsyncTasks) != 1 {
		t.Fatalf("SharingOverview() = %#v", sharing)
	}
	if !sharing.Shares[0].ReadOnly || sharing.Shares[0].Purpose != "DEFAULT_SHARE" || !sharing.Shares[0].AuditEnabled || !sharing.Shares[0].AAPLNameMangling || len(sharing.Shares[0].HostsAllow) != 1 {
		t.Fatalf("SharingOverview().Shares[0] = %#v", sharing.Shares[0])
	}
	if sharing.Shares[1].Path != "/mnt/tank/data" || sharing.Shares[1].MapRootUser != "root" || len(sharing.Shares[1].Security) != 1 {
		t.Fatalf("SharingOverview().Shares[1] = %#v", sharing.Shares[1])
	}
	if sharing.RsyncTasks[0].Destination != "backup:archive" || sharing.RsyncTasks[0].RemotePort != 873 || sharing.RsyncTasks[0].ScheduleMinute != "15" {
		t.Fatalf("SharingOverview().RsyncTasks[0] = %#v", sharing.RsyncTasks[0])
	}
	if err := service.Disconnect(); err != nil {
		t.Fatalf("Disconnect() error = %v", err)
	}
}

func TestSummarizePoolCapacityDeduplicatesPoolGUID(t *testing.T) {
	size, allocated, free := summarizePoolCapacity([]StoragePool{
		{ID: 1, Name: "tank", GUID: "zfs-123", Size: 1000, Allocated: 400, Free: 600},
		{ID: 2, Name: "tank-alias", GUID: "zfs-123", Size: 1000, Allocated: 400, Free: 600},
		{ID: 3, Name: "backup", GUID: "zfs-456", Size: 2000, Allocated: 500, Free: 1500},
	})
	if size != 3000 || allocated != 900 || free != 2100 {
		t.Fatalf("summarizePoolCapacity() = (%d, %d, %d), want (3000, 900, 2100)", size, allocated, free)
	}
}

func TestIdentityMutations(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.CloseNow()
		auth := readServiceRequest(t, conn)
		writeServiceResponse(t, conn, auth.ID, map[string]any{"response_type": "SUCCESS"})
		info := readServiceRequest(t, conn)
		writeServiceResponse(t, conn, info.ID, map[string]any{"hostname": "vault"})
		for _, expected := range []string{"user.create", "user.update", "group.create", "group.update", "api_key.create", "api_key.update"} {
			req := readServiceRequest(t, conn)
			if req.Method != expected {
				t.Errorf("method = %q, want %q", req.Method, expected)
			}
			if len(req.Params) == 0 {
				t.Errorf("%s params are empty", req.Method)
			}
			if expected == "user.create" {
				data := req.Params[0].(map[string]any)
				if data["username"] != "alex" || data["group_create"] != true || data["smb"] != true ||
					data["random_password"] != true || data["home"] != "/mnt/tank/home/alex" ||
					data["home_create"] != true || data["home_mode"] != "750" ||
					data["ssh_password_enabled"] != true || data["userns_idmap"] != "DIRECT" {
					t.Errorf("user.create params = %#v", data)
				}
			}
			if expected == "user.update" {
				data := req.Params[1].(map[string]any)
				if data["group"] != float64(4) || data["shell"] != "/usr/bin/zsh" || data["email"] != "alex@example.com" {
					t.Errorf("user.update params = %#v", data)
				}
			}
			result := any(nil)
			if expected == "user.create" {
				result = map[string]any{"id": 7, "password": "generated-password"}
			}
			if expected == "user.update" {
				result = map[string]any{"id": 7, "password": nil}
			}
			if expected == "api_key.create" {
				result = map[string]any{"id": 9, "key": "secret-once"}
			}
			if expected == "api_key.update" {
				result = map[string]any{"id": 9, "key": "reset-once"}
			}
			writeServiceResponse(t, conn, req.ID, result)
		}
		// Keep the mock endpoint alive until the service closes the session.
		_, _, _ = conn.Read(context.Background())
	}))
	defer server.Close()
	service := &TrueNASService{profilesPath: t.TempDir() + "/servers.json"}
	if _, err := service.Connect(strings.Replace(server.URL, "http://", "ws://", 1), "admin", "password", "password", false, false, false); err != nil {
		t.Fatal(err)
	}
	createdUser, err := service.SaveUser(UserMutation{Username: " alex ", FullName: "Alex", RandomPassword: true, SMB: true, GroupCreate: true, Home: "/mnt/tank/home/alex", HomeCreate: true, HomeMode: "750", Shell: "/usr/bin/zsh", SSHPasswordEnabled: true, UserNSIDMap: "DIRECT", Groups: []int{4}, SudoCommands: []string{" /usr/bin/ls "}})
	if err != nil {
		t.Fatal(err)
	}
	if createdUser.ID != 7 || createdUser.Password != "generated-password" {
		t.Fatalf("SaveUser(create) = %#v", createdUser)
	}
	if _, err := service.SaveUser(UserMutation{ID: 3, Username: "alex", FullName: "Alex Kim", Email: "alex@example.com", Home: "/mnt/tank/home/alex", Shell: "/usr/bin/zsh", PrimaryGroupID: 4, SMB: true}); err != nil {
		t.Fatal(err)
	}
	if err := service.SaveGroup(GroupMutation{Name: "editors", SMB: true}); err != nil {
		t.Fatal(err)
	}
	if err := service.SaveGroup(GroupMutation{ID: 4, Name: "writers"}); err != nil {
		t.Fatal(err)
	}
	created, err := service.SaveAPIKey(APIKeyMutation{Name: "automation", Username: "alex"})
	if err != nil || created.Key != "secret-once" {
		t.Fatalf("SaveAPIKey(create) = %#v, %v", created, err)
	}
	updated, err := service.SaveAPIKey(APIKeyMutation{ID: 9, Name: "automation", Reset: true})
	if err != nil || updated.Key != "reset-once" {
		t.Fatalf("SaveAPIKey(update) = %#v, %v", updated, err)
	}
	if err := service.Disconnect(); err != nil {
		t.Fatal(err)
	}
}

func TestSharingMutations(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.CloseNow()
		auth := readServiceRequest(t, conn)
		writeServiceResponse(t, conn, auth.ID, map[string]any{"response_type": "SUCCESS"})
		info := readServiceRequest(t, conn)
		writeServiceResponse(t, conn, info.ID, map[string]any{"hostname": "vault"})

		for _, expected := range []string{"sharing.smb.update", "sharing.nfs.update", "rsynctask.update"} {
			req := readServiceRequest(t, conn)
			if req.Method != expected {
				t.Errorf("method = %q, want %q", req.Method, expected)
			}
			if len(req.Params) != 2 {
				t.Fatalf("%s params = %#v", req.Method, req.Params)
			}
			data, ok := req.Params[1].(map[string]any)
			if !ok {
				t.Fatalf("%s data = %#v", req.Method, req.Params[1])
			}
			switch expected {
			case "sharing.smb.update":
				if req.Params[0] != float64(10) || data["name"] != "media" || data["path"] != "/mnt/tank/media" || data["readonly"] != true {
					t.Errorf("sharing.smb.update params = %#v", req.Params)
				}
				options, _ := data["options"].(map[string]any)
				hosts, _ := options["hostsallow"].([]any)
				if len(hosts) != 1 || hosts[0] != "10.0.0.10" {
					t.Errorf("sharing.smb.update options = %#v", options)
				}
				if options["purpose"] != "DEFAULT_SHARE" || options["aapl_name_mangling"] != true {
					t.Errorf("sharing.smb.update options = %#v", options)
				}
				audit, _ := data["audit"].(map[string]any)
				if audit["enable"] != true {
					t.Errorf("sharing.smb.update audit = %#v", audit)
				}
			case "sharing.nfs.update":
				if req.Params[0] != float64(11) || data["path"] != "/mnt/tank/data" || data["maproot_user"] != "root" || data["ro"] != true {
					t.Errorf("sharing.nfs.update params = %#v", req.Params)
				}
				if _, legacy := data["paths"]; legacy {
					t.Error("sharing.nfs.update unexpectedly sent legacy paths field")
				}
			case "rsynctask.update":
				if req.Params[0] != float64(12) || data["mode"] != "SSH" || data["direction"] != "PUSH" || data["remoteport"] != float64(22) || data["ssh_credentials"] != float64(5) {
					t.Errorf("rsynctask.update params = %#v", req.Params)
				}
				schedule, _ := data["schedule"].(map[string]any)
				if schedule["minute"] != "15" || schedule["hour"] != "2" || schedule["dow"] != "1-5" {
					t.Errorf("rsynctask.update schedule = %#v", schedule)
				}
			}
			writeServiceResponse(t, conn, req.ID, nil)
		}
		_, _, _ = conn.Read(context.Background())
	}))
	defer server.Close()

	service := &TrueNASService{profilesPath: t.TempDir() + "/servers.json"}
	if _, err := service.Connect(strings.Replace(server.URL, "http://", "ws://", 1), "admin", "password", "password", false, false, false); err != nil {
		t.Fatal(err)
	}
	if err := service.SaveShare(ShareMutation{
		ID: 10, Protocol: "smb", Name: " media ", Path: "/mnt/tank/media", Purpose: "DEFAULT_SHARE",
		Enabled: true, ReadOnly: true, Browsable: true, AuditEnabled: true, AAPLNameMangling: true,
		HostsAllow: []string{" 10.0.0.10 ", ""},
	}); err != nil {
		t.Fatal(err)
	}
	if err := service.SaveShare(ShareMutation{
		ID: 11, Protocol: "NFS", Path: "/mnt/tank/data", ReadOnly: true, MapRootUser: "root",
		Networks: []string{"10.0.0.0/24"}, Security: []string{"SYS", "KRB5"}, Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := service.SaveRsyncTask(RsyncTaskMutation{
		ID: 12, Path: "/mnt/tank/data", User: "backup", Mode: "ssh", RemoteHost: "backup.local",
		RemotePort: 22, RemotePath: "/archive", SSHCredentialID: 5, Direction: "push", Description: "Nightly",
		ScheduleMinute: "15", ScheduleHour: "2", ScheduleDayOfMonth: "*", ScheduleMonth: "*", ScheduleDayOfWeek: "1-5",
		Recursive: true, Times: true, PreservePermissions: true, Enabled: true, ValidateRemotePath: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := service.Disconnect(); err != nil {
		t.Fatal(err)
	}
}

func TestSharingCreateACLAndRsyncDelete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.CloseNow()
		auth := readServiceRequest(t, conn)
		writeServiceResponse(t, conn, auth.ID, map[string]any{"response_type": "SUCCESS"})
		info := readServiceRequest(t, conn)
		writeServiceResponse(t, conn, info.ID, map[string]any{"hostname": "vault"})

		for _, expected := range []string{
			"sharing.smb.share_precheck", "sharing.smb.create", "sharing.nfs.create",
			"rsynctask.create", "sharing.smb.getacl", "sharing.smb.setacl", "rsynctask.delete",
		} {
			req := readServiceRequest(t, conn)
			if req.Method != expected {
				t.Errorf("method = %q, want %q", req.Method, expected)
			}
			switch expected {
			case "sharing.smb.share_precheck":
				if len(req.Params) != 1 || req.Params[0].(map[string]any)["name"] != "archive" {
					t.Errorf("sharing.smb.share_precheck params = %#v", req.Params)
				}
			case "sharing.smb.create":
				if len(req.Params) != 1 {
					t.Fatalf("sharing.smb.create params = %#v", req.Params)
				}
				data := req.Params[0].(map[string]any)
				options := data["options"].(map[string]any)
				remotePaths := options["remote_path"].([]any)
				if data["path"] != "EXTERNAL" || data["purpose"] != "EXTERNAL_SHARE" || options["purpose"] != "EXTERNAL_SHARE" || len(remotePaths) != 1 || remotePaths[0] != `server.example.com\archive` {
					t.Errorf("sharing.smb.create data = %#v", data)
				}
			case "sharing.nfs.create":
				data := req.Params[0].(map[string]any)
				aliases := data["aliases"].([]any)
				if data["path"] != "/mnt/tank/export" || len(aliases) != 1 || aliases[0] != "/export" {
					t.Errorf("sharing.nfs.create data = %#v", data)
				}
			case "rsynctask.create":
				if len(req.Params) != 1 || req.Params[0].(map[string]any)["remotemodule"] != "backup" {
					t.Errorf("rsynctask.create params = %#v", req.Params)
				}
			case "sharing.smb.getacl":
				writeServiceResponse(t, conn, req.ID, map[string]any{
					"share_name": "archive",
					"share_acl": []any{map[string]any{
						"ae_perm": "FULL", "ae_type": "ALLOWED", "ae_who_sid": "S-1-1-0",
						"ae_who_id": nil, "ae_who_str": "Everyone",
					}},
				})
				continue
			case "sharing.smb.setacl":
				data := req.Params[0].(map[string]any)
				entries := data["share_acl"].([]any)
				entry := entries[0].(map[string]any)
				if data["share_name"] != "archive" || entry["ae_perm"] != "CHANGE" || entry["ae_who_str"] != "editors" {
					t.Errorf("sharing.smb.setacl data = %#v", data)
				}
			case "rsynctask.delete":
				if len(req.Params) != 1 || req.Params[0] != float64(9) {
					t.Errorf("rsynctask.delete params = %#v", req.Params)
				}
			}
			writeServiceResponse(t, conn, req.ID, nil)
		}
		_, _, _ = conn.Read(context.Background())
	}))
	defer server.Close()

	service := &TrueNASService{profilesPath: t.TempDir() + "/servers.json"}
	if _, err := service.Connect(strings.Replace(server.URL, "http://", "ws://", 1), "admin", "password", "password", false, false, false); err != nil {
		t.Fatal(err)
	}
	if err := service.SaveShare(ShareMutation{Protocol: "SMB", Name: "archive", Path: "EXTERNAL", Purpose: "EXTERNAL_SHARE", RemotePath: []string{`server.example.com\archive`}, Enabled: true, Browsable: true}); err != nil {
		t.Fatal(err)
	}
	if err := service.SaveShare(ShareMutation{Protocol: "NFS", Path: "/mnt/tank/export", Aliases: []string{"/export"}, Security: []string{"SYS"}, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := service.SaveRsyncTask(RsyncTaskMutation{Path: "/mnt/tank/data", User: "backup", Mode: "MODULE", RemoteHost: "backup.local", RemoteModule: "backup", Direction: "PUSH", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	acl, err := service.GetSMBShareACL("archive")
	if err != nil || acl.ShareName != "archive" || len(acl.Entries) != 1 || acl.Entries[0].SID != "S-1-1-0" {
		t.Fatalf("GetSMBShareACL() = %#v, %v", acl, err)
	}
	if err := service.SaveSMBShareACL(SMBShareACL{ShareName: "archive", Entries: []SMBShareACLEntry{{Permission: "CHANGE", EntryType: "ALLOWED", Name: "editors"}}}); err != nil {
		t.Fatal(err)
	}
	if err := service.DeleteRsyncTask(9); err != nil {
		t.Fatal(err)
	}
	if err := service.Disconnect(); err != nil {
		t.Fatal(err)
	}
}

func TestSMBShareOptionsPurposeConstraints(t *testing.T) {
	fcp, err := smbShareOptions(ShareMutation{Purpose: "FCP_SHARE", AAPLNameMangling: false})
	if err != nil {
		t.Fatal(err)
	}
	if fcp["aapl_name_mangling"] != true {
		t.Fatalf("FCP aapl_name_mangling = %#v, want true", fcp["aapl_name_mangling"])
	}
	external, err := smbShareOptions(ShareMutation{Purpose: "EXTERNAL_SHARE", RemotePath: []string{" server-a.example.com\\archive ", "server-b.example.com\\archive"}})
	if err != nil {
		t.Fatal(err)
	}
	paths, ok := external["remote_path"].([]string)
	if !ok || len(paths) != 2 || paths[0] != "server-a.example.com\\archive" {
		t.Fatalf("EXTERNAL remote_path = %#v", external["remote_path"])
	}
	if _, err := smbShareOptions(ShareMutation{Purpose: "TIME_LOCKED_SHARE", GracePeriod: 59}); err == nil {
		t.Fatal("TIME_LOCKED_SHARE accepted grace period below schema minimum")
	}
}

func TestNetworkManagement(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.CloseNow()
		auth := readServiceRequest(t, conn)
		writeServiceResponse(t, conn, auth.ID, map[string]any{"response_type": "SUCCESS"})
		info := readServiceRequest(t, conn)
		writeServiceResponse(t, conn, info.ID, map[string]any{"hostname": "vault"})

		for _, expected := range []string{"interface.query", "staticroute.query", "network.configuration.config", "network.general.summary", "interface.has_pending_changes", "interface.checkin_waiting"} {
			req := readServiceRequest(t, conn)
			if req.Method != expected {
				t.Errorf("method = %q, want %q", req.Method, expected)
			}
			var result any
			switch expected {
			case "interface.query":
				result = []any{map[string]any{
					"id": "enp1s0", "name": "enp1s0", "type": "PHYSICAL", "description": "LAN",
					"ipv4_dhcp": false, "ipv6_auto": true, "mtu": 1500,
					"aliases": []any{map[string]any{"type": "INET", "address": "192.168.1.10", "netmask": 24}},
					"state":   map[string]any{"link_state": "LINK_STATE_UP", "mtu": 1500, "active_media_type": "Ethernet", "active_media_subtype": "1000baseT", "hardware_link_address": "00:11:22:33:44:55"},
				}}
			case "staticroute.query":
				result = []any{map[string]any{"id": 3, "destination": "10.20.0.0/16", "gateway": "192.168.1.1", "description": "Office"}}
			case "network.configuration.config":
				result = map[string]any{"id": 1, "hostname": "vault", "domain": "lan", "ipv4gateway": "192.168.1.1", "nameserver1": "1.1.1.1", "nameserver2": "", "nameserver3": "", "service_announcement": map[string]any{"mdns": true}}
			case "network.general.summary":
				result = map[string]any{"ips": map[string]any{"enp1s0": map[string]any{"IPV4": []string{"192.168.1.10/24"}, "IPV6": []string{}}}, "default_routes": []string{"192.168.1.1"}, "nameservers": []string{"1.1.1.1"}}
			case "interface.has_pending_changes":
				result = true
			case "interface.checkin_waiting":
				result = 45
			}
			writeServiceResponse(t, conn, req.ID, result)
		}

		for _, expected := range []string{"network.configuration.update", "interface.update", "interface.create", "interface.commit", "interface.checkin", "interface.rollback", "interface.delete", "staticroute.create", "staticroute.update", "staticroute.delete"} {
			req := readServiceRequest(t, conn)
			if req.Method != expected {
				t.Errorf("method = %q, want %q", req.Method, expected)
			}
			switch expected {
			case "network.configuration.update":
				data := req.Params[0].(map[string]any)
				if data["hostname"] != "vault" || data["nameserver1"] != "1.1.1.1" || data["nameserver2"] != "9.9.9.9" {
					t.Errorf("network.configuration.update params = %#v", req.Params)
				}
			case "interface.update":
				data := req.Params[1].(map[string]any)
				if req.Params[0] != "enp1s0" || data["ipv4_dhcp"] != false || data["mtu"] != float64(9000) {
					t.Errorf("interface.update params = %#v", req.Params)
				}
			case "interface.create":
				data := req.Params[0].(map[string]any)
				if data["type"] != "BRIDGE" || data["name"] != "br0" {
					t.Errorf("interface.create params = %#v", req.Params)
				}
			case "interface.commit":
				options := req.Params[0].(map[string]any)
				if options["rollback"] != true || options["checkin_timeout"] != float64(60) {
					t.Errorf("interface.commit params = %#v", req.Params)
				}
			}
			writeServiceResponse(t, conn, req.ID, nil)
		}
		_, _, _ = conn.Read(context.Background())
	}))
	defer server.Close()

	service := &TrueNASService{profilesPath: t.TempDir() + "/servers.json"}
	if _, err := service.Connect(strings.Replace(server.URL, "http://", "ws://", 1), "admin", "password", "password", false, false, false); err != nil {
		t.Fatal(err)
	}
	overview, err := service.NetworkOverview()
	if err != nil {
		t.Fatal(err)
	}
	if len(overview.Interfaces) != 1 || overview.Interfaces[0].Aliases[0].Netmask != 24 || overview.Configuration.NameServers[0] != "1.1.1.1" || !overview.PendingChanges || overview.CheckinRemaining != 45 {
		t.Fatalf("NetworkOverview() = %#v", overview)
	}
	if err := service.SaveNetworkConfiguration(NetworkConfigurationMutation{Hostname: "vault", Domain: "lan", IPv4Gateway: "192.168.1.1", NameServers: []string{"1.1.1.1", "9.9.9.9"}, AnnounceMDNS: true}); err != nil {
		t.Fatal(err)
	}
	if err := service.SaveNetworkInterface(NetworkInterfaceMutation{ID: "enp1s0", Description: "LAN", MTU: 9000, IPv6Auto: true, Aliases: []NetworkAliasMutation{{Type: "INET", Address: "192.168.1.10", Netmask: 24}}}); err != nil {
		t.Fatal(err)
	}
	if err := service.SaveNetworkInterface(NetworkInterfaceMutation{Name: "br0", Type: "BRIDGE", MTU: 1500, BridgeMembers: []string{"enp2s0"}, EnableLearning: true}); err != nil {
		t.Fatal(err)
	}
	if err := service.CommitNetworkChanges(60); err != nil {
		t.Fatal(err)
	}
	if err := service.CheckinNetworkChanges(); err != nil {
		t.Fatal(err)
	}
	if err := service.RollbackNetworkChanges(); err != nil {
		t.Fatal(err)
	}
	if err := service.DeleteNetworkInterface("vlan10"); err != nil {
		t.Fatal(err)
	}
	if err := service.SaveStaticRoute(StaticRouteMutation{Destination: "10.30.0.0/16", Gateway: "192.168.1.1", Description: "Branch"}); err != nil {
		t.Fatal(err)
	}
	if err := service.SaveStaticRoute(StaticRouteMutation{ID: 3, Destination: "10.20.0.0/16", Gateway: "192.168.1.254", Description: "Office"}); err != nil {
		t.Fatal(err)
	}
	if err := service.DeleteStaticRoute(3); err != nil {
		t.Fatal(err)
	}
	if err := service.Disconnect(); err != nil {
		t.Fatal(err)
	}
}

func TestParseUserNSIDMap(t *testing.T) {
	for _, test := range []struct {
		input string
		want  any
	}{
		{"", nil},
		{"direct", "DIRECT"},
		{"65536", uint64(65536)},
	} {
		got, err := parseUserNSIDMap(test.input)
		if err != nil || got != test.want {
			t.Fatalf("parseUserNSIDMap(%q) = %#v, %v; want %#v", test.input, got, err, test.want)
		}
	}
	if _, err := parseUserNSIDMap("not-a-number"); err == nil {
		t.Fatal("parseUserNSIDMap(invalid) returned nil error")
	}
}

type serviceRequest struct {
	ID     uint64 `json:"id"`
	Method string `json:"method"`
	Params []any  `json:"params"`
}

func readServiceRequest(t *testing.T, conn *websocket.Conn) serviceRequest {
	t.Helper()
	_, payload, err := conn.Read(context.Background())
	if err != nil {
		t.Fatalf("conn.Read() error = %v", err)
	}
	var request serviceRequest
	if err := json.Unmarshal(payload, &request); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	return request
}

func writeServiceResponse(t *testing.T, conn *websocket.Conn, id uint64, result any) {
	t.Helper()
	payload, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := conn.Write(context.Background(), websocket.MessageText, payload); err != nil {
		t.Fatalf("conn.Write() error = %v", err)
	}
}
