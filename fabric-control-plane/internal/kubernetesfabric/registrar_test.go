package kubernetesfabric

import "testing"

func TestRegistrarHasNoDelegationOrRevocationPrivileges(t *testing.T) {
	identity := Object{"attrs": Object{"hf.Registrar.Roles": "*", "hf.Revoker": true}}
	v := Object{"spec": Object{"ca": Object{"registry": Object{"identities": []any{identity}}}}}
	restrictRegistrar(v)
	attrs := object(identity["attrs"])
	if attrs["hf.Registrar.Roles"] != "peer,orderer,admin,client" || attrs["hf.Registrar.DelegateRoles"] != "" || attrs["hf.Revoker"] != false || attrs["hf.IntermediateCA"] != false {
		t.Fatal("bootstrap privileges were not narrowed")
	}
}
