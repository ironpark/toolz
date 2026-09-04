# 문제 해결

[← AGENTS.md](../../AGENTS.md)

## 먼저 실행

```bash
ppwk doctor      # 환경 점검
ppwk fsck        # 데이터 정합성 점검
```

`fsck` 가 문제를 보고하면 사람에게 먼저 알리세요. `--fix` 는 일부 항목만 자동 처리하고 나머지는 판단이 필요합니다.

## 작업이 사라졌다

세션 잠금이 풀려 다른 에이전트가 회수한 경우입니다.

```bash
ppwk doctor      # 내 상태
ppwk agents      # 전체 현황
```

작업을 다시 받으세요. 세션은 자동으로 재등록됩니다.

```bash
ppwk next --claim
```

회수가 느리게 느껴진다면 통합 수준을 올릴 수 있습니다. `doctor` 가 현재 수준을 보여줍니다.

```bash
ppwk hook install --agent-tools      # 즉시 감지
ppwk release --mine                  # 대화 종료 시 직접 정리
```

아무것도 하지 않아도 동작하지만, 이 경우 죽은 세션의 작업이 최대 8시간 방치될 수 있습니다. 대화를 마칠 때 `release --mine` 을 부르면 즉시 정리됩니다.

## worktree 가 사용 중이라고 나온다

```
error: worktree /repo-a is in use by claude-code:repo-a (session 7f3a..., pid 48211)
```

같은 worktree 에서 다른 에이전트가 작업 중입니다. worktree 는 `HEAD` 와 index 를 하나만 가지므로, 둘이 같은 디렉터리에서 작업하면 서로의 파일 수정을 덮어씁니다.

새 worktree 를 만드세요.

```bash
git worktree add ../repo-b -b feature/my-work
cd ../repo-b && ppwk next --claim
```

## 다른 에이전트가 오래 붙잡고 있다

```bash
ppwk agents
# agent-c  alive  T009  holding 3h12m
```

잠금 방식은 살아있는 프로세스의 작업을 자동 회수하지 않습니다. 비정상적으로 길면 사람에게 알리세요. 판단 후 강제 회수가 가능합니다.

```bash
ppwk release T009 --force
```

## exit 4 가 반복된다

경쟁이 심한 상황입니다. 같은 이슈를 재시도하는 대신 `next` 로 돌아가세요. `next` 가 자동으로 다음 후보를 찾습니다.

## exit 3 이 났다

잘못된 상태 전이입니다. 현재 상태를 확인하고 사람에게 보고하세요.

```bash
ppwk show <id>
ppwk history <id>
```

## exit 6 이 났다

`ppwk` 버전이 저장소 스키마와 맞지 않습니다. 업그레이드가 필요하니 멈추고 보고하세요.

## 후보가 계속 비어 있다

```bash
ppwk list --status open          # open 이슈가 있는지
ppwk next --dry-run              # 왜 후보에서 빠지는지
```

의존성 미충족이나 phase gate 때문일 수 있습니다. 순환 의존이 의심되면 `fsck` 가 검출합니다.

## lock 오류가 난다

다른 프로세스가 같은 ref 를 쓰는 중입니다. 잠시 후 재시도하면 대부분 해결됩니다. `doctor` 가 stale lock 을 보고하면 사람에게 알리세요. lock 파일은 사람이 확인 후 처리합니다.
