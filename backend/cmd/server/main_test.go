package main

import (
	"testing"
	"time"
)

func TestDurationEnvironment(t *testing.T) {
	t.Setenv("TEST_REFRESH_INTERVAL", "")
	duration, err := durationEnvironment("TEST_REFRESH_INTERVAL", 15*time.Minute)
	if err != nil || duration != 15*time.Minute {
		t.Fatalf("fallback duration = %v, err = %v", duration, err)
	}

	t.Setenv("TEST_REFRESH_INTERVAL", "30m")
	duration, err = durationEnvironment("TEST_REFRESH_INTERVAL", 15*time.Minute)
	if err != nil || duration != 30*time.Minute {
		t.Fatalf("configured duration = %v, err = %v", duration, err)
	}

	for _, value := range []string{"invalid", "0s", "-1m"} {
		t.Setenv("TEST_REFRESH_INTERVAL", value)
		if _, err := durationEnvironment("TEST_REFRESH_INTERVAL", 15*time.Minute); err == nil {
			t.Fatalf("durationEnvironment(%q) did not return an error", value)
		}
	}
}
