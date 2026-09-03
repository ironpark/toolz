# 이슈 만들기

[← AGENTS.md](../../AGENTS.md)

```bash
ppwk add "제목"
    --priority high|med|low|none   기본 med. none 은 백로그
    --label backend                반복 가능
    --depends-on T001              반복 가능
    --body-file notes.md           긴 설명
    --plan P01 --phase p2          계획에 편입
    [--seq 30]                     생략 시 자동
```

제목은 한 줄로 명확하게 쓰고, 상세 내용은 body 에 담습니다.

## 언제 이슈를 만드나

작업 중 범위를 넘는 할 일을 발견하면 그 자리에서 처리하는 대신 이슈로 남깁니다. 다른 에이전트가 병렬로 가져갈 수 있습니다.

## 의존성

`--depends-on` 으로 지정한 이슈가 `done` 이 되어야 후보에 나타납니다.

```bash
ppwk add "마이그레이션" --depends-on T001 --depends-on T002
```

선행 이슈가 `cancelled` 면 의존은 충족되지 않습니다. 이 경우 의존을 직접 정리하세요.

```bash
ppwk edit T005 --remove-depends-on T001
```

## 백로그

당장 하지 않을 것은 `--priority none` 으로 둡니다. `next` 가 고르지 않지만 목록에는 남습니다.

```bash
ppwk add "언젠가 리팩터링" --priority none
ppwk list --priority none
ppwk edit T042 --priority low     # 꺼내기
```

## 수정

```bash
ppwk edit T001 --title "새 제목" --priority high
ppwk edit T001 --add-label urgent --remove-label backend
```

`edit` 은 메타데이터만 바꾸며 상태는 유지합니다.
