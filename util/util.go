package util

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	logging "github.com/ipfs/go-log"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	swarm "github.com/libp2p/go-libp2p-swarm"
	"github.com/mr-tron/base58"
	ma "github.com/multiformats/go-multiaddr"
	sym "github.com/textileio/go-threads/crypto/symmetric"
	"go.uber.org/zap/zapcore"
)

var (
	bootstrapPeers = []string{
		"/ip4/104.210.43.77/tcp/4001/ipfs/12D3KooWSdGmRz5JQidqrtmiPGVHkStXpbSAMnbCcW8abq6zuiDP", // us-west
		"/ip4/20.39.232.27/tcp/4001/ipfs/12D3KooWLnUv9MWuRM6uHirRPBM4NwRj54n4gNNnBtiFiwPiv3Up",  // eu-west
		"/ip4/34.87.103.105/tcp/4001/ipfs/12D3KooWA5z2C3z1PNKi36Bw1MxZhBD8nv7UbB7YQP6WcSWYNwRQ", // as-southeast
	}
)

// CanDial returns whether or not the address is dialable.
func CanDial(addr ma.Multiaddr, s *swarm.Swarm) bool {
	parts := strings.Split(addr.String(), "/"+ma.ProtocolWithCode(ma.P_P2P).Name)
	addr, _ = ma.NewMultiaddr(parts[0])
	tr := s.TransportForDialing(addr)
	return tr != nil && tr.CanDial(addr)
}

// DecodeKey from a string into a symmetric key.
func DecodeKey(k string) (*sym.Key, error) {
	b, err := base58.Decode(k)
	if err != nil {
		return nil, err
	}
	return sym.FromBytes(b)
}

// SetupDefaultLoggingConfig sets up a standard logging configuration.
func SetupDefaultLoggingConfig(repoPath string) {
	_ = os.Setenv("GOLOG_LOG_FMT", "color")
	_ = os.Setenv("GOLOG_FILE", filepath.Join(repoPath, "log", "threads.log"))
	logging.SetupLogging()
	logging.SetAllLoggers(logging.LevelError)
}

// SetLogLevels sets levels for the given systems.
func SetLogLevels(systems map[string]logging.LogLevel) error {
	for sys, level := range systems {
		l := zapcore.Level(level)
		if sys == "*" {
			for _, s := range logging.GetSubsystems() {
				if err := logging.SetLogLevel(s, l.CapitalString()); err != nil {
					return err
				}
			}
		}
		if err := logging.SetLogLevel(sys, l.CapitalString()); err != nil {
			return err
		}
	}
	return nil
}

func LoadKey(pth string) crypto.PrivKey {
	var priv crypto.PrivKey
	_, err := os.Stat(pth)
	if os.IsNotExist(err) {
		priv, _, err = crypto.GenerateKeyPair(crypto.Ed25519, 0)
		if err != nil {
			panic(err)
		}
		key, err := crypto.MarshalPrivateKey(priv)
		if err != nil {
			panic(err)
		}
		if err = ioutil.WriteFile(pth, key, 0400); err != nil {
			panic(err)
		}
	} else if err != nil {
		panic(err)
	} else {
		key, err := ioutil.ReadFile(pth)
		if err != nil {
			panic(err)
		}
		priv, err = crypto.UnmarshalPrivateKey(key)
		if err != nil {
			panic(err)
		}
	}
	return priv
}

func DefaultBoostrapPeers() []peer.AddrInfo {
	ais, err := ParseBootstrapPeers(bootstrapPeers)
	if err != nil {
		panic("coudn't parse default bootstrap peers")
	}
	return ais
}

func ParseBootstrapPeers(addrs []string) ([]peer.AddrInfo, error) {
	maddrs := make([]ma.Multiaddr, len(addrs))
	for i, addr := range addrs {
		var err error
		maddrs[i], err = ma.NewMultiaddr(addr)
		if err != nil {
			return nil, err
		}
	}
	return peer.AddrInfosFromP2pAddrs(maddrs...)
}

func TCPAddrFromMultiAddr(maddr ma.Multiaddr) (addr string, err error) {
	if maddr == nil {
		err = fmt.Errorf("invalid address")
		return
	}
	ip4, err := maddr.ValueForProtocol(ma.P_IP4)
	if err != nil {
		return
	}
	tcp, err := maddr.ValueForProtocol(ma.P_TCP)
	if err != nil {
		return
	}
	return fmt.Sprintf("%s:%s", ip4, tcp), nil
}

func MustParseAddr(str string) ma.Multiaddr {
	addr, err := ma.NewMultiaddr(str)
	if err != nil {
		panic(err)
	}
	return addr
}
