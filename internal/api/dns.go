// Copyright (c) BunnyWay d.o.o.
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/publicsuffix"
	"io"
	"net/http"
	"strconv"
	"strings"
)

var ErrRecordNotFound = errors.New("DNS record not found")

var DNSRecordTypeMap = map[string]uint8{
	"A":     0,
	"AAAA":  1,
	"CNAME": 2,
	"TXT":   3,
	"PZ":    7,
	"SRV":   8,
	"NS":    12,
}

type DnsZone struct {
	Id                            int64       `json:"Id,omitempty"`
	Domain                        string      `json:"Domain"`
	CustomNameserversEnabled      bool        `json:"CustomNameserversEnabled"`
	Nameserver1                   string      `json:"Nameserver1"`
	Nameserver2                   string      `json:"Nameserver2"`
	SoaEmail                      string      `json:"SoaEmail"`
	LoggingEnabled                bool        `json:"LoggingEnabled"`
	LoggingIPAnonymizationEnabled bool        `json:"LoggingIPAnonymizationEnabled"`
	LogAnonymizationType          uint8       `json:"LogAnonymizationType"`
	Records                       []DnsRecord `json:"Records"`
}

type DnsRecord struct {
	Zone                  int64   `json:"-"`
	Id                    int64   `json:"Id,omitempty"`
	Type                  uint8   `json:"Type"`
	Ttl                   int64   `json:"Ttl"`
	Value                 string  `json:"Value"`
	PullzoneId            int64   `json:"PullZoneId,omitempty"`
	Name                  string  `json:"Name"`
	Weight                int64   `json:"Weight,omitempty"`
	Priority              int64   `json:"Priority"`
	Port                  int64   `json:"Port"`
	Flags                 int64   `json:"Flags"`
	Tag                   string  `json:"Tag"`
	Accelerated           bool    `json:"Accelerated"`
	AcceleratedPullZoneId int64   `json:"AcceleratedPullZoneId"`
	LinkName              string  `json:"LinkName,omitempty"`
	MonitorType           uint8   `json:"MonitorType"`
	GeolocationLatitude   float64 `json:"GeolocationLatitude"`
	GeolocationLongitude  float64 `json:"GeolocationLongitude"`
	LatencyZone           string  `json:"LatencyZone"`
	SmartRoutingType      uint8   `json:"SmartRoutingType"`
	Disabled              bool    `json:"Disabled"`
	Comment               string  `json:"Comment"`
}

func (c *Client) GetDnsZones(ctx context.Context) ([]DnsZone, error) {
	resp, err := c.doRequest(http.MethodGet, fmt.Sprintf("%s/dnszone", c.apiUrl), nil)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	defer func() {
		resp.Body.Close()
		_, _ = io.Copy(io.Discard, resp.Body)
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		Items []DnsZone `json:"Items"`
	}

	err = json.Unmarshal(body, &result)
	if err != nil {
		return nil, err
	}

	return result.Items, nil
}

func (c *Client) SearchDnsZone(hostname string) (string, DnsZone, error) {
	eTLDp1, err := publicsuffix.EffectiveTLDPlusOne(hostname)
	if err != nil {
		return "", DnsZone{}, err
	}

	subdomain := ""
	if hostname != eTLDp1 {
		var found bool
		subdomain, found = strings.CutSuffix(hostname, eTLDp1)
		if !found {
			logrus.WithFields(logrus.Fields{
				"hostname": hostname,
				"eTLDp1":   eTLDp1,
			}).Warn("strings.CutSuffix failed")

			return "", DnsZone{}, ErrRecordNotFound
		}

		subdomain = subdomain[0 : len(subdomain)-1]
	}

	zone, err := c.searchDnsZone(eTLDp1)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"err":       err,
			"hostname":  hostname,
			"subdomain": subdomain,
			"eTLDp1":    eTLDp1,
		}).Error("SearchDnsZone")

		return "", DnsZone{}, errors.New("no DnsZone found")
	}

	return subdomain, zone, nil
}

func (c *Client) searchDnsZone(domain string) (DnsZone, error) {
	var data DnsZone
	resp, err := c.doRequest(http.MethodGet, fmt.Sprintf("%s/dnszone?search=%s", c.apiUrl, domain), nil)
	if err != nil {
		return data, err
	}

	if resp.StatusCode != http.StatusOK {
		return data, errors.New(resp.Status)
	}

	bodyResp, err := io.ReadAll(resp.Body)
	if err != nil {
		return data, err
	}

	_ = resp.Body.Close()
	var result struct {
		Items []DnsZone `json:"Items"`
	}

	err = json.Unmarshal(bodyResp, &result)
	if err != nil {
		return data, err
	}

	for _, record := range result.Items {
		if record.Domain == domain {
			return record, nil
		}
	}

	return data, fmt.Errorf("DNS zone \"%s\" not found", domain)
}

func (c *Client) GetDnsZone(id int64) (DnsZone, error) {
	var data DnsZone
	resp, err := c.doRequest(http.MethodGet, fmt.Sprintf("%s/dnszone/%d", c.apiUrl, id), nil)
	if err != nil {
		return data, err
	}

	if resp.StatusCode != http.StatusOK {
		return data, errors.New(resp.Status)
	}

	bodyResp, err := io.ReadAll(resp.Body)
	if err != nil {
		return data, err
	}

	_ = resp.Body.Close()
	err = json.Unmarshal(bodyResp, &data)
	if err != nil {
		return data, err
	}

	return data, nil
}

func (c *Client) GetDnsRecord(zoneId int64, id int64) (DnsRecord, error) {
	zone, err := c.GetDnsZone(zoneId)
	if err != nil {
		return DnsRecord{}, err
	}

	for _, record := range zone.Records {
		if record.Id == id {
			record.Zone = zoneId
			return record, nil
		}
	}

	return DnsRecord{}, errors.New("DNS record not found")
}

func (c *Client) CreateDnsRecord(data DnsRecord) error {
	dnsZoneId := data.Zone
	if dnsZoneId == 0 {
		return errors.New("zone is required")
	}

	data, err := convertDnsRecordForApiSave(data)
	if err != nil {
		return err
	}

	body, err := json.Marshal(data)
	if err != nil {
		return err
	}

	resp, err := c.doRequest(http.MethodPut, fmt.Sprintf("%s/dnszone/%d/records", c.apiUrl, dnsZoneId), bytes.NewReader(body))
	if err != nil {
		return err
	}

	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusCreated {
		return errors.New("create DNS record failed with " + resp.Status)
	}

	return nil
}

func convertDnsRecordForApiSave(record DnsRecord) (DnsRecord, error) {
	if record.Type == DNSRecordTypeMap["PZ"] {
		if len(record.LinkName) == 0 {
			return DnsRecord{}, errors.New("linkname should contain the Pullzone ID")
		}

		id, err := strconv.ParseInt(record.LinkName, 10, 64)
		if err != nil {
			return DnsRecord{}, err
		}

		record.PullzoneId = id
		record.LinkName = ""
		record.Value = ""
	}

	return record, nil
}

func (c *Client) DeleteDnsRecord(zoneId int64, id int64) error {
	resp, err := c.doRequest(http.MethodDelete, fmt.Sprintf("%s/dnszone/%d/records/%d", c.apiUrl, zoneId, id), nil)
	if err != nil {
		return err
	}

	if resp.StatusCode == http.StatusNotFound {
		return ErrRecordNotFound
	}

	if resp.StatusCode != http.StatusNoContent {
		return errors.New(resp.Status)
	}

	return nil
}
