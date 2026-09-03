# 결정 기록

[← AGENTS.md](../../AGENTS.md)

다른 에이전트가 알아야 할 결정을 남깁니다. 브랜치와 무관하게 모든 worktree 에서 즉시 보입니다.

## 기록

```bash
ppwk decide "저장소는 SQLite" \
    --context "단일 머신, 동시 쓰기 적음" \
    --option SQLite --option PostgreSQL \
    --decision SQLite \
    --consequences "동시 쓰기 확장 시 재검토" \
    --issue T001
# → D007
```

`--issue` 로 연결하면 그 이슈의 `show` 에 함께 표시됩니다.

## 조회

```bash
ppwk decisions                  # 유효한 것만
ppwk decisions --issue T001     # 이 이슈와 관련된 결정
ppwk decisions --search sqlite
ppwk decisions show D007
```

**작업을 시작하기 전에 관련 결정을 확인하세요.** 세션이 바뀌어도 같은 논의를 반복하지 않게 됩니다.

## 바꾸기

결정은 수정할 수 없습니다. 바뀌면 새 결정으로 대체합니다.

```bash
ppwk decide "저장소는 PostgreSQL" --supersedes D007 --context "동시 쓰기가 늘어남" ...
# → D012
```

이전 결정은 그대로 남고, 목록에서는 최신 것만 보입니다. `decisions history D012` 로 변천을 볼 수 있습니다.

## 무엇을 기록하나

- 기술 선택 (라이브러리, 저장소, 프로토콜)
- 설계 방향 (구조, 경계, 규약)
- 기각한 대안과 이유 — 기각도 결정입니다

작업 진행 상황은 결정이 아닙니다. 그건 이슈의 상태로 표현합니다.

## 저장소에 남기기

```bash
ppwk export --decisions -o docs/decisions/
```

ADR 마크다운 파일이 생성됩니다. 이건 평범하게 커밋하세요. 원본은 여전히 `ppwk` 안에 있고, 파일은 파생물입니다.
