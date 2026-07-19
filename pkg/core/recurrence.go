package core

import (
	"errors"
	"fmt"
	"slices"
	"time"

	rrule "github.com/teambition/rrule-go"
)

func validateRecurrence(set *rrule.Set, dtstart time.Time) error {
	if set == nil {
		return nil
	}
	set.DTStart(dtstart)
	rule := set.GetRRule()
	if rule == nil {
		return errors.New("recurrence requires an RRULE")
	}
	option := rule.OrigOptions
	if option.Count == 0 && option.Until.IsZero() {
		return errors.New("RRULE requires COUNT or UNTIL")
	}
	if option.Count != 0 && !option.Until.IsZero() {
		return errors.New("RRULE cannot contain both COUNT and UNTIL")
	}
	return nil
}

func recurrenceBetween(set *rrule.Set, from, to time.Time) []time.Time {
	times := set.Between(from, to, true)
	if len(times) != 0 && times[len(times)-1].Equal(to) {
		times = times[:len(times)-1]
	}
	return times
}

func recurrenceIndex(set *rrule.Set, at time.Time) (int, bool) {
	rule := set.GetRRule()
	times := rule.Between(rule.GetDTStart(), at, true)
	if len(times) == 0 || !times[len(times)-1].Equal(at.Truncate(time.Second)) {
		return -1, false
	}
	return len(times) - 1, true
}

func recurrenceLast(set *rrule.Set) time.Time {
	times := set.GetRRule().All()
	if len(times) == 0 {
		return time.Time{}
	}
	return times[len(times)-1]
}

func capRecurrenceBefore(set *rrule.Set, cutoff time.Time) (*rrule.Set, error) {
	rule := set.GetRRule()
	previous := rule.Before(cutoff, false)
	if previous.IsZero() {
		return nil, fmt.Errorf("no occurrence before %s", cutoff)
	}

	option := rule.OrigOptions
	option.Count = 0
	option.Until = previous
	before, _ := splitTimes(set.GetExDate(), cutoff)

	if previous.Equal(rule.GetDTStart()) && !containsTime(before, previous) {
		return nil, nil
	}
	return newRecurrence(option, before)
}

func splitRecurrence(set *rrule.Set, splitAt, newStart time.Time, replacement *rrule.Set) (*rrule.Set, *rrule.Set, int, error) {
	index, ok := recurrenceIndex(set, splitAt)
	if !ok {
		return nil, nil, -1, fmt.Errorf("%s is not an occurrence", splitAt)
	}
	if index == 0 {
		return nil, nil, 0, nil
	}
	before, err := capRecurrenceBefore(set, splitAt)
	if err != nil {
		return nil, nil, -1, err
	}

	option := replacement.GetRRule().OrigOptions
	if count := set.GetRRule().OrigOptions.Count; count != 0 {
		option.Count = max(count-index, 1)
		option.Until = time.Time{}
	}

	_, future := splitTimes(set.GetExDate(), splitAt)
	shiftTimes(future, newStart.Sub(splitAt))
	after, err := newRecurrence(option, future)
	if err != nil {
		return nil, nil, -1, err
	}
	return before, after, index, validateRecurrence(after, newStart)
}

func shiftRecurrence(set *rrule.Set, oldStart, newStart time.Time, replacement *rrule.Set) (*rrule.Set, error) {
	if replacement == nil {
		replacement = set
	}
	next, err := rrule.StrToRRuleSet(replacement.String())
	if err != nil {
		return nil, err
	}
	exceptions := slices.Clone(set.GetExDate())
	shiftTimes(exceptions, newStart.Sub(oldStart))
	next.SetExDates(exceptions)
	return next, validateRecurrence(next, newStart)
}

func newRecurrence(option rrule.ROption, exceptions []time.Time) (*rrule.Set, error) {
	rule, err := rrule.NewRRule(option)
	if err != nil {
		return nil, err
	}
	set := &rrule.Set{}
	set.RRule(rule)
	set.SetExDates(exceptions)
	return set, nil
}

func splitTimes(times []time.Time, cutoff time.Time) (before, after []time.Time) {
	for _, at := range times {
		if at.Before(cutoff) {
			before = append(before, at)
		} else {
			after = append(after, at)
		}
	}
	return before, after
}

func shiftTimes(times []time.Time, shift time.Duration) {
	for i := range times {
		times[i] = times[i].Add(shift)
	}
}

func containsTime(times []time.Time, want time.Time) bool {
	return slices.ContainsFunc(times, func(at time.Time) bool { return at.Equal(want) })
}
