// Copyright 2017 Istio Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package model

import (
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/gogo/protobuf/proto"
	"github.com/hashicorp/go-multierror"

	"istio.io/istio/pkg/config"
	"istio.io/istio/pkg/config/labels"
)

// UnixAddressPrefix is the prefix used to indicate an address is for a Unix Domain socket. It is used in
// ServiceEntry.Endpoint.Address message.
const UnixAddressPrefix = "unix://"

// Validate checks that each name conforms to the spec and has a ProtoMessage
func (descriptor ConfigDescriptor) Validate() error {
	var errs error
	descriptorTypes := make(map[string]bool)
	messages := make(map[string]bool)
	clusterMessages := make(map[string]bool)

	for _, v := range descriptor {
		if !labels.IsDNS1123Label(v.Type) {
			errs = multierror.Append(errs, fmt.Errorf("invalid type: %q", v.Type))
		}
		if !labels.IsDNS1123Label(v.Plural) {
			errs = multierror.Append(errs, fmt.Errorf("invalid plural: %q", v.Type))
		}
		if proto.MessageType(v.MessageName) == nil {
			errs = multierror.Append(errs, fmt.Errorf("cannot discover proto message type: %q", v.MessageName))
		}
		if _, exists := descriptorTypes[v.Type]; exists {
			errs = multierror.Append(errs, fmt.Errorf("duplicate type: %q", v.Type))
		}
		descriptorTypes[v.Type] = true
		if v.ClusterScoped {
			if _, exists := clusterMessages[v.MessageName]; exists {
				errs = multierror.Append(errs, fmt.Errorf("duplicate message type: %q", v.MessageName))
			}
			clusterMessages[v.MessageName] = true
		} else {
			if _, exists := messages[v.MessageName]; exists {
				errs = multierror.Append(errs, fmt.Errorf("duplicate message type: %q", v.MessageName))
			}
			messages[v.MessageName] = true
		}
	}
	return errs
}

// Validate ensures that the service object is well-defined
func (s *Service) Validate() error {
	var errs error
	if len(s.Hostname) == 0 {
		errs = multierror.Append(errs, fmt.Errorf("invalid empty hostname"))
	}
	parts := strings.Split(string(s.Hostname), ".")
	for _, part := range parts {
		if !labels.IsDNS1123Label(part) {
			errs = multierror.Append(errs, fmt.Errorf("invalid hostname part: %q", part))
		}
	}

	// Require at least one port
	if len(s.Ports) == 0 {
		errs = multierror.Append(errs, fmt.Errorf("service must have at least one declared port"))
	}

	// Port names can be empty if there exists only one port
	for _, port := range s.Ports {
		if port.Name == "" {
			if len(s.Ports) > 1 {
				errs = multierror.Append(errs,
					fmt.Errorf("empty port names are not allowed for services with multiple ports"))
			}
		} else if !labels.IsDNS1123Label(port.Name) {
			errs = multierror.Append(errs, fmt.Errorf("invalid name: %q", port.Name))
		}
		if err := config.ValidatePort(port.Port); err != nil {
			errs = multierror.Append(errs,
				fmt.Errorf("invalid service port value %d for %q: %v", port.Port, port.Name, err))
		}
	}
	return errs
}

// Validate ensures that the service instance is well-defined
func (instance *ServiceInstance) Validate() error {
	var errs error
	if instance.Service == nil {
		errs = multierror.Append(errs, fmt.Errorf("missing service in the instance"))
	} else if err := instance.Service.Validate(); err != nil {
		errs = multierror.Append(errs, err)
	}

	if err := instance.Labels.Validate(); err != nil {
		errs = multierror.Append(errs, err)
	}

	if err := config.ValidatePort(instance.Endpoint.Port); err != nil {
		errs = multierror.Append(errs, err)
	}

	port := instance.Endpoint.ServicePort
	if port == nil {
		errs = multierror.Append(errs, fmt.Errorf("missing service port"))
	} else if instance.Service != nil {
		expected, ok := instance.Service.Ports.Get(port.Name)
		if !ok {
			errs = multierror.Append(errs, fmt.Errorf("missing service port %q", port.Name))
		} else {
			if expected.Port != port.Port {
				errs = multierror.Append(errs,
					fmt.Errorf("unexpected service port value %d, expected %d", port.Port, expected.Port))
			}
			if expected.Protocol != port.Protocol {
				errs = multierror.Append(errs,
					fmt.Errorf("unexpected service protocol %s, expected %s", port.Protocol, expected.Protocol))
			}
		}
	}

	return errs
}

// ValidateNetworkEndpointAddress checks the Address field of a NetworkEndpoint. If the family is TCP, it checks the
// address is a valid IP address. If the family is Unix, it checks the address is a valid socket file path.
func ValidateNetworkEndpointAddress(n *NetworkEndpoint) error {
	switch n.Family {
	case AddressFamilyTCP:
		ipAddr := net.ParseIP(n.Address) // Typically it is an IP address
		if ipAddr == nil {
			if err := config.ValidateFQDN(n.Address); err != nil { // Otherwise could be an FQDN
				return errors.New("invalid address " + n.Address)
			}
		}
	case AddressFamilyUnix:
		return config.ValidateUnixAddress(n.Address)
	default:
		panic(fmt.Sprintf("unhandled Family %v", n.Family))
	}
	return nil
}
