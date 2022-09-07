// Copyright 2022 Authors of spidernet-io
// SPDX-License-Identifier: Apache-2.0

package ip

import (
	"bytes"
	"fmt"
	"net"
	"sort"
	"strings"

	"github.com/asaskevich/govalidator"

	"github.com/spidernet-io/spiderpool/pkg/constant"
	"github.com/spidernet-io/spiderpool/pkg/types"
)

func ParseIPRanges(version types.IPVersion, ipRanges []string) ([]net.IP, error) {
	var sum []net.IP
	for _, r := range ipRanges {
		ips, err := ParseIPRange(version, r)
		if err != nil {
			return nil, err
		}
		sum = append(sum, ips...)
	}

	return sum, nil
}

func ParseIPRange(version types.IPVersion, ipRange string) ([]net.IP, error) {
	if err := IsIPRange(version, ipRange); err != nil {
		return nil, err
	}

	arr := strings.Split(ipRange, "-")
	n := len(arr)
	var ips []net.IP
	if n == 1 {
		ips = append(ips, net.ParseIP(arr[0]))
	}

	if n == 2 {
		cur := net.ParseIP(arr[0])
		end := net.ParseIP(arr[1])
		for Cmp(cur, end) <= 0 {
			ips = append(ips, cur)
			cur = NextIP(cur)
		}
	}

	return ips, nil
}

func ConvertIPsToIPRanges(version types.IPVersion, ips []net.IP) ([]string, error) {
	if err := IsIPVersion(version); err != nil {
		return nil, err
	}

	for _, ip := range ips {
		if (version == constant.IPv4 && ip.To4() == nil) ||
			(version == constant.IPv6 && ip.To4() != nil) {
			return nil, fmt.Errorf("%wv%d IP '%s'", ErrInvalidIP, version, ip.String())
		}
	}

	sort.Slice(ips, func(i, j int) bool {
		return bytes.Compare(ips[i].To16(), ips[j].To16()) < 0
	})

	var ipRanges []string
	var start, end int
	for {
		if start == len(ips) {
			break
		}

		if end+1 < len(ips) && ips[end+1].Equal(NextIP(ips[end])) {
			end++
			continue
		}

		if start == end {
			ipRanges = append(ipRanges, ips[start].String())
		} else {
			ipRanges = append(ipRanges, fmt.Sprintf("%s-%s", ips[start], ips[end]))
		}

		start = end + 1
		end = start
	}

	return ipRanges, nil
}

func ContainsIPRange(version types.IPVersion, subnet string, ipRange string) (bool, error) {
	ipNet, err := ParseCIDR(version, subnet)
	if err != nil {
		return false, err
	}
	ips, err := ParseIPRange(version, ipRange)
	if err != nil {
		return false, err
	}

	n := len(ips)
	if n == 1 {
		return ipNet.Contains(ips[0]), nil
	}

	return ipNet.Contains(ips[0]) && ipNet.Contains(ips[n-1]), nil
}

func IsIPRangeOverlap(version types.IPVersion, ipRange1, ipRange2 string) (bool, error) {
	if err := IsIPVersion(version); err != nil {
		return false, err
	}
	if err := IsIPRange(version, ipRange1); err != nil {
		return false, err
	}
	if err := IsIPRange(version, ipRange2); err != nil {
		return false, err
	}

	ips1, _ := ParseIPRange(version, ipRange1)
	ips2, _ := ParseIPRange(version, ipRange2)
	if len(ips1) > len(IPsDiffSet(ips1, ips2)) {
		return true, nil
	}

	return false, nil
}

// IsIPRange verifies the format of the IP range string. An IP range can
// be an single IP address in the style of '192.168.1.0', or an address
// range in the form of '192.168.1.0-192.168.1.10'.
//
// The following formats are invalid:
// 1. '192.168.1.0 - 192.168.1.10', there can be no space between two IP addresses.
// 2. '192.168.1.1-2001:db8:a0b:12f0::1', invalid combination of IPv4 and IPv6.
// 3. '192.168.1.10-192.168.1.1', the IP range must be ordered.
func IsIPRange(version types.IPVersion, ipRange string) error {
	if err := IsIPVersion(version); err != nil {
		return err
	}

	if (version == constant.IPv4 && !IsIPv4IPRange(ipRange)) ||
		(version == constant.IPv6 && !IsIPv6IPRange(ipRange)) {
		return fmt.Errorf("%w in IPv%d '%s'", ErrInvalidIPRangeFormat, version, ipRange)
	}

	return nil
}

func IsIPv4IPRange(ipRange string) bool {
	ips := strings.Split(ipRange, "-")
	n := len(ips)
	if n > 2 {
		return false
	}

	if n == 1 {
		return govalidator.IsIPv4(ips[0])
	}

	if n == 2 {
		if !govalidator.IsIPv4(ips[0]) || !govalidator.IsIPv4(ips[1]) {
			return false
		}
		if Cmp(net.ParseIP(ips[0]), net.ParseIP(ips[1])) == 1 {
			return false
		}
	}

	return true
}

func IsIPv6IPRange(ipRange string) bool {
	ips := strings.Split(ipRange, "-")
	n := len(ips)
	if n > 2 {
		return false
	}

	if n == 1 {
		return govalidator.IsIPv6(ips[0])
	}

	if n == 2 {
		if !govalidator.IsIPv6(ips[0]) || !govalidator.IsIPv6(ips[1]) {
			return false
		}
		if Cmp(net.ParseIP(ips[0]), net.ParseIP(ips[1])) == 1 {
			return false
		}
	}

	return true
}
