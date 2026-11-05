package feishu

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestBotClientCachesTokenAndRedactsErrors(t *testing.T) {
	var tokenCalls, sends atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		switch r.URL.Path {
		case "/open-apis/auth/v3/tenant_access_token/internal":
			tokenCalls.Add(1)
			_, _ = w.Write([]byte(`{"code":0,"tenant_access_token":"t-1","expire":7200}`))
		case "/open-apis/im/v1/messages":
			sends.Add(1)
			var body map[string]string
			_ = json.Unmarshal(raw, &body)
			if r.Header.Get("Authorization") != "Bearer t-1" || r.URL.Query().Get("receive_id_type") != "open_id" || body["msg_type"] != "text" || body["uuid"] != "key-1" {
				t.Errorf("unexpected send: %v %s", r.Header, raw)
			}
			if body["receive_id"] == "ou_outside" {
				_, _ = w.Write([]byte(`{"code":230013,"msg":"SECRET provider detail"}`))
				return
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{}}`))
		}
	}))
	defer server.Close()
	c := NewBotClient(server.URL, "app", "secret", server.Client())
	for range 2 {
		if err := c.Send(context.Background(), "ou_a", "hello", "key-1"); err != nil {
			t.Fatal(err)
		}
	}
	if tokenCalls.Load() != 1 || sends.Load() != 2 {
		t.Fatal("token not cached", tokenCalls.Load(), sends.Load())
	}
	err := c.Send(context.Background(), "ou_outside", "hello", "key-1")
	if err == nil || !c.Permanent(err) || strings.Contains(err.Error(), "SECRET") {
		t.Fatal("availability error not permanent or leaked", err)
	}
}
