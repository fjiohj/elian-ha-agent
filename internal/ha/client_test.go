package ha

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientProxiesAllowedAPIPath(t *testing.T) {
	var gotAuth, gotPath, gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "ha-token")
	if err != nil {
		t.Fatal(err)
	}
	status, body, err := client.Do(context.Background(), http.MethodGet, "/api/states?limit=1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if status != http.StatusOK || string(body) != `{"ok":true}` {
		t.Fatalf("请求失败: %d %s", status, body)
	}
	if gotAuth != "Bearer ha-token" || gotPath != "/api/states" || gotQuery != "limit=1" {
		t.Fatalf("代理请求错误: auth=%q path=%q query=%q", gotAuth, gotPath, gotQuery)
	}
}

func TestClientRejectsNonAPIPath(t *testing.T) {
	client, err := NewClient("http://127.0.0.1:8123", "token")
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = client.Do(context.Background(), http.MethodGet, "/manifest.json", nil)
	if err == nil {
		t.Fatal("非 /api/ 路径应被拒绝")
	}
}

func TestClientRejectsPathTraversal(t *testing.T) {
	client, err := NewClient("http://127.0.0.1:8123", "token")
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = client.Do(context.Background(), http.MethodGet, "/api/../manifest.json", nil)
	if err == nil {
		t.Fatal("路径穿越应被拒绝")
	}
}

func TestClientHandlesBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"entity_id":"light.test"}` {
			t.Fatalf("body=%s", body)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "token")
	if err != nil {
		t.Fatal(err)
	}
	status, _, err := client.Do(
		context.Background(),
		http.MethodPost,
		"/api/services/light/turn_on",
		[]byte(`{"entity_id":"light.test"}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	if status != http.StatusCreated {
		t.Fatalf("status=%d", status)
	}
}
