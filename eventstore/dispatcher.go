package eventstore

import (
	"bytes"
	"encoding/gob"
	"sync"

	"context"

	datastore "github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/query"
	core "github.com/textileio/go-textile-core/store"
	"golang.org/x/sync/errgroup"
)

var (
	dsDispatcherPrefix = dsStorePrefix.ChildString("dispatcher")
)

// Reducer applies an event to an existing state.
type Reducer interface {
	Reduce(event core.Event) error
}

// dispatcher is used to dispatch events to registered reducers.
//
// This is different from generic pub-sub systems because reducers are not subscribed to particular events.
// Every event is dispatched to every registered reducer. When a given reducer is registered, it returns a `token`,
// which can be used to deregister the reducer later.
type dispatcher struct {
	store    datastore.Datastore
	reducers []Reducer
	lock     sync.RWMutex
	lastID   int
}

// NewDispatcher creates a new EventDispatcher
func newDispatcher(store datastore.Datastore) *dispatcher {
	return &dispatcher{
		store: store,
	}
}

// Store returns the internal event store.
func (d *dispatcher) Store() datastore.Datastore {
	return d.store
}

// Register takes a reducer to be invoked with each dispatched event
func (d *dispatcher) Register(reducer Reducer) {
	d.lock.Lock()
	defer d.lock.Unlock()
	d.lastID++
	d.reducers = append(d.reducers, reducer)
}

// Dispatch dispatches a payload to all registered reducers.
func (d *dispatcher) Dispatch(event core.Event) error {
	d.lock.Lock()
	defer d.lock.Unlock()
	// Key format: <timestamp>/<entity-id>/<type>
	// @todo: This is up for debate, its a 'fake' Event struct right now anyway
	key := dsDispatcherPrefix.ChildString(string(event.Time())).ChildString(event.EntityID().String()).ChildString(event.Type())
	// Encode and add an Event to event store
	b := bytes.Buffer{}
	e := gob.NewEncoder(&b)
	if err := e.Encode(event); err != nil {
		return err
	}
	if err := d.Store().Put(key, b.Bytes()); err != nil {
		return err
	}
	// Safe to fire off reducers now that event is persisted
	g, _ := errgroup.WithContext(context.Background())
	for _, reducer := range d.reducers {
		reducer := reducer
		// Launch each reducer in a separate goroutine
		g.Go(func() error {
			return reducer.Reduce(event)
		})
	}
	// Wait for all reducers to complete or error out
	if err := g.Wait(); err != nil {
		return err
	}
	return nil
}

// Query searches the internal event store and returns a query result.
// This is a syncronouse version of github.com/ipfs/go-datastore's Query method
func (d *dispatcher) Query(query query.Query) ([]query.Entry, error) {
	result, err := d.store.Query(query)
	if err != nil {
		return nil, err
	}
	return result.Rest()
}
