package turncred

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

const (
	DefaultRealm      = "raypx2"
	DefaultSecretFile = "/run/secrets/coturn-rest-secret"
)

type Config struct {
	STUNURLs             []string
	TURNURLs             []string
	SharedSecretFile     string
	CredentialTTLSeconds int
	Realm                string
}

func ValidateConfig(cfg Config) error {
	for _, raw := range cfg.STUNURLs {
		if err := validateURI(raw, "stun", false); err != nil {
			return err
		}
	}
	for _, raw := range cfg.TURNURLs {
		if err := validateURI(raw, "turn", true); err != nil {
			return err
		}
	}
	return nil
}

func EvaluateTURN(cfg Config) (enabled bool, secret []byte, reason string, err error) {
	if err := ValidateConfig(cfg); err != nil {
		return false, nil, "", err
	}
	path := cfg.SharedSecretFile
	if path == "" {
		return false, nil, "shared_secret_file empty", nil
	}
	secret, err = LoadSharedSecret(path)
	if err != nil {
		return false, nil, err.Error(), nil
	}
	return true, secret, "", nil
}

func validateURI(raw, wantScheme string, requireUDP bool) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid %s URL %q: %w", wantScheme, raw, err)
	}
	if !strings.EqualFold(u.Scheme, wantScheme) {
		return fmt.Errorf("URL %q must use scheme %s", raw, wantScheme)
	}
	host := u.Hostname()
	if host == "" && u.Opaque != "" {
		opaque := u.Opaque
		if i := strings.Index(opaque, "?"); i >= 0 {
			opaque = opaque[:i]
		}
		if h, _, splitErr := net.SplitHostPort(opaque); splitErr == nil {
			host = h
		} else {
			host = opaque
		}
	}
	if host == "" || host == "127.0.0.1" || host == "::1" || host == "localhost" {
		return fmt.Errorf("URL %q must use a public host, not loopback", raw)
	}
	if requireUDP {
		if u.Query().Get("transport") != "udp" {
			return fmt.Errorf("TURN URL %q must include transport=udp", raw)
		}
	}
	return nil
}
