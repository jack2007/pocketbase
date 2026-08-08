package crypto_test

import (
	"testing"

	centercrypto "github.com/pocketbase/pocketbase/apps/raypx2-center/internal/crypto"
)

func TestGenerateAndVerifyEnrollSecret(t *testing.T) {
	plain, hash, err := centercrypto.GenerateEnrollSecret()
	if err != nil || plain == "" || hash == "" {
		t.Fatalf("GenerateEnrollSecret() = (%q, %q, %v)", plain, hash, err)
	}
	if !centercrypto.VerifySecret(hash, plain) {
		t.Fatal("VerifySecret rejected generated secret")
	}
	if centercrypto.VerifySecret(hash, "wrong") {
		t.Fatal("VerifySecret accepted wrong secret")
	}
}

func TestHashTokenUsesBcrypt(t *testing.T) {
	hash, err := centercrypto.HashToken("session-token")
	if err != nil {
		t.Fatal(err)
	}
	if hash == "session-token" {
		t.Fatal("HashToken returned plaintext")
	}
	if !centercrypto.VerifySecret(hash, "session-token") {
		t.Fatal("VerifySecret rejected token hash")
	}
}
