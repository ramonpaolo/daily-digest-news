package main

import "testing"

func TestParseSchedule(t *testing.T) {
	hour, minute, err := parseSchedule("08:05")
	if err != nil || hour != 8 || minute != 5 {
		t.Fatalf("parseSchedule() = %d:%d, %v; want 8:5", hour, minute, err)
	}
	if _, _, err := parseSchedule("invalid"); err == nil {
		t.Fatal("parseSchedule(invalid) succeeded, want error")
	}
}
