package domain

import "time"

type RainAccumulationEntry struct {
	DayOfWeek   string    `json:"day_of_week"`
	RainMM      float64   `json:"rain_mm"`
	PeriodStart time.Time `json:"period_start"`
	PeriodEnd   time.Time `json:"period_end"`
}

type WeatherRainAccumulation struct {
	StateBase
	Entries map[string]RainAccumulationEntry `json:"entries"`
}

func (WeatherRainAccumulation) DomainType() DomainType { return DomainWeatherRainAccumulation }

type WeatherCurrent struct {
	StateBase
	Temperature   float64 `json:"temperature"`
	FeelsLike     float64 `json:"feels_like"`
	DewPoint      float64 `json:"dew_point"`
	Humidity      float64 `json:"humidity"`
	WindSpeed     float64 `json:"wind_speed"`
	WindDirection int     `json:"wind_direction"`
	WindGust      float64 `json:"wind_gust"`
	Pressure      float64 `json:"pressure"`
	Precipitation float64 `json:"precipitation"`
	RainEvent     float64 `json:"rain_event"`   // mm since last dry period
	RainDaily     float64 `json:"rain_daily"`   // mm past 24h
	RainWeekly    float64 `json:"rain_weekly"`  // mm this week
	RainMonthly   float64 `json:"rain_monthly"` // mm this month
	RainYearly    float64 `json:"rain_yearly"`  // mm this year
	UV            float64 `json:"uv"`
	Visibility    float64 `json:"visibility"`
	Condition     string  `json:"condition"`
}

func (WeatherCurrent) DomainType() DomainType { return DomainWeatherCurrent }

type WeatherForecast struct {
	StateBase
	Periods []ForecastPeriod `json:"periods"`
}

func (WeatherForecast) DomainType() DomainType { return DomainWeatherForecast }

type WeatherHourlyForecast struct {
	StateBase
	Hours []HourlyForecastPeriod `json:"hours"`
}

func (WeatherHourlyForecast) DomainType() DomainType { return DomainWeatherForecastHourly }

type HourlyForecastPeriod struct {
	StartsAt time.Time `json:"starts_at"`
	EndsAt   time.Time `json:"ends_at"`

	Condition    string   `json:"condition"`
	Temperature  *float64 `json:"temperature,omitempty"`
	FeelsLike    *float64 `json:"feels_like,omitempty"`
	PrecipChance *int     `json:"precip_chance,omitempty"`

	WindDirection string   `json:"wind_direction,omitempty"`
	WindSpeed     *float64 `json:"wind_speed,omitempty"`
	WindGust      *float64 `json:"wind_gust,omitempty"`

	Humidex   *float64 `json:"humidex,omitempty"`
	WindChill *float64 `json:"wind_chill,omitempty"`
	UVIndex   *float64 `json:"uv_index,omitempty"`

	IconURL string `json:"icon_url,omitempty"`
}

type ForecastPeriod struct {
	StartsAt      time.Time `json:"starts_at"`
	EndsAt        time.Time `json:"ends_at"`
	Label         string    `json:"label"`
	High          *float64  `json:"high,omitempty"`
	Low           *float64  `json:"low,omitempty"`
	Condition     string    `json:"condition"`
	PrecipChance  int       `json:"precip_chance"`
	PrecipAmount  float64   `json:"precip_amount"`
	WindSpeed     float64   `json:"wind_speed"`
	WindDirection int       `json:"wind_direction"`
	Summary       string    `json:"summary"`
}

type WeatherAlerts struct {
	StateBase
	Alerts []WeatherAlert `json:"alerts"`
}

func (WeatherAlerts) DomainType() DomainType { return DomainWeatherAlerts }

type WeatherAlert struct {
	ID        string        `json:"id"`
	Title     string        `json:"title"`
	Severity  AlertSeverity `json:"severity"`
	Onset     time.Time     `json:"onset"`
	Expires   time.Time     `json:"expires"`
	Summary   string        `json:"summary"`
	SourceURL string        `json:"source_url"`
}

type WeatherAirQuality struct {
	StateBase
	Location  string               `json:"location"`
	Province  string               `json:"province"`
	SourceURL string               `json:"source_url"`
	Observed  *AQHIValue           `json:"observed,omitempty"`
	Forecasts []AQHIForecastPeriod `json:"forecasts"`
	IssuedAt  time.Time            `json:"issued_at,omitempty"`
}

func (WeatherAirQuality) DomainType() DomainType { return DomainWeatherAirQuality }

type AQHIValue struct {
	Value *int   `json:"value,omitempty"`
	Risk  string `json:"risk,omitempty"`
}

type AQHIForecastPeriod struct {
	Label string     `json:"label"`
	Max   *AQHIValue `json:"max,omitempty"`
}

type WeatherPollen struct {
	StateBase
	RegionCode string      `json:"region_code,omitempty"`
	Source     string      `json:"source"`
	Current    *PollenDay  `json:"current,omitempty"`
	Days       []PollenDay `json:"days"`
}

func (WeatherPollen) DomainType() DomainType { return DomainWeatherPollen }

type PollenDay struct {
	Date                  string           `json:"date"`
	Aggregate             *PollenAggregate `json:"aggregate,omitempty"`
	Types                 []PollenRow      `json:"types"`
	Plants                []PollenRow      `json:"plants"`
	HealthRecommendations []string         `json:"health_recommendations,omitempty"`
}

type PollenAggregate struct {
	Value       int    `json:"value"`
	Label       string `json:"label"`
	Code        string `json:"code"`
	Category    string `json:"category,omitempty"`
	Description string `json:"description,omitempty"`
	Color       string `json:"color,omitempty"`
}

type PollenRow struct {
	Code        string       `json:"code"`
	DisplayName string       `json:"display_name"`
	InSeason    bool         `json:"in_season"`
	UPI         *PollenIndex `json:"upi,omitempty"`
}

type PollenIndex struct {
	Value       int    `json:"value"`
	Category    string `json:"category,omitempty"`
	Description string `json:"description,omitempty"`
	Color       string `json:"color,omitempty"`
}
