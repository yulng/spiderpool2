// Copyright 2022 Authors of spidernet-io
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"os"
	"strings"

	"github.com/spidernet-io/spiderpool/pkg/constant"
	spiderpoolip "github.com/spidernet-io/spiderpool/pkg/ip"
)

const (
	ENVNamespace                = "SPIDERPOOL_NAMESPACE"
	ENVSpiderpoolControllerName = "SPIDERPOOL_CONTROLLER_NAME"

	ENVDefaultIPv4SubnetName = "SPIDERPOOL_INIT_DEFAULT_IPV4_SUBNET_NAME"
	ENVDefaultIPv4IPPoolName = "SPIDERPOOL_INIT_DEFAULT_IPV4_IPPOOL_NAME"
	ENVDefaultIPv4CIDR       = "SPIDERPOOL_INIT_DEFAULT_IPV4_IPPOOL_SUBNET"
	ENVDefaultIPv4IPRanges   = "SPIDERPOOL_INIT_DEFAULT_IPV4_IPPOOL_IPRANGES"
	ENVDefaultIPv4Gateway    = "SPIDERPOOL_INIT_DEFAULT_IPV4_IPPOOL_GATEWAY"

	ENVDefaultIPv6SubnetName = "SPIDERPOOL_INIT_DEFAULT_IPV6_SUBNET_NAME"
	ENVDefaultIPv6IPPoolName = "SPIDERPOOL_INIT_DEFAULT_IPV6_IPPOOL_NAME"
	ENVDefaultIPv6CIDR       = "SPIDERPOOL_INIT_DEFAULT_IPV6_IPPOOL_SUBNET"
	ENVDefaultIPv6IPRanges   = "SPIDERPOOL_INIT_DEFAULT_IPV6_IPPOOL_IPRANGES"
	ENVDefaultIPv6Gateway    = "SPIDERPOOL_INIT_DEFAULT_IPV6_IPPOOL_GATEWAY"
)

type InitDefaultConfig struct {
	Namespace      string
	ControllerName string

	V4SubnetName string
	V4IPPoolName string
	V4CIDR       string
	V4IPRanges   []string
	V4Gateway    string

	V6SubnetName string
	V6IPPoolName string
	V6CIDR       string
	V6IPRanges   []string
	V6Gateway    string
}

func NewInitDefaultConfig() InitDefaultConfig {
	return parseENVAsDefault()
}

func parseENVAsDefault() InitDefaultConfig {
	config := InitDefaultConfig{}
	config.Namespace = strings.ReplaceAll(os.Getenv(ENVNamespace), "\"", "")
	if len(config.Namespace) == 0 {
		logger.Sugar().Fatalf("ENV %s %w", ENVNamespace, constant.ErrMissingRequiredParam)
	}
	config.ControllerName = strings.ReplaceAll(os.Getenv(ENVSpiderpoolControllerName), "\"", "")
	if len(config.ControllerName) == 0 {
		logger.Sugar().Fatalf("ENV %s %w", ENVSpiderpoolControllerName, constant.ErrMissingRequiredParam)
	}

	// IPv4
	config.V4SubnetName = strings.ReplaceAll(os.Getenv(ENVDefaultIPv4SubnetName), "\"", "")
	config.V4IPPoolName = strings.ReplaceAll(os.Getenv(ENVDefaultIPv4IPPoolName), "\"", "")
	if len(config.V4SubnetName) == 0 && len(config.V4IPPoolName) == 0 {
		logger.Info("Ignore creating default IPv4 Subnet or IPPool")
		return config
	}

	config.V4CIDR = strings.ReplaceAll(os.Getenv(ENVDefaultIPv4CIDR), "\"", "")
	if len(config.V4CIDR) == 0 {
		logger.Sugar().Fatalf("ENV %s %w, if you need to create a default IPv4 Subnet or IPPool", ENVDefaultIPv4CIDR, constant.ErrMissingRequiredParam)
	}
	if err := spiderpoolip.IsCIDR(constant.IPv4, config.V4CIDR); err != nil {
		logger.Sugar().Fatalf("ENV %s %s: %v", ENVDefaultIPv4CIDR, config.V4CIDR, err)
	}

	config.V4Gateway = strings.ReplaceAll(os.Getenv(ENVDefaultIPv4Gateway), "\"", "")
	if len(config.V4Gateway) > 0 {
		if err := spiderpoolip.IsIP(constant.IPv4, config.V4Gateway); err != nil {
			logger.Sugar().Fatalf("ENV %s %s: %v", ENVDefaultIPv4Gateway, config.V4Gateway, err)
		}
	}

	v := os.Getenv(ENVDefaultIPv4IPRanges)
	if len(v) > 0 {
		v = strings.ReplaceAll(v, "\"", "")
		v = strings.ReplaceAll(v, "\\", "")
		v = strings.ReplaceAll(v, "[", "")
		v = strings.ReplaceAll(v, "]", "")
		v = strings.ReplaceAll(v, ",", " ")
		ranges := strings.Fields(v)

		for _, r := range ranges {
			if err := spiderpoolip.IsIPRange(constant.IPv4, r); err != nil {
				logger.Sugar().Fatalf("ENV %s %s: %v", ENVDefaultIPv4IPRanges, ranges, err)
			}
		}
		config.V4IPRanges = ranges
	}

	// IPv6
	config.V6SubnetName = strings.ReplaceAll(os.Getenv(ENVDefaultIPv6SubnetName), "\"", "")
	config.V6IPPoolName = strings.ReplaceAll(os.Getenv(ENVDefaultIPv6IPPoolName), "\"", "")
	if len(config.V6SubnetName) == 0 && len(config.V6IPPoolName) == 0 {
		logger.Info("Ignore creating default IPv6 Subnet or IPPool")
		return config
	}

	config.V6CIDR = strings.ReplaceAll(os.Getenv(ENVDefaultIPv6CIDR), "\"", "")
	if len(config.V6CIDR) == 0 {
		logger.Sugar().Fatalf("ENV %s %w, if you need to create a default IPv6 Subnet or IPPool", ENVDefaultIPv6CIDR, constant.ErrMissingRequiredParam)
	}
	if err := spiderpoolip.IsCIDR(constant.IPv6, config.V6CIDR); err != nil {
		logger.Sugar().Fatalf("ENV %s %s: %v", ENVDefaultIPv6CIDR, config.V6CIDR, err)
	}

	config.V6Gateway = strings.ReplaceAll(os.Getenv(ENVDefaultIPv6Gateway), "\"", "")
	if len(config.V6Gateway) > 0 {
		if err := spiderpoolip.IsIP(constant.IPv6, config.V6Gateway); err != nil {
			logger.Sugar().Fatalf("ENV %s %s: %v", ENVDefaultIPv6Gateway, config.V6Gateway, err)
		}
	}

	v = os.Getenv(ENVDefaultIPv6IPRanges)
	if len(v) > 0 {
		v = strings.ReplaceAll(v, "\"", "")
		v = strings.ReplaceAll(v, "\\", "")
		v = strings.ReplaceAll(v, "[", "")
		v = strings.ReplaceAll(v, "]", "")
		v = strings.ReplaceAll(v, ",", " ")
		ranges := strings.Fields(v)

		for _, r := range ranges {
			if err := spiderpoolip.IsIPRange(constant.IPv6, r); err != nil {
				logger.Sugar().Fatalf("ENV %s %s: %v", ENVDefaultIPv6IPRanges, ranges, err)
			}
		}
		config.V6IPRanges = ranges
	}

	if config.V4SubnetName == config.V6SubnetName && len(config.V4SubnetName) != 0 {
		logger.Sugar().Fatalf(
			"ENV %s %s\nENV %s %s\nDefault IPv4 Subnet name cannot be the same as IPv6 one",
			ENVDefaultIPv4SubnetName,
			config.V4SubnetName,
			ENVDefaultIPv6SubnetName,
			config.V6SubnetName,
		)
	}
	if config.V4IPPoolName == config.V6IPPoolName && len(config.V4IPPoolName) != 0 {
		logger.Sugar().Fatalf(
			"ENV %s %s\nENV %s %s\nDefault IPv4 IPPool name cannot be the same as IPv6 one",
			ENVDefaultIPv4IPPoolName,
			config.V4IPPoolName,
			ENVDefaultIPv6IPPoolName,
			config.V6IPPoolName,
		)
	}
	logger.Sugar().Infof("Init default config: %+v", config)

	return config
}
