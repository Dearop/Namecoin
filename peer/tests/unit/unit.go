package unit

import (
	"go.dedis.ch/cs438/peer"
	"go.dedis.ch/cs438/peer/impl"
	"go.dedis.ch/cs438/transport"
	"go.dedis.ch/cs438/transport/channel"
	"go.dedis.ch/cs438/transport/udp"
)

var peerFac peer.Factory = impl.NewPeer

var channelFac transport.Factory = channel.NewTransport
var udpFac transport.Factory = udp.NewUDP

type NamecoinChain = impl.NamecoinChain
type NamecoinState = impl.NamecoinState

var LoadNamecoinChain = impl.LoadNamecoinChain
var NewNamecoinState = impl.NewState
var ApplyNamecoinBlock = impl.ApplyBlockToState

// Isolation prefixes for Namecoin in the blockchain store
var (
	NamecoinBlockPrefix = impl.NamecoinBlockPrefix

	// NamecoinLastBlockKey stores the hash of the last applied Namecoin block
	// at the time of the last successful replay
	NamecoinLastBlockKey = impl.NamecoinLastBlockKey
)
