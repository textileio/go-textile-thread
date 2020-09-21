package net

import (
	"context"
	"fmt"
	nnet "net"
	"sync"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/gogo/status"
	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	pstore "github.com/libp2p/go-libp2p-core/peerstore"
	gostream "github.com/libp2p/go-libp2p-gostream"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/textileio/go-threads/cbor"
	core "github.com/textileio/go-threads/core/net"
	"github.com/textileio/go-threads/core/thread"
	sym "github.com/textileio/go-threads/crypto/symmetric"
	pb "github.com/textileio/go-threads/net/pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/connectivity"
)

var (
	// DialTimeout is the max time duration to wait when dialing a peer.
	DialTimeout = time.Second * 10
	PushTimeout = time.Second * 10
	PullTimeout = time.Second * 10
)

// getLogs in a thread.
func (s *server) getLogs(ctx context.Context, id thread.ID, pid peer.ID) ([]thread.LogInfo, error) {
	sk, err := s.net.store.ServiceKey(id)
	if err != nil {
		return nil, err
	}
	if sk == nil {
		return nil, fmt.Errorf("a service-key is required to request logs")
	}

	body := &pb.GetLogsRequest_Body{
		ThreadID:   &pb.ProtoThreadID{ID: id},
		ServiceKey: &pb.ProtoKey{Key: sk},
	}
	sig, key, err := s.signRequestBody(body)
	if err != nil {
		return nil, err
	}
	req := &pb.GetLogsRequest{
		Header: &pb.Header{
			PubKey:    &pb.ProtoPubKey{PubKey: key},
			Signature: sig,
		},
		Body: body,
	}

	log.Debugf("getting %s logs from %s...", id, pid)

	client, err := s.dial(pid)
	if err != nil {
		return nil, err
	}
	cctx, cancel := context.WithTimeout(ctx, PullTimeout)
	defer cancel()
	reply, err := client.GetLogs(cctx, req)
	if err != nil {
		log.Warnf("get logs from %s failed: %s", pid, err)
		return nil, err
	}

	log.Debugf("received %d logs from %s", len(reply.Logs), pid)

	lgs := make([]thread.LogInfo, len(reply.Logs))
	for i, l := range reply.Logs {
		lgs[i] = logFromProto(l)
	}

	return lgs, nil
}

// pushLog to a peer.
func (s *server) pushLog(ctx context.Context, id thread.ID, lg thread.LogInfo, pid peer.ID, sk *sym.Key, rk *sym.Key) error {
	body := &pb.PushLogRequest_Body{
		ThreadID: &pb.ProtoThreadID{ID: id},
		Log:      logToProto(lg),
	}
	if sk != nil {
		body.ServiceKey = &pb.ProtoKey{Key: sk}
	}
	if rk != nil {
		body.ReadKey = &pb.ProtoKey{Key: rk}
	}
	sig, key, err := s.signRequestBody(body)
	if err != nil {
		return err
	}
	lreq := &pb.PushLogRequest{
		Header: &pb.Header{
			PubKey:    &pb.ProtoPubKey{PubKey: key},
			Signature: sig,
		},
		Body: body,
	}

	log.Debugf("pushing log %s to %s...", lg.ID, pid)

	client, err := s.dial(pid)
	if err != nil {
		return fmt.Errorf("dial %s failed: %w", pid, err)
	}
	cctx, cancel := context.WithTimeout(ctx, PushTimeout)
	defer cancel()
	_, err = client.PushLog(cctx, lreq)
	if err != nil {
		return fmt.Errorf("push log to %s failed: %w", pid, err)
	}
	return err
}

// records maintains an ordered list of records from multiple sources.
type records struct {
	sync.RWMutex
	m map[peer.ID]map[cid.Cid]core.Record
	s map[peer.ID][]core.Record
}

// newRecords creates an instance of records.
func newRecords() *records {
	return &records{
		m: make(map[peer.ID]map[cid.Cid]core.Record),
		s: make(map[peer.ID][]core.Record),
	}
}

// List all records.
func (r *records) List() map[peer.ID][]core.Record {
	r.RLock()
	defer r.RUnlock()
	return r.s
}

// Store a record.
func (r *records) Store(p peer.ID, key cid.Cid, value core.Record) {
	r.Lock()
	defer r.Unlock()
	if _, ok := r.m[p]; !ok {
		r.m[p] = make(map[cid.Cid]core.Record)
		r.s[p] = make([]core.Record, 0)
	}
	if _, ok := r.m[p][key]; ok {
		return
	}
	r.m[p][key] = value

	// Sanity check
	if len(r.s[p]) > 0 && r.s[p][len(r.s[p])-1].Cid() != value.PrevID() {
		panic("there is a gap in records list")
	}

	r.s[p] = append(r.s[p], value)
}

// getRecords from log addresses.
func (s *server) getRecords(
	ctx context.Context,
	tid thread.ID,
	lid peer.ID,
	offsets map[peer.ID]cid.Cid,
	limit int,
) (map[peer.ID][]core.Record, error) {
	sk, err := s.net.store.ServiceKey(tid)
	if err != nil {
		return nil, err
	}
	if sk == nil {
		return nil, fmt.Errorf("a service-key is required to request records")
	}

	pblgs := make([]*pb.GetRecordsRequest_Body_LogEntry, 0, len(offsets))
	for lid, offset := range offsets {
		pblgs = append(pblgs, &pb.GetRecordsRequest_Body_LogEntry{
			LogID:  &pb.ProtoPeerID{ID: lid},
			Offset: &pb.ProtoCid{Cid: offset},
			Limit:  int32(limit),
		})
	}

	body := &pb.GetRecordsRequest_Body{
		ThreadID:   &pb.ProtoThreadID{ID: tid},
		ServiceKey: &pb.ProtoKey{Key: sk},
		Logs:       pblgs,
	}
	sig, key, err := s.signRequestBody(body)
	if err != nil {
		return nil, err
	}
	req := &pb.GetRecordsRequest{
		Header: &pb.Header{
			PubKey:    &pb.ProtoPubKey{PubKey: key},
			Signature: sig,
		},
		Body: body,
	}

	logAddrs, err := s.net.store.Addrs(tid, lid)
	if err != nil {
		return nil, err
	}

	var (
		recs = newRecords()
		wg   sync.WaitGroup
	)

	// Pull from each address
	for _, addr := range logAddrs {
		wg.Add(1)

		go withErrLog(addr, func(addr ma.Multiaddr) error {
			defer wg.Done()
			pid, ok, err := s.callablePeer(addr)
			if err != nil {
				return err
			} else if !ok {
				// skip calling itself
				return nil
			}

			log.Debugf("getting records from %s...", pid)

			client, err := s.dial(pid)
			if err != nil {
				return fmt.Errorf("dial %s failed: %w", pid, err)
			}

			cctx, cancel := context.WithTimeout(ctx, PullTimeout)
			defer cancel()
			reply, err := client.GetRecords(cctx, req)
			if err != nil {
				log.Warnf("get records from %s failed: %s", pid, err)
				return nil
			}

			for _, l := range reply.Logs {
				var logID = l.LogID.ID
				log.Debugf("received %d records in log %s from %s", len(l.Records), logID, pid)

				if l.Log != nil && len(l.Log.Addrs) > 0 {
					if err = s.net.store.AddAddrs(tid, logID, addrsFromProto(l.Log.Addrs), pstore.PermanentAddrTTL); err != nil {
						return err
					}
				}

				pk, err := s.net.store.PubKey(tid, logID)
				if err != nil {
					return err
				}

				if pk == nil {
					if l.Log == nil || l.Log.PubKey == nil {
						// cannot verify received records
						continue
					}
					if err := s.net.store.AddPubKey(tid, logID, l.Log.PubKey); err != nil {
						return err
					}
					pk = l.Log.PubKey
				}

				for _, r := range l.Records {
					rec, err := cbor.RecordFromProto(r, sk)
					if err != nil {
						return err
					}
					if err = rec.Verify(pk); err != nil {
						return err
					}
					recs.Store(logID, rec.Cid(), rec)
				}
			}
			return nil
		})
	}
	wg.Wait()

	return recs.List(), nil
}

// pushRecord to log addresses and thread topic.
func (s *server) pushRecord(ctx context.Context, id thread.ID, lid peer.ID, rec core.Record) error {
	// Collect known writers
	addrs := make([]ma.Multiaddr, 0)
	info, err := s.net.store.GetThread(id)
	if err != nil {
		return err
	}
	for _, l := range info.Logs {
		addrs = append(addrs, l.Addrs...)
	}

	pbrec, err := cbor.RecordToProto(ctx, s.net, rec)
	if err != nil {
		return err
	}
	body := &pb.PushRecordRequest_Body{
		ThreadID: &pb.ProtoThreadID{ID: id},
		LogID:    &pb.ProtoPeerID{ID: lid},
		Record:   pbrec,
	}
	sig, key, err := s.signRequestBody(body)
	if err != nil {
		return err
	}
	req := &pb.PushRecordRequest{
		Header: &pb.Header{
			PubKey:    &pb.ProtoPubKey{PubKey: key},
			Signature: sig,
		},
		Body: body,
	}

	// Push to each address
	for _, addr := range addrs {
		go withErrLog(addr, func(addr ma.Multiaddr) error {
			pid, ok, err := s.callablePeer(addr)
			if err != nil {
				return err
			} else if !ok {
				// skip calling itself
				return nil
			}

			client, err := s.dial(pid)
			if err != nil {
				return fmt.Errorf("dial %s failed: %w", pid, err)
			}
			cctx, cancel := context.WithTimeout(context.Background(), PushTimeout)
			defer cancel()
			if _, err = client.PushRecord(cctx, req); err != nil {
				if status.Convert(err).Code() == codes.NotFound { // Send the missing log
					log.Debugf("pushing log %s to %s...", lid, pid)
					l, err := s.net.store.GetLog(id, lid)
					if err != nil {
						return err
					}
					body := &pb.PushLogRequest_Body{
						ThreadID: &pb.ProtoThreadID{ID: id},
						Log:      logToProto(l),
					}
					sig, key, err := s.signRequestBody(body)
					if err != nil {
						return err
					}
					lreq := &pb.PushLogRequest{
						Header: &pb.Header{
							PubKey:    &pb.ProtoPubKey{PubKey: key},
							Signature: sig,
						},
						Body: body,
					}
					if _, err = client.PushLog(cctx, lreq); err != nil {
						log.Warnf("push log to %s failed: %s", pid, err)
						return nil
					}
					return nil
				}
				log.Warnf("push record to %s failed: %s", pid, err)
				return nil
			}
			return nil
		})
	}

	// Finally, publish to the thread's topic
	if s.ps != nil {
		if err = s.ps.Publish(ctx, id, req); err != nil {
			log.Errorf("error publishing record: %s", err)
		}
	}

	return nil
}

// callablePeer attempts to obtain external peer ID from the multiaddress.
func (s *server) callablePeer(addr ma.Multiaddr) (peer.ID, bool, error) {
	p, err := addr.ValueForProtocol(ma.P_P2P)
	if err != nil {
		return "", false, err
	}

	pid, err := peer.Decode(p)
	if err != nil {
		return "", false, err
	}

	if pid.String() == s.net.host.ID().String() {
		return pid, false, nil
	}

	return pid, true, nil
}

// dial attempts to open a gRPC connection over libp2p to a peer.
func (s *server) dial(peerID peer.ID) (pb.ServiceClient, error) {
	s.Lock()
	defer s.Unlock()
	conn, ok := s.conns[peerID]
	if ok {
		if conn.GetState() == connectivity.Shutdown {
			if err := conn.Close(); err != nil {
				log.Errorf("error closing connection: %v", err)
			}
		} else {
			return pb.NewServiceClient(conn), nil
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), DialTimeout)
	defer cancel()
	conn, err := grpc.DialContext(ctx, peerID.Pretty(), s.opts...)
	if err != nil {
		return nil, err
	}
	s.conns[peerID] = conn
	return pb.NewServiceClient(conn), nil
}

// getLibp2pDialer returns a WithContextDialer option for libp2p dialing.
func (s *server) getLibp2pDialer() grpc.DialOption {
	return grpc.WithContextDialer(func(ctx context.Context, peerIDStr string) (nnet.Conn, error) {
		id, err := peer.Decode(peerIDStr)
		if err != nil {
			return nil, fmt.Errorf("grpc tried to dial non peerID: %w", err)
		}

		conn, err := gostream.Dial(ctx, s.net.host, id, thread.Protocol)
		if err != nil {
			return nil, fmt.Errorf("gostream dial failed: %w", err)
		}

		return conn, nil
	})
}

// signRequestBody signs an outbound request body with the hosts's private key.
func (s *server) signRequestBody(msg proto.Marshaler) (sig []byte, pk crypto.PubKey, err error) {
	payload, err := msg.Marshal()
	if err != nil {
		return
	}
	sk := s.net.getPrivKey()
	if sk == nil {
		err = fmt.Errorf("private key for host not found")
		return
	}
	sig, err = sk.Sign(payload)
	if err != nil {
		return
	}
	return sig, sk.GetPublic(), nil
}

func withErrLog(addr ma.Multiaddr, f func(addr ma.Multiaddr) error) {
	if err := f(addr); err != nil {
		log.Error(err.Error())
	}
}
