package core

import "time"

func addUnit(t time.Time, value int, unit TimeUnit) time.Time {
	switch unit {
	case Day:
		return t.AddDate(0, 0, value)
	case Week:
		return t.AddDate(0, 0, 7*value)
	case TwoWeeks:
		return t.AddDate(0, 0, 14*value)
	case Month:
		return t.AddDate(0, value, 0)
	case Year:
		return t.AddDate(value, 0, 0)
	default:
		return t
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
	case TwoWeeks:
		twoWeeks := int(searchStart.Sub(eventStart).Hours() / (24 * 14))
		return addUnit(eventStart, twoWeeks, TwoWeeks)
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
