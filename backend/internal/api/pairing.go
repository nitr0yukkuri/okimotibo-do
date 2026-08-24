package api

import (
	"strings"
	"time"
)

const pairingTTL = 10 * time.Minute
const pairingCodeLength = 8

func normalizePairingCode(code string) string {
	return strings.ToUpper(strings.NewReplacer("-", "", " ", "").Replace(strings.TrimSpace(code)))
}
