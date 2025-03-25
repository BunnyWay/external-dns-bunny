// Copyright (c) BunnyWay d.o.o.
// SPDX-License-Identifier: Apache-2.0

package dnsprovider

import (
	"context"
	"errors"
	"fmt"
	"github.com/bunnyway/external-dns-bunny/internal/api"
	"github.com/sirupsen/logrus"
	"os"
	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"
	"sigs.k8s.io/external-dns/provider"
)

func NewProvider() provider.Provider {
	apiKey := os.Getenv("BUNNYNET_API_KEY")
	apiUrl := os.Getenv("BUNNYNET_API_URL")
	if apiUrl == "" {
		apiUrl = "https://api.bunny.net"
	}

	apiClient := api.NewClient(apiUrl, apiKey, "external-dns-bunny/dev")

	return bunnyProvider{
		api: apiClient,
	}
}

type bunnyProvider struct {
	provider.BaseProvider
	api *api.Client
}

func (b bunnyProvider) Records(ctx context.Context) ([]*endpoint.Endpoint, error) {
	zones, err := b.api.GetDnsZones(ctx)
	if err != nil {
		return nil, err
	}

	var endpoints []*endpoint.Endpoint
	for _, zone := range zones {
		for _, record := range zone.Records {
			e := convertRecordToEndpoint(zone, record)
			if e == nil {
				continue
			}

			endpoints = append(endpoints, e)
		}
	}

	return endpoints, nil
}

func convertRecordToEndpoint(zone api.DnsZone, record api.DnsRecord) *endpoint.Endpoint {
	name := record.Name + "." + zone.Domain
	if record.Name == "" {
		name = zone.Domain
	}

	var recordType string
	for k, v := range api.DNSRecordTypeMap {
		if v == record.Type {
			recordType = k
			break
		}
	}

	if recordType == "" {
		return nil
	}

	return endpoint.NewEndpointWithTTL(
		name,
		recordType,
		endpoint.TTL(record.Ttl),
		record.Value,
	)
}

func (b bunnyProvider) ApplyChanges(ctx context.Context, changes *plan.Changes) error {
	for _, change := range changes.Delete {
		err := b.endpointDelete(change)
		if err != nil {
			return err
		}
	}

	for _, change := range changes.UpdateOld {
		err := b.endpointDelete(change)
		if err != nil {
			return err
		}
	}

	for _, change := range changes.Create {
		err := b.endpointCreate(change)
		if err != nil {
			return err
		}
	}

	for _, change := range changes.UpdateNew {
		err := b.endpointCreate(change)
		if err != nil {
			return err
		}
	}

	return nil
}

func (b bunnyProvider) endpointCreate(endpoint *endpoint.Endpoint) error {
	logrus.WithField("endpoint", endpoint).Info("endpointCreate")

	subdomain, zone, err := b.api.SearchDnsZone(endpoint.DNSName)
	if err != nil {
		return err
	}

	logrus.WithFields(logrus.Fields{
		"zone.id":     zone.Id,
		"zone.domain": zone.Domain,
		"subdomain":   subdomain,
	}).Info("api.SearchDnsZone")

	recordType, ok := api.DNSRecordTypeMap[endpoint.RecordType]
	if !ok {
		return fmt.Errorf("Unsupported DNS record type: %s", endpoint.RecordType)
	}

	ttl := int64(endpoint.RecordTTL)
	if ttl < 1 {
		ttl = 300
	}

	for _, target := range endpoint.Targets {
		r := api.DnsRecord{
			Zone:  zone.Id,
			Name:  subdomain,
			Type:  recordType,
			Value: target,
			Ttl:   ttl,
		}

		err = b.api.CreateDnsRecord(r)
		if err != nil {
			return err
		}
	}

	return nil
}

func (b bunnyProvider) endpointDelete(endpoint *endpoint.Endpoint) error {
	logrus.WithField("endpoint", endpoint).Info("endpointDelete")

	recordType, ok := api.DNSRecordTypeMap[endpoint.RecordType]
	if !ok {
		return nil
	}

	subdomain, zone, err := b.api.SearchDnsZone(endpoint.DNSName)
	if err != nil {
		return err
	}

	logrus.WithFields(logrus.Fields{
		"zone.id":     zone.Id,
		"zone.domain": zone.Domain,
		"subdomain":   subdomain,
	}).Info("api.SearchDnsZone")

	for _, target := range endpoint.Targets {
		for _, record := range zone.Records {
			if record.Name != subdomain || record.Type != recordType || record.Value != target {
				continue
			}

			err = b.api.DeleteDnsRecord(zone.Id, record.Id)
			if err != nil {
				if errors.Is(err, api.ErrRecordNotFound) {
					continue
				}

				return err
			}
		}
	}

	return nil
}
