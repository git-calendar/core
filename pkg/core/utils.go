package core

import "time"

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

func getFirstCandidate2(searchStart time.Time, event *Event) (time.Time, int) {
	switch event.Repeat.Frequency {
	case Day:
		diffHours := searchStart.Sub(event.From).Hours()
		cycleHours := 24.0 * float64(event.Repeat.Interval)
		cycles := int(diffHours / cycleHours)
		days := cycles * int(event.Repeat.Interval)
		return addUnit(event.From, days, Day), cycles
	case Week:
		diffHours := searchStart.Sub(event.From).Hours()
		cycleHours := 24.0 * 7 * float64(event.Repeat.Interval)
		cycles := int(diffHours / cycleHours)
		weeks := cycles * int(event.Repeat.Interval)
		return addUnit(event.From, weeks, Week), cycles
	case Month:
		diffMonths := (searchStart.Year()-event.From.Year())*12 + int(searchStart.Month()-event.From.Month())
		cycles := diffMonths / int(event.Repeat.Interval)
		months := cycles * int(event.Repeat.Interval)
		candidate := addUnit(event.From, months, Month)
		if candidate.Before(searchStart) {
			candidate = addUnit(event.From, int(event.Repeat.Interval), Month)
			cycles++
		}
		return candidate, cycles
	case Year:
		diffYears := searchStart.Year() - event.From.Year()
		cycles := diffYears / int(event.Repeat.Interval)
		years := cycles * int(event.Repeat.Interval)
		candidate := addUnit(event.From, years, Year)
		if candidate.Before(searchStart) {
			candidate = addUnit(event.From, int(event.Repeat.Interval), Year)
			cycles++
		}
		return candidate, cycles
	default:
		return event.From, -1
	}
}

func getFirstCandidate(eventStart, searchStart time.Time, unit TimeUnit) time.Time {
	switch unit {
	case Day:
		days := int(searchStart.Sub(eventStart).Hours() / 24)
		return addUnit(eventStart, days, Day)
	case Week:
		weeks := int(searchStart.Sub(eventStart).Hours() / (24 * 7))
		return addUnit(eventStart, weeks, Week)
	case Month:
		months := (searchStart.Year()-eventStart.Year())*12 + int(searchStart.Month()-eventStart.Month())
		return addUnit(eventStart, months, Month)
	case Year:
		years := searchStart.Year() - eventStart.Year()
		return addUnit(eventStart, years, Year)
	default:
		return eventStart
	}
}

func getEarlierTime(t1, t2 time.Time) time.Time {
	if t1.Before(t2) {
		return t1
	}
	return t2
}
