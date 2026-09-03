package board

import (
	"testing"
)

// RefStats 는 issues 와 archive 를 나눠 세고, packed 여부를 loose 로 구분한다.
func TestRefStatsCountsAndPacking(t *testing.T) {
	b, dir := initBoardDir(t)
	live := mustAdd(t, b, AddOptions{Title: "살아있음"})
	gone := mustAdd(t, b, AddOptions{Title: "끝남"})
	transitionAll(t, b, gone.ID, ActionStart, ActionDone)

	stats, err := b.RefStats()
	if err != nil {
		t.Fatal(err)
	}
	if stats.Issues != 1 || stats.Archive != 1 || stats.Total() != 2 {
		t.Fatalf("stats = %+v", stats)
	}
	if stats.Loose < 2 {
		t.Fatalf("loose = %d, want >= 2 (issues %s, archive %s)", stats.Loose, live.ID, gone.ID)
	}

	// pack-refs 뒤에는 파일이 사라지므로 loose 가 줄고, ref 개수는 그대로다.
	runGit(t, dir, "pack-refs", "--all")
	packed, err := b.RefStats()
	if err != nil {
		t.Fatal(err)
	}
	if packed.Issues != 1 || packed.Archive != 1 {
		t.Fatalf("pack-refs 후 stats = %+v", packed)
	}
	if packed.Loose >= stats.Loose {
		t.Fatalf("loose 가 줄지 않았습니다: %d → %d", stats.Loose, packed.Loose)
	}
}
