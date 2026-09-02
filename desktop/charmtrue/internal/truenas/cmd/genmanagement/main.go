package main

import (
	"bufio"
	"flag"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var row = regexp.MustCompile(`^\| \[\x60([^\x60]+)\x60\]`)

type method struct {
	Name, Namespace, Receiver, Leaf, Kind string
	Destructive                           bool
}
type spec struct {
	domain, doc string
	want        int
	namespaces  []mapping
}
type mapping struct{ prefix, receiver string }

func main() {
	root := flag.String("root", "", "docs root")
	out := flag.String("out", ".", "output dir")
	flag.Parse()
	specs := []spec{
		{"system", "system.md", 102, []mapping{{"system.security.info.", "SystemSecurityInfoService"}, {"boot.environment.", "BootEnvironmentService"}, {"initshutdownscript.", "InitShutdownScriptService"}, {"system.advanced.", "SystemAdvancedService"}, {"system.general.", "SystemGeneralService"}, {"system.ntpserver.", "NTPServerService"}, {"system.reboot.", "SystemRebootService"}, {"system.security.", "SystemSecurityService"}, {"system.global.", "SystemGlobalService"}, {"systemdataset.", "SystemDatasetService"}, {"cronjob.", "CronJobService"}, {"service.", "DaemonService"}, {"tunable.", "TunableService"}, {"update.", "UpdateService"}, {"config.", "ConfigService"}, {"boot.", "BootService"}, {"system.", "SystemCoreService"}}},
		{"identity", "identity.md", 49, []mapping{{"auth.twofactor.", "TwoFactorService"}, {"api_key.", "APIKeyService"}, {"privilege.", "PrivilegeService"}, {"group.", "GroupService"}, {"user.", "UserService"}, {"auth.", "AuthService"}}}}
	for _, s := range specs {
		generate(filepath.Join(*root, "domains", s.doc), *out, s)
	}
}
func generate(doc, out string, s spec) {
	f, e := os.Open(doc)
	must(e)
	defer f.Close()
	var ms []method
	scan := bufio.NewScanner(f)
	for scan.Scan() {
		x := row.FindStringSubmatch(scan.Text())
		if x == nil || x[1] == "system.reboot" {
			continue
		}
		ns, recv, leaf := classify(x[1], s.namespaces)
		k := kind(leaf)
		ms = append(ms, method{x[1], ns, recv, leaf, k, k == "destructive"})
	}
	must(scan.Err())
	if len(ms) != s.want {
		panic(fmt.Sprintf("%s: got %d methods, want %d", s.domain, len(ms), s.want))
	}
	sort.Slice(ms, func(i, j int) bool { return ms[i].Name < ms[j].Name })
	var b strings.Builder
	fmt.Fprintf(&b, "// Code generated from TrueNAS API v25.10.5; DO NOT EDIT.\npackage truenas\nimport(\"context\";\"encoding/json\")\ntype %sMethod struct{Name,Service,Kind string;Destructive bool}\nvar %sMethods=[...]%sMethod{\n", title(s.domain), title(s.domain), title(s.domain))
	for _, m := range ms {
		fmt.Fprintf(&b, "{Name:%q,Service:%q,Kind:%q,Destructive:%t},\n", m.Name, m.Receiver, m.Kind, m.Destructive)
	}
	fmt.Fprintf(&b, "}\nfunc %sMethodByName(n string)(%sMethod,bool){for _,m:=range %sMethods{if m.Name==n{return m,true}};return %sMethod{},false}\n", title(s.domain), title(s.domain), title(s.domain), title(s.domain))
	for _, m := range ms {
		fmt.Fprintf(&b, "func(s %s)%s(ctx context.Context,r ManagementCall)(json.RawMessage,error){return managementCall(ctx,s.managementCaller,%q,%q,%q,r)}\n", m.Receiver, goName(m.Leaf), s.domain, m.Namespace, m.Leaf)
	}
	src, e := format.Source([]byte(b.String()))
	must(e)
	must(os.WriteFile(filepath.Join(out, s.domain+"_generated.go"), src, 0644))
	var md strings.Builder
	fmt.Fprintf(&md, "# %s API 구현 현황\n\nTrueNAS API v25.10.5 기준. `go generate ./internal/truenas`로 생성한다.\n\n| 상태 | 메서드 | 종류 | 위험 | Wrapper |\n|---|---|---|---|---|\n", title(s.domain))
	for _, m := range ms {
		fmt.Fprintf(&md, "| ✅ | `%s` | %s | %v | `%s.%s` |\n", m.Name, m.Kind, m.Destructive, m.Receiver, goName(m.Leaf))
	}
	must(os.WriteFile(filepath.Join(filepath.Dir(filepath.Dir(doc)), s.domain+"-implementation.md"), []byte(md.String()), 0644))
}
func classify(n string, m []mapping) (string, string, string) {
	for _, x := range m {
		if strings.HasPrefix(n, x.prefix) {
			return strings.TrimSuffix(x.prefix, "."), x.receiver, strings.TrimPrefix(n, x.prefix)
		}
	}
	panic(n)
}
func goName(s string) string {
	var b strings.Builder
	up := true
	for _, r := range s {
		if r == '_' || r == '.' {
			up = true
			continue
		}
		if up {
			r = []rune(strings.ToUpper(string(r)))[0]
			up = false
		}
		b.WriteRune(r)
	}
	return b.String()
}
func title(s string) string { return strings.ToUpper(s[:1]) + s[1:] }
func kind(s string) string {
	switch s {
	case "delete", "destroy", "reset", "shutdown", "reboot", "detach", "replace", "set_password", "unset_2fa_secret", "terminate_session", "terminate_other_sessions":
		return "destructive"
	case "query", "get_instance", "config", "info", "status", "state", "version", "version_short", "ready", "my_keys", "me", "sessions", "roles":
		return "read"
	case "create":
		return "create"
	default:
		return "change"
	}
}
func must(e error) {
	if e != nil {
		panic(e)
	}
}
