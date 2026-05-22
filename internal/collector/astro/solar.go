package astro

import (
	"math"
	"time"
)

const (
	d2r = math.Pi / 180.0
	r2d = 180.0 / math.Pi
)

// toJD converts a UTC time to Julian Day Number.
func toJD(t time.Time) float64 {
	t = t.UTC()
	y, m, d := t.Date()
	h, min, s, ns := t.Hour(), t.Minute(), t.Second(), t.Nanosecond()

	D := float64(d) + (float64(h)+float64(min)/60+float64(s)/3600+float64(ns)/3.6e12)/24
	Y, M := float64(y), float64(m)
	if M <= 2 {
		Y--
		M += 12
	}
	A := math.Floor(Y / 100)
	B := 2 - A + math.Floor(A/4)
	return math.Floor(365.25*(Y+4716)) + math.Floor(30.6001*(M+1)) + D + B - 1524.5
}

type solarParams struct {
	sinDecl float64 // sin of declination
	cosDecl float64 // cos of declination
	eqTime  float64 // equation of time (minutes)
}

// calcParams derives the sun's declination and equation of time for a given date.
// Uses date at 12:00 UTC as the reference point.
func calcParams(date time.Time) solarParams {
	y, m, d := date.UTC().Date()
	jd := toJD(time.Date(y, m, d, 12, 0, 0, 0, time.UTC))
	T := (jd - 2451545.0) / 36525.0

	// Geometric mean longitude and anomaly
	L0 := math.Mod(280.46646+T*(36000.76983+T*0.0003032), 360)
	M := 357.52911 + T*(35999.05029-0.0001537*T)
	Mr := M * d2r

	// Equation of center → true longitude → apparent longitude
	C := math.Sin(Mr)*(1.914602-T*(0.004817+0.000014*T)) +
		math.Sin(2*Mr)*(0.019993-0.000101*T) +
		math.Sin(3*Mr)*0.000289
	omega := 125.04 - 1934.136*T
	lambdaR := (L0 + C - 0.00569 - 0.00478*math.Sin(omega*d2r)) * d2r

	// Obliquity correction
	eps0 := 23 + (26+(21.448-T*(46.8150+T*(0.00059-T*0.001813)))/60)/60
	epsR := (eps0 + 0.00256*math.Cos(omega*d2r)) * d2r

	// Declination
	sinDecl := math.Sin(epsR) * math.Sin(lambdaR)
	cosDecl := math.Cos(math.Asin(sinDecl))

	// Equation of time (minutes)
	ecc := 0.016708634 - T*(0.000042037+0.0000001267*T)
	y2 := math.Tan(epsR / 2)
	y2 *= y2
	L0r := L0 * d2r
	eqTime := 4 * r2d * (y2*math.Sin(2*L0r) -
		2*ecc*math.Sin(Mr) +
		4*ecc*y2*math.Sin(Mr)*math.Cos(2*L0r) -
		0.5*y2*y2*math.Sin(4*L0r) -
		1.25*ecc*ecc*math.Sin(2*Mr))

	return solarParams{sinDecl: sinDecl, cosDecl: cosDecl, eqTime: eqTime}
}

// solarNoon returns the UTC time of solar noon for the given date and longitude.
func solarNoon(date time.Time, lon float64) time.Time {
	y, m, d := date.UTC().Date()
	p := calcParams(date)
	noonMin := 720 - 4*lon - p.eqTime
	return minutesUTC(y, m, d, noonMin)
}

// sunTime returns the UTC time of sunrise or sunset (rise=true/false) for the given
// date, latitude, longitude, and solar elevation (degrees).
// Returns the zero Time and false when the sun does not cross the elevation (polar conditions).
func sunTime(date time.Time, lat, lon, elevation float64, rise bool) (time.Time, bool) {
	y, m, d := date.UTC().Date()
	p := calcParams(date)

	latR := lat * d2r
	eleR := elevation * d2r
	cosHA := (math.Sin(eleR) - math.Sin(latR)*p.sinDecl) / (math.Cos(latR) * p.cosDecl)
	if cosHA > 1 || cosHA < -1 {
		return time.Time{}, false
	}
	ha := math.Acos(cosHA) * r2d

	noonMin := 720 - 4*lon - p.eqTime
	var offsetMin float64
	if rise {
		offsetMin = noonMin - 4*ha
	} else {
		offsetMin = noonMin + 4*ha
	}
	return minutesUTC(y, m, d, offsetMin), true
}

// Sunrise returns the UTC sunrise time for the local calendar date carried by date.
func Sunrise(date time.Time, lat, lon float64) (time.Time, bool) {
	y, m, d := date.Date()
	return sunTime(time.Date(y, m, d, 12, 0, 0, 0, time.UTC), lat, lon, -0.833, true)
}

// Sunset returns the UTC sunset time for the local calendar date carried by date.
func Sunset(date time.Time, lat, lon float64) (time.Time, bool) {
	y, m, d := date.Date()
	return sunTime(time.Date(y, m, d, 12, 0, 0, 0, time.UTC), lat, lon, -0.833, false)
}

// minutesUTC converts a minute offset from midnight UTC into a time.Time.
// Offsets outside [0, 1440) roll over to adjacent days correctly.
func minutesUTC(y int, m time.Month, d int, mins float64) time.Time {
	base := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	return base.Add(time.Duration(math.Round(mins*60)) * time.Second)
}
