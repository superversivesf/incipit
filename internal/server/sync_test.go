package server

import (
	"net/http"
	"strings"
	"testing"
)

func TestSyncHealthcheck_NoAuth(t *testing.T) {
	ts := newTestServerWithUser(t)
	defer ts.Close()

	resp := ts.unauthedRequest(t, "GET", "/syncs/healthcheck")
	assertStatus(t, resp, http.StatusOK)

	var body map[string]string
	decodeJSON(t, resp, &body)
	if body["state"] != "OK" {
		t.Errorf("state = %q, want 'OK'", body["state"])
	}
}

func TestSyncAuth_WithCredentials(t *testing.T) {
	ts := newTestServerWithUser(t)
	defer ts.Close()

	resp := ts.authedGet(t, "/syncs/auth")
	assertStatus(t, resp, http.StatusOK)

	var body map[string]string
	decodeJSON(t, resp, &body)
	if body["username"] != "testuser" {
		t.Errorf("username = %q, want 'testuser'", body["username"])
	}
}

func TestSyncAuth_NoCredentials_401(t *testing.T) {
	ts := newTestServerWithUser(t)
	defer ts.Close()

	resp := ts.unauthedRequest(t, "GET", "/syncs/auth")
	assertStatus(t, resp, http.StatusUnauthorized)
}

func TestSyncPutProgress(t *testing.T) {
	ts := newTestServerWithUser(t)
	defer ts.Close()

	resp := ts.authedPutJSON(t, "/syncs/progress/testhash123", map[string]interface{}{
		"percentage": 0.318,
		"progress":   "/body/DocFragment[20]/body/p[22]/img.0",
		"device":     "Kobo",
	})
	assertStatus(t, resp, http.StatusOK)
}

func TestSyncGetProgress(t *testing.T) {
	ts := newTestServerWithUser(t)
	defer ts.Close()

	// PUT progress
	ts.authedPutJSON(t, "/syncs/progress/testhash", map[string]interface{}{
		"percentage": 0.318,
		"progress":   "/body/1",
		"device":     "Kobo",
	})

	// GET it back
	resp := ts.authedGet(t, "/syncs/progress/testhash")
	assertStatus(t, resp, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, resp, &body)
	if body["percentage"] != 0.318 {
		t.Errorf("percentage = %v, want 0.318", body["percentage"])
	}
	if body["device"] != "Kobo" {
		t.Errorf("device = %v, want 'Kobo'", body["device"])
	}
}

func TestSyncGetProgress_NotFound_404(t *testing.T) {
	ts := newTestServerWithUser(t)
	defer ts.Close()

	resp := ts.authedGet(t, "/syncs/progress/nonexistent")
	assertStatus(t, resp, http.StatusNotFound)
}

func TestSyncProgress_OverwriteLatest(t *testing.T) {
	ts := newTestServerWithUser(t)
	defer ts.Close()

	// First save at 30%
	ts.authedPutJSON(t, "/syncs/progress/sharehash", map[string]interface{}{
		"percentage": 0.30,
		"progress":   "/body/1",
		"device":     "Kobo",
	})

	// Second save at 35% — should overwrite
	ts.authedPutJSON(t, "/syncs/progress/sharehash", map[string]interface{}{
		"percentage": 0.35,
		"progress":   "/body/2",
		"device":     "Phone",
	})

	resp := ts.authedGet(t, "/syncs/progress/sharehash")
	var body map[string]interface{}
	decodeJSON(t, resp, &body)
	if body["percentage"] != 0.35 {
		t.Errorf("percentage = %v, want 0.35 (latest writer wins)", body["percentage"])
	}
	if body["device"] != "Phone" {
		t.Errorf("device = %v, want 'Phone'", body["device"])
	}
}

func TestSyncProgress_PerUserIsolation(t *testing.T) {
	ts := newTestServerWithUser(t)
	defer ts.Close()

	// User 1 saves at 50%
	ts.authedPutJSON(t, "/syncs/progress/sharedhash", map[string]interface{}{
		"percentage": 0.50,
		"progress":   "/alice/pos",
		"device":     "Kobo",
	})

	// Create user 2 and save at 75%
	ts.seedUser(t, "user2", "pass2", "user")

	req2, _ := http.NewRequest("PUT", ts.URL+"/syncs/progress/sharedhash",
		strings.NewReader(`{"percentage":0.75,"progress":"/bob/pos","device":"Phone"}`))
	req2.SetBasicAuth("user2", "pass2")
	req2.Header.Set("Content-Type", "application/json")
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()

	// User 1 gets 50%
	resp := ts.authedGet(t, "/syncs/progress/sharedhash")
	var body1 map[string]interface{}
	decodeJSON(t, resp, &body1)
	if body1["percentage"] != 0.50 {
		t.Errorf("user1 percentage = %v, want 0.50", body1["percentage"])
	}

	// User 2 gets 75%
	req3, _ := http.NewRequest("GET", ts.URL+"/syncs/progress/sharedhash", nil)
	req3.SetBasicAuth("user2", "pass2")
	resp3, err := http.DefaultClient.Do(req3)
	if err != nil {
		t.Fatal(err)
	}
	var body2 map[string]interface{}
	decodeJSON(t, resp3, &body2)
	if body2["percentage"] != 0.75 {
		t.Errorf("user2 percentage = %v, want 0.75", body2["percentage"])
	}
}
