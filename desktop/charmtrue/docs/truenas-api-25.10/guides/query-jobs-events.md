# 쿼리, 작업, 이벤트

[← 문서 홈](../README.md)

## 쿼리

`*.query`는 일반적으로 `[filters, options]`를 받는다.

```json
[[["type", "=", "HDD"], ["rotationrate", ">", 5400]],
 {"limit": 20, "order_by": ["name"]}]
```

- 필터: `[field, operator, value]`
- 연산자: `=`, `!=`, `>`, `>=`, `<`, `<=`, `~`, `in`, `nin`, `rin`, `rnin`,
  `^`, `!^`, `$`, `!$`
- 여러 조건은 기본 AND이며 `['OR', [...]]` 같은 논리 조건도 지원한다.
- 대표 옵션: `count`, `get`, `limit`, `offset`, `select`, `order_by`

공식 문서: [Query methods](https://api.truenas.com/v25.10/query_methods.html)

## 작업(Job)

오래 걸리는 메서드는 Job으로 실행된다. 원래 호출의 최종 응답을 기다리거나
`core.get_jobs`를 조회·구독한다. `core.job_wait`, `core.job_abort`,
`core.job_download_logs`로 대기, 중단, 로그 다운로드를 처리한다. 재접속 후에는 Job ID로
복구 조회한다. 공식 문서: [Jobs](https://api.truenas.com/v25.10/jobs.html)

## 이벤트

`core.subscribe`와 `core.unsubscribe`를 사용한다. 서버 알림에는 요청 `id`가 없으며
`collection_update` 또는 `notify_unsubscribed`로 전달된다. 컬렉션 변경은 보통
`added`, `changed`, `removed`다. 재연결 시 구독을 다시 설정한다.
