package common

import (
	"fmt"
	"testing"
	"time"

	. "github.com/franela/goblin"
)

func TestParseDates(t *testing.T) {
	g := Goblin(t)
	now, _ := time.Parse("2006-01-02", "2024-07-31")

	y := "::year::,::year:-1:"
	m := "::year::-::month:-9:-::day::/::daysuntilnow::"
	d := "::year::-::month::-::day:+1:/1"
	g.Describe("Parsedates", func() {
		g.It("Should parse year: "+y, func() {
			date := ParseDates(y, now)
			w := now.AddDate(-1, 0, 0)
			wants := fmt.Sprintf("%d,%d", now.Year(), w.Year())
			g.Assert(date).Equal(wants)
		})
		g.It("Should parse month: "+m, func() {
			date := ParseDates(m, now)
			w := now.AddDate(0, -9, 0)
			daysUntilNow := int(time.Since(w).Hours() / 24)
			wants := fmt.Sprintf("%d-%d-%d/%d", w.Year(), w.Month(), w.Day(), daysUntilNow)
			g.Assert(date).Equal(wants)
		})
		g.It("should parse day: "+d, func() {
			date := ParseDates(d, now)
			w := now.AddDate(0, 0, 1)
			wants := fmt.Sprintf("%d-%d-%d/%d", w.Year(), w.Month(), w.Day(), 1)
			g.Assert(date).Equal(wants)
		})
	})
}
