package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Duration unmarshals YAML duration strings such as "1h", "30s", "15m".
type Duration struct {
	time.Duration
}

func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	dur, err := time.ParseDuration(value.Value)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", value.Value, err)
	}
	d.Duration = dur
	return nil
}

type Config struct {
	Location     LocationConfig     `yaml:"location"`
	Server       ServerConfig       `yaml:"server"`
	Tiles        TilesConfig        `yaml:"tiles"`
	Collectors   CollectorsConfig   `yaml:"collectors"`
	Connectivity ConnectivityConfig `yaml:"connectivity"`
}

// ConnectivityConfig controls the internet connectivity probe.
type ConnectivityConfig struct {
	// CheckURL is the URL polled to test internet reachability.
	// Defaults to http://connectivitycheck.gstatic.com/generate_204.
	CheckURL string `yaml:"check_url"`
	// Interval between probes. Defaults to 30s.
	Interval Duration `yaml:"interval"`
}

type LocationConfig struct {
	Name        string  `yaml:"name"`
	Lat         float64 `yaml:"lat"`
	Lon         float64 `yaml:"lon"`
	Timezone    string  `yaml:"timezone"`
	CountryCode string  `yaml:"country_code"` // ISO 3166-1 alpha-2, e.g. "CA"
}

type ServerConfig struct {
	Host               string   `yaml:"host"`                 // hostname or address to listen on; defaults to "localhost"
	Port               int      `yaml:"port"`                 // defaults to 8080
	DataDir            string   `yaml:"data_dir"`             // defaults to "./data"
	CSPImgSrc          []string `yaml:"csp_img_src"`          // extra external image sources for Content-Security-Policy img-src
	CORSAllowedOrigins []string `yaml:"cors_allowed_origins"` // exact browser origins allowed for cross-origin API access
}

type TilesConfig struct {
	RadiusKM  float64 `yaml:"radius_km"`
	CachePath string  `yaml:"cache_path"`
	SourceKey string  `yaml:"source_key"` // e.g. "20251215.pmtiles"; auto-discovered if empty
}

type CollectorsConfig struct {
	WeatherEcowitt      *EcowittConfig             `yaml:"weather_ecowitt"`
	WeatherEnvCanada    *EnvCanadaWeatherConfig    `yaml:"weather_envcanada"`
	AirQualityEnvCanada *EnvCanadaAirQualityConfig `yaml:"air_quality_envcanada"`
	SolarEnvCanada      *EnvCanadaSolarConfig      `yaml:"solar_envcanada"`
	Transit             *TransitConfig             `yaml:"transit"`
	Traffic             *TrafficConfig             `yaml:"traffic"`
	AircraftDump1090    *Dump1090Config            `yaml:"aircraft_dump1090"`
	House               *HouseConfig               `yaml:"house"`
	RSSFeeds            []RSSFeedConfig            `yaml:"rss_feeds"`
	FlagCanada          *FlagCanadaConfig          `yaml:"flag_canada"`
	Waterways           []WaterwayConfig           `yaml:"waterways"`
	Municipal           []MunicipalConfig          `yaml:"municipal"`
	IMAP                *IMAPConfig                `yaml:"imap"`
	WasteCollection     *WasteCollectionConfig     `yaml:"waste_collection"`
}

type EcowittConfig struct {
	Enabled      bool     `yaml:"enabled"`
	URL          string   `yaml:"url"`
	PollInterval Duration `yaml:"poll_interval"`
}

type EnvCanadaWeatherConfig struct {
	Enabled                    bool     `yaml:"enabled"`
	StationCode                string   `yaml:"station_code"` // optional: bypass auto-discovery
	Province                   string   `yaml:"province"`     // required when station_code is set
	PollIntervalForecast       Duration `yaml:"poll_interval_forecast"`
	PollIntervalHourlyForecast Duration `yaml:"poll_interval_hourly_forecast"`
	PollIntervalAlerts         Duration `yaml:"poll_interval_alerts"`
}

type EnvCanadaAirQualityConfig struct {
	Enabled      bool     `yaml:"enabled"`
	StationCode  string   `yaml:"station_code"` // optional: bypass auto-discovery
	Province     string   `yaml:"province"`     // required when station_code is set
	Location     string   `yaml:"location"`     // optional AQHI site override
	PollInterval Duration `yaml:"poll_interval"`
}

type EnvCanadaSolarConfig struct {
	Enabled      bool     `yaml:"enabled"`
	PollInterval Duration `yaml:"poll_interval"`
	Region       *int     `yaml:"region"` // nil = auto-select from lat/lon
}

type TransitConfig struct {
	RTPollInterval        Duration `yaml:"rt_poll_interval"`
	StaticRefreshInterval Duration `yaml:"static_refresh_interval"`
}

type TrafficConfig struct {
	Enabled               bool                  `yaml:"enabled"`
	PollIntervalIncidents Duration              `yaml:"poll_interval_incidents"`
	PollIntervalCameras   Duration              `yaml:"poll_interval_cameras"`
	Sources               []TrafficSourceConfig `yaml:"sources"`
}

type TrafficSourceConfig struct {
	ID                string `yaml:"id"`
	Type              string `yaml:"type"`
	Enabled           *bool  `yaml:"enabled"`
	Province          string `yaml:"province"`
	URL               string `yaml:"url"`
	IncidentsURL      string `yaml:"incidents_url"`
	CamerasURL        string `yaml:"cameras_url"`
	RoadConditionsURL string `yaml:"road_conditions_url"`
	PublicURL         string `yaml:"public_url"`
}

type Dump1090Config struct {
	Enabled      bool     `yaml:"enabled"`
	URL          string   `yaml:"url"`
	PollInterval Duration `yaml:"poll_interval"`
	RadiusKM     float64  `yaml:"radius_km"` // filter to this radius; 0 = no filter (default 250)
}

type HouseConfig struct {
	Enabled      bool     `yaml:"enabled"`
	URL          string   `yaml:"url"`
	PollInterval Duration `yaml:"poll_interval"`
}

type RSSFeedConfig struct {
	ID           string   `yaml:"id"`
	URL          string   `yaml:"url"`
	Category     string   `yaml:"category"`
	PollInterval Duration `yaml:"poll_interval"`
}

type FlagCanadaConfig struct {
	Enabled      bool     `yaml:"enabled"`
	URL          string   `yaml:"url"` // override default half-masting page URL
	PollInterval Duration `yaml:"poll_interval"`
}

type WaterwayConfig struct {
	ID           string   `yaml:"id"`
	Parser       string   `yaml:"parser"`
	URL          string   `yaml:"url"`
	PollInterval Duration `yaml:"poll_interval"`
	AlertOn      []string `yaml:"alert_on"`
}

type MunicipalConfig struct {
	ID           string   `yaml:"id"`
	Parser       string   `yaml:"parser"`
	URL          string   `yaml:"url"`
	PollInterval Duration `yaml:"poll_interval"`
	Domain       string   `yaml:"domain"`
}

type WasteCollectionConfig struct {
	Enabled   bool   `yaml:"enabled"`
	DataPath  string `yaml:"data_path"`
	WeekStart string `yaml:"week_start"` // "sunday" (default), "monday", ..., "saturday"
}

type IMAPConfig struct {
	Enabled      bool     `yaml:"enabled"`
	Host         string   `yaml:"host"`
	Port         int      `yaml:"port"`
	Username     string   `yaml:"username"`
	Password     string   `yaml:"password"`
	PollInterval Duration `yaml:"poll_interval"`
}

// Load reads a YAML config file, expands ${ENV_VAR} references, and parses it.
func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	expanded := os.ExpandEnv(string(raw))
	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return &cfg, nil
}
