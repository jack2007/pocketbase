package turncred

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
)

func RESTUsername(expiryUnix int64, sessionID, connectionID string, epoch uint64) string {
	return fmt.Sprintf("%d:%s_%s_%d", expiryUnix, sessionID, connectionID, epoch)
}

func RESTPassword(secret []byte, username string) string {
	mac := hmac.New(sha1.New, secret)
	_, _ = mac.Write([]byte(username))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func IssueREST(secret []byte, sessionID, connectionID string, epoch uint64, nowUnix, ttlSeconds int64) (username, password string) {
	username = RESTUsername(nowUnix+ttlSeconds, sessionID, connectionID, epoch)
	password = RESTPassword(secret, username)
	return username, password
}
