//go:build coturn

package turncred

import (
	"bytes"
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha1"
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

const (
	stunMagic         = 0x2112A442
	stunAllocate      = 0x0003
	stunRefresh       = 0x0004
	stunAttrUsername  = 0x0006
	stunAttrErrorCode = 0x0009
	stunAttrRealm     = 0x0014
	stunAttrNonce     = 0x0015
	stunAttrLifetime  = 0x000D
	stunAttrIntegrity = 0x0008
	stunAttrReqTrans  = 0x0019
)

func TestRefreshAfterExpiry(t *testing.T) {
	root := repoRoot(t)
	bin := os.Getenv("TURNSERVER_BIN")
	if bin == "" {
		bin = filepath.Join(root, "build-coturn", "bin", "turnserver")
	}
	if _, err := os.Stat(bin); err != nil {
		t.Skip("turnserver not built; run ./scripts/build-coturn.sh")
	}

	dir := t.TempDir()
	secretPath := filepath.Join(dir, "secret")
	confPath := filepath.Join(dir, "turnserver.conf")
	if err := os.WriteFile(secretPath, []byte("compat-secret"), 0640); err != nil {
		t.Fatal(err)
	}
	conf := `listening-ip=127.0.0.1
listening-port=34780
use-auth-secret
realm=raypx2
no-tcp
no-tls
no-tcp-relay
user-quota=2
`
	if err := os.WriteFile(confPath, []byte(conf), 0644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(bin, "-c", confPath, "--static-auth-secret=compat-secret")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() }()
	time.Sleep(400 * time.Millisecond)

	expiry := time.Now().Add(2 * time.Second).Unix()
	user := RESTUsername(expiry, "sessR", "conn-1", 1)
	pass := RESTPassword([]byte("compat-secret"), user)

	conn, err := net.Dial("udp", "127.0.0.1:34780")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	realm, nonce := mustAllocateUnauth(t, conn)
	realm, nonce, err = allocateAuth(t, conn, user, pass, realm, nonce)
	if err != nil {
		t.Fatalf("allocate: %v", err)
	}
	time.Sleep(3 * time.Second)
	err = refreshAuth(t, conn, user, pass, realm, nonce)
	if err != nil {
		t.Logf("REFRESH_COMPAT=fail sha=7c24c88a4c13ef79edce9e645bef578eb7e5a6ad err=%v", err)
		t.Fatalf("authenticated Refresh after expiry failed: %v", err)
	}
	t.Log("REFRESH_COMPAT=ok sha=7c24c88a4c13ef79edce9e645bef578eb7e5a6ad")
}

func mustAllocateUnauth(t *testing.T, conn net.Conn) (realm, nonce string) {
	t.Helper()
	msg := stunRequest(stunAllocate, []stunAttr{{stunAttrReqTrans, []byte{17, 0, 0, 0}}})
	if _, err := conn.Write(msg); err != nil {
		t.Fatal(err)
	}
	resp := readSTUN(t, conn)
	code := stunErrorCode(resp)
	if code != 401 {
		t.Fatalf("expected 401, got %d", code)
	}
	return string(findStunAttr(resp, stunAttrRealm)), string(findStunAttr(resp, stunAttrNonce))
}

func allocateAuth(t *testing.T, conn net.Conn, user, pass, realm, nonce string) (string, string, error) {
	t.Helper()
	for attempt := 0; attempt < 2; attempt++ {
		attrs := []stunAttr{
			{stunAttrReqTrans, []byte{17, 0, 0, 0}},
			{stunAttrUsername, []byte(user)},
			{stunAttrRealm, []byte(realm)},
			{stunAttrNonce, []byte(nonce)},
		}
		msg := stunRequestWithIntegrity(stunAllocate, attrs, user, realm, pass)
		if _, err := conn.Write(msg); err != nil {
			return realm, nonce, err
		}
		resp := readSTUN(t, conn)
		code := stunErrorCode(resp)
		if code == 0 {
			return realm, nonce, nil
		}
		if code == 438 {
			realm, nonce = string(findStunAttr(resp, stunAttrRealm)), string(findStunAttr(resp, stunAttrNonce))
			continue
		}
		return realm, nonce, fmt.Errorf("allocate error %d", code)
	}
	return realm, nonce, fmt.Errorf("allocate stale nonce retry exhausted")
}

func refreshAuth(t *testing.T, conn net.Conn, user, pass, realm, nonce string) error {
	t.Helper()
	lifetime := make([]byte, 4)
	binary.BigEndian.PutUint32(lifetime, 600)
	for attempt := 0; attempt < 2; attempt++ {
		attrs := []stunAttr{
			{stunAttrLifetime, lifetime},
			{stunAttrUsername, []byte(user)},
			{stunAttrRealm, []byte(realm)},
			{stunAttrNonce, []byte(nonce)},
		}
		msg := stunRequestWithIntegrity(stunRefresh, attrs, user, realm, pass)
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		if _, err := conn.Write(msg); err != nil {
			return err
		}
		resp := readSTUN(t, conn)
		code := stunErrorCode(resp)
		if code == 0 {
			return nil
		}
		if code == 438 {
			if v := findStunAttr(resp, stunAttrRealm); len(v) > 0 {
				realm = string(v)
			}
			if v := findStunAttr(resp, stunAttrNonce); len(v) > 0 {
				nonce = string(v)
			}
			continue
		}
		return fmt.Errorf("refresh error %d", code)
	}
	return fmt.Errorf("refresh stale nonce retry exhausted")
}

type stunAttr struct {
	typ uint16
	val []byte
}

func stunRequest(typ uint16, attrs []stunAttr) []byte {
	var body bytes.Buffer
	for _, a := range attrs {
		writeAttr(&body, a.typ, a.val)
	}
	return stunHeader(typ, body.Bytes(), nil)
}

func stunRequestWithIntegrity(typ uint16, attrs []stunAttr, user, realm, pass string) []byte {
	var body bytes.Buffer
	for _, a := range attrs {
		writeAttr(&body, a.typ, a.val)
	}
	writeAttr(&body, stunAttrIntegrity, make([]byte, 20))
	raw := stunHeader(typ, body.Bytes(), nil)
	key := md5.Sum([]byte(user + ":" + realm + ":" + pass))
	mac := hmac.New(sha1.New, key[:])
	// coturn HMAC covers the message up to (but excluding) the entire
	// MESSAGE-INTEGRITY attribute: 4-byte header + 20-byte value.
	_, _ = mac.Write(raw[:len(raw)-24])
	copy(raw[len(raw)-20:], mac.Sum(nil))
	return raw
}

func stunHeader(typ uint16, body, tid []byte) []byte {
	if tid == nil {
		tid = make([]byte, 12)
		_, _ = rand.Read(tid)
	}
	raw := make([]byte, 20+len(body))
	binary.BigEndian.PutUint16(raw[0:2], typ)
	binary.BigEndian.PutUint16(raw[2:4], uint16(len(body)))
	binary.BigEndian.PutUint32(raw[4:8], stunMagic)
	copy(raw[8:20], tid)
	copy(raw[20:], body)
	return raw
}

func writeAttr(buf *bytes.Buffer, typ uint16, val []byte) {
	var hdr [4]byte
	binary.BigEndian.PutUint16(hdr[0:2], typ)
	binary.BigEndian.PutUint16(hdr[2:4], uint16(len(val)))
	buf.Write(hdr[:])
	buf.Write(val)
	if pad := (4 - len(val)%4) % 4; pad > 0 {
		buf.Write(make([]byte, pad))
	}
}

func readSTUN(t *testing.T, conn net.Conn) []byte {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 2048)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	return buf[:n]
}

func findStunAttr(msg []byte, typ uint16) []byte {
	for i := 20; i+4 <= len(msg); {
		at := binary.BigEndian.Uint16(msg[i : i+2])
		al := int(binary.BigEndian.Uint16(msg[i+2 : i+4]))
		start := i + 4
		end := start + al
		if end > len(msg) {
			return nil
		}
		if at == typ {
			return msg[start:end]
		}
		i = end + (4-al%4)%4
	}
	return nil
}

func stunErrorCode(msg []byte) int {
	val := findStunAttr(msg, stunAttrErrorCode)
	if len(val) < 4 {
		return 0
	}
	return int(val[2])*100 + int(val[3])
}
