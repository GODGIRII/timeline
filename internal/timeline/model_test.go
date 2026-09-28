package timeline

import "testing"

func TestDeadline(t *testing.T) {
	for _, d := range []Deadline{{Date: "2028-02-29"}, {At: "2026-09-28T17:30:00+05:30"}, {At: "2026-09-28T12:00:00Z"}} {
		if err := d.Validate(); err != nil {
			t.Errorf("valid deadline %+v: %v", d, err)
		}
	}
	for _, d := range []Deadline{{}, {Date: "2026-02-29"}, {Date: "2026-13-01"}, {At: "2026-09-28T17:30:00"}, {Date: "2026-09-28", At: "2026-09-28T12:00:00Z"}} {
		if err := d.Validate(); err == nil {
			t.Errorf("invalid deadline accepted: %+v", d)
		}
	}
}
