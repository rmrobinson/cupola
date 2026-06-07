package envcanada

import (
	"encoding/xml"
	"testing"
	"time"
)

func TestParseCurrentConditionsFeedMapsFields(t *testing.T) {
	feed := parseCurrentConditionsTestFeed(t, `
<feed xmlns="http://www.w3.org/2005/Atom">
  <entry>
    <title>Current Conditions: Mostly Cloudy, 15.3°C</title>
    <updated>2026-06-07T19:00:00Z</updated>
    <category term="Current Conditions"/>
    <summary type="html">
      &lt;b&gt;Condition:&lt;/b&gt; Mostly Cloudy&lt;br/&gt;
      &lt;b&gt;Temperature:&lt;/b&gt; 15.3&amp;deg;C&lt;br/&gt;
      &lt;b&gt;Pressure / Tendency:&lt;/b&gt; 101.2 kPa falling&lt;br/&gt;
      &lt;b&gt;Humidity:&lt;/b&gt; 64 %&lt;br/&gt;
      &lt;b&gt;Wind:&lt;/b&gt; SW 18 gust 34 km/h&lt;br/&gt;
      &lt;b&gt;Humidex:&lt;/b&gt; 18&lt;br/&gt;
      &lt;b&gt;Visibility:&lt;/b&gt; 24 km
    </summary>
  </entry>
  <entry>
    <title>Tonight: Cloudy. Low 12.</title>
    <category term="Weather Forecasts"/>
    <summary>Forecast summary</summary>
  </entry>
</feed>`)
	now := time.Date(2026, 6, 7, 20, 0, 0, 0, time.UTC)

	got, err := parseCurrentConditionsFeed(feed, now)
	if err != nil {
		t.Fatalf("parseCurrentConditionsFeed() error = %v", err)
	}

	if !got.UpdatedAt.Equal(now) {
		t.Fatalf("UpdatedAt = %s, want %s", got.UpdatedAt, now)
	}
	if got.Condition != "Mostly Cloudy" {
		t.Fatalf("Condition = %q, want Mostly Cloudy", got.Condition)
	}
	if got.Temperature != 15.3 || got.FeelsLike != 18 || got.Humidity != 64 {
		t.Fatalf("temperature/feels/humidity = %.1f %.1f %.1f, want 15.3 18 64", got.Temperature, got.FeelsLike, got.Humidity)
	}
	if got.Pressure != 1012 {
		t.Fatalf("Pressure = %.1f, want 1012", got.Pressure)
	}
	if got.WindDirection != 225 || got.WindSpeed != 18 || got.WindGust != 34 {
		t.Fatalf("wind = dir %d speed %.1f gust %.1f, want 225 18 34", got.WindDirection, got.WindSpeed, got.WindGust)
	}
	if got.Visibility != 24 {
		t.Fatalf("Visibility = %.1f, want 24", got.Visibility)
	}
	if got.UV != 0 || got.RainDaily != 0 || got.Precipitation != 0 {
		t.Fatalf("EC-unavailable fields should remain zero: uv=%.1f rain=%.1f precip=%.1f", got.UV, got.RainDaily, got.Precipitation)
	}
}

func TestParseCurrentConditionsFeedMissingOptionalFields(t *testing.T) {
	feed := parseCurrentConditionsTestFeed(t, `
<feed xmlns="http://www.w3.org/2005/Atom">
  <entry>
    <title>Current Conditions: Clear, -4.5°C</title>
    <category term="Current Conditions"/>
    <summary type="html">
      &lt;b&gt;Condition:&lt;/b&gt; Clear&lt;br/&gt;
      &lt;b&gt;Temperature:&lt;/b&gt; -4.5&amp;deg;C&lt;br/&gt;
      &lt;b&gt;Wind:&lt;/b&gt; Calm
    </summary>
  </entry>
</feed>`)

	got, err := parseCurrentConditionsFeed(feed, time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))
	if err != nil {
		t.Fatalf("parseCurrentConditionsFeed() error = %v", err)
	}
	if got.Condition != "Clear" || got.Temperature != -4.5 {
		t.Fatalf("condition/temp = %q %.1f, want Clear -4.5", got.Condition, got.Temperature)
	}
	if got.Humidity != 0 || got.Pressure != 0 || got.WindSpeed != 0 || got.WindDirection != 0 || got.WindGust != 0 || got.FeelsLike != 0 {
		t.Fatalf("optional fields should remain zero: %+v", got)
	}
}

func parseCurrentConditionsTestFeed(t *testing.T, raw string) *atomFeed {
	t.Helper()
	var feed atomFeed
	if err := xml.Unmarshal([]byte(raw), &feed); err != nil {
		t.Fatalf("unmarshal feed: %v", err)
	}
	return &feed
}
