# 계획 (plan / phase)

[← AGENTS.md](../../AGENTS.md)

계획은 이슈를 단계(phase)로 묶어 순서를 강제합니다.

```bash
ppwk plan list
ppwk plan show P01        # phase 별 진행 상황
```

## plan show 읽기

```
P01  storage 레이어 재작성          [active]

  p1  스키마 설계                    3/3  done
  p2  구현                          1/4  working      ← 현재 phase
      T004  done     agent-a   SQLite storage 구현
      T005  working  agent-b   parser cleanup
      T006  open     -         에러 처리
  p3  마이그레이션                   0/2  blocked (gate: manual)
```

진행률과 현재 phase 는 **계산된 값**입니다. 저장되어 있지 않으므로 항상 최신입니다.

## gate

다음 phase 가 열리는 조건입니다.

| gate | 열리는 시점 |
|---|---|
| `all_done` | 직전 phase 의 모든 작업이 done 또는 cancelled |
| `any_done` | 직전 phase 에서 하나라도 done |
| `manual` | 사람이 `plan advance` 를 실행 |

gate 로 대기 중인 작업은 `status` 가 `open` 인 채로 `next` 후보에서만 제외됩니다.

## 배정 순서

계획에 속한 작업은 `seq` 순서를 따릅니다. **`seq` 가 `priority` 보다 우선합니다.** 계획 작성자가 의도한 순서가 우선순위보다 앞서므로, high priority 작업이라도 seq 가 뒤면 나중에 배정됩니다.

## 계획 만들기

```bash
ppwk plan new "storage 레이어 재작성" --priority high
ppwk plan phase add P01 "스키마 설계" --gate all_done
ppwk plan phase add P01 "구현"
ppwk plan advance P01 p3          # manual gate 개방
ppwk plan close P01
```
