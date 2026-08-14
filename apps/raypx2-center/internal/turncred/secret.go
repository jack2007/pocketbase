package turncred

import (
	"fmt"
	"os"
	"strings"
)

func LoadSharedSecret(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode().Perm()&0o004 != 0 {
		return nil, fmt.Errorf("secret file %s is world-readable (mode %04o)", path, info.Mode().Perm())
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	secret := strings.TrimRight(string(raw), "\n")
	if secret == "" {
		return nil, fmt.Errorf("secret file %s is empty", path)
	}
	return []byte(secret), nil
}
