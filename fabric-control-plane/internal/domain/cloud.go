package domain

import (
	"net/netip"
	"regexp"
	"strings"
)

func validateCloudIntent(d Deployment) []Issue {
	issues := []Issue{}
	c := d.Cloud
	if c == nil {
		return issues
	}
	add := func(field, message string) {
		issues = append(issues, Issue{"error", "spec.deployment.cloud." + field, message})
	}
	if d.Engine != "bevel" {
		add("provider", "cloud settings require the Bevel engine")
	}
	if c.ClusterMode != "new" && c.ClusterMode != "existing" {
		add("clusterMode", "choose new or existing")
	}
	if c.VPCMode != "new" && c.VPCMode != "existing" {
		add("vpcMode", "choose new or existing")
	}
	if c.AccountID != "" && !regexp.MustCompile(`^[0-9]{12}$`).MatchString(c.AccountID) {
		add("accountId", "AWS account ID must contain 12 digits")
	}
	if c.MinNodes < 1 || c.DesiredNodes < c.MinNodes || c.MaxNodes < c.DesiredNodes || c.MaxNodes > 1000 {
		add("desiredNodes", "node counts must satisfy 1 <= minimum <= desired <= maximum <= 1000")
	}
	ranges := []netip.Prefix{}
	for _, item := range []struct{ key, value string }{{"vpcCidr", c.VPCCIDR}, {"serviceCidr", c.ServiceCIDR}, {"podCidr", c.PodCIDR}} {
		if item.value == "" {
			continue
		}
		p, err := netip.ParsePrefix(item.value)
		if err != nil || !p.Addr().Is4() {
			add(item.key, "use a valid IPv4 CIDR")
			continue
		}
		for _, previous := range ranges {
			if p.Overlaps(previous) {
				add(item.key, "cluster network ranges must not overlap")
			}
		}
		ranges = append(ranges, p)
	}
	values := []string{c.RoleARN, c.Region, c.TenantID, c.SubscriptionID, c.ResourceGroup, c.Domain, c.ClusterName, c.InstanceType, c.KubeVersion}
	values = append(values, c.Addons...)
	if len(c.Addons) > 32 {
		add("addons", "too many add-ons")
	}
	for _, value := range values {
		if len(value) > 256 || strings.ContainsAny(value, "\r\n\x00{}") {
			add("configuration", "values must be bounded plain text, without templates or control characters")
			break
		}
	}
	return issues
}
