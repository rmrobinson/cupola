package astro

import "math"

// JD of a known new moon: 2000-01-06 18:14 UTC
const refNewMoonJD = 2451549.258

// Synodic month (new moon to new moon) in days.
const synodicMonth = 29.530588853

// moonPhase returns the current phase as a fraction 0.0–1.0.
//
//	0.00 = new moon
//	0.25 = first quarter
//	0.50 = full moon
//	0.75 = last quarter
func moonPhase(jd float64) float64 {
	p := math.Mod((jd-refNewMoonJD)/synodicMonth, 1.0)
	if p < 0 {
		p += 1.0
	}
	return p
}

// moonIllumination returns the fraction of the lunar disc that is illuminated (0–1).
func moonIllumination(phase float64) float64 {
	return (1 - math.Cos(2*math.Pi*phase)) / 2
}

// moonPhaseName maps a phase fraction to its common name.
func moonPhaseName(phase float64) string {
	switch {
	case phase < 0.033 || phase >= 0.967:
		return "new"
	case phase < 0.217:
		return "waxing crescent"
	case phase < 0.283:
		return "first quarter"
	case phase < 0.467:
		return "waxing gibbous"
	case phase < 0.533:
		return "full"
	case phase < 0.717:
		return "waning gibbous"
	case phase < 0.783:
		return "last quarter"
	default:
		return "waning crescent"
	}
}
