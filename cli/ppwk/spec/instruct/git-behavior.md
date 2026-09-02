# git 동작

[← AGENTS.md](../../AGENTS.md)

## 보드 데이터의 위치

이슈는 `refs/paperwork/*` 에 저장되며 소스 커밋 히스토리와 분리됩니다.

```
refs/paperwork/
├─ issues/<id>       활성 이슈
├─ plans/<id>        계획
└─ archive/<id>      종료된 이슈
```

에이전트 생존 정보는 ref 가 아니라 `$GIT_COMMON_DIR/paperwork/locks/` 의 파일입니다. 런타임 신호라 공유 저장소에 둘 이유가 없고, 이 디렉터리도 모든 worktree 가 공유합니다.

결과적으로:

- 브랜치를 바꿔도 이슈 목록이 유지됩니다
- `git status` 는 깨끗하게 남습니다
- 워킹 디렉터리에 파일이 생기지 않습니다
- `git log <브랜치>` 에 이슈 커밋이 섞이지 않습니다

## worktree 간 공유

같은 저장소의 모든 worktree 가 `$GIT_COMMON_DIR` 을 공유하므로, 다른 에이전트가 방금 바꾼 상태가 fetch 없이 즉시 보입니다.

## git log --all

`--all` 은 모든 ref 를 포함하므로 이슈 커밋까지 나옵니다. 소스 히스토리만 보려면:

```bash
git log --exclude='refs/paperwork/*' --all
```

`init` 이 아래 별칭을 제안합니다.

```bash
git config alias.la "log --exclude=refs/paperwork/* --all"
```

## 원격 공유

`git push` 는 기본 refspec 이 브랜치만 다루므로 이슈 데이터는 로컬에 머뭅니다.

`git push --mirror` 는 모든 ref 를 올려 이슈 제목·본문·에이전트 신원까지 원격에 공개합니다. 사용 여부를 팀과 합의해 두세요.

의도적으로 동기화하려면 refspec 을 설정에 두는 편이 안전합니다.

```bash
git config --add remote.origin.push 'refs/paperwork/*:refs/paperwork/*'
```

## 직접 조작을 피하는 이유

`git update-ref` 를 손으로 부르면 compare-and-swap 규약을 우회하게 되어 다른 에이전트의 변경을 덮어쓸 수 있습니다. 상태 변경은 `paperwork` 명령으로 하세요.
