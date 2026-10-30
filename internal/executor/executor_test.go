package executor

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Ahngbeom/rest-tui/internal/httpfile"
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
	if resp.Redirects != nil {
		t.Errorf("Redirects = %v, want nil", resp.Redirects)
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
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want errors.Is DeadlineExceeded", err)
	}
}

func TestExecute_ConnectionRefusedReturnsError(t *testing.T) {
	req := httpfile.Request{Method: "GET", URL: "http://127.0.0.1:1"}

	_, err := Execute(context.Background(), req, 2*time.Second)
	if err == nil {
		t.Fatal("expected connection error, got nil")
	}
}

func TestExecute_RecordsRedirectHops(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/a", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/b", http.StatusFound) })
	mux.HandleFunc("/b", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/c", http.StatusTemporaryRedirect)
	})
	mux.HandleFunc("/c", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := Execute(context.Background(), httpfile.Request{Method: "POST", URL: srv.URL + "/a", Body: "x"}, 2*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want 200", resp.StatusCode)
	}
	want := []string{"302 → GET " + srv.URL + "/b", "307 → GET " + srv.URL + "/c"}
	if !reflect.DeepEqual(resp.Redirects, want) {
		t.Errorf("Redirects = %v, want %v", resp.Redirects, want)
	}
}

func TestExecute_RedirectLimitUnchanged(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/loop", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/loop", http.StatusFound) })
	srv := httptest.NewServer(mux)
	defer srv.Close()

	_, err := Execute(context.Background(), httpfile.Request{Method: "GET", URL: srv.URL + "/loop"}, 2*time.Second)
	if err == nil || !strings.Contains(err.Error(), "stopped after 10 redirects") {
		t.Fatalf("err = %v, want 'stopped after 10 redirects'", err)
	}
}

func slowBodyServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	}))
}

func TestExecute_SlowBodyTimeoutIsDeadlineExceeded(t *testing.T) {
	srv := slowBodyServer(t)
	defer srv.Close()

	_, err := Execute(context.Background(), httpfile.Request{Method: "GET", URL: srv.URL}, 50*time.Millisecond)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want errors.Is DeadlineExceeded", err)
	}
}

func TestExecute_CanceledDuringBodyIsCanceled(t *testing.T) {
	srv := slowBodyServer(t)
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(50*time.Millisecond, cancel)
	_, err := Execute(ctx, httpfile.Request{Method: "GET", URL: srv.URL}, 5*time.Second)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want errors.Is Canceled", err)
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
