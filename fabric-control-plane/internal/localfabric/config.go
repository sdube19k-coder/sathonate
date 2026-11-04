package localfabric

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type object = map[string]any

func compose(root string, passwords map[string]string) object {
	services := object{}
	volumes := object{}
	mount := filepath.ToSlash(root) + ":/work"
	for i, o := range []string{"orderers", "org1", "org2"} {
		services["ca-"+o] = object{
			"image": "hyperledger/fabric-ca:" + CAVersion, "container_name": project + "-ca-" + o, "command": []string{"fabric-ca-server", "start", "--config", "/etc/hyperledger/fabric-ca-server/smp-bootstrap.json"},
			"environment": object{"FABRIC_CA_SERVER_HOME": "/etc/hyperledger/fabric-ca-server", "FABRIC_CA_SERVER_CA_NAME": "ca-" + o, "FABRIC_CA_SERVER_TLS_ENABLED": "true", "FABRIC_CA_SERVER_CSR_HOSTS": "ca-" + o + ",localhost", "FABRIC_CA_SERVER_PORT": "7054"},
			"volumes":     []string{filepath.ToSlash(filepath.Join(root, "cas", o)) + ":/etc/hyperledger/fabric-ca-server"}, "ports": []string{fmt.Sprintf("127.0.0.1:%d:7054", 17054+i*1000)}, "networks": []string{"fabric"}, "restart": "unless-stopped",
		}
	}
	for i := 1; i <= 3; i++ {
		n := fmt.Sprintf("orderer%d", i)
		base := "/work/organizations/orderers/" + n
		volumes[n] = object{}
		services[n] = object{"image": "hyperledger/fabric-orderer:" + Version, "container_name": project + "-" + n, "command": []string{"orderer"}, "environment": object{
			"FABRIC_LOGGING_SPEC": "INFO", "ORDERER_GENERAL_LISTENADDRESS": "0.0.0.0", "ORDERER_GENERAL_LISTENPORT": "7050", "ORDERER_GENERAL_LOCALMSPID": "OrdererMSP", "ORDERER_GENERAL_LOCALMSPDIR": base + "/msp", "ORDERER_GENERAL_BOOTSTRAPMETHOD": "none", "ORDERER_CHANNELPARTICIPATION_ENABLED": "true",
			"ORDERER_GENERAL_TLS_ENABLED": "true", "ORDERER_GENERAL_TLS_PRIVATEKEY": base + "/tls/server.key", "ORDERER_GENERAL_TLS_CERTIFICATE": base + "/tls/server.crt", "ORDERER_GENERAL_TLS_ROOTCAS": "[" + base + "/tls/ca.crt]",
			"ORDERER_GENERAL_CLUSTER_CLIENTCERTIFICATE": base + "/tls/server.crt", "ORDERER_GENERAL_CLUSTER_CLIENTPRIVATEKEY": base + "/tls/server.key", "ORDERER_GENERAL_CLUSTER_ROOTCAS": "[" + base + "/tls/ca.crt]",
			"ORDERER_ADMIN_LISTENADDRESS": "0.0.0.0:7053", "ORDERER_ADMIN_TLS_ENABLED": "true", "ORDERER_ADMIN_TLS_CERTIFICATE": base + "/tls/server.crt", "ORDERER_ADMIN_TLS_PRIVATEKEY": base + "/tls/server.key", "ORDERER_ADMIN_TLS_ROOTCAS": "[" + base + "/tls/ca.crt]", "ORDERER_ADMIN_TLS_CLIENTROOTCAS": "[" + base + "/tls/ca.crt]",
			"ORDERER_OPERATIONS_LISTENADDRESS": "0.0.0.0:9443",
		}, "volumes": []string{mount + ":ro", n + ":/var/hyperledger/production/orderer"}, "ports": []string{fmt.Sprintf("127.0.0.1:%d:7050", 17050+(i-1)*1000)}, "networks": []string{"fabric"}, "restart": "unless-stopped"}
	}
	for i := 1; i <= 2; i++ {
		n := fmt.Sprintf("peer%d", i)
		o := fmt.Sprintf("org%d", i)
		base := "/work/organizations/" + o + "/" + n
		volumes[n] = object{}
		services[n] = object{"image": "hyperledger/fabric-peer:" + Version, "container_name": project + "-" + n, "command": []string{"peer", "node", "start"}, "environment": object{
			"FABRIC_LOGGING_SPEC": "INFO", "CORE_PEER_ID": n, "CORE_PEER_ADDRESS": n + ":7051", "CORE_PEER_LISTENADDRESS": "0.0.0.0:7051", "CORE_PEER_CHAINCODEADDRESS": n + ":7052", "CORE_PEER_CHAINCODELISTENADDRESS": "0.0.0.0:7052", "CORE_PEER_GOSSIP_EXTERNALENDPOINT": n + ":7051", "CORE_PEER_GOSSIP_BOOTSTRAP": n + ":7051", "CORE_PEER_LOCALMSPID": fmt.Sprintf("Org%dMSP", i), "CORE_PEER_MSPCONFIGPATH": base + "/msp",
			"CORE_PEER_TLS_ENABLED": "true", "CORE_PEER_TLS_CERT_FILE": base + "/tls/server.crt", "CORE_PEER_TLS_KEY_FILE": base + "/tls/server.key", "CORE_PEER_TLS_ROOTCERT_FILE": base + "/tls/ca.crt", "CORE_LEDGER_STATE_STATEDATABASE": "goleveldb", "CORE_OPERATIONS_LISTENADDRESS": "0.0.0.0:9443",
		}, "volumes": []string{mount + ":ro", n + ":/var/hyperledger/production"}, "ports": []string{fmt.Sprintf("127.0.0.1:%d:7051", 17051+(i-1)*1000)}, "networks": []string{"fabric"}, "restart": "unless-stopped"}
	}
	services["tools"] = object{"image": "hyperledger/fabric-tools:" + Version, "container_name": project + "-tools", "command": []string{"sleep", "infinity"}, "environment": object{"FABRIC_CFG_PATH": "/work"}, "volumes": []string{mount}, "networks": []string{"fabric"}, "restart": "unless-stopped"}
	id, _ := os.ReadFile(filepath.Join(root, "contract-id.txt"))
	services["contract"] = object{"image": "hyperledger/fabric-tools:" + Version, "container_name": project + "-contract", "command": []string{"/contract/local-chaincode"}, "environment": object{"CHAINCODE_ID": strings.TrimSpace(string(id))}, "volumes": []string{filepath.ToSlash(filepath.Join(root, "contract-bin")) + ":/contract:ro", filepath.ToSlash(filepath.Join(root, "organizations", "org1", "contract", "tls")) + ":/tls:ro"}, "networks": []string{"fabric"}, "restart": "unless-stopped"}
	for i := 1; i <= 2; i++ {
		services[fmt.Sprintf("peer%d", i)].(object)["environment"].(object)["CORE_CHAINCODE_EXTERNALBUILDERS"] = `[{"name":"ccaas_builder","path":"/opt/hyperledger/ccaas_builder","propagateEnvironment":["CHAINCODE_AS_A_SERVICE_BUILDER_CONFIG"]}]`
	}
	return object{"name": project, "services": services, "volumes": volumes, "networks": object{"fabric": object{"name": project}}}
}

func policies(msp string, orderer bool) object {
	write := "OR('" + msp + ".admin', '" + msp + ".client')"
	read := "OR('" + msp + ".admin', '" + msp + ".peer', '" + msp + ".client')"
	if orderer {
		read = "OR('" + msp + ".member')"
		write = read
	}
	return object{"Readers": object{"Type": "Signature", "Rule": read}, "Writers": object{"Type": "Signature", "Rule": write}, "Admins": object{"Type": "Signature", "Rule": "OR('" + msp + ".admin')"}, "Endorsement": object{"Type": "Signature", "Rule": "OR('" + msp + ".peer')"}}
}
func implicit(orderer bool) object {
	p := object{"Readers": object{"Type": "ImplicitMeta", "Rule": "ANY Readers"}, "Writers": object{"Type": "ImplicitMeta", "Rule": "ANY Writers"}, "Admins": object{"Type": "ImplicitMeta", "Rule": "MAJORITY Admins"}}
	if orderer {
		p["BlockValidation"] = object{"Type": "ImplicitMeta", "Rule": "ANY Writers"}
	} else {
		p["LifecycleEndorsement"] = object{"Type": "ImplicitMeta", "Rule": "MAJORITY Endorsement"}
		p["Endorsement"] = object{"Type": "ImplicitMeta", "Rule": "MAJORITY Endorsement"}
	}
	return p
}
func configtx() object {
	orderOrg := object{"Name": "OrdererOrg", "ID": "OrdererMSP", "MSPDir": "/work/organizations/orderers/msp", "Policies": policies("OrdererMSP", true), "OrdererEndpoints": []string{"orderer1:7050", "orderer2:7050", "orderer3:7050"}}
	orgs := []object{}
	for i := 1; i <= 2; i++ {
		m := fmt.Sprintf("Org%dMSP", i)
		orgs = append(orgs, object{"Name": m, "ID": m, "MSPDir": fmt.Sprintf("/work/organizations/org%d/msp", i), "Policies": policies(m, false), "AnchorPeers": []object{{"Host": fmt.Sprintf("peer%d", i), "Port": 7051}}})
	}
	consenters := []object{}
	for i := 1; i <= 3; i++ {
		p := fmt.Sprintf("/work/organizations/orderers/orderer%d/tls/server.crt", i)
		consenters = append(consenters, object{"Host": fmt.Sprintf("orderer%d", i), "Port": 7050, "ClientTLSCert": p, "ServerTLSCert": p})
	}
	return object{"Profiles": object{"Sathonate": object{
		"Policies": object{"Readers": object{"Type": "ImplicitMeta", "Rule": "ANY Readers"}, "Writers": object{"Type": "ImplicitMeta", "Rule": "ANY Writers"}, "Admins": object{"Type": "ImplicitMeta", "Rule": "MAJORITY Admins"}}, "Capabilities": object{"V2_0": true},
		"Orderer":     object{"OrdererType": "etcdraft", "Addresses": []string{"orderer1:7050", "orderer2:7050", "orderer3:7050"}, "EtcdRaft": object{"Consenters": consenters, "Options": object{"TickInterval": "500ms", "ElectionTick": 10, "HeartbeatTick": 1, "MaxInflightBlocks": 5, "SnapshotIntervalSize": 16777216}}, "BatchTimeout": "2s", "BatchSize": object{"MaxMessageCount": 10, "AbsoluteMaxBytes": 103809024, "PreferredMaxBytes": 524288}, "Organizations": []object{orderOrg}, "Policies": implicit(true), "Capabilities": object{"V2_0": true}},
		"Application": object{"Organizations": orgs, "Policies": implicit(false), "Capabilities": object{"V2_5": true}},
	}}}
}
func copyFile(source, target string) error {
	b, e := os.ReadFile(source)
	if e != nil {
		return e
	}
	if e = os.MkdirAll(filepath.Dir(target), 0700); e != nil {
		return e
	}
	return os.WriteFile(target, b, 0600)
}
func first(dir string) (string, error) {
	entries, e := os.ReadDir(dir)
	if e != nil {
		return "", e
	}
	for _, v := range entries {
		if !v.IsDir() {
			return filepath.Join(dir, v.Name()), nil
		}
	}
	return "", fmt.Errorf("missing certificate material in %s", dir)
}
func (m *Manager) normalize(org string, ids []struct{ name, kind string }) error {
	ca, e := first(filepath.Join(m.root, "organizations", org, "admin", "msp", "cacerts"))
	if e != nil {
		return e
	}
	tlsCA, e := first(filepath.Join(m.root, "organizations", org, "admin", "tls", "tlscacerts"))
	if e != nil {
		return e
	}
	dirs := []string{filepath.Join(m.root, "organizations", org, "msp")}
	for _, id := range ids {
		dirs = append(dirs, filepath.Join(m.root, "organizations", org, id.name, "msp"))
		base := filepath.Join(m.root, "organizations", org, id.name, "tls")
		for _, pair := range [][2]string{{"signcerts", "server.crt"}, {"keystore", "server.key"}, {"tlscacerts", "ca.crt"}} {
			src, e := first(filepath.Join(base, pair[0]))
			if e != nil {
				return e
			}
			if e = copyFile(src, filepath.Join(base, pair[1])); e != nil {
				return e
			}
		}
	}
	config := "NodeOUs:\n  Enable: true\n"
	for _, kind := range []string{"Client", "Peer", "Admin", "Orderer"} {
		config += fmt.Sprintf("  %sOUIdentifier:\n    Certificate: cacerts/ca.pem\n    OrganizationalUnitIdentifier: %s\n", kind, strings.ToLower(kind))
	}
	for _, dir := range dirs {
		if e = copyFile(ca, filepath.Join(dir, "cacerts", "ca.pem")); e != nil {
			return e
		}
		if e = copyFile(tlsCA, filepath.Join(dir, "tlscacerts", "ca.pem")); e != nil {
			return e
		}
		if e = os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(config), 0600); e != nil {
			return e
		}
	}
	return nil
}
