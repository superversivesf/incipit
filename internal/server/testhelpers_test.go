package server

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/jason/incipit/internal/config"
	"github.com/jason/incipit/internal/db"
	"github.com/jason/incipit/internal/models"
	"golang.org/x/crypto/bcrypt"
)

type testServer struct {
	*httptest.Server
	database *db.DB
}

func newTestServer(t *testing.T) *testServer {
	t.Helper()
	cfg := config.Config{
		DBPath:     filepath.Join(t.TempDir(), "test.db"),
		Port:       "0",
		StorageDir: t.TempDir(),
	}
	srv, err := New(cfg)
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	return &testServer{
		Server:   httptest.NewServer(srv.Handler),
		database: srv.DB,
	}
}

func newTestServerWithUser(t *testing.T) *testServer {
	t.Helper()
	ts := newTestServer(t)
	ts.seedUser(t, "testuser", "testpass", "user")
	return ts
}

func newTestServerWithAdmin(t *testing.T) *testServer {
	t.Helper()
	ts := newTestServer(t)
	ts.seedUser(t, "admin", "adminpass", "admin")
	return ts
}

func (ts *testServer) seedUser(t *testing.T, username, password, role string) {
	t.Helper()
	md5sum := md5.Sum([]byte(password))
	md5hex := hex.EncodeToString(md5sum[:])
	bcryptHash, err := bcrypt.GenerateFromPassword([]byte(md5hex), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	if _, err := ts.database.CreateUser(username, string(bcryptHash), role); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
}

func (ts *testServer) seedBook(t *testing.T, b models.Book) int64 {
	t.Helper()
	id, err := ts.database.InsertBook(&b)
	if err != nil {
		t.Fatalf("InsertBook: %v", err)
	}
	return id
}

func (ts *testServer) authedGet(t *testing.T, path string) *http.Response {
	t.Helper()
	return ts.authedRequest(t, "GET", path, nil, "")
}

func (ts *testServer) authedPostJSON(t *testing.T, path string, body interface{}) *http.Response {
	t.Helper()
	data, _ := json.Marshal(body)
	return ts.authedRequest(t, "POST", path, bytes.NewReader(data), "application/json")
}

func (ts *testServer) authedPutJSON(t *testing.T, path string, body interface{}) *http.Response {
	t.Helper()
	data, _ := json.Marshal(body)
	return ts.authedRequest(t, "PUT", path, bytes.NewReader(data), "application/json")
}

func (ts *testServer) authedDelete(t *testing.T, path string) *http.Response {
	t.Helper()
	return ts.authedRequest(t, "DELETE", path, nil, "")
}

func (ts *testServer) authedRequest(t *testing.T, method, path string, body io.Reader, contentType string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(method, ts.URL+path, body)
	req.SetBasicAuth("testuser", "testpass")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	return resp
}

func (ts *testServer) unauthedRequest(t *testing.T, method, path string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(method, ts.URL+path, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	return resp
}

func readBody(t *testing.T, resp *http.Response) []byte {
	t.Helper()
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatalf("readBody: %v", err)
	}
	return body
}

func decodeJSON(t *testing.T, resp *http.Response, v interface{}) {
	t.Helper()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatalf("decodeJSON: %v", err)
	}
	resp.Body.Close()
}

func assertStatus(t *testing.T, resp *http.Response, want int) {
	t.Helper()
	if resp.StatusCode != want {
		t.Errorf("status = %d, want %d", resp.StatusCode, want)
	}
}

func bookURL(id int64) string {
	return "/api/books/" + strconv.FormatInt(id, 10)
}

var _ = context.Background
