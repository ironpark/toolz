package board

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/ironpark/toolz/cli/ppwk/internal/refstore"
)

// RefStats 는 refs/ppwk/ 아래가 얼마나 쌓였는지다 (§9.2).
type RefStats struct {
	// Issues 와 Archive 는 각 prefix 의 ref 개수다.
	Issues  int `json:"issues"`
	Archive int `json:"archive"`
	// Loose 는 아직 packed-refs 로 들어가지 않고 파일로 남은 ref 수다.
	Loose int `json:"loose"`
}

// Total 은 refs/ppwk/ 아래 전체 ref 수다.
func (s RefStats) Total() int { return s.Issues + s.Archive }

// RefStats 는 보드 ref 의 크기를 센다.
//
// 별도 gc 명령을 두지 않는 대신 여기서 세어 doctor 가 보고한다. 정리는
// git 이 이미 잘 하는 일이고 (`git gc` 가 pack-refs 와 dangling commit
// 정리를 함께 한다), 우리가 더할 수 있는 것은 "지금 얼마나 쌓였는지" 뿐이다.
//
// loose 를 따로 세는 이유는 ppwk 가 ref 를 update-ref 로만 쓰기 때문이다.
// update-ref 는 auto-gc 를 유발하지 않으므로, 사람이 커밋을 전혀 하지 않는
// 저장소에서는 loose ref 가 계속 쌓이기만 한다.
func (b *Board) RefStats() (RefStats, error) {
	var stats RefStats
	for _, prefix := range []struct {
		name  string
		count *int
	}{{refstore.Issues, &stats.Issues}, {refstore.Archive, &stats.Archive}} {
		refs, err := b.store.List(prefix.name)
		if err != nil {
			return stats, err
		}
		*prefix.count = len(refs)
	}
	stats.Loose = countLooseRefs(filepath.Join(b.git.CommonDir(), "refs", "ppwk"))
	return stats, nil
}

// countLooseRefs 는 디렉터리 아래의 ref 파일을 센다.
//
// packed-refs 에 들어간 ref 는 파일이 없으므로 여기 잡히지 않는다. 그것이
// 정확히 우리가 보려는 차이다. reftable backend 에는 이 디렉터리가 아예
// 없으며, 그 경우 0 이 맞는 답이다.
func countLooseRefs(dir string) int {
	count := 0
	_ = filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			// 디렉터리가 없는 것은 오류가 아니다 — 전부 packed 되었거나
			// reftable 이다.
			return nil
		}
		if entry.IsDir() || strings.HasSuffix(path, ".lock") {
			return nil
		}
		count++
		return nil
	})
	return count
}
