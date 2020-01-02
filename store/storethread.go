package store

import (
	"context"
	"sync"
	"time"

	format "github.com/ipfs/go-ipld-format"
	"github.com/libp2p/go-libp2p-core/peer"
	threadcbor "github.com/textileio/go-threads/cbor"
	service "github.com/textileio/go-threads/core/service"
	"github.com/textileio/go-threads/core/thread"
	"github.com/textileio/go-threads/util"
)

const (
	addRecordTimeout  = time.Second * 10
	fetchEventTimeout = time.Second * 15
)

// SingleThreadAdapter connects a Store with a Service
type singleThreadAdapter struct {
	api        service.Service
	store      *Store
	threadID   thread.ID
	ownLogID   peer.ID
	closeChan  chan struct{}
	goRoutines sync.WaitGroup

	lock    sync.Mutex
	started bool
	closed  bool
}

// NewSingleThreadAdapter returns a new Adapter which maps
// a Store with a single Thread
func newSingleThreadAdapter(store *Store, threadID thread.ID) *singleThreadAdapter {
	a := &singleThreadAdapter{
		api:       store.Service(),
		threadID:  threadID,
		store:     store,
		closeChan: make(chan struct{}),
	}

	return a
}

// Close closes the storehead and stops listening both directions
// of thread<->store
func (a *singleThreadAdapter) Close() {
	a.lock.Lock()
	defer a.lock.Unlock()
	if a.closed {
		return
	}
	a.closed = true
	close(a.closeChan)
	a.goRoutines.Wait()
}

// Start starts connection from Store to Service, and viceversa
func (a *singleThreadAdapter) Start() {
	a.lock.Lock()
	defer a.lock.Unlock()
	if a.started {
		return
	}
	a.started = true
	li, err := util.GetOrCreateOwnLog(a.api, a.threadID)
	if err != nil {
		log.Fatalf("error when getting/creating own log for thread %s: %v", a.threadID, err)
	}
	a.ownLogID = li.ID

	var wg sync.WaitGroup
	wg.Add(2)
	go a.threadToStore(&wg)
	go a.storeToThread(&wg)
	wg.Wait()
	a.goRoutines.Add(2)
}

func (a *singleThreadAdapter) threadToStore(wg *sync.WaitGroup) {
	defer a.goRoutines.Done()
	sub := a.api.Subscribe(service.ThreadID(a.threadID))
	defer sub.Discard()
	wg.Done()
	for {
		select {
		case <-a.closeChan:
			log.Debug("closing thread-to-store flow on thread %s", a.threadID)
			return
		case rec, ok := <-sub.Channel():
			if !ok {
				log.Errorf("notification channel closed, not listening to external changes anymore")
				return
			}
			if rec.LogID() == a.ownLogID {
				continue // Ignore our own events since Store already dispatches to Store reducers
			}
			ctx, cancel := context.WithTimeout(context.Background(), fetchEventTimeout)
			event, err := threadcbor.EventFromRecord(ctx, a.api, rec.Value())
			if err != nil {
				block, err := a.getBlockWithRetry(ctx, rec.Value(), 3, time.Millisecond*500)
				if err != nil { // ToDo: Buffer them and retry...
					log.Fatalf("error when getting block from record: %v", err)
				}
				event, err = threadcbor.EventFromNode(block)
				if err != nil {
					log.Fatalf("error when decoding block to event: %v", err)
				}
			}
			readKey, err := a.api.Store().ReadKey(a.threadID)
			if err != nil {
				log.Fatalf("error when getting read key for thread %s: %v", a.threadID, err)
			}
			if readKey == nil {
				log.Fatalf("read key not found for thread %s/%s", a.threadID, rec.LogID())
			}
			node, err := event.GetBody(ctx, a.api, readKey)
			if err != nil {
				log.Fatalf("error when getting body of event on thread %s/%s: %v", a.threadID, rec.LogID(), err)
			}
			storeEvents, err := a.store.eventsFromBytes(node.RawData())
			if err != nil {
				log.Fatalf("error when unmarshaling event from bytes: %v", err)
			}
			log.Debugf("dispatching to store external new record: %s/%s", rec.ThreadID(), rec.LogID())
			if err := a.store.dispatch(storeEvents); err != nil {
				log.Fatal(err)
			}
			cancel()
		}
	}
}

func (a *singleThreadAdapter) storeToThread(wg *sync.WaitGroup) {
	defer a.goRoutines.Done()
	l := a.store.localEventListen()
	defer l.Discard()
	wg.Done()

	for {
		select {
		case <-a.closeChan:
			log.Infof("closing store-to-thread flow on thread %s", a.threadID)
			return
		case node, ok := <-l.Channel():
			if !ok {
				log.Errorf("ending sending store local event to own thread since channel was closed for thread %s", a.threadID)
				return
			}
			ctx, cancel := context.WithTimeout(context.Background(), addRecordTimeout)
			if _, err := a.api.AddRecord(ctx, a.threadID, node); err != nil {
				log.Fatalf("error writing record: %v", err)
			}
			cancel()
		}
	}
}

func (a *singleThreadAdapter) getBlockWithRetry(ctx context.Context, rec service.Record, cantRetries int, backoffTime time.Duration) (format.Node, error) {
	var err error
	for i := 1; i <= cantRetries; i++ {
		n, err := rec.GetBlock(ctx, a.api)
		if err == nil {
			return n, nil
		}
		log.Warningf("error when fetching block %s in retry %d", rec.Cid(), i)
		time.Sleep(backoffTime)
		backoffTime *= 2
	}
	return nil, err
}
