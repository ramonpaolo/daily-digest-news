package schedule

import (
	"testing"
	"time"
)

func TestNextReturnsTodayWhenScheduleHasNotPassed(t *testing.T) {
	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 3, 7, 30, 0, 0, loc)

	next := Next(now, 8, 0)
	want := time.Date(2026, 8, 3, 8, 0, 0, 0, loc)
	if !next.Equal(want) {
		t.Fatalf("Next() = %v, want %v", next, want)
	}
}

func TestNextReturnsTomorrowAfterSchedule(t *testing.T) {
	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 3, 8, 1, 0, 0, loc)

	next := Next(now, 8, 0)
	want := time.Date(2026, 8, 4, 8, 0, 0, 0, loc)
	if !next.Equal(want) {
		t.Fatalf("Next() = %v, want %v", next, want)
	}
}
