package server

import (
	"crypto/md5"
	"encoding/hex"
	"net/http"
	"testing"
)

func TestHealth_NoAuth_200(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	resp := ts.unauthedRequest(t, "GET", "/health")
	assertStatus(t, resp, http.StatusOK)
}

func TestHealth_ReturnsJSON(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	resp := ts.unauthedRequest(t, "GET", "/health")
	assertStatus(t, resp, http.StatusOK)

	var body map[string]string
	decodeJSON(t, resp, &body)
	if body["status"] != "ok" {
		t.Errorf("status = %q, want %q", body["status"], "ok")
	}
}

func TestAuth_NoCredentials_401(t *testing.T) {
	ts := newTestServerWithUser(t)
	defer ts.Close()

	resp := ts.unauthedRequest(t, "GET", "/api/books")
	assertStatus(t, resp, http.StatusUnauthorized)
	if resp.Header.Get("WWW-Authenticate") == "" {
		t.Error("missing WWW-Authenticate header")
	}
}

func TestAuth_ValidCredentials_200(t *testing.T) {
	ts := newTestServerWithUser(t)
	defer ts.Close()

	resp := ts.authedGet(t, "/api/books")
	assertStatus(t, resp, http.StatusOK)
}

func TestAuth_WrongPassword_401(t *testing.T) {
	ts := newTestServerWithUser(t)
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/books", nil)
	req.SetBasicAuth("testuser", "wrongpass")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusUnauthorized)
}

func TestAuth_NonexistentUser_401(t *testing.T) {
	ts := newTestServerWithUser(t)
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/books", nil)
	req.SetBasicAuth("nobody", "testpass")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusUnauthorized)
}

func TestAuth_PlaintextPasswordWorks(t *testing.T) {
	ts := newTestServerWithUser(t)
	defer ts.Close()

	// Browser sends plaintext, server should MD5-hash it and compare
	req, _ := http.NewRequest("GET", ts.URL+"/api/books", nil)
	req.SetBasicAuth("testuser", "testpass")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)
}

func TestAuth_MD5HashPasswordWorks(t *testing.T) {
	ts := newTestServerWithUser(t)
	defer ts.Close()

	// KOReader sends md5(password) directly
	req, _ := http.NewRequest("GET", ts.URL+"/api/books", nil)
	req.SetBasicAuth("testuser", md5Of("testpass"))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)
}

func md5Of(s string) string {
	h := md5.Sum([]byte(s))
	return hex.EncodeToString(h[:])
}

func TestAuth_HealthAndSyncHealthcheck_NoAuth(t *testing.T) {
	ts := newTestServerWithUser(t)
	defer ts.Close()

	resp := ts.unauthedRequest(t, "GET", "/health")
	assertStatus(t, resp, http.StatusOK)

	resp = ts.unauthedRequest(t, "GET", "/syncs/healthcheck")
	assertStatus(t, resp, http.StatusOK)
}
