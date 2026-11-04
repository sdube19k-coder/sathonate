package deployment

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestIsolationDoesNotGrantGlobalEgressOrCrossNamespaceFabric(t *testing.T) {
	raw, err := json.Marshal(isolationManifest("customer", []string{"10.96.0.1", "172.18.0.3"}))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, forbidden := range []string{"0.0.0.0/0", "::/0", "ClusterRole", "hostNetwork", "privileged"} {
		if strings.Contains(text, forbidden) {
			t.Fatal("broad isolation escape", forbidden)
		}
	}
	var list struct {
		Items []struct {
			Kind string
			Spec map[string]any
		}
	}
	if json.Unmarshal(raw, &list) != nil {
		t.Fatal("invalid manifest")
	}
	policies := 0
	for _, item := range list.Items {
		if item.Kind != "NetworkPolicy" {
			continue
		}
		policies++
		for _, direction := range []string{"egress", "ingress"} {
			rules, _ := item.Spec[direction].([]any)
			for _, v := range rules {
				rule := v.(map[string]any)
				key := "to"
				if direction == "ingress" {
					key = "from"
				}
				peers, _ := rule[key].([]any)
				for _, p := range peers {
					peer := p.(map[string]any)
					if selector, ok := peer["namespaceSelector"]; ok {
						labels, ok := selector.(map[string]any)["matchLabels"].(map[string]any)
						if !ok || labels["kubernetes.io/metadata.name"] != "kube-system" {
							t.Fatal("cross namespace allowance")
						}
					}
				}
			}
		}
	}
	if policies != 3 {
		t.Fatal("containment policies missing")
	}
}
