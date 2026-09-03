# 인증과 보안

[← 문서 홈](../README.md)

연결 직후 [`auth.login_ex`](https://api.truenas.com/v25.10/api_methods_auth.login_ex.html)를
호출한다. API 키에는 키를 생성한 사용자명도 함께 보낸다.

```json
{
  "jsonrpc": "2.0", "id": 1, "method": "auth.login_ex",
  "params": [{
    "mechanism": "API_KEY_PLAIN", "username": "admin",
    "api_key": "<api-key>", "login_options": {"user_info": false}
  }]
}
```

비밀번호 인증은 `PASSWORD_PLAIN`과 `password`를 사용한다. 응답의 `response_type`이
`SUCCESS`이면 완료다. `OTP_REQUIRED`이면 `auth.login_ex_continue`로 후속 인증한다.

`auth.login_with_api_key`는 27에서 제거 예정인 호환 메서드이므로 신규 구현에 쓰지
않는다. 26부터 제공되는 SCRAM도 25.10 전용 구현에 섞지 않는다.

- API 키는 비밀번호와 같은 수준의 비밀로 취급한다.
- 운영에서는 `wss`를 사용한다.
- `ws://`와 인증서 검증 생략은 사용자가 위험을 수락한 개발 환경에만 허용한다.
- 인증 정보와 토큰은 로그 및 오류 메시지에서 제거한다.
