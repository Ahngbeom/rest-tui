package executor

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bahn/rest-tui/internal/httpfile"
)

func TestExecute_GetCapturesStatusHeadersAndBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("server saw method %q, want GET", r.Method)
		}
		w.Header().Set("X-Custom", "yes")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	req := httpfile.Request{Method: "GET", URL: srv.URL}

	resp, err := Execute(context.Background(), req, 5*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("StatusCode = %d, want %d", resp.StatusCode, http.StatusCreated)
	}
	if string(resp.Body) != `{"ok":true}` {
		t.Errorf("Body = %q", resp.Body)
	}
	if got := findHeader(resp.Headers, "X-Custom"); got != "yes" {
		t.Errorf("X-Custom header = %q, want yes", got)
	}
	if resp.Duration <= 0 {
		t.Errorf("Duration = %v, want > 0", resp.Duration)
	}
}

func TestExecute_PostSendsHeadersAndBody(t *testing.T) {
	var gotBody []byte
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	req := httpfile.Request{
		Method: "POST",
		URL:    srv.URL,
		Headers: []httpfile.Header{
			{Name: "Authorization", Value: "Bearer secret"},
			{Name: "Content-Type", Value: "application/json"},
		},
		Body: `{"name":"Bob"}`,
	}

	resp, err := Execute(context.Background(), req, 5*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d", resp.StatusCode)
	}
	if gotAuth != "Bearer secret" {
		t.Errorf("server saw Authorization %q", gotAuth)
	}
	if string(gotBody) != `{"name":"Bob"}` {
		t.Errorf("server saw body %q", gotBody)
	}
}

func TestExecute_TimeoutReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	req := httpfile.Request{Method: "GET", URL: srv.URL}

	_, err := Execute(context.Background(), req, 10*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

func TestExecute_ConnectionRefusedReturnsError(t *testing.T) {
	req := httpfile.Request{Method: "GET", URL: "http://127.0.0.1:1"}

	_, err := Execute(context.Background(), req, 2*time.Second)
	if err == nil {
		t.Fatal("expected connection error, got nil")
	}
}

func findHeader(headers []httpfile.Header, name string) string {
	for _, h := range headers {
		if h.Name == name {
			return h.Value
		}
	}
	return ""
}
