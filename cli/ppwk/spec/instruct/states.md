# 상태와 전이

[← AGENTS.md](../../AGENTS.md)

```
open       배정 대기. next 의 후보
claimed    예약됨. 아직 시작 전
working    작업 중
blocked    막힘. 사유가 기록되어 있음
done       완료 (archive 로 이동)
cancelled  취소 (archive 로 이동)
```

## 전이

| 명령 | 전이 |
|---|---|
| `claim <id>` | open → claimed (예약만) |
| `start <id>` | open → working, claimed → working |
| `done <id>` | working → done |
| `block <id> [--on ID] [--message T]` | working → blocked |
| `unblock <id>` | blocked → working |
| `release <id>` | claimed → open |
| `cancel <id>` | any → cancelled |

표에 없는 전이는 exit 3 으로 거부됩니다.

## 알아둘 것

**`done` 은 종료 상태입니다.** 완료된 이슈는 다시 열 수 없습니다. 후속 작업이 필요하면 새 이슈를 만드세요.

**이미 `done` 인 이슈에 `done` 을 다시 부르면 exit 3 입니다.** 멱등 성공으로 처리하지 않으므로, 중복 호출은 로직 오류로 드러납니다.

**`start` 는 `open` 에서 바로 됩니다.** claim 을 겸하므로 보통 `claim` 을 따로 부를 일이 없습니다. `claim` 은 "예약만 하고 시작은 나중에" 가 필요할 때 씁니다.

**다른 에이전트 소유 이슈는 exit 3 으로 거부됩니다.**

**phase gate 로 대기 중인 작업은 `status` 가 `open` 입니다.** 표시상의 `blocked (gate)` 는 계산된 값이며 저장된 상태가 아닙니다.
