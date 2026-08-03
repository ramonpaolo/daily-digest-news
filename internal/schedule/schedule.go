package schedule

import "time"

func Next(now time.Time, hour, minute int) time.Time {
	today := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
	if now.Before(today) {
		return today
	}
	return today.AddDate(0, 0, 1)
}
