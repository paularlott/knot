package spacejobs

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// cronSchedule is a parsed 5-field cron expression (minute hour day-of-month
// month day-of-week) supporting "*", numbers, ranges (a-b), lists (a,b,c) and
// steps (*/n, a-b/n). Day and month names are not supported. Times are
// evaluated in the location of the time passed in, so the agent schedules in
// the container's local timezone (TZ from the user's profile).
type cronSchedule struct {
	minutes  uint64 // bit i set = minute i matches, 0..59
	hours    uint64 // bit i set = hour i matches, 0..23
	days     uint32 // bit i set = day i matches, 1..31
	months   uint16 // bit i set = month i matches, 1..12
	weekdays uint8  // bit i set = weekday i matches, 0..6 (0 = Sunday)

	domRestricted bool // day-of-month field is not "*"
	dowRestricted bool // day-of-week field is not "*"
}

type cronField struct {
	min  int
	max  int
	star bool
}

// parseCron parses a 5-field cron expression and verifies that it matches at
// least one time within the next few years (so "0 0 30 2 *" is rejected rather
// than silently never firing).
func parseCron(expr string) (*cronSchedule, error) {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return nil, fmt.Errorf("expected 5 fields (minute hour day month weekday), got %d", len(fields))
	}

	specs := []cronField{
		{0, 59, false}, // minute
		{0, 23, false}, // hour
		{1, 31, false}, // day of month
		{1, 12, false}, // month
		{0, 7, false},  // day of week (7 wraps to 0 = Sunday)
	}

	var sets [5]uint64
	for i, raw := range fields {
		set, err := parseCronField(raw, specs[i])
		if err != nil {
			return nil, fmt.Errorf("field %d (%s): %w", i+1, [...]string{"minute", "hour", "day", "month", "weekday"}[i], err)
		}
		sets[i] = set
	}

	sched := &cronSchedule{
		minutes:       sets[0],
		hours:         sets[1],
		months:        uint16(sets[3]),
		domRestricted: fields[2] != "*",
		dowRestricted: fields[4] != "*",
	}
	sched.days = uint32(sets[2])

	// Weekday: collapse bit 7 (Sunday alias) onto bit 0.
	weekdays := sets[4]
	if weekdays&(1<<7) != 0 {
		weekdays |= 1
	}
	weekdays &= 0x7f
	sched.weekdays = uint8(weekdays)

	// Reject schedules that can never match (e.g. day 30 in February).
	if _, ok := sched.next(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)); !ok {
		return nil, fmt.Errorf("schedule never matches a date")
	}

	return sched, nil
}

// parseCronField parses one cron field into a bitmask. A field set to "*"
// returns star=true so the caller can detect restricted day fields.
func parseCronField(raw string, spec cronField) (uint64, error) {
	if raw == "" {
		return 0, fmt.Errorf("empty value")
	}

	var set uint64
	for _, part := range strings.Split(raw, ",") {
		if part == "" {
			return 0, fmt.Errorf("empty list element in %q", raw)
		}

		value := part
		step := 1
		if idx := strings.IndexByte(value, '/'); idx >= 0 {
			var err error
			step, err = strconv.Atoi(value[idx+1:])
			if err != nil || step <= 0 {
				return 0, fmt.Errorf("invalid step in %q", part)
			}
			value = value[:idx]
		}

		lo, hi := spec.min, spec.max
		isStar := value == "*"
		switch {
		case isStar:
			// full range
		case strings.Contains(value, "-"):
			bounds := strings.SplitN(value, "-", 2)
			var err error
			lo, err = strconv.Atoi(bounds[0])
			if err != nil {
				return 0, fmt.Errorf("invalid range in %q", part)
			}
			hi, err = strconv.Atoi(bounds[1])
			if err != nil {
				return 0, fmt.Errorf("invalid range in %q", part)
			}
		default:
			var err error
			lo, err = strconv.Atoi(value)
			if err != nil {
				return 0, fmt.Errorf("invalid number in %q", part)
			}
			if step > 1 {
				// "n/step" means from n to the field maximum.
				hi = spec.max
			} else {
				hi = lo
			}
		}

		if lo < spec.min || hi > spec.max || lo > hi {
			return 0, fmt.Errorf("value out of range (%d-%d) in %q", spec.min, spec.max, part)
		}

		for v := lo; v <= hi; v += step {
			set |= 1 << uint(v)
		}
	}

	return set, nil
}

// matches reports whether t (evaluated in its own location) satisfies the
// schedule at minute granularity.
func (s *cronSchedule) matches(t time.Time) bool {
	if s.minutes&(1<<uint(t.Minute())) == 0 {
		return false
	}
	if s.hours&(1<<uint(t.Hour())) == 0 {
		return false
	}
	if s.months&(1<<uint(t.Month())) == 0 {
		return false
	}

	// Standard cron semantics: when both day-of-month and day-of-week are
	// restricted, the day matches if either does; otherwise both must match.
	domMatch := s.days&(1<<uint(t.Day())) != 0
	dowMatch := s.weekdays&(1<<uint(t.Weekday())) != 0
	if s.domRestricted && s.dowRestricted {
		return domMatch || dowMatch
	}
	return domMatch && dowMatch
}

// next returns the first time strictly after `from` that matches the schedule,
// in from's location. ok is false if no match exists within the search window.
func (s *cronSchedule) next(from time.Time) (time.Time, bool) {
	t := from.Truncate(time.Minute).Add(time.Minute)

	// Cap the search at five years so impossible schedules terminate.
	limit := t.Add(5 * 365 * 24 * time.Hour)
	for t.Before(limit) {
		if s.matches(t) {
			return t, true
		}
		t = t.Add(time.Minute)
	}

	return time.Time{}, false
}
