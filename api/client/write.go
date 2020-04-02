package client

import (
	"encoding/json"
	"fmt"

	pb "github.com/textileio/go-threads/api/pb"
	"github.com/textileio/go-threads/core/thread"
	"github.com/textileio/go-threads/db"
)

// WriteTransaction encapsulates a write transaction.
type WriteTransaction struct {
	client         pb.API_WriteTransactionClient
	dbID           thread.ID
	auth           *thread.Auth
	collectionName string
}

// Start starts the write transaction.
func (t *WriteTransaction) Start() (EndTransactionFunc, error) {
	body := &pb.StartTransactionRequest_Body{
		DbID:           t.dbID.Bytes(),
		CollectionName: t.collectionName,
	}
	header, err := getHeader(t.auth, body)
	if err != nil {
		return nil, err
	}
	innerReq := &pb.StartTransactionRequest{
		Header: header,
		Body:   body,
	}
	option := &pb.WriteTransactionRequest_StartTransactionRequest{
		StartTransactionRequest: innerReq,
	}
	if err := t.client.Send(&pb.WriteTransactionRequest{
		Option: option,
	}); err != nil {
		return nil, err
	}
	return t.end, nil
}

// Has runs a has query in the active transaction.
func (t *WriteTransaction) Has(instanceIDs ...string) (bool, error) {
	body := &pb.HasRequest_Body{
		InstanceIDs: instanceIDs,
	}
	header, err := getHeader(t.auth, body)
	if err != nil {
		return false, err
	}
	innerReq := &pb.HasRequest{
		Header: header,
		Body:   body,
	}
	option := &pb.WriteTransactionRequest_HasRequest{
		HasRequest: innerReq,
	}
	if err = t.client.Send(&pb.WriteTransactionRequest{
		Option: option,
	}); err != nil {
		return false, err
	}
	var resp *pb.WriteTransactionReply
	if resp, err = t.client.Recv(); err != nil {
		return false, err
	}
	switch x := resp.GetOption().(type) {
	case *pb.WriteTransactionReply_HasReply:
		return x.HasReply.GetExists(), nil
	default:
		return false, fmt.Errorf("WriteTransactionReply.Option has unexpected type %T", x)
	}
}

// FindByID gets the instance with the specified ID.
func (t *WriteTransaction) FindByID(instanceID string, instance interface{}) error {
	body := &pb.FindByIDRequest_Body{
		InstanceID: instanceID,
	}
	header, err := getHeader(t.auth, body)
	if err != nil {
		return err
	}
	innerReq := &pb.FindByIDRequest{
		Header: header,
		Body:   body,
	}
	option := &pb.WriteTransactionRequest_FindByIDRequest{
		FindByIDRequest: innerReq,
	}
	if err = t.client.Send(&pb.WriteTransactionRequest{
		Option: option,
	}); err != nil {
		return err
	}
	var resp *pb.WriteTransactionReply
	if resp, err = t.client.Recv(); err != nil {
		return err
	}
	switch x := resp.GetOption().(type) {
	case *pb.WriteTransactionReply_FindByIDReply:
		err := json.Unmarshal(x.FindByIDReply.GetInstance(), instance)
		return err
	default:
		return fmt.Errorf("WriteTransactionReply.Option has unexpected type %T", x)
	}
}

// Find finds instances by query.
func (t *WriteTransaction) Find(query *db.Query, dummy interface{}) (interface{}, error) {
	queryBytes, err := json.Marshal(query)
	if err != nil {
		return nil, err
	}
	body := &pb.FindRequest_Body{
		QueryJSON: queryBytes,
	}
	header, err := getHeader(t.auth, body)
	if err != nil {
		return nil, err
	}
	innerReq := &pb.FindRequest{
		Header: header,
		Body:   body,
	}
	option := &pb.WriteTransactionRequest_FindRequest{
		FindRequest: innerReq,
	}
	if err = t.client.Send(&pb.WriteTransactionRequest{
		Option: option,
	}); err != nil {
		return nil, err
	}
	var resp *pb.WriteTransactionReply
	if resp, err = t.client.Recv(); err != nil {
		return nil, err
	}
	switch x := resp.GetOption().(type) {
	case *pb.WriteTransactionReply_FindReply:
		return processFindReply(x.FindReply, dummy)
	default:
		return nil, fmt.Errorf("WriteTransactionReply.Option has unexpected type %T", x)
	}
}

// Create creates new instances of objects.
func (t *WriteTransaction) Create(items ...interface{}) ([]string, error) {
	values, err := marshalItems(items)
	if err != nil {
		return nil, err
	}
	body := &pb.CreateRequest_Body{
		Instances: values,
	}
	header, err := getHeader(t.auth, body)
	if err != nil {
		return nil, err
	}
	innerReq := &pb.CreateRequest{
		Header: header,
		Body:   body,
	}
	option := &pb.WriteTransactionRequest_CreateRequest{
		CreateRequest: innerReq,
	}
	if err = t.client.Send(&pb.WriteTransactionRequest{
		Option: option,
	}); err != nil {
		return nil, err
	}
	var resp *pb.WriteTransactionReply
	if resp, err = t.client.Recv(); err != nil {
		return nil, err
	}
	switch x := resp.GetOption().(type) {
	case *pb.WriteTransactionReply_CreateReply:
		return x.CreateReply.GetInstanceIDs(), nil
	default:
		return nil, fmt.Errorf("WriteTransactionReply.Option has unexpected type %T", x)
	}
}

// Save saves existing instances.
func (t *WriteTransaction) Save(items ...interface{}) error {
	values, err := marshalItems(items)
	if err != nil {
		return err
	}
	body := &pb.SaveRequest_Body{
		Instances: values,
	}
	header, err := getHeader(t.auth, body)
	if err != nil {
		return err
	}
	innerReq := &pb.SaveRequest{
		Header: header,
		Body:   body,
	}
	option := &pb.WriteTransactionRequest_SaveRequest{
		SaveRequest: innerReq,
	}
	if err = t.client.Send(&pb.WriteTransactionRequest{
		Option: option,
	}); err != nil {
		return err
	}
	var resp *pb.WriteTransactionReply
	if resp, err = t.client.Recv(); err != nil {
		return err
	}
	switch x := resp.GetOption().(type) {
	case *pb.WriteTransactionReply_SaveReply:
		return nil
	default:
		return fmt.Errorf("WriteTransactionReply.Option has unexpected type %T", x)
	}
}

// Delete deletes data.
func (t *WriteTransaction) Delete(instanceIDs ...string) error {
	body := &pb.DeleteRequest_Body{
		InstanceIDs: instanceIDs,
	}
	header, err := getHeader(t.auth, body)
	if err != nil {
		return err
	}
	innerReq := &pb.DeleteRequest{
		Header: header,
		Body:   body,
	}
	option := &pb.WriteTransactionRequest_DeleteRequest{
		DeleteRequest: innerReq,
	}
	if err := t.client.Send(&pb.WriteTransactionRequest{
		Option: option,
	}); err != nil {
		return err
	}
	var resp *pb.WriteTransactionReply
	if resp, err = t.client.Recv(); err != nil {
		return err
	}
	switch x := resp.GetOption().(type) {
	case *pb.WriteTransactionReply_DeleteReply:
		return nil
	default:
		return fmt.Errorf("WriteTransactionReply.Option has unexpected type %T", x)
	}
}

// end ends the active transaction.
func (t *WriteTransaction) end() error {
	return t.client.CloseSend()
}
