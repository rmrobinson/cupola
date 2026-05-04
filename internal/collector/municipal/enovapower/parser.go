// Package enovapower parses the Enova Power OMS outage API
// (POST https://oms.enovapower.com/Outages/Home/UpdatePushpin) into
// municipal.alerts.
// Register via import side-effect: _ "github.com/rmrobinson/cupola/internal/collector/municipal/enovapower"
// Config URL should be https://oms.enovapower.com/Outages/ — the parser appends /Home/UpdatePushpin.
package enovapower

import (
	"encoding/xml"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/rmrobinson/cupola/internal/collector/municipal"
	"github.com/rmrobinson/cupola/internal/domain"
)

func init() {
	municipal.RegisterAlertsParser("enova.power", func() municipal.AlertsParser {
		return &Parser{}
	})
}

// Parser implements municipal.AlertsParser for the Enova Power OMS.
type Parser struct{}

func (p *Parser) Parse(rawURL string) ([]domain.MunicipalAlert, error) {
	apiURL := strings.TrimRight(rawURL, "/") + "/Home/UpdatePushpin"

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Post(apiURL, "application/x-www-form-urlencoded", nil)
	if err != nil {
		return nil, fmt.Errorf("enova.power: post %s: %w", apiURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("enova.power: post %s: status %d", apiURL, resp.StatusCode)
	}

	var dataset omsDataset
	if err := xml.NewDecoder(resp.Body).Decode(&dataset); err != nil {
		return nil, fmt.Errorf("enova.power: decode XML: %w", err)
	}

	var alerts []domain.MunicipalAlert
	for _, c := range dataset.Cases {
		if a := caseToAlert(c); a != nil {
			alerts = append(alerts, *a)
		}
	}
	return alerts, nil
}

// omsDataset is the root XML element returned by the Enova OMS API.
type omsDataset struct {
	Cases []omsCase `xml:"OMSCASES"`
}

type omsCase struct {
	Serial      string `xml:"SERIAL"`
	Planned     string `xml:"PLANNED"`
	OutTime     string `xml:"OUTTIME"`
	InitCust    string `xml:"INITCUST"`
	CurCust     string `xml:"CURCUST"`
	RestoreTime string `xml:"RESTORETIM"`
	RestRange   string `xml:"RESTRANGE"`
	DescCause   string `xml:"DESC_CAUSE"`
	PublicMsg   string `xml:"PUBLICMSG"`
	WorkStat    string `xml:"WORKSTAT"`
	CaseStat    string `xml:"CASESTAT"`
	JobStat     string `xml:"JOBSTAT"`
}

func caseToAlert(c omsCase) *domain.MunicipalAlert {
	if c.Serial == "" {
		return nil
	}

	planned := strings.EqualFold(strings.TrimSpace(c.Planned), "true")
	title := buildTitle(c, planned)
	desc := strings.TrimSpace(c.PublicMsg)
	if desc == "" {
		desc = strings.TrimSpace(c.DescCause)
	}

	startsAt := parseOmsTime(c.OutTime)
	endsAt := parseOmsTime(c.RestoreTime)

	severity := domain.SeverityWarning
	if planned {
		severity = domain.SeverityInfo
	}

	var startsPtr, endsPtr *time.Time
	if !startsAt.IsZero() {
		startsPtr = &startsAt
	}
	if !endsAt.IsZero() {
		endsPtr = &endsAt
	}

	pub := startsAt
	if pub.IsZero() {
		pub = time.Now().UTC()
	}

	return &domain.MunicipalAlert{
		ID:          "enova.power:" + c.Serial,
		Title:       title,
		Description: desc,
		AlertType:   "power-outage",
		Severity:    severity,
		StartsAt:    startsPtr,
		EndsAt:      endsPtr,
		PublishedAt: pub,
	}
}

func buildTitle(c omsCase, planned bool) string {
	kind := "Unplanned"
	if planned {
		kind = "Planned"
	}
	cause := strings.TrimSpace(c.DescCause)
	if cause == "" || strings.EqualFold(cause, "under investigation") {
		cause = "Outage"
	}
	return fmt.Sprintf("Enova Power %s %s", kind, cause)
}

var omsTimeFormats = []string{
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05",
	"1/2/2006 3:04:05 PM",
	"1/2/2006 15:04:05",
}

func parseOmsTime(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	for _, f := range omsTimeFormats {
		if t, err := time.ParseInLocation(f, s, time.Local); err == nil {
			return t.UTC()
		}
	}
	log.Printf("[enova.power] unparseable time %q", s)
	return time.Time{}
}
