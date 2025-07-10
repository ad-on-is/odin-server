package common

import (
	"fmt"
	"testing"
	"time"

	. "github.com/franela/goblin"
)

func TestParseDates(t *testing.T) {
	g := Goblin(t)
	now := time.Now()
	y := "::year::,::year:-1:"
	m := "::year::-::month:-1:-::day::/::monthdays::"
	d := "::year::-::month::-::day:+1:/1"
	g.Describe("Parsedates", func() {
		g.It("Should parse year: "+y, func() {
			year := now.Year()
			date := ParseDates(y)
			wants := fmt.Sprintf("%d,%d", year, year-1)
			g.Assert(date).Equal(wants)
		})
		g.It("Should parse month: "+m, func() {
			date := ParseDates(m)
			w := now.AddDate(0, -1, 0)
			wants := fmt.Sprintf("%d-%d-%d/%d", w.Year(), w.Month(), w.Day(), daysInMonth(w))
			g.Assert(date).Equal(wants)
		})
		g.It("should parse day: "+d, func() {
			date := ParseDates(d)
			w := now.AddDate(0, 0, 1)
			wants := fmt.Sprintf("%d-%d-%d/%d", w.Year(), w.Month(), w.Day(), 1)
			g.Assert(date).Equal(wants)
		})
	})
	// g.It("Should parse date"), func() {
	//
	// })
}
