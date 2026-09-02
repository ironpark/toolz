# 조회

[← AGENTS.md](../../AGENTS.md)

```bash
paperwork list                              # 활성 이슈
paperwork list --status open                # 배정 대기 중
paperwork list --owner $PAPERWORK_AGENT   # 내 작업
paperwork list --sort next                  # 배정될 순서대로
paperwork list --plan P01                   # 특정 계획만
paperwork list --archived                   # 종료된 이슈
paperwork show T001                         # 이슈 상세
paperwork history T001                      # 이슈 이력
paperwork list --mine                       # 이 대화에서 잡은 작업
paperwork agents                            # 에이전트 상태
paperwork doctor                            # 내 상태
```

모든 명령이 `--json` 을 지원합니다. 스크립트에서는 `--json` 과 `jq` 를 쓰세요.

```bash
paperwork list --status open --json | jq -r '.data[].id'
```

`--mine` 은 지금 대화에서 claim 한 작업만 보여줍니다. 대화를 마칠 때 한 번에 반납할 수 있습니다.

```bash
paperwork release --mine
```

`--sort next` 는 `next` 가 쓰는 것과 같은 정렬입니다. 다음에 무엇이 배정될지 미리 볼 때 씁니다.
