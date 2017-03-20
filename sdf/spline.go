//-----------------------------------------------------------------------------
/*

Interpolate using Cubic Splines

x(t) = a + bt + ct^2 + dt^3 for t in [0,1]
y(t) = a + bt + ct^2 + dt^3 for t in [0,1]

1st and 2nd derivatives are continuous across intervals.
2nd derivatives == 0 at the endpoints (natural splines).
See: http://mathworld.wolfram.com/CubicSpline.html

*/
//-----------------------------------------------------------------------------

package sdf

import "fmt"

//-----------------------------------------------------------------------------

// Solve the tridiagonal matrix equation m.x = d, return x
// See: https://en.wikipedia.org/wiki/Tridiagonal_matrix_algorithm
func TriDiagonal(m []V3, d []float64) []float64 {
	// Sanity checks
	n := len(m)
	if len(d) != n {
		panic("bad sizes rows(m) != rows(d)")
	}
	if m[0].X != 0 || m[n-1].Z != 0 {
		panic("bad values for tridiagonal matrix")
	}
	if m[0].Y == 0 {
		panic("m[0].Y == 0")
	}
	cp := make([]float64, n) // c-prime
	x := make([]float64, n)  // d-prime -> x solution
	// elimination
	cp[0] = m[0].Z / m[0].Y
	x[0] = d[0] / m[0].Y
	for i := 1; i < n; i++ {
		denom := m[i].Y - m[i].X*cp[i-1]
		if denom == 0 {
			panic("denom == 0")
		}
		cp[i] = m[i].Z / denom
		x[i] = (d[i] - m[i].X*x[i-1]) / denom
	}
	// back substitution
	for i := n - 2; i >= 0; i-- {
		x[i] -= cp[i] * x[i+1]
	}
	return x
}

//-----------------------------------------------------------------------------

type CubicPolynomial struct {
	a, b, c, d float64 // polynomial coefficients
}

// Return the function value for a given t value.
func (p *CubicPolynomial) f0(t float64) float64 {
	return p.a + t*(p.b+t*(p.c+p.d*t))
}

// Return the first derivative for a given t value.
func (p *CubicPolynomial) f1(t float64) float64 {
	return p.b + t*(2*p.c+3*p.d*t)
}

// Return the second derivative for a given t value.
func (p *CubicPolynomial) f2(t float64) float64 {
	return 2*p.c + 6*p.d*t
}

// Set polynomial coefficent values.
func (p *CubicPolynomial) Set(y0, y1, D0, D1 float64) {
	p.a = y0
	p.b = D0
	p.c = 3*(y1-y0) - 2*D0 - D1
	p.d = 2*(y0-y1) + D0 + D1
}

// Return the t values for f1 == 0 (local minima/maxima)
func (p *CubicPolynomial) f1_zeroes() []float64 {

	fmt.Printf("p: a %f b %f c %f d %f\n", p.a, p.b, p.c, p.d)

	fmt.Printf("%s\n", FloatDecode(p.d))
	fmt.Printf("%s\n", FloatDecode(3*p.d))

	fmt.Printf("q: a %f b %f c %f\n", 3*p.d, 2*p.c, p.b)

	t, _ := quadratic(3*p.d, 2*p.c, p.b)
	fmt.Printf("t %v\n", t)
	return t
}

//-----------------------------------------------------------------------------

type CubicSpline struct {
	idx    int             // index within spline set
	p0, p1 V2              // end points of cubic spline
	px, py CubicPolynomial // cubic polynomial
}

// Return the function value for a given t value.
func (s *CubicSpline) f0(t float64) V2 {
	return V2{s.px.f0(t), s.py.f0(t)}
}

// Return the first derivative for a given t value.
func (s *CubicSpline) f1(t float64) V2 {
	return V2{s.px.f1(t), s.py.f1(t)}
}

// Return the second derivative for a given t value.
func (s *CubicSpline) f2(t float64) V2 {
	return V2{s.px.f2(t), s.py.f2(t)}
}

// Return the bounding box for a spline.
func (s *CubicSpline) BoundingBox() Box2 {
	p := V2Set{s.p0, s.p1}
	// x minima/maxima
	for _, t := range s.px.f1_zeroes() {
		p = append(p, s.f0(Clamp(t, 0, 1)))
	}
	// y minima/maxima
	for _, t := range s.py.f1_zeroes() {
		p = append(p, s.f0(Clamp(t, 0, 1)))
	}
	return Box2{p.Min(), p.Max()}
}

//-----------------------------------------------------------------------------

type CubicSplineSDF2 struct {
	spline []CubicSpline // cubic splines
	bb     Box2          // bounding box
}

// Return the spline and t value for a given t value.
func (s *CubicSplineSDF2) Find(t float64) (*CubicSpline, float64) {
	n := len(s.spline)
	t = Clamp(t, 0, float64(n))
	i := int(t)
	t -= float64(i)
	// correct for the last spline
	if i == n {
		i -= 1
		t = 1
	}
	return &s.spline[i], t
}

// Return the function value for a given t value.
func (s *CubicSplineSDF2) F0(t float64) V2 {
	cs, t := s.Find(t)
	return cs.f0(t)
}

// Return a polygon approximating the cubic spline.
func (s *CubicSplineSDF2) Polygonize(n int) *Polygon {
	p := NewPolygon()
	dt := float64(len(s.spline)) / float64(n-1)
	t := 0.0
	for i := 0; i < n; i++ {
		p.AddV2(s.F0(t))
		t += dt
	}
	return p
}

func CubicSpline2D(knot []V2) SDF2 {
	if len(knot) < 2 {
		panic("cubic splines at least 2 knots")
	}
	s := CubicSplineSDF2{}

	// Build and solve the tridiagonal matrices
	n := len(knot)
	m := make([]V3, n)
	dx := make([]float64, n)
	dy := make([]float64, n)
	for i := 1; i < n-1; i++ {
		m[i] = V3{1, 4, 1}
		dx[i] = 3 * (knot[i+1].X - knot[i-1].X)
		dy[i] = 3 * (knot[i+1].Y - knot[i-1].Y)
	}
	// Special case the end splines.
	// Assume the 2nd derivative at the end points is 0.
	m[0] = V3{0, 2, 1}
	dx[0] = 3 * (knot[1].X - knot[0].X)
	dy[0] = 3 * (knot[1].Y - knot[0].Y)
	m[n-1] = V3{1, 2, 0}
	dx[n-1] = 3 * (knot[n-1].X - knot[n-2].X)
	dy[n-1] = 3 * (knot[n-1].Y - knot[n-2].Y)
	// solve to give the first derivatives at the knot points
	xx := TriDiagonal(m, dx)
	xy := TriDiagonal(m, dy)

	// The solution data are the first derivatives.
	// Reformat as the cubic polynomial coefficients.
	s.spline = make([]CubicSpline, n-1)
	for i := 0; i < n-1; i++ {
		s.spline[i].idx = i
		s.spline[i].p0 = knot[i]
		s.spline[i].p1 = knot[i+1]
		s.spline[i].px.Set(knot[i].X, knot[i+1].X, xx[i], xx[i+1])
		s.spline[i].py.Set(knot[i].Y, knot[i+1].Y, xy[i], xy[i+1])
	}

	// work out the bounding box
	s.bb = s.spline[0].BoundingBox()
	for i := 1; i < n-1; i++ {
		s.bb = s.bb.Extend(s.spline[i].BoundingBox())
	}
	return &s
}

func (s *CubicSplineSDF2) Evaluate(p V2) float64 {
	return 0
}

func (s *CubicSplineSDF2) BoundingBox() Box2 {
	return s.bb
}

//-----------------------------------------------------------------------------
