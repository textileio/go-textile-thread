// Package db provides a DB which manage collections
package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"sync"
	"time"

	ds "github.com/ipfs/go-datastore"
	kt "github.com/ipfs/go-datastore/keytransform"
	"github.com/ipfs/go-datastore/query"
	logging "github.com/ipfs/go-log"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/textileio/go-threads/broadcast"
	core "github.com/textileio/go-threads/core/db"
	lstore "github.com/textileio/go-threads/core/logstore"
	s "github.com/textileio/go-threads/core/service"
	"github.com/textileio/go-threads/core/thread"
	sym "github.com/textileio/go-threads/crypto/symmetric"
	"github.com/textileio/go-threads/util"
)

const (
	idFieldName = "ID"
	busTimeout  = time.Second * 10
)

var (
	log = logging.Logger("db")

	// ErrInvalidCollectionType indicates the provided default type isn't compatible
	// with a Collection type.
	ErrInvalidCollectionType = errors.New("the collection type should be a non-nil pointer to a struct that has an ID property")

	dsDBPrefix  = ds.NewKey("/db")
	dsDBSchemas = dsDBPrefix.ChildString("schema")
	dsDBIndexes = dsDBPrefix.ChildString("index")
)

// DB is the aggregate-root of events and state. External/remote events
// are dispatched to the DB, and are internally processed to impact collection
// states. Likewise, local changes in collections registered produce events dispatched
// externally.
type DB struct {
	io.Closer

	datastore  ds.TxnDatastore
	dispatcher *dispatcher
	eventcodec core.EventCodec
	adapter    *singleThreadAdapter

	lock            sync.RWMutex
	collectionNames map[string]*Collection
	jsonMode        bool
	closed          bool

	localEventsBus      *localEventsBus
	stateChangedNotifee *stateChangedNotifee
}

// NewDB creates a new DB, which will *own* ds and dispatcher for internal use.
// Saying it differently, ds and dispatcher shouldn't be used externally.
func NewDB(ctx context.Context, ts s.Service, id thread.ID, opts ...Option) (*DB, error) {
	config := &Config{}
	for _, opt := range opts {
		if err := opt(config); err != nil {
			return nil, err
		}
	}

	if _, err := ts.GetThread(ctx, id); err != nil {
		if errors.Is(err, lstore.ErrThreadNotFound) {
			if _, err = ts.CreateThread(ctx, id, s.FollowKey(sym.New()), s.ReadKey(sym.New())); err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}
	return newDB(ts, id, config)
}

// NewDBFromAddr creates a new DB from a thread hosted by another peer at address,
// which will *own* ds and dispatcher for internal use.
// Saying it differently, ds and dispatcher shouldn't be used externally.
func NewDBFromAddr(ctx context.Context, ts s.Service, addr ma.Multiaddr, followKey, readKey *sym.Key, opts ...Option) (*DB, error) {
	config := &Config{}
	for _, opt := range opts {
		if err := opt(config); err != nil {
			return nil, err
		}
	}

	ti, err := ts.AddThread(ctx, addr, s.FollowKey(followKey), s.ReadKey(readKey))
	if err != nil {
		return nil, err
	}
	d, err := newDB(ts, ti.ID, config)
	if err != nil {
		return nil, err
	}

	go func() {
		if err := ts.PullThread(ctx, ti.ID); err != nil {
			log.Errorf("error pulling thread %s", ti.ID.String())
		}
	}()
	return d, nil
}

// newDB is used directly by a db manager to create new dbs
// with the same config.
func newDB(ts s.Service, id thread.ID, config *Config, collections ...CollectionConfig) (*DB, error) {
	if config.Datastore == nil {
		datastore, err := newDefaultDatastore(config.RepoPath, config.LowMem)
		if err != nil {
			return nil, err
		}
		config.Datastore = datastore
	}
	if config.EventCodec == nil {
		config.EventCodec = newDefaultEventCodec(config.JsonMode)
	}
	if !managedDatastore(config.Datastore) {
		if config.Debug {
			if err := util.SetLogLevels(map[string]logging.LogLevel{
				"db": logging.LevelDebug,
			}); err != nil {
				return nil, err
			}
		}
	}

	d := &DB{
		datastore:           config.Datastore,
		dispatcher:          newDispatcher(config.Datastore),
		eventcodec:          config.EventCodec,
		collectionNames:     make(map[string]*Collection),
		jsonMode:            config.JsonMode,
		localEventsBus:      &localEventsBus{bus: broadcast.NewBroadcaster(0)},
		stateChangedNotifee: &stateChangedNotifee{},
	}
	if d.jsonMode {
		if err := d.reCreateCollections(); err != nil {
			return nil, err
		}
	}
	d.dispatcher.Register(d)

	for _, cc := range collections {
		if _, err := d.NewCollection(cc); err != nil {
			return nil, err
		}
	}

	adapter := newSingleThreadAdapter(d, ts, id)
	d.adapter = adapter
	adapter.Start()

	return d, nil
}

// reCreateCollections loads and registers schemas from the datastore.
func (d *DB) reCreateCollections() error {
	results, err := d.datastore.Query(query.Query{
		Prefix: dsDBSchemas.String(),
	})
	if err != nil {
		return err
	}
	defer results.Close()

	for res := range results.Next() {
		name := ds.RawKey(res.Key).Name()
		var indexes []IndexConfig
		index, err := d.datastore.Get(dsDBIndexes.ChildString(name))
		if err == nil && index != nil {
			_ = json.Unmarshal(index, &indexes)
		}
		if _, err := d.NewCollection(CollectionConfig{
			Name:    name,
			Schema:  string(res.Value),
			Indexes: indexes,
		}); err != nil {
			return err
		}
	}
	return nil
}

// NewCollectionFromInstance creates a new collection in the db
// by infering type from a defaultInstance.
func (d *DB) NewCollectionFromInstance(name string, defaultInstance interface{}) (*Collection, error) {
	d.lock.Lock()
	defer d.lock.Unlock()

	diType := reflect.TypeOf(defaultInstance)
	if reflect.ValueOf(defaultInstance).IsNil() || diType.Kind() != reflect.Ptr || diType.Elem().Kind() != reflect.Struct {
		return nil, ErrInvalidCollectionType
	}

	if _, ok := d.collectionNames[name]; ok {
		return nil, fmt.Errorf("already registered collection")
	}

	if !isValidCollection(defaultInstance) {
		return nil, ErrInvalidCollectionType
	}

	c := newCollection(name, defaultInstance, d)
	d.collectionNames[name] = c
	return c, nil
}

// CollectionConfig describes a new Collection.
type CollectionConfig struct {
	Name    string
	Schema  string
	Indexes []IndexConfig
}

// NewCollection creates a new collection in the db with a JSON schema.
func (d *DB) NewCollection(config CollectionConfig) (*Collection, error) {
	d.lock.Lock()
	defer d.lock.Unlock()

	if _, ok := d.collectionNames[config.Name]; ok {
		return nil, fmt.Errorf("already registered collection")
	}

	c := newCollectionFromSchema(config.Name, config.Schema, d)
	key := dsDBSchemas.ChildString(config.Name)
	exists, err := d.datastore.Has(key)
	if err != nil {
		return nil, err
	}
	if !exists {
		if err := d.datastore.Put(key, []byte(config.Schema)); err != nil {
			return nil, err
		}
	}

	for _, cfg := range config.Indexes {
		// @todo: Should check to make sure this is a valid field path for this schema
		if err := c.AddIndex(cfg.Path, cfg.Unique); err != nil {
			return nil, err
		}
	}

	indexBytes, err := json.Marshal(config.Indexes)
	if err != nil {
		return nil, err
	}
	indexKey := dsDBIndexes.ChildString(config.Name)
	exists, err = d.datastore.Has(indexKey)
	if err != nil {
		return nil, err
	}
	if !exists {
		if err := d.datastore.Put(indexKey, indexBytes); err != nil {
			return nil, err
		}
	}

	d.collectionNames[config.Name] = c
	return c, nil
}

// GetCollection returns a collection by name.
func (d *DB) GetCollection(name string) *Collection {
	return d.collectionNames[name]
}

// Reduce processes txn events into the collections.
func (d *DB) Reduce(events []core.Event) error {
	codecActions, err := d.eventcodec.Reduce(
		events,
		d.datastore,
		baseKey,
		defaultIndexFunc(d),
	)
	if err != nil {
		return err
	}
	actions := make([]Action, len(codecActions))
	for i, ca := range codecActions {
		var actionType ActionType
		switch codecActions[i].Type {
		case core.Create:
			actionType = ActionCreate
		case core.Save:
			actionType = ActionSave
		case core.Delete:
			actionType = ActionDelete
		default:
			panic("eventcodec action not recognized")
		}
		actions[i] = Action{Collection: ca.Collection, Type: actionType, ID: ca.InstanceID}
	}
	d.notifyStateChanged(actions)

	return nil
}

// Close closes the db.
func (d *DB) Close() error {
	d.lock.Lock()
	defer d.lock.Unlock()
	if d.closed {
		return nil
	}
	d.closed = true

	if d.adapter != nil {
		d.adapter.Close()
	}
	d.localEventsBus.bus.Discard()
	if !managedDatastore(d.datastore) {
		if err := d.datastore.Close(); err != nil {
			return err
		}
	}
	d.stateChangedNotifee.close()
	return nil
}

// dispatch applies external events to the db. This function guarantee
// no interference with registered collection states, and viceversa.
func (d *DB) dispatch(events []core.Event) error {
	d.lock.Lock()
	defer d.lock.Unlock()
	return d.dispatcher.Dispatch(events)
}

// eventFromBytes generates an Event from its binary representation using
// the underlying EventCodec configured in the DB.
func (d *DB) eventsFromBytes(data []byte) ([]core.Event, error) {
	return d.eventcodec.EventsFromBytes(data)
}

func (d *DB) readTxn(c *Collection, f func(txn *Txn) error) error {
	d.lock.RLock()
	defer d.lock.RUnlock()

	txn := &Txn{collection: c, readonly: true}
	defer txn.Discard()
	if err := f(txn); err != nil {
		return err
	}
	return nil
}

// managedDatastore returns whether or not the datastore is
// being wrapped by an external datastore.
func managedDatastore(ds ds.Datastore) bool {
	_, ok := ds.(kt.KeyTransform)
	return ok
}

func (d *DB) writeTxn(c *Collection, f func(txn *Txn) error) error {
	d.lock.Lock()
	defer d.lock.Unlock()

	txn := &Txn{collection: c}
	defer txn.Discard()
	if err := f(txn); err != nil {
		return err
	}
	return txn.Commit()
}

func isValidCollection(t interface{}) (valid bool) {
	defer func() {
		if err := recover(); err != nil {
			valid = false
		}
	}()
	v := reflect.ValueOf(t)
	if v.Type().Kind() != reflect.Ptr {
		v = reflect.New(reflect.TypeOf(v))
	}
	return v.Elem().FieldByName(idFieldName).IsValid()
}

func defaultIndexFunc(s *DB) func(collection string, key ds.Key, oldData, newData []byte, txn ds.Txn) error {
	return func(collection string, key ds.Key, oldData, newData []byte, txn ds.Txn) error {
		indexer := s.GetCollection(collection)
		if err := indexDelete(indexer, txn, key, oldData); err != nil {
			return err
		}
		if newData != nil {
			if err := indexAdd(indexer, txn, key, newData); err != nil {
				return err
			}
		}
		return nil
	}
}
