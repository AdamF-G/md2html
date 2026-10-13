package charts

// voronoiCells returns, for each point, the part of the box (x0,y0)–(x1,y1)
// nearer that point than any other: a convex polygon, possibly empty when
// the point lies far outside the box. Each cell starts as the box and is
// clipped, one site at a time, to the half-plane on its own side of the
// perpendicular bisector (Sutherland–Hodgman against a line). It is O(n²).
// A point equidistant from two sites belongs to the earlier one: each
// bisector is moved a hair towards the later site, the same line for both
// cells, so the cells still tile the box.
func voronoiCells(points [][2]float64, x0, y0, x1, y1 float64) [][][2]float64 {
	cells := make([][][2]float64, len(points))
	for i, p := range points {
		cell := [][2]float64{{x0, y0}, {x1, y0}, {x1, y1}, {x0, y1}}
		for j, q := range points {
			if j == i || (q[0] == p[0] && q[1] == p[1]) {
				continue // coincident sites have no bisector; they share a cell
			}
			cell = clipNearer(cell, p, q, j > i)
			if len(cell) == 0 {
				break
			}
		}
		cells[i] = cell
	}
	return cells
}

// clipNearer keeps the part of the convex polygon poly at least as near p
// as q: the points x where (x - m)·(q - p) <= 0, m the midpoint of p and q.
// With wins, ties go to p (the line moves towards q); without, to q.
func clipNearer(poly [][2]float64, p, q [2]float64, wins bool) [][2]float64 {
	nx, ny := q[0]-p[0], q[1]-p[1]
	mx, my := (p[0]+q[0])/2, (p[1]+q[1])/2
	tie := 1e-9 * (nx*nx + ny*ny) // a shift of a billionth of the sites' distance
	if !wins {
		tie = -tie
	}
	side := func(v [2]float64) float64 { return (v[0]-mx)*nx + (v[1]-my)*ny - tie }
	var out [][2]float64
	for k, cur := range poly {
		prev := poly[(k+len(poly)-1)%len(poly)]
		sc, sp := side(cur), side(prev)
		if (sc <= 0) != (sp <= 0) {
			t := sp / (sp - sc)
			out = append(out, [2]float64{prev[0] + t*(cur[0]-prev[0]), prev[1] + t*(cur[1]-prev[1])})
		}
		if sc <= 0 {
			out = append(out, cur)
		}
	}
	return out
}
