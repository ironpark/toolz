---
plan_name: checkout-v2
---
# GOALS

## 문제와 사용자 관점의 최종 결과

새 checkout 흐름을 안전하게 출시한다.

## 측정 가능한 목표

전환율과 결제 성공률을 기존 수준 이상으로 유지한다.

## 지원 범위와 비목표

웹 checkout만 포함하며 모바일 앱은 제외한다.

## reference source / commit / license

기존 결제 API 계약을 따른다.

## 계획 전체의 완료 기준

점진 배포 후 핵심 지표가 안정적이다.

# SCOPE

결제 화면과 주문 생성 API를 이전한다.

# CONTEXT

## 현재 구현과 병목

인증과 결제 UI가 강하게 결합되어 있다.

## 목표 구조와 invariant

주문 생성은 멱등성을 유지하고 기존 checkout은 롤백 가능해야 한다.

# PHASES

## PHASE — API Contract

```yaml
phase: 0
slug: api-contract
perf_phase: false
depends_on: []
status: planned
entry_condition: null
```

### 계획된 작업

- 주문 생성 API 계약과 fixture를 추가한다.

### 완료 조건

- 계약 테스트가 통과한다.

## PHASE — Checkout UI

```yaml
phase: 1
slug: checkout-ui
perf_phase: false
depends_on: [0]
status: planned
entry_condition: null
```

### 계획된 작업

- 새 checkout UI를 feature flag 뒤에 구현한다.

### 완료 조건

- E2E checkout 시나리오가 통과한다.

## PHASE — Gradual Rollout

```yaml
phase: 2
slug: gradual-rollout
perf_phase: true
depends_on: [1]
status: conditional
entry_condition: checkout UI의 오류율이 기존 흐름과 같거나 낮을 때만 착수한다.
```

### 계획된 작업

- 트래픽을 단계적으로 전환한다.

### 완료 조건

- 전환율과 결제 성공률을 기록한다.

# VERIFICATION

go test ./... && E2E checkout suite

# ORDERING

계약을 먼저 고정한 뒤 UI를 구현하고, 지표가 안정적일 때만 배포한다.

# NEXT

```yaml
next_phase: 0
```

API 계약과 fixture부터 추가한다.
