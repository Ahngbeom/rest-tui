package env

import (
	"math/rand/v2"
	"regexp"
	"strconv"
	"testing"
	"time"
)

var uuidV4RE = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func testNow() time.Time { return time.Date(2026, 10, 30, 18, 0, 0, 0, time.FixedZone("KST", 9*3600)) }

func TestDynamicValue_Formats(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	cases := []struct {
		expr string
		ok   func(string) bool
	}{
		{"$uuid", uuidV4RE.MatchString},
		{"$random.uuid", uuidV4RE.MatchString},
		{"$timestamp", func(s string) bool { return s == "1793350800" }},
		{"$isoTimestamp", func(s string) bool { return s == "2026-10-30T09:00:00Z" }},
		{"$random.alphabetic(12)", regexp.MustCompile(`^[A-Za-z]{12}$`).MatchString},
		{"$random.alphanumeric(12)", regexp.MustCompile(`^[A-Za-z0-9_]{12}$`).MatchString},
		{"$random.hexadecimal(12)", regexp.MustCompile(`^[0-9a-f]{12}$`).MatchString},
		{"$random.alphabetic(0)", func(s string) bool { return s == "" }},
		{"$random.email", regexp.MustCompile(`^[a-z0-9]{8}@example\.com$`).MatchString},
	}
	for _, c := range cases {
		got, ok := dynamicValue(c.expr, testNow(), r)
		if !ok || !c.ok(got) {
			t.Errorf("dynamicValue(%q) = %q, %v", c.expr, got, ok)
		}
	}
}

func TestDynamicValue_IntegerBounds(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	for i := 0; i < 500; i++ {
		s, ok := dynamicValue("$random.integer(5,8)", testNow(), r)
		n, _ := strconv.Atoi(s)
		if !ok || n < 5 || n >= 8 {
			t.Fatalf("$random.integer(5,8) = %q, %v; want 5 <= n < 8", s, ok)
		}
		s, ok = dynamicValue("$randomInt", testNow(), r)
		n, _ = strconv.Atoi(s)
		if !ok || n < 0 || n > 1000 {
			t.Fatalf("$randomInt = %q, %v; want 0..1000", s, ok)
		}
	}
	if s, ok := dynamicValue("$random.integer(-3, -2)", testNow(), r); !ok || s != "-3" {
		t.Errorf("$random.integer(-3, -2) = %q, %v; want -3", s, ok)
	}
}

func TestDynamicValue_RejectsUnknownAndMalformed(t *testing.T) {
	for _, expr := range []string{"$foo", "$env.HOME", "$random.float(1, 2)", "$random.integer(8, 5)", "$random.integer(a, b)", "$random.integer(1)", "$random.integer", "$random.alphabetic(-1)", "$random.alphabetic(4097)", "$random.alphabetic", "$uuid(1)", "$uuid()"} {
		if got, ok := dynamicValue(expr, testNow(), rand.New(rand.NewPCG(1, 2))); ok {
			t.Errorf("dynamicValue(%q) = %q, true; want ok=false", expr, got)
		}
	}
}
