package charts

import (
	"math"
	"testing"
)

func TestVoronoiCellsPartitionTheBox(t *testing.T) {
	pts := [][2]float64{{10, 10}, {90, 10}, {50, 90}}
	cells := voronoiCells(pts, 0, 0, 100, 100)
	if len(cells) != 3 {
		t.Fatalf("%d cells", len(cells))
	}
	// Each sample point lies in exactly the cell of its nearest site.
	for _, s := range [][2]float64{{5, 5}, {95, 5}, {50, 95}, {50, 40}, {20, 60}} {
		near, best := -1, math.MaxFloat64
		for i, p := range pts {
			if d := math.Hypot(s[0]-p[0], s[1]-p[1]); d < best {
				near, best = i, d
			}
		}
		for i, c := range cells {
			if inside(c, s) != (i == near) {
				t.Errorf("sample %v: cell %d says %v, nearest is %d", s, i, inside(c, s), near)
			}
		}
	}
}

// inside is the ray-casting point-in-polygon test.
func inside(poly [][2]float64, p [2]float64) bool {
	in := false
	for i, j := 0, len(poly)-1; i < len(poly); j, i = i, i+1 {
		a, b := poly[i], poly[j]
		if (a[1] > p[1]) != (b[1] > p[1]) && p[0] < (b[0]-a[0])*(p[1]-a[1])/(b[1]-a[1])+a[0] {
			in = !in
		}
	}
	return in
}
