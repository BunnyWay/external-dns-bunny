// Copyright (c) BunnyWay d.o.o.
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"errors"
	"io"
	"net/http"
)

type Client struct {
	apiKey    string
	apiUrl    string
	userAgent string
}

var ErrUnauthorized = errors.New("unauthorized")

var httpClient = &http.Client{
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

func (c *Client) doRequest(method string, url string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}

	req.Header.Add("AccessKey", c.apiKey)
	req.Header.Add("User-Agent", c.userAgent)

	if body != nil {
		req.Header.Add("Content-Type", "application/json")
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return resp, err
	}

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, ErrUnauthorized
	}

	return resp, err
}

func NewClient(apiUrl string, apiKey string, userAgent string) *Client {
	return &Client{
		apiKey:    apiKey,
		apiUrl:    apiUrl,
		userAgent: userAgent,
	}
}
