package eventstore

import (
	"bytes"
	"encoding/binary"
	"sync"
	"testing"
	"time"

	datastore "github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/query"
	ipldformat "github.com/ipfs/go-ipld-format"
	core "github.com/textileio/go-textile-core/store"
)

func TestNewEventDispatcher(t *testing.T) {
	t.Parallel()
	eventstore := NewTxMapDatastore()
	dispatcher := newDispatcher(eventstore)
	event := newNullEvent(time.Now())
	dispatcher.Dispatch(event)
}

func TestRegister(t *testing.T) {
	t.Parallel()
	eventstore := NewTxMapDatastore()
	dispatcher := newDispatcher(eventstore)
	dispatcher.Register(&nullReducer{})
	if len(dispatcher.reducers) < 1 {
		t.Error("expected callbacks map to have non-zero length")
	}
}

func TestDispatchLock(t *testing.T) {
	t.Parallel()
	eventstore := NewTxMapDatastore()
	dispatcher := newDispatcher(eventstore)
	dispatcher.Register(&slowReducer{})
	event := newNullEvent(time.Now())
	t1 := time.Now()
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := dispatcher.Dispatch(event); err != nil {
			t.Error("unexpected error in dispatch call")
		}
	}()
	if err := dispatcher.Dispatch(event); err != nil {
		t.Error("unexpected error in dispatch call")
	}
	wg.Wait()
	t2 := time.Now()
	if t2.Sub(t1) < (4 * time.Second) {
		t.Error("reached this point too soon")
	}
}

func TestDispatch(t *testing.T) {
	t.Parallel()
	eventstore := NewTxMapDatastore()
	dispatcher := newDispatcher(eventstore)
	event := newNullEvent(time.Now())
	if err := dispatcher.Dispatch(event); err != nil {
		t.Error("unexpected error in dispatch call")
	}
	results, err := dispatcher.Query(query.Query{})
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
	dispatcher.Register(&errorReducer{})
	err = dispatcher.Dispatch(event)
	if err == nil {
		t.Error("expected error in dispatch call")
	}
	if err.Error() != "error" {
		t.Errorf("`%s` should be `error`", err)
	}
	results, err = dispatcher.Query(query.Query{})
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if len(results) > 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
}

func TestValidStore(t *testing.T) {
	t.Parallel()
	eventstore := NewTxMapDatastore()
	dispatcher := newDispatcher(eventstore)
	store := dispatcher.Store()
	if store == nil {
		t.Error("store should not be nil")
	}
	if ok, _ := store.Has(datastore.NewKey("blah")); ok {
		t.Error("store should be empty")
	}
}

func TestDispatcherQuery(t *testing.T) {
	t.Parallel()
	eventstore := NewTxMapDatastore()
	dispatcher := newDispatcher(eventstore)
	var events []core.Event
	n := 100
	for i := 1; i <= n; i++ {
		events = append(events, newNullEvent(time.Now()))
		time.Sleep(time.Millisecond)
	}
	for _, event := range events {
		if err := dispatcher.Dispatch(event); err != nil {
			t.Error("unexpected error in dispatch call")
		}
	}
	results, err := dispatcher.Query(query.Query{
		Orders: []query.Order{query.OrderByKey{}},
	})
	if err != nil {
		t.Errorf("unexpected error: %s", err.Error())
	}
	if len(results) != n {
		t.Errorf("expected %d result, got %d", n, len(results))
	}
}

func newNullEvent(t time.Time) core.Event {
	return &nullEvent{Timestamp: t}
}

type nullEvent struct {
	Timestamp time.Time
}

func (n *nullEvent) Body() []byte {
	return nil
}

func (n *nullEvent) Time() []byte {
	t := n.Timestamp.UnixNano()
	buf := new(bytes.Buffer)
	// Use big endian to preserve lexicographic sorting
	binary.Write(buf, binary.BigEndian, t)
	return buf.Bytes()
}

func (n *nullEvent) EntityID() core.EntityID {
	return "null"
}

func (n *nullEvent) Type() string {
	return "null"
}

func (n *nullEvent) Node() (ipldformat.Node, error) {
	return nil, nil
}

// Sanity check
var _ core.Event = (*nullEvent)(nil)
