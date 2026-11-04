package kubernetesfabric

import (
	"crypto/sha256"
	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"google.golang.org/protobuf/proto"
	"testing"
)

func TestCanonicalGenesisRequiresBlockZeroAndExactChannel(t *testing.T) {
	header, _ := proto.Marshal(&common.ChannelHeader{ChannelId: "owned-channel", Type: int32(common.HeaderType_CONFIG)})
	payload, _ := proto.Marshal(&common.Payload{Header: &common.Header{ChannelHeader: header}})
	envelope, _ := proto.Marshal(&common.Envelope{Payload: payload})
	block := &common.Block{Header: &common.BlockHeader{}, Data: &common.BlockData{Data: [][]byte{envelope}}}
	hash := sha256.Sum256(envelope)
	block.Header.DataHash = hash[:]
	raw, _ := proto.Marshal(block)
	if validateGenesis(raw, "owned-channel") != nil {
		t.Fatal("canonical genesis rejected")
	}
	if validateGenesis(raw, "foreign-channel") == nil {
		t.Fatal("foreign channel accepted")
	}
	block.Header.Number = 1
	raw, _ = proto.Marshal(block)
	if validateGenesis(raw, "owned-channel") == nil || validateGenesis([]byte("invalid"), "owned-channel") == nil {
		t.Fatal("non-genesis block accepted")
	}
}
