package turncred

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestRESTUsernameFormat(t *testing.T) {
	got := RESTUsername(1710000000, "sessA", "conn-7", 2)
	if got != "1710000000:sessA_conn-7_2" {
		t.Fatalf("got %q", got)
	}
	if strings.Count(got, ":") != 1 {
		t.Fatalf("userid must not contain extra colons: %q", got)
	}
}

func TestRESTPasswordVector(t *testing.T) {
	username := RESTUsername(1710000000, "sessA", "conn-7", 2)
	got := RESTPassword([]byte("shared-secret"), username)
	decoded, err := base64.StdEncoding.DecodeString(got)
	if err != nil || len(decoded) != 20 {
		t.Fatalf("password %q is not 20-byte base64 hmac: %v", got, err)
	}
	if got != RESTPassword([]byte("shared-secret"), username) {
		t.Fatal("password must be deterministic")
	}
	user, pass := IssueREST([]byte("shared-secret"), "sessA", "conn-7", 2, 1710000000-86400, 86400)
	if user != username || pass != got {
		t.Fatalf("IssueREST=%q,%q", user, pass)
	}
}
