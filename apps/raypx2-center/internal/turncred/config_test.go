package turncred

import "testing"

func TestValidateConfigRequiresUDPTransport(t *testing.T) {
	err := ValidateConfig(Config{
		TURNURLs: []string{"turn:turn.example.com:3478"},
	})
	if err == nil {
		t.Fatal("expected error for TURN URL without transport=udp")
	}
}

func TestValidateConfigAcceptsUDPTurnAndPublicSTUN(t *testing.T) {
	err := ValidateConfig(Config{
		STUNURLs: []string{"stun:turn.example.com:3478"},
		TURNURLs: []string{"turn:turn.example.com:3478?transport=udp"},
		Realm:    DefaultRealm,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestValidateConfigRejectsLoopbackHost(t *testing.T) {
	err := ValidateConfig(Config{
		TURNURLs: []string{"turn:127.0.0.1:3478?transport=udp"},
	})
	if err == nil {
		t.Fatal("expected error for loopback TURN host")
	}
}

func TestEvaluateTURNDisablesWhenSecretMissing(t *testing.T) {
	enabled, secret, reason, err := EvaluateTURN(Config{
		TURNURLs:         []string{"turn:turn.example.com:3478?transport=udp"},
		SharedSecretFile: "/no/such/coturn-rest-secret",
	})
	if err != nil {
		t.Fatalf("missing secret must not be fatal: %v", err)
	}
	if enabled || secret != nil || reason == "" {
		t.Fatalf("enabled=%v secret=%v reason=%q", enabled, secret, reason)
	}
}
