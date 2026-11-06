package main

import (
	"github.com/hyperledger/fabric-chaincode-go/v2/shim"
	"testing"
)

type versionStub struct {
	shim.ChaincodeStubInterface
	args []string
}

func (s versionStub) GetFunctionAndParameters() (string, []string) { return "CodeVersion", s.args }

func TestCodeVersionComesFromExecutable(t *testing.T) {
	previous := codeVersion
	defer func() { codeVersion = previous }()
	codeVersion = "approved-executable-v2"
	r := (&contract{}).Invoke(versionStub{})
	if r.Status != 200 || string(r.Payload) != codeVersion {
		t.Fatal("executable version unavailable")
	}
	r = (&contract{}).Invoke(versionStub{args: []string{"pretend-v3"}})
	if r.Status == 200 {
		t.Fatal("caller substituted executable version")
	}
}
