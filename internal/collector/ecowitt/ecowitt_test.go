package ecowitt

import (
	"math"
	"testing"
)

func TestFeelsLikeUsesHumidexFromDewPoint(t *testing.T) {
	got := feelsLike(30, 24, 5)
	want := 41.3
	if math.Abs(got-want) > 0.1 {
		t.Fatalf("feelsLike(30, 24, 5) = %.2f, want %.2f", got, want)
	}
}

func TestFeelsLikeDoesNotInflateDryWarmWeather(t *testing.T) {
	got := feelsLike(27.8, 7, 5)
	want := 27.8
	if math.Abs(got-want) > 0.1 {
		t.Fatalf("feelsLike(27.8, 7, 5) = %.2f, want %.2f", got, want)
	}
}

func TestFeelsLikeUsesWindChillBelowFreezing(t *testing.T) {
	got := feelsLike(-5, -8, 20)
	want := -11.6
	if math.Abs(got-want) > 0.1 {
		t.Fatalf("feelsLike(-5, -8, 20) = %.2f, want %.2f", got, want)
	}
}
