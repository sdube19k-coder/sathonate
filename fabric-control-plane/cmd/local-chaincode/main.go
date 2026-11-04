// local-chaincode is Sathonate's minimal development smoke contract, using Fabric's official Go shim.
package main

import (
	"log"
	"os"

	"github.com/hyperledger/fabric-chaincode-go/v2/shim"
	pb "github.com/hyperledger/fabric-protos-go-apiv2/peer"
)

type contract struct{}

func (*contract) Init(shim.ChaincodeStubInterface) *pb.Response { return shim.Success(nil) }
func (*contract) Invoke(stub shim.ChaincodeStubInterface) *pb.Response {
	function, args := stub.GetFunctionAndParameters()
	switch function {
	case "Put":
		if len(args) != 2 || len(args[0]) == 0 || len(args[0]) > 128 || len(args[1]) > 4096 {
			return shim.Error("Put requires a key and a value (max 4KiB)")
		}
		if e := stub.PutState(args[0], []byte(args[1])); e != nil {
			return shim.Error("write failed")
		}
		return shim.Success([]byte(stub.GetTxID()))
	case "Get":
		if len(args) != 1 {
			return shim.Error("Get requires a key")
		}
		b, e := stub.GetState(args[0])
		if e != nil {
			return shim.Error("read failed")
		}
		if b == nil {
			return shim.Error("key not found")
		}
		return shim.Success(b)
	default:
		return shim.Error("supported functions: Put, Get")
	}
}
func main() {
	certPath := os.Getenv("CHAINCODE_TLS_CERT")
	if certPath == "" {
		certPath = "/tls/server.crt"
	}
	keyPath := os.Getenv("CHAINCODE_TLS_KEY")
	if keyPath == "" {
		keyPath = "/tls/server.key"
	}
	cert, e := os.ReadFile(certPath)
	if e != nil {
		log.Fatal(e)
	}
	key, e := os.ReadFile(keyPath)
	if e != nil {
		log.Fatal(e)
	}
	address := os.Getenv("CHAINCODE_SERVER_ADDRESS")
	if address == "" {
		address = "0.0.0.0:9999"
	}
	server := shim.ChaincodeServer{CCID: os.Getenv("CHAINCODE_ID"), Address: address, CC: &contract{}, TLSProps: shim.TLSProperties{Cert: cert, Key: key}}
	if e = server.Start(); e != nil {
		log.Fatal(e)
	}
}
