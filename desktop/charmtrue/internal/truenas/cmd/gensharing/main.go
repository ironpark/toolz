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
	Name, Service, GoName, Kind string
	Destructive                 bool
}

func main() {
	doc, out := flag.String("doc", "", "doc"), flag.String("out", "", "out")
	check := flag.String("checklist", "", "checklist")
	flag.Parse()
	f, e := os.Open(*doc)
	must(e)
	defer f.Close()
	var ms []method
	s := bufio.NewScanner(f)
	for s.Scan() {
		x := row.FindStringSubmatch(s.Text())
		if x == nil {
			continue
		}
		svc, leaf := classify(x[1])
		ms = append(ms, method{x[1], svc, goName(leaf), kind(leaf), leaf == "delete" || leaf == "setacl"})
	}
	must(s.Err())
	if len(ms) != 35 {
		panic(fmt.Sprintf("got %d sharing methods, want 35", len(ms)))
	}
	sort.Slice(ms, func(i, j int) bool { return ms[i].Name < ms[j].Name })
	var b strings.Builder
	b.WriteString("// Code generated from TrueNAS API v25.10.5; DO NOT EDIT.\npackage truenas\nimport(\"context\";\"encoding/json\")\ntype SharingMethod struct{Name,Service,Kind string;Destructive bool}\nvar SharingMethods=[...]SharingMethod{\n")
	for _, m := range ms {
		fmt.Fprintf(&b, "{Name:%q,Service:%q,Kind:%q,Destructive:%t},\n", m.Name, m.Service, m.Kind, m.Destructive)
	}
	b.WriteString("}\nfunc SharingMethodByName(n string)(SharingMethod,bool){for _,m:=range SharingMethods{if m.Name==n{return m,true}};return SharingMethod{},false}\n")
	for _, m := range ms {
		fmt.Fprintf(&b, "func(s %s)%s(ctx context.Context,r SharingCall)(json.RawMessage,error){return s.Call(ctx,%q,r)}\n", m.Service, m.GoName, relative(m.Name))
	}
	src, e := format.Source([]byte(b.String()))
	must(e)
	must(os.MkdirAll(filepath.Dir(*out), 0755))
	must(os.WriteFile(*out, src, 0644))
	var md strings.Builder
	md.WriteString("# 파일 공유 API 구현 현황\n\nTrueNAS API v25.10.5 기준. `go generate ./internal/truenas`로 생성한다.\n\n| 상태 | 메서드 | 종류 | 위험 | Wrapper |\n|---|---|---|---|---|\n")
	for _, m := range ms {
		fmt.Fprintf(&md, "| ✅ | `%s` | %s | %v | `%s.%s` |\n", m.Name, m.Kind, m.Destructive, m.Service, m.GoName)
	}
	must(os.WriteFile(*check, []byte(md.String()), 0644))
}
func classify(n string) (string, string) {
	for _, p := range []struct{ p, s string }{{"sharing.nfs.", "NFSShareService"}, {"sharing.smb.", "SMBShareService"}, {"rsynctask.", "RsyncTaskService"}, {"ftp.", "FTPService"}, {"nfs.", "NFSService"}, {"smb.", "SMBService"}, {"ssh.", "SSHService"}} {
		if strings.HasPrefix(n, p.p) {
			return p.s, strings.TrimPrefix(n, p.p)
		}
	}
	panic(n)
}
func relative(n string) string { _, x := classify(n); return x }
func goName(s string) string {
	var b strings.Builder
	u := true
	for _, r := range s {
		if r == '_' || r == '.' {
			u = true
			continue
		}
		if u {
			r = []rune(strings.ToUpper(string(r)))[0]
			u = false
		}
		b.WriteRune(r)
	}
	return b.String()
}
func kind(s string) string {
	switch s {
	case "config", "query", "get_instance", "getacl", "presets", "client_count", "get_nfs3_clients", "get_nfs4_clients", "bindip_choices", "bindiface_choices", "unixcharset_choices":
		return "read"
	case "create":
		return "create"
	case "delete", "setacl":
		return "destructive"
	default:
		return "change"
	}
}
func must(e error) {
	if e != nil {
		panic(e)
	}
}
