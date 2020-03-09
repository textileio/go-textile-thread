package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/google/uuid"
	ma "github.com/multiformats/go-multiaddr"
	pb "github.com/textileio/go-threads/api/pb"
	core "github.com/textileio/go-threads/core/db"
	"github.com/textileio/go-threads/crypto/symmetric"
	"github.com/textileio/go-threads/db"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// service is a gRPC service for a db manager.
type service struct {
	manager *db.Manager
}

// NewDB adds a new db into the manager.
func (s *service) NewDB(context.Context, *pb.NewDBRequest) (*pb.NewDBReply, error) {
	log.Debugf("received new db request")

	id, _, err := s.manager.NewDB()
	if err != nil {
		return nil, err
	}

	return &pb.NewDBReply{
		ID: id.String(),
	}, nil
}

// NewCollection registers a JSON schema with a db.
func (s *service) NewCollection(_ context.Context, req *pb.NewCollectionRequest) (*pb.NewCollectionReply, error) {
	log.Debugf("received register schema request in db %s", req.DBID)

	d, err := s.getDB(req.DBID)
	if err != nil {
		return nil, err
	}
	indexes := make([]*db.IndexConfig, len(req.Indexes))
	for i, index := range req.Indexes {
		indexes[i] = &db.IndexConfig{
			Path:   index.Path,
			Unique: index.Unique,
		}
	}
	if _, err = d.NewCollection(req.Name, req.Schema, indexes...); err != nil {
		return nil, err
	}

	return &pb.NewCollectionReply{}, nil
}

func (s *service) Start(_ context.Context, req *pb.StartRequest) (*pb.StartReply, error) {
	d, err := s.getDB(req.GetDBID())
	if err != nil {
		return nil, err
	}
	if err := d.Start(); err != nil {
		return nil, err
	}
	return &pb.StartReply{}, nil
}

func (s *service) GetDBLink(ctx context.Context, req *pb.GetDBLinkRequest) (*pb.GetDBLinkReply, error) {
	var err error
	var d *db.DB
	if d, err = s.getDB(req.GetDBID()); err != nil {
		return nil, err
	}
	tid, _, err := d.ThreadID()
	if err != nil {
		return nil, err
	}
	tinfo, err := d.Service().GetThread(ctx, tid)
	if err != nil {
		return nil, err
	}
	host := d.Service().Host()
	id, _ := ma.NewComponent("p2p", host.ID().String())
	thread, _ := ma.NewComponent("thread", tid.String())
	addrs := host.Addrs()
	res := make([]string, len(addrs))
	for i := range addrs {
		res[i] = addrs[i].Encapsulate(id).Encapsulate(thread).String()
	}
	reply := &pb.GetDBLinkReply{
		Addresses: res,
		FollowKey: tinfo.FollowKey.Bytes(),
		ReadKey:   tinfo.ReadKey.Bytes(),
	}
	return reply, nil
}

func (s *service) StartFromAddress(_ context.Context, req *pb.StartFromAddressRequest) (*pb.StartFromAddressReply, error) {
	var err error
	var d *db.DB
	var addr ma.Multiaddr
	var readKey, followKey *symmetric.Key
	if d, err = s.getDB(req.GetDBID()); err != nil {
		return nil, err
	}
	if addr, err = ma.NewMultiaddr(req.GetAddress()); err != nil {
		return nil, err
	}
	if readKey, err = symmetric.NewKey(req.GetReadKey()); err != nil {
		return nil, err
	}
	if followKey, err = symmetric.NewKey(req.GetFollowKey()); err != nil {
		return nil, err
	}
	if err = d.StartFromAddr(addr, followKey, readKey); err != nil {
		return nil, err
	}
	return &pb.StartFromAddressReply{}, nil
}

// ModelCreate adds a new instance of a model to a db.
func (s *service) ModelCreate(_ context.Context, req *pb.ModelCreateRequest) (*pb.ModelCreateReply, error) {
	log.Debugf("received model create request for model %s", req.ModelName)
	model, err := s.getModel(req.DBID, req.ModelName)
	if err != nil {
		return nil, err
	}
	return s.processCreateRequest(req, model.Create)
}

func (s *service) ModelSave(_ context.Context, req *pb.ModelSaveRequest) (*pb.ModelSaveReply, error) {
	model, err := s.getModel(req.DBID, req.ModelName)
	if err != nil {
		return nil, err
	}
	return s.processSaveRequest(req, model.Save)
}

func (s *service) ModelDelete(_ context.Context, req *pb.ModelDeleteRequest) (*pb.ModelDeleteReply, error) {
	model, err := s.getModel(req.DBID, req.ModelName)
	if err != nil {
		return nil, err
	}
	return s.processDeleteRequest(req, model.Delete)
}

func (s *service) ModelHas(_ context.Context, req *pb.ModelHasRequest) (*pb.ModelHasReply, error) {
	model, err := s.getModel(req.DBID, req.ModelName)
	if err != nil {
		return nil, err
	}
	return s.processHasRequest(req, model.Has)
}

func (s *service) ModelFind(_ context.Context, req *pb.ModelFindRequest) (*pb.ModelFindReply, error) {
	model, err := s.getModel(req.DBID, req.ModelName)
	if err != nil {
		return nil, err
	}
	return s.processFindRequest(req, model.FindJSON)
}

func (s *service) ModelFindByID(_ context.Context, req *pb.ModelFindByIDRequest) (*pb.ModelFindByIDReply, error) {
	model, err := s.getModel(req.DBID, req.ModelName)
	if err != nil {
		return nil, err
	}
	return s.processFindByIDRequest(req, model.FindByID)
}

func (s *service) ReadTransaction(stream pb.API_ReadTransactionServer) error {
	firstReq, err := stream.Recv()
	if err != nil {
		return err
	}

	var dbID, modelName string
	switch x := firstReq.GetOption().(type) {
	case *pb.ReadTransactionRequest_StartTransactionRequest:
		dbID = x.StartTransactionRequest.GetDBID()
		modelName = x.StartTransactionRequest.GetModelName()
	case nil:
		return fmt.Errorf("no ReadTransactionRequest type set")
	default:
		return fmt.Errorf("ReadTransactionRequest.Option has unexpected type %T", x)
	}

	model, err := s.getModel(dbID, modelName)
	if err != nil {
		return err
	}

	return model.ReadTxn(func(txn *db.Txn) error {
		for {
			req, err := stream.Recv()
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return err
			}
			switch x := req.GetOption().(type) {
			case *pb.ReadTransactionRequest_ModelHasRequest:
				innerReply, err := s.processHasRequest(x.ModelHasRequest, txn.Has)
				if err != nil {
					return err
				}
				option := &pb.ReadTransactionReply_ModelHasReply{ModelHasReply: innerReply}
				if err := stream.Send(&pb.ReadTransactionReply{Option: option}); err != nil {
					return err
				}
			case *pb.ReadTransactionRequest_ModelFindByIDRequest:
				innerReply, err := s.processFindByIDRequest(x.ModelFindByIDRequest, txn.FindByID)
				if err != nil {
					return err
				}
				option := &pb.ReadTransactionReply_ModelFindByIDReply{ModelFindByIDReply: innerReply}
				if err := stream.Send(&pb.ReadTransactionReply{Option: option}); err != nil {
					return err
				}
			case *pb.ReadTransactionRequest_ModelFindRequest:
				innerReply, err := s.processFindRequest(x.ModelFindRequest, txn.FindJSON)
				if err != nil {
					return err
				}
				option := &pb.ReadTransactionReply_ModelFindReply{ModelFindReply: innerReply}
				if err := stream.Send(&pb.ReadTransactionReply{Option: option}); err != nil {
					return err
				}
			case nil:
				return fmt.Errorf("no ReadTransactionRequest type set")
			default:
				return fmt.Errorf("ReadTransactionRequest.Option has unexpected type %T", x)
			}
		}
	})
}

func (s *service) WriteTransaction(stream pb.API_WriteTransactionServer) error {
	firstReq, err := stream.Recv()
	if err != nil {
		return err
	}

	var dbID, modelName string
	switch x := firstReq.GetOption().(type) {
	case *pb.WriteTransactionRequest_StartTransactionRequest:
		dbID = x.StartTransactionRequest.GetDBID()
		modelName = x.StartTransactionRequest.GetModelName()
	case nil:
		return fmt.Errorf("no WriteTransactionRequest type set")
	default:
		return fmt.Errorf("WriteTransactionRequest.Option has unexpected type %T", x)
	}

	model, err := s.getModel(dbID, modelName)
	if err != nil {
		return err
	}

	return model.WriteTxn(func(txn *db.Txn) error {
		for {
			req, err := stream.Recv()
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return err
			}
			switch x := req.GetOption().(type) {
			case *pb.WriteTransactionRequest_ModelHasRequest:
				innerReply, err := s.processHasRequest(x.ModelHasRequest, txn.Has)
				if err != nil {
					return err
				}
				option := &pb.WriteTransactionReply_ModelHasReply{ModelHasReply: innerReply}
				if err := stream.Send(&pb.WriteTransactionReply{Option: option}); err != nil {
					return err
				}
			case *pb.WriteTransactionRequest_ModelFindByIDRequest:
				innerReply, err := s.processFindByIDRequest(x.ModelFindByIDRequest, txn.FindByID)
				if err != nil {
					return err
				}
				option := &pb.WriteTransactionReply_ModelFindByIDReply{ModelFindByIDReply: innerReply}
				if err := stream.Send(&pb.WriteTransactionReply{Option: option}); err != nil {
					return err
				}
			case *pb.WriteTransactionRequest_ModelFindRequest:
				innerReply, err := s.processFindRequest(x.ModelFindRequest, txn.FindJSON)
				if err != nil {
					return err
				}
				option := &pb.WriteTransactionReply_ModelFindReply{ModelFindReply: innerReply}
				if err := stream.Send(&pb.WriteTransactionReply{Option: option}); err != nil {
					return err
				}
			case *pb.WriteTransactionRequest_ModelCreateRequest:
				innerReply, err := s.processCreateRequest(x.ModelCreateRequest, txn.Create)
				if err != nil {
					return err
				}
				option := &pb.WriteTransactionReply_ModelCreateReply{ModelCreateReply: innerReply}
				if err := stream.Send(&pb.WriteTransactionReply{Option: option}); err != nil {
					return err
				}
			case *pb.WriteTransactionRequest_ModelSaveRequest:
				innerReply, err := s.processSaveRequest(x.ModelSaveRequest, txn.Save)
				if err != nil {
					return err
				}
				option := &pb.WriteTransactionReply_ModelSaveReply{ModelSaveReply: innerReply}
				if err := stream.Send(&pb.WriteTransactionReply{Option: option}); err != nil {
					return err
				}
			case *pb.WriteTransactionRequest_ModelDeleteRequest:
				innerReply, err := s.processDeleteRequest(x.ModelDeleteRequest, txn.Delete)
				if err != nil {
					return err
				}
				option := &pb.WriteTransactionReply_ModelDeleteReply{ModelDeleteReply: innerReply}
				if err := stream.Send(&pb.WriteTransactionReply{Option: option}); err != nil {
					return err
				}
			case nil:
				return fmt.Errorf("no WriteTransactionRequest type set")
			default:
				return fmt.Errorf("WriteTransactionRequest.Option has unexpected type %T", x)
			}
		}
	})
}

// Listen returns a stream of entities, trigged by a local or remote state change.
func (s *service) Listen(req *pb.ListenRequest, server pb.API_ListenServer) error {
	d, err := s.getDB(req.DBID)
	if err != nil {
		return err
	}

	options := make([]db.ListenOption, len(req.GetFilters()))
	for i, filter := range req.GetFilters() {
		var listenActionType db.ListenActionType
		switch filter.GetAction() {
		case pb.ListenRequest_Filter_ALL:
			listenActionType = db.ListenAll
		case pb.ListenRequest_Filter_CREATE:
			listenActionType = db.ListenCreate
		case pb.ListenRequest_Filter_DELETE:
			listenActionType = db.ListenDelete
		case pb.ListenRequest_Filter_SAVE:
			listenActionType = db.ListenSave
		default:
			return status.Errorf(codes.InvalidArgument, "invalid filter action %v", filter.GetAction())
		}
		options[i] = db.ListenOption{
			Type:  listenActionType,
			Model: filter.GetModelName(),
			ID:    core.EntityID(filter.EntityID),
		}
	}

	l, err := d.Listen(options...)
	if err != nil {
		return err
	}
	defer l.Close()

	for {
		err = nil
		select {
		case <-server.Context().Done():
			return nil
		case action, ok := <-l.Channel():
			if !ok {
				return nil
			}
			var replyAction pb.ListenReply_Action
			var entity []byte
			switch action.Type {
			case db.ActionCreate:
				replyAction = pb.ListenReply_CREATE
				entity, err = s.entityForAction(d, action)
			case db.ActionDelete:
				replyAction = pb.ListenReply_DELETE
			case db.ActionSave:
				replyAction = pb.ListenReply_SAVE
				entity, err = s.entityForAction(d, action)
			default:
				err = status.Errorf(codes.Internal, "unknown action type %v", action.Type)
			}
			if err != nil {
				return err
			}
			reply := &pb.ListenReply{
				ModelName: action.Model,
				EntityID:  action.ID.String(),
				Action:    replyAction,
				Entity:    entity,
			}
			if err := server.Send(reply); err != nil {
				return err
			}
		}
	}
}

func (s *service) entityForAction(db *db.DB, action db.Action) ([]byte, error) {
	model := db.GetModel(action.Model)
	if model == nil {
		return nil, status.Error(codes.NotFound, "model not found")
	}
	var res string
	if err := model.FindByID(action.ID, &res); err != nil {
		return nil, err
	}
	return []byte(res), nil
}

func (s *service) processCreateRequest(req *pb.ModelCreateRequest, createFunc func(...interface{}) error) (*pb.ModelCreateReply, error) {
	values := make([]interface{}, len(req.Values))
	for i, v := range req.Values {
		s := v
		values[i] = &s
	}
	if err := createFunc(values...); err != nil {
		return nil, err
	}

	reply := &pb.ModelCreateReply{
		Entities: make([]string, len(values)),
	}
	for i, v := range values {
		reply.Entities[i] = *(v.(*string))
	}
	return reply, nil
}

func (s *service) processSaveRequest(req *pb.ModelSaveRequest, saveFunc func(...interface{}) error) (*pb.ModelSaveReply, error) {
	values := make([]interface{}, len(req.Values))
	for i, v := range req.Values {
		s := v
		values[i] = &s
	}
	if err := saveFunc(values...); err != nil {
		return nil, err
	}
	return &pb.ModelSaveReply{}, nil
}

func (s *service) processDeleteRequest(req *pb.ModelDeleteRequest, deleteFunc func(...core.EntityID) error) (*pb.ModelDeleteReply, error) {
	entityIDs := make([]core.EntityID, len(req.GetEntityIDs()))
	for i, ID := range req.GetEntityIDs() {
		entityIDs[i] = core.EntityID(ID)
	}
	if err := deleteFunc(entityIDs...); err != nil {
		return nil, err
	}
	return &pb.ModelDeleteReply{}, nil
}

func (s *service) processHasRequest(req *pb.ModelHasRequest, hasFunc func(...core.EntityID) (bool, error)) (*pb.ModelHasReply, error) {
	entityIDs := make([]core.EntityID, len(req.GetEntityIDs()))
	for i, ID := range req.GetEntityIDs() {
		entityIDs[i] = core.EntityID(ID)
	}
	exists, err := hasFunc(entityIDs...)
	if err != nil {
		return nil, err
	}
	return &pb.ModelHasReply{Exists: exists}, nil
}

func (s *service) processFindByIDRequest(req *pb.ModelFindByIDRequest, findFunc func(id core.EntityID, v interface{}) error) (*pb.ModelFindByIDReply, error) {
	entityID := core.EntityID(req.EntityID)
	var result string
	if err := findFunc(entityID, &result); err != nil {
		return nil, err
	}
	return &pb.ModelFindByIDReply{Entity: result}, nil
}

func (s *service) processFindRequest(req *pb.ModelFindRequest, findFunc func(q *db.JSONQuery) (ret []string, err error)) (*pb.ModelFindReply, error) {
	q := &db.JSONQuery{}
	if err := json.Unmarshal(req.GetQueryJSON(), q); err != nil {
		return nil, err
	}
	stringEntities, err := findFunc(q)
	if err != nil {
		return nil, err
	}
	byteEntities := make([][]byte, len(stringEntities))
	for i, stringEntity := range stringEntities {
		byteEntities[i] = []byte(stringEntity)
	}
	return &pb.ModelFindReply{Entities: byteEntities}, nil
}

func (s *service) getDB(idStr string) (*db.DB, error) {
	id, err := uuid.Parse(idStr)
	if err != nil {
		return nil, err
	}
	d := s.manager.GetDB(id)
	if d == nil {
		return nil, status.Error(codes.NotFound, "db not found")
	}
	return d, nil
}

func (s *service) getModel(dbID string, modelName string) (*db.Model, error) {
	d, err := s.getDB(dbID)
	if err != nil {
		return nil, status.Error(codes.NotFound, "db not found")
	}
	model := d.GetModel(modelName)
	if model == nil {
		return nil, status.Error(codes.NotFound, "model not found")
	}
	return model, nil
}
