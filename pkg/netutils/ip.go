// Licensed to FORTH/ICS under one or more contributor
// license agreements. See the NOTICE file distributed with
// this work for additional information regarding copyright
// ownership. FORTH/ICS licenses this file to you under
// the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package netutils

import (
	"io/ioutil"
	"net"
	"net/http"

	"github.com/pkg/errors"
)

// GetPublicIP asks a public IP API to return our public IP.
func GetPublicIP() (net.IP, error) {
	url := "https://api.ipify.org?format=text"
	// https://www.ipify.org
	// http://myexternalip.com
	// http://api.ident.me
	// http://whatismyipaddress.com/api
	resp, err := http.Get(url)

	if err != nil {
		return nil, errors.Wrap(err, "cannot contact public IP address API")
	}

	defer resp.Body.Close()

	ipStr, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "ip decoding error")
	}

	return net.ParseIP(string(ipStr)), nil
}