//-----------------------------------------------------------------------------
/*

Fuel Pump Ring Nut Tool

Many cars have a fuel pump in the tank held in place by a plastic ringnut.
This is a tool for removing them.

This design is for the Mazda 2006 RX-8 (Series1)
Other ring nuts are similar, so feel free to modify.

Notes:
Mazda Tool: SST# 49-F042-001

*/
//-----------------------------------------------------------------------------

package main

import (
	"log"

	"github.com/deadsy/sdfx/obj"
	"github.com/deadsy/sdfx/render"
	"github.com/deadsy/sdfx/sdf"
)

//-----------------------------------------------------------------------------

// material shrinkage
const shrink = 1.0 / 0.999 // PLA ~0.1%
//const shrink = 1.0/0.995; // ABS ~0.5%

//-----------------------------------------------------------------------------

const innerDiameter = 132.0
const ringWidth = 19.0
const outerDiameter = innerDiameter + (2.0 * ringWidth)
const ringHeight = 16.0
const topGap = 90.0
const screwDiameter = 25.4 * (3.0/16.0)
const screwX = (topGap * 0.5) + (screwDiameter * 1.5)
const screwY = innerDiameter * 0.22

const numTabs = 18
const tabDepth = 3.5
const tabWidth = 3.5
const extraTab = true // The rx-8 puts an additional tab on the ring

const sideThickness = 2.5 * tabDepth
const topThickness = 2.0 * tabDepth

//-----------------------------------------------------------------------------

func outerBody() (sdf.SDF3, error) {
	h := (ringHeight + topThickness) * 2.0
	r := (outerDiameter * 0.5) + sideThickness
	round := topThickness * 0.5
	return sdf.Cylinder3D(h, r, round)
}

func innerCavity() (sdf.SDF3, error) {
	h := ringHeight * 2.0
	r := outerDiameter * 0.5
	round := ringHeight * 0.1
	s0, err := sdf.Cylinder3D(h, r, round)
	if err != nil {
		return nil, err
	}
	// central bore
	h = (ringHeight + topThickness) * 2.0
	r = innerDiameter * 0.5
	s1, err := sdf.Cylinder3D(h, r, 0)
	if err != nil {
		return nil, err
	}

	s1 = sdf.Cut3D(s1, sdf.V3{topGap * 0.5, 0, 0}, sdf.V3{-1, 0, 0})
	s1 = sdf.Cut3D(s1, sdf.V3{-topGap * 0.5, 0, 0}, sdf.V3{1, 0, 0})

	return sdf.Union3D(s0, s1), nil
}

func tab() (sdf.SDF3, error) {
	size := sdf.V3{
		X: tabWidth,
		Y: ringWidth + tabDepth,
		Z: (ringHeight + tabDepth) * 2.0,
	}
	s, err := sdf.Box3D(size, 0)
	if err != nil {
		return nil, err
	}
	yofs := (size.Y + innerDiameter) * 0.5
	s = sdf.Transform3D(s, sdf.Translate3d(sdf.V3{0, yofs, 0}))
	return s, nil
}

func tabs() (sdf.SDF3, error) {
	t, err := tab()
	if err != nil {
		return nil, err
	}

	theta := sdf.Tau / numTabs
	s := sdf.RotateUnion3D(t, numTabs, sdf.Rotate3d(sdf.V3{0, 0, 1}, theta))
	s = sdf.Transform3D(s, sdf.Rotate3d(sdf.V3{0, 0, 1}, theta*0.5))

	if extraTab {
		s = sdf.Union3D(s, t)
	}

	return s, nil
}

func screwHole() (sdf.SDF3, error) {

	l := ringHeight + topThickness
	r := screwDiameter * 0.5

	s, err := obj.CounterSunkHole3D(l, r)
	if err != nil {
		return nil, err
	}

	zofs := (l * 0.5) + ringHeight
	s = sdf.Transform3D(s, sdf.Translate3d(sdf.V3{0, 0, -zofs}))

	return s, nil
}

func screwHoles() (sdf.SDF3, error) {
	s, err := screwHole()
	if err != nil {
		return nil, err
	}
	s0 := sdf.Transform3D(s, sdf.Translate3d(sdf.V3{screwX, screwY, 0}))
	s1 := sdf.Transform3D(s, sdf.Translate3d(sdf.V3{-screwX, screwY, 0}))
	s2 := sdf.Transform3D(s, sdf.Translate3d(sdf.V3{screwX, -screwY, 0}))
	s3 := sdf.Transform3D(s, sdf.Translate3d(sdf.V3{-screwX, -screwY, 0}))
	s4 := sdf.Transform3D(s, sdf.Translate3d(sdf.V3{screwX, 0, 0}))
	s5 := sdf.Transform3D(s, sdf.Translate3d(sdf.V3{-screwX, 0, 0}))
	return sdf.Union3D(s0, s1, s2, s3, s4, s5), nil
}

func tool() (sdf.SDF3, error) {

	// make the body
	body, err := outerBody()
	if err != nil {
		return nil, err
	}

	// make the cavity
	cavity, err := innerCavity()
	if err != nil {
		return nil, err
	}

	// make the tabs
	tabs, err := tabs()
	if err != nil {
		return nil, err
	}

	// make the screw holes
	screws, err := screwHoles()
	if err != nil {
		return nil, err
	}

	s := sdf.Difference3D(body, sdf.Union3D(cavity, tabs, screws))

	// cut it on the xy plane
	s = sdf.Cut3D(s, sdf.V3{0, 0, 0}, sdf.V3{0, 0, -1})
	return s, nil
}

//-----------------------------------------------------------------------------

func main() {
	s, err := tool()
	if err != nil {
		log.Fatalf("error: %s", err)
	}
	s = sdf.ScaleUniform3D(s, shrink)
	render.ToSTL(s, 300, "tool.stl", &render.MarchingCubesOctree{})
}

//-----------------------------------------------------------------------------