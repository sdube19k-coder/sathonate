package kubernetesfabric

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"google.golang.org/protobuf/proto"
	"os"
	"time"
)

func validateGenesis(raw []byte, channel string) error {
	var block common.Block
	if proto.Unmarshal(raw, &block) != nil || block.Header == nil || block.Header.Number != 0 || block.Data == nil || len(block.Data.Data) != 1 {
		return secureDenied
	}
	dataHash := sha256.Sum256(bytes.Join(block.Data.Data, nil))
	if !bytes.Equal(block.Header.DataHash, dataHash[:]) {
		return secureDenied
	}
	var envelope common.Envelope
	var payload common.Payload
	var header common.ChannelHeader
	if proto.Unmarshal(block.Data.Data[0], &envelope) != nil || proto.Unmarshal(envelope.Payload, &payload) != nil || payload.Header == nil || proto.Unmarshal(payload.Header.ChannelHeader, &header) != nil || header.ChannelId != channel || header.Type != int32(common.HeaderType_CONFIG) {
		return secureDenied
	}
	return nil
}

func (w *Worker) fetchCanonicalGenesis(channel string) error {
	path := "/workspace/canonical-genesis.block"
	deadline := time.Now().Add(45 * time.Second)
	for {
		_, e := w.directPeer("org1", "channel-admin", "channel", "fetch", "0", path, "-c", channel, "-o", "orderer-1."+w.Namespace+":7050", "--tls", "--cafile", "/workspace/org1-root-tls.pem")
		if e == nil {
			raw, e := os.ReadFile(path)
			if e != nil || validateGenesis(raw, channel) != nil {
				return secureDenied
			}
			if privateFile("/workspace/genesis.block", raw) != nil {
				return secureDenied
			}
			hash := sha256.Sum256(raw)
			return w.event("channels", "Verified canonical channel genesis SHA256 "+hex.EncodeToString(hash[:]))
		}
		if w.ctx.Err() != nil || time.Now().After(deadline) {
			return e
		}
		select {
		case <-w.ctx.Done():
			return w.ctx.Err()
		case <-time.After(time.Second):
		}
	}
}
