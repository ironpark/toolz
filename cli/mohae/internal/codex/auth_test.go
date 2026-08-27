package codex

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestReadAccountVariants(t *testing.T) {
	client, server := connect(t, Options{})

	tests := []struct {
		name   string
		result string
		check  func(*testing.T, *AccountInfo)
	}{
		{
			name:   "no account",
			result: `{"account":null,"requiresOpenaiAuth":true}`,
			check: func(t *testing.T, info *AccountInfo) {
				if info.Account != nil || !info.RequiresOpenaiAuth {
					t.Fatalf("info = %+v", info)
				}
			},
		},
		{
			name:   "api key",
			result: `{"account":{"type":"apiKey"},"requiresOpenaiAuth":true}`,
			check: func(t *testing.T, info *AccountInfo) {
				if info.Account == nil || info.Account.Type != AccountAPIKey {
					t.Fatalf("account = %+v", info.Account)
				}
			},
		},
		{
			name:   "chatgpt",
			result: `{"account":{"type":"chatgpt","email":"user@example.com","planType":"pro"},"requiresOpenaiAuth":true}`,
			check: func(t *testing.T, info *AccountInfo) {
				if info.Account.Email == nil || *info.Account.Email != "user@example.com" {
					t.Fatalf("email = %v", info.Account.Email)
				}
				if info.Account.PlanType != "pro" {
					t.Fatalf("planType = %q", info.Account.PlanType)
				}
			},
		},
		{
			name:   "bedrock",
			result: `{"account":{"type":"amazonBedrock","credentialSource":"awsManaged"},"requiresOpenaiAuth":false}`,
			check: func(t *testing.T, info *AccountInfo) {
				if info.Account.CredentialSource != CredentialSourceAWSManaged || info.RequiresOpenaiAuth {
					t.Fatalf("info = %+v", info)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			done := serve(t, func() {
				req := server.expect("account/read")
				if string(req.Params) != `{"refreshToken":false}` {
					t.Errorf("params = %s", req.Params)
				}
				server.send(map[string]any{"id": json.RawMessage(req.ID), "result": json.RawMessage(tc.result)})
			})
			info, err := client.ReadAccount(context.Background(), false)
			<-done
			if err != nil {
				t.Fatalf("ReadAccount: %v", err)
			}
			tc.check(t, info)
		})
	}
}

func TestReadAccountRefreshToken(t *testing.T) {
	client, server := connect(t, Options{})
	done := serve(t, func() {
		req := server.expect("account/read")
		if string(req.Params) != `{"refreshToken":true}` {
			t.Errorf("params = %s", req.Params)
		}
		server.respond(req, map[string]any{"account": nil, "requiresOpenaiAuth": false})
	})
	if _, err := client.ReadAccount(context.Background(), true); err != nil {
		t.Fatalf("ReadAccount: %v", err)
	}
	<-done
}

func TestLoginAPIKeyFlow(t *testing.T) {
	client, server := connect(t, Options{})

	done := serve(t, func() {
		req := server.expect("account/login/start")
		if string(req.Params) != `{"type":"apiKey","apiKey":"sk-test"}` {
			t.Errorf("params = %s", req.Params)
		}
		server.respond(req, map[string]any{"type": "apiKey"})
		server.notify(MethodLoginCompleted, map[string]any{"loginId": nil, "success": true, "error": nil})
		server.notify(MethodAccountUpdated, map[string]any{"authMode": "apikey", "planType": nil})
	})

	updates := client.AccountUpdates()
	if err := client.LoginAPIKey(context.Background(), "sk-test"); err != nil {
		t.Fatalf("LoginAPIKey: %v", err)
	}

	completed, err := client.AwaitLogin(context.Background(), "")
	<-done
	if err != nil {
		t.Fatalf("AwaitLogin: %v", err)
	}
	if !completed.Success || completed.LoginID != "" {
		t.Fatalf("completed = %+v", completed)
	}

	select {
	case update := <-updates:
		if update.AuthMode != AuthModeAPIKey {
			t.Fatalf("update = %+v", update)
		}
	case <-time.After(fakeTimeout):
		t.Fatal("no account update")
	}
}

func TestLoginChatGPTBrowserFlowAndCancel(t *testing.T) {
	client, server := connect(t, Options{})

	done := serve(t, func() {
		req := server.expect("account/login/start")
		if string(req.Params) != `{"type":"chatgpt","useHostedLoginSuccessPage":true,"appBrand":"chatgpt"}` {
			t.Errorf("params = %s", req.Params)
		}
		server.respond(req, map[string]any{
			"type": "chatgpt", "loginId": "login-1", "authUrl": "https://chatgpt.com/auth",
		})

		req = server.expect("account/login/cancel")
		if string(req.Params) != `{"loginId":"login-1"}` {
			t.Errorf("params = %s", req.Params)
		}
		server.respond(req, map[string]any{})
		server.notify(MethodLoginCompleted, map[string]any{
			"loginId": "login-1", "success": false, "error": "canceled",
		})
	})

	login, err := client.LoginChatGPT(context.Background(), &ChatGPTLoginOptions{
		UseHostedLoginSuccessPage: true, AppBrand: "chatgpt",
	})
	if err != nil {
		t.Fatalf("LoginChatGPT: %v", err)
	}
	if login.LoginID != "login-1" || login.AuthURL == "" {
		t.Fatalf("login = %+v", login)
	}

	if err := client.CancelLogin(context.Background(), login.LoginID); err != nil {
		t.Fatalf("CancelLogin: %v", err)
	}
	completed, err := client.AwaitLogin(context.Background(), "login-1")
	<-done
	if err == nil {
		t.Fatal("AwaitLogin succeeded for a canceled login")
	}
	if completed == nil || completed.Error != "canceled" {
		t.Fatalf("completed = %+v", completed)
	}
}

func TestLoginDeviceCodeFlow(t *testing.T) {
	client, server := connect(t, Options{})

	done := serve(t, func() {
		req := server.expect("account/login/start")
		if string(req.Params) != `{"type":"chatgptDeviceCode"}` {
			t.Errorf("params = %s", req.Params)
		}
		server.respond(req, map[string]any{
			"type": "chatgptDeviceCode", "loginId": "login-2",
			"verificationUrl": "https://auth.openai.com/codex/device", "userCode": "ABCD-1234",
		})
	})

	login, err := client.LoginChatGPTDeviceCode(context.Background())
	<-done
	if err != nil {
		t.Fatalf("LoginChatGPTDeviceCode: %v", err)
	}
	if login.UserCode != "ABCD-1234" || login.VerificationURL == "" || login.LoginID != "login-2" {
		t.Fatalf("login = %+v", login)
	}
}

func TestAwaitLoginIgnoresOtherLoginIDs(t *testing.T) {
	client, server := connect(t, Options{})

	type result struct {
		completed *LoginCompletedParams
		err       error
	}
	results := make(chan result, 1)
	ready := make(chan struct{})
	go func() {
		close(ready)
		completed, err := client.AwaitLogin(context.Background(), "login-mine")
		results <- result{completed, err}
	}()
	<-ready
	time.Sleep(50 * time.Millisecond)

	server.notify(MethodLoginCompleted, map[string]any{"loginId": "login-other", "success": true})
	select {
	case res := <-results:
		t.Fatalf("AwaitLogin resolved on the wrong login: %+v", res)
	case <-time.After(100 * time.Millisecond):
	}

	server.notify(MethodLoginCompleted, map[string]any{"loginId": "login-mine", "success": true})
	select {
	case res := <-results:
		if res.err != nil || res.completed.LoginID != "login-mine" {
			t.Fatalf("res = %+v", res)
		}
	case <-time.After(fakeTimeout):
		t.Fatal("AwaitLogin did not resolve")
	}
}

func TestAwaitLoginContextCancel(t *testing.T) {
	client, _ := connect(t, Options{})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := client.AwaitLogin(ctx, "login-1"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want DeadlineExceeded", err)
	}

	client.mu.Lock()
	pending := len(client.logins)
	client.mu.Unlock()
	if pending != 0 {
		t.Fatalf("waiters leaked: %d", pending)
	}
}

func TestAwaitLoginClientClosed(t *testing.T) {
	client, _ := connect(t, Options{})

	go func() {
		time.Sleep(50 * time.Millisecond)
		_ = client.Close()
	}()
	if _, err := client.AwaitLogin(context.Background(), "login-1"); !errors.Is(err, ErrClosed) {
		t.Fatalf("err = %v, want ErrClosed", err)
	}
}

func TestLogout(t *testing.T) {
	client, server := connect(t, Options{})

	done := serve(t, func() {
		req := server.expect("account/logout")
		server.respond(req, map[string]any{})
		server.notify(MethodAccountUpdated, map[string]any{"authMode": nil, "planType": nil})
	})

	updates := client.AccountUpdates()
	if err := client.Logout(context.Background()); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	<-done

	select {
	case update := <-updates:
		if update.AuthMode != "" {
			t.Fatalf("update = %+v", update)
		}
	case <-time.After(fakeTimeout):
		t.Fatal("no account update after logout")
	}
}
