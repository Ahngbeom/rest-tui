package env

import (
	"fmt"
	"math/rand/v2"
	"strconv"
	"strings"
	"time"
)

const (
	alphabeticChars   = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	alphanumericChars = alphabeticChars + "0123456789_"
	hexadecimalChars  = "0123456789abcdef"
	emailLocalChars   = "abcdefghijklmnopqrstuvwxyz0123456789"
	maxRandomLength   = 4096
)

// dynamicValue generates the value of an IntelliJ HTTP Client dynamic
// variable such as "$uuid" or "$random.integer(1, 10)". It reports false for
// unknown names and malformed or out-of-range arguments.
func dynamicValue(expr string, now time.Time, r *rand.Rand) (string, bool) {
	name, rest, hasArgs := strings.Cut(expr, "(")
	var args []int
	if hasArgs {
		inner, ok := strings.CutSuffix(rest, ")")
		if !ok {
			return "", false
		}
		for _, a := range strings.Split(inner, ",") {
			n, err := strconv.Atoi(strings.TrimSpace(a))
			if err != nil {
				return "", false
			}
			args = append(args, n)
		}
	}

	switch name {
	case "$uuid", "$random.uuid", "$timestamp", "$isoTimestamp", "$randomInt", "$random.email":
		if hasArgs {
			return "", false
		}
	case "$random.alphabetic", "$random.alphanumeric", "$random.hexadecimal":
		if len(args) != 1 || args[0] < 0 || args[0] > maxRandomLength {
			return "", false
		}
	case "$random.integer":
		if len(args) != 2 || args[0] >= args[1] {
			return "", false
		}
	default:
		return "", false
	}

	switch name {
	case "$uuid", "$random.uuid":
		return randomUUID(r), true
	case "$timestamp":
		return strconv.FormatInt(now.Unix(), 10), true
	case "$isoTimestamp":
		return now.UTC().Format(time.RFC3339), true
	case "$randomInt":
		return strconv.Itoa(r.IntN(1001)), true
	case "$random.integer":
		lo, hi := int64(args[0]), int64(args[1])
		return strconv.FormatInt(lo+int64(r.Uint64N(uint64(hi-lo))), 10), true
	case "$random.alphabetic":
		return randomString(r, alphabeticChars, args[0]), true
	case "$random.alphanumeric":
		return randomString(r, alphanumericChars, args[0]), true
	case "$random.hexadecimal":
		return randomString(r, hexadecimalChars, args[0]), true
	case "$random.email":
		return randomString(r, emailLocalChars, 8) + "@example.com", true
	}
	return "", false
}

func randomString(r *rand.Rand, chars string, n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = chars[r.IntN(len(chars))]
	}
	return string(b)
}

func randomUUID(r *rand.Rand) string {
	var b [16]byte
	for i := range b {
		b[i] = byte(r.Uint32())
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
