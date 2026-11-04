package localfabric

import (
	"errors"
	"strings"
)

// Fabric CA accepts JSON configuration. Secret URLs/id.secret values must never
// reach Docker argv or container metadata; the private temporary file is removed
// after execution. Windows deployments must protect the runtime directory ACL.
func protectedCAClientConfig(args []string) (object, []string, error) {
	config := object{}
	public := []string{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-u", "--url":
			i++
			if i >= len(args) {
				return nil, nil, errors.New("invalid CA input")
			}
			config["url"] = args[i]
		case "--id.secret":
			i++
			if i >= len(args) {
				return nil, nil, errors.New("invalid CA input")
			}
			config["id"] = object{"secret": args[i]}
		default:
			if strings.Contains(args[i], "https://") || strings.Contains(args[i], "--id.secret=") {
				return nil, nil, errors.New("unsupported private CA argument")
			}
			public = append(public, args[i])
		}
	}
	return config, public, nil
}
func caBootstrap(password string) object {
	return object{"version": "1.5.0", "ca": object{"certfile": "/etc/hyperledger/fabric-ca-server/ca-cert.pem", "keyfile": "/etc/hyperledger/fabric-ca-server/ca-key.pem"}, "db": object{"type": "sqlite3", "datasource": "/etc/hyperledger/fabric-ca-server/fabric-ca-server.db"}, "bccsp": object{"default": "SW", "sw": object{"hash": "SHA2", "security": 256, "filekeystore": object{"keystore": "/etc/hyperledger/fabric-ca-server/msp/keystore"}}}, "registry": object{"maxenrollments": -1, "identities": []object{{"name": "registrar", "pass": password, "type": "client", "affiliation": "", "attrs": object{"hf.Registrar.Roles": "peer,orderer,admin,client", "hf.Registrar.DelegateRoles": "", "hf.Revoker": false, "hf.IntermediateCA": false, "hf.GenCRL": false, "hf.Registrar.Attributes": "", "hf.AffiliationMgr": false}}}}}
}
