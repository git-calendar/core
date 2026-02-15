package core

import (
	"time"
)

func addUnit(t time.Time, value int, unit TimeUnit) time.Time {
	switch unit {
	case Day:
		return t.AddDate(0, 0, value)
	case Week:
		return t.AddDate(0, 0, 7*value)
	case Month:
		return t.AddDate(0, value, 0)
	case Year:
		return t.AddDate(value, 0, 0)
	default:
		return t
	}
}

func getFirstCandidate(searchStart time.Time, event *Event) (time.Time, int) {
	switch event.Repeat.Frequency {
	case Day:
		diffHours := searchStart.Sub(event.From).Hours()
		cycleHours := 24.0 * float64(event.Repeat.Interval)
		cycles := int(diffHours / cycleHours)
		days := cycles * event.Repeat.Interval
		return addUnit(event.From, days, Day), cycles
	case Week:
		diffHours := searchStart.Sub(event.From).Hours()
		cycleHours := 24.0 * 7 * float64(event.Repeat.Interval)
		cycles := int(diffHours / cycleHours)
		weeks := cycles * event.Repeat.Interval
		return addUnit(event.From, weeks, Week), cycles
	case Month:
		diffMonths := (searchStart.Year()-event.From.Year())*12 + int(searchStart.Month()-event.From.Month())
		cycles := diffMonths / event.Repeat.Interval
		months := cycles * event.Repeat.Interval
		candidate := addUnit(event.From, months, Month)
		if candidate.Before(searchStart) {
			candidate = addUnit(event.From, event.Repeat.Interval, Month)
			cycles++
		}
		return candidate, cycles
	case Year:
		diffYears := searchStart.Year() - event.From.Year()
		cycles := diffYears / event.Repeat.Interval
		years := cycles * event.Repeat.Interval
		candidate := addUnit(event.From, years, Year)
		if candidate.Before(searchStart) {
			candidate = addUnit(event.From, event.Repeat.Interval, Year)
			cycles++
		}
		return candidate, cycles
	default:
		return event.From, -1
	}
}
