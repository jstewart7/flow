package envoy

import (
	"fmt"
	"time"
	"errors"
	"math/rand"
	"reflect"

	"github.com/unitoftime/flow/net"
	// "github.com/unitoftime/flow/net/rpc"
)

var ErrTimeout = errors.New("timeout reached")
var ErrDisconnected = errors.New("socket disconnected")


type rpcClient interface {
	doRpc(any, time.Duration) (any, error)
	doMsg(any) error
}

type MsgDefinition interface {
	MsgType() any
}

type RpcDefinition interface {
	ReqType() any
	RespType() any
}

type clientSetter interface {
	setClient(rpcClient)
}

type RpcHandler interface {
	Handler() (reflect.Type, HandlerFunc)
}

type MsgHandler interface {
	Handler() (reflect.Type, MessageHandlerFunc)
}

type MsgDef[A any] struct {
	handler MessageHandlerFunc
	client rpcClient
}

func (d *MsgDef[A]) setClient(client rpcClient) {
	d.client = client
}

func (d MsgDef[A]) Handler() (reflect.Type, MessageHandlerFunc) {
	var a A
	return reflect.TypeOf(a), d.handler
}

func (d *MsgDef[A]) Register(handler func(A)) {
	d.handler = makeMsgHandler(handler)
}

func (d MsgDef[A]) MsgType() any {
	var a A
	return a
}

func (d MsgDef[A]) Send(msg A) error {
	return d.client.doMsg(msg)
}


type RpcDef[Req, Resp any] struct {
	handler HandlerFunc
	client rpcClient
}

func (d *RpcDef[Req, Resp]) setClient(client rpcClient) {
	d.client = client
}

func (d RpcDef[Req, Resp]) ReqType() any {
	var req Req
	return req
}

func (d RpcDef[Req, Resp]) RespType() any {
	var resp Resp
	return resp
}

func (d *RpcDef[Req, Resp]) Register(handler func(Req) Resp) {
	d.handler = makeRpcHandler(handler)
}

// func (d RpcDef[Req, Resp]) Get(client *Client[S, C]) *Call[Req, Resp] {
// 	return NewCall[Req, Resp](client)
// }

func (d RpcDef[Req, Resp]) Call(req Req) (Resp, error) {
	var resp Resp
	anyResp, err := d.client.doRpc(req, 5 * time.Second)
	if err != nil { return resp, err }
	resp, ok := anyResp.(Resp)
	if !ok { panic("Mismatched type!") }
	return resp, nil
}

func (d RpcDef[Req, Resp]) Handler() (reflect.Type, HandlerFunc) {
	var req Req
	return reflect.TypeOf(req), d.handler
}

func DefineService(def any) ServiceDefinition {
	ty := reflect.TypeOf(def)
	fmt.Println(ty)
	numField := ty.NumField()
	fmt.Println(numField)

	requests := make([]any, 0)
	responses := make([]any, 0)
	for i := 0; i < numField; i++ {
		field := ty.Field(i)
		fmt.Println(field)

		fieldAny := reflect.New(field.Type).Elem().Interface()

		fmt.Printf("Type: %T\n", fieldAny)
		switch rpcDef := fieldAny.(type) {
		case RpcDefinition:
			fmt.Println("HERE")
			reqStruct := rpcDef.ReqType()
			requests = append(requests, reqStruct)

			respStruct := rpcDef.RespType()
			responses = append(responses, respStruct)
		case MsgDefinition:
			msgStruct := rpcDef.MsgType()
			requests = append(requests, msgStruct)
		default:
			panic("Error: Fields must all either be RpcDef or MsgDef")
		}
	}

	return ServiceDefinition{
		Requests: net.NewUnion(requests...),
		Responses: net.NewUnion(responses...),
	}
}

// TODO - maybe define everything based on the Call definitions and the Msg definitions?

// Needs
// 1. Bidirectional RPCs
// 2. Fire-and-forget style RPCs (ie just send it and don't block, like a msg)
// 3. Easy setup and management
// 4. Different reliability levels

type InterfaceDef[S, C any] struct {
	Service S
	Client C
	serviceApi ServiceDefinition
	clientApi ServiceDefinition
}

func NewInterfaceDef[S, C any]() InterfaceDef[S, C] {
	var serviceApi S
	var clientApi C
	return InterfaceDef[S, C]{
		serviceApi: DefineService(serviceApi),
		clientApi: DefineService(clientApi),
	}
}

// func NewInterfaceDef[S, C any]() InterfaceDef[S, C] {
// 	serviceApi := new(S)
// 	clientApi := new(C)
// 	return InterfaceDef[S, C]{
// 		serviceApi: NewServiceDef(serviceApi),
// 		clientApi: NewServiceDef(clientApi),
// 	}
// }

func (d InterfaceDef[S, C]) NewClient() *Client[C, S] {
	// Note: The C and S are reversed because we call the service and serve the client
	client := newClient[C, S](d.clientApi, d.serviceApi)

	return client
}

func (d InterfaceDef[S, C]) NewServer() *Client[S, C] {
	client := newClient[S, C](d.serviceApi, d.clientApi)

	return client
}


// TODO - I should make an interface to better capture the fact that this is just for serialization
type ServiceDefinition struct {
	Requests, Responses *net.UnionBuilder
}

// TODO - could I make servicedef generic on the interface type. Then when I register handlers I just pass in a struct which implements the interface?

// TODO - I think I'd prefer this to be based on method name and not based on input argument type
// TODO - reordering the definition, or switching between a message and an RPC will break api compatibility
func NewServiceDef(def any) ServiceDefinition {
	ty := reflect.TypeOf(def).Elem()
	fmt.Println(ty)
	numMethod := ty.NumMethod()
	fmt.Println(numMethod)

	requests := make([]any, 0)
	responses := make([]any, 0)
	for i := 0; i < numMethod; i++ {
		method := ty.Method(i)
		fmt.Println(method)

		numInputs := method.Type.NumIn()
		// for j := 0; j < numInputs; j++ {
		// 	in := method.Type.In(j)
		// 	fmt.Println(in)
		// }

		if numInputs != 1 { panic("We only support methods of form: func (req) (resp, error) or func (req) error") }
		reqType := method.Type.In(0)
		reqStruct := reflect.New(reqType).Elem().Interface()
		requests = append(requests, reqStruct)

		numOutputs := method.Type.NumOut()
		// for j := 0; j < numOutputs; j++ {
		// 	out := method.Type.Out(j)
		// 	fmt.Println(out)
		// }

		if numOutputs == 1 {
			// Rpc doesn't expect a response
		} else if numOutputs == 2 {
			// Rpc expects a response
			respType := method.Type.Out(0)
			respStruct := reflect.New(respType).Elem().Interface()
			responses = append(responses, respStruct)
		} else {
			panic("We only support methods of form: func (req) (resp, error) or func (req) error")
		}

		// TODO - check last argument should be an error
	}

	return ServiceDefinition{
		Requests: net.NewUnion(requests...),
		Responses: net.NewUnion(responses...),
	}
}

var rpcSerdes serdes
func init() {
	rpcSerdes = serdes{
		union: net.NewUnion(
			Request{},
			Response{},
			Message{},
		),
	}
}
type serdes struct {
	union *net.UnionBuilder
}
func (s *serdes) Marshal(v any) ([]byte, error) {
	return s.union.Serialize(v)
}
func (s *serdes) Unmarshal(dat []byte) (any, error) {
	return s.union.Deserialize(dat)
}

type Request struct {
	Id uint32 // Tracks the request Id
	Data []byte
}

type Response struct {
	Id uint32 // Tracks the request Id
	Data []byte
}

type Message struct {
	Data []byte
}

func newClient[S, C any](serviceDef, clientDef ServiceDefinition) *Client[S, C] {
	rngSrc := rand.NewSource(time.Now().UnixNano())
	rng := rand.New(rngSrc) // TODO - push this up to the client?

	client := &Client[S, C]{
		serviceDef: serviceDef,
		clientDef: clientDef,

		// TODO - can I make this so that I make client generic on the interface, then I just pass the interface in here and have it get called. Then I'd just call the like interface function 2 or interface function 3 or something?
		handlers: make(map[reflect.Type]HandlerFunc),
		messageHandlers: make(map[reflect.Type]MessageHandlerFunc),
		activeCalls: make(map[uint32]chan Response),

		rng: rng,
	}

	return client
}

func (c *Client[S, C]) Connect(sock net.Socket) {
	c.registerHandlers(c.Handler)
	c.registerCallers(&c.Call)

	c.sock = sock
	c.start() // This doesn't block
}

type Client[S, C any] struct {
	sock net.Socket

	Handler S
	Call C

	serviceDef, clientDef ServiceDefinition

	handlers map[reflect.Type]HandlerFunc
	messageHandlers map[reflect.Type]MessageHandlerFunc
	activeCalls map[uint32]chan Response

	rng *rand.Rand
}

func (c *Client[S, C]) Close() error {
	// TODO - what to do about active calls and handlers?
	return c.sock.Close()
}


// TODO - get rid of this
func (c *Client[S, C]) Closed() bool {
	return c.sock.Closed()
}

func (c *Client[S, C]) start() {
	dat := make([]byte, 8 * 1024) // TODO - hardcoded
	go func() {
		for {
			if c.sock.Closed() {
				return // sockets can never redial
			}

			err := c.sock.Read(dat)
			if err != nil {
				fmt.Println("ERROR: ", err)
				// TODO!!!!! - this might cause the for loop to spin if we are trying to reconnect, for example
				time.Sleep(10 * time.Millisecond)
				continue
			}

			msg, err := rpcSerdes.Unmarshal(dat)
			if err != nil {
				fmt.Println("ERROR: ", err)
				continue
			}

			// If the message was empty, just continue to the next one
			if msg == nil { continue }

			switch typedMsg := msg.(type) {
			case Request:
				resp, err := c.HandleRequest(typedMsg)
				if err != nil {
					fmt.Println("ERROR: ", err)
				}

				respDat, err := rpcSerdes.Marshal(resp)
				if err != nil {
					fmt.Println("ERROR: ", err)
				}

				err = c.sock.Write(respDat)
				if err != nil {
					fmt.Println("ERROR: ", err)
				}
			case Response:
				err := c.HandleResponse(typedMsg)
				if err != nil {
					fmt.Println("ERROR: ", err)
				}
			case Message:
				err := c.HandleMessage(typedMsg)
				if err != nil {
					fmt.Println("ERROR: ", err)
				}
			default:
				fmt.Printf("Unknown message type: %T\n", typedMsg)
			}
		}
	}()
}


type MessageHandlerFunc func(req any)
type HandlerFunc func(req any) any

func (c *Client[S, C]) HandleResponse(rpcResp Response) error {
	callChan, ok := c.activeCalls[rpcResp.Id]
	if !ok {
		return fmt.Errorf("Disassociated Response")
	}

	// Send the response to the appropriate call
	callChan <-rpcResp

	// Cleanup
	close(callChan)
	c.activeCalls[rpcResp.Id] = nil
	delete(c.activeCalls, rpcResp.Id)
	return nil
}

func (c *Client[S, C]) HandleRequest(rpcReq Request) (Response, error) {
	rpcResp := Response{
		Id: rpcReq.Id,
	}

	reqVal, err := c.serviceDef.Requests.Deserialize(rpcReq.Data)
	if err != nil { return rpcResp, err }
	reqValType := reflect.TypeOf(reqVal)

	handler, ok := c.handlers[reqValType]
	if !ok {
		return rpcResp, fmt.Errorf("RPC Handler not set for type: %T", reqVal)
	}

	anyResp := handler(reqVal)

	data, err := c.serviceDef.Responses.Serialize(anyResp)
	if err != nil {
		return rpcResp, err
	}

	rpcResp.Data = data
	return rpcResp, err
}

func (c *Client[S, C]) HandleMessage(msg Message) error {
	msgVal, err := c.serviceDef.Requests.Deserialize(msg.Data)
	if err != nil { return err }
	msgValType := reflect.TypeOf(msgVal)

	handler, ok := c.messageHandlers[msgValType]
	if !ok {
		return fmt.Errorf("Message Handler not set for type: %T", msgVal)
	}

	handler(msgVal)
	return nil
}

// func RegisterMessage[S, C, M any](client *Client[S, C], handler func(M) error) {
// 	var msgVal M
// 	msgValType := reflect.TypeOf(msgVal)
// 	_, exists := client.messageHandlers[msgValType]
// 	if exists {
// 		panic("Cant reregister the same handler type")
// 	}

// 	// Create a handler function
// 	generalHandlerFunc := makeMsgHandler(handler)

// 	// Store the handler function
// 	client.messageHandlers[msgValType] = generalHandlerFunc
// }

// TODO - Note: caller must be passed in as a pointer
func (client *Client[S, C]) registerCallers(caller any) {
	ty := reflect.TypeOf(caller)
	val := reflect.ValueOf(caller)
	numField := ty.Elem().NumField()
	for i := 0; i < numField; i++ {
		// field := ty.Field(i)
		// fmt.Println(field)
		field := val.Elem().Field(i).Addr()
		fmt.Println(field)

		fieldAny := field.Interface()

		fmt.Printf("Type: %T\n", fieldAny)
		switch rpcDef := fieldAny.(type) {
		case clientSetter:
			rpcDef.setClient(client)
		default:
			panic("Error: Must be a clientSetter")
		}
	}
}

func (client *Client[S, C]) registerHandlers(service any) {
	ty := reflect.TypeOf(service)
	val := reflect.ValueOf(service)
	numField := ty.NumField()
	for i := 0; i < numField; i++ {
		// field := ty.Field(i)
		// fmt.Println(field)
		field := val.Field(i)
		fmt.Println(field)

		// fieldAny := reflect.New(field.Type).Elem().Interface()
		fieldAny := field.Interface()

		fmt.Printf("Type: %T\n", fieldAny)
		switch rpcHandler := fieldAny.(type) {
		case RpcHandler:
			reqType, handler := rpcHandler.Handler()
			if handler == nil { panic("All Handlers must be defined!") }
			client.registerRpc(reqType, handler)
		case MsgHandler:
			msgType, handler := rpcHandler.Handler()
			if handler == nil { panic("All Handlers must be defined!") }
			client.registerMsg(msgType, handler)
		default:
			panic("ERROR") // TODO - must be an RpcDef or a message Def
		}
	}
}

func (client *Client[S, C]) registerRpc(reqValType reflect.Type, handler HandlerFunc) {
	if handler == nil { panic("Handler must not be nil!") }
	_, exists := client.handlers[reqValType]
	if exists {
		panic("Cant reregister the same handler type")
	}

	// Store the handler function
	client.handlers[reqValType] = handler
}

func (client *Client[S, C]) registerMsg(msgValType reflect.Type, handler MessageHandlerFunc) {

	_, exists := client.messageHandlers[msgValType]
	if exists {
		panic("Cant reregister the same handler type")
	}

	// Store the handler function
	client.messageHandlers[msgValType] = handler
}

// func Register[S, C, Req, Resp any](client *Client[S, C], handler func(Req) (Resp, error)) {
// 	var reqVal Req
// 	reqValType := reflect.TypeOf(reqVal)
// 	_, exists := client.handlers[reqValType]
// 	if exists {
// 		panic("Cant reregister the same handler type")
// 	}

// 	// Create a handler function
// 	generalHandlerFunc := makeRpcHandler(handler)

// 	// Store the handler function
// 	client.handlers[reqValType] = generalHandlerFunc
// }

func makeRpcHandler[Req, Resp any](handler func(Req) Resp) HandlerFunc {
	return func(anyReq any) any {
		req, ok := anyReq.(Req)
		if !ok {
			panic(fmt.Errorf("Mismatched request types: %T", req))
		}

		res := handler(req)

		return res
	}
}

func makeMsgHandler[M any](handler func(M)) MessageHandlerFunc {
	return func(anyMsg any) {
		msg, ok := anyMsg.(M)
		if !ok {
			panic(fmt.Errorf("Mismatched message types: %T", anyMsg))
		}

		handler(msg)
	}
}

// Client - making requests
// func (c *Client) MakeRequest(req any) (any, error) {
// 	dat, err := c.reqSerdes.Serialize(req)
// 	if err != nil { return err }

// 	reqDat, err := rpcSerdes.Marshal(Request{
// 		Id: 0, // TODO
// 		Data: dat,
// 	})
// 	if err != nil { return err }

// 	err = c.sock.Write(reqDat)
// 	return err
// }

// func Rpc[Req, Resp any](f func(Req) (Resp, error))

// func GetCall[Req, Resp any](client *Client, _ func(Req) (Resp, error)) *Call[Req, Resp] {
// 	return NewCall[Req, Resp](client)
// }

func NewCall[S, C, Req, Resp any](client *Client[S, C], rpc RpcDef[Req, Resp]) *Call[S, C, Req, Resp] {
	return &Call[S, C, Req, Resp]{
		client: client,
		timeout: 5 * time.Second,
	}
}

// func NewCall[S, C, Req, Resp any](client *Client[S, C]) *Call[S, C, Req, Resp] {
// 	return &Call[S, C, Req, Resp]{
// 		client: client,
// 		timeout: 5 * time.Second,
// 	}
// }
type Call[S, C, Req, Resp any] struct {
	client *Client[S, C]
	timeout time.Duration
}


func (c *Call[S, C, Req, Resp]) Do(req Req) (Resp, error) {
	var resp Resp
	rpcReq, err := c.client.MakeRequest(req)
	if err != nil { return resp, err }

	// TODO!!! - check if this ID is already being used, if it is, then use a different one

	// Send over socket
	reqDat, err := rpcSerdes.Marshal(rpcReq)
	if err != nil { return resp, err }

	// Make a channel to wait for a response on this Id
	// TODO - you need to clean this up on any error
	respChan := make(chan Response)
	c.client.activeCalls[rpcReq.Id] = respChan

	// TODO - retry sending? Or push to a queue to be batch sent?
	err = c.client.sock.Write(reqDat)
	if err != nil { return resp, err }

	select {
	case rpcResp := <-respChan:
		anyResp, err := c.client.UnmakeResponse(rpcResp)
		if err != nil { return resp, err }
		resp, ok := anyResp.(Resp)
		if !ok { panic("Mismatched type!") }
		return resp, nil
	case <-time.After(c.timeout):
		// TODO - I need to cleanup the channel here
		return resp, ErrTimeout
	}
}

func (c *Client[S, C]) doRpc(req any, timeout time.Duration) (any, error) {
	rpcReq, err := c.MakeRequest(req)
	if err != nil { return nil, err }

	// TODO!!! - check if this ID is already being used, if it is, then use a different one

	// Send over socket
	reqDat, err := rpcSerdes.Marshal(rpcReq)
	if err != nil { return nil, err }

	// Make a channel to wait for a response on this Id
	// TODO - you need to clean this up on any error
	respChan := make(chan Response)
	c.activeCalls[rpcReq.Id] = respChan

	// TODO - retry sending? Or push to a queue to be batch sent?
	err = c.sock.Write(reqDat)
	if err != nil {
		// TODO - snuffing underlying error because if we coudln't send it means we are trying to reconnect.
		return nil, ErrDisconnected
	}

	select {
	case rpcResp := <-respChan:
		return c.UnmakeResponse(rpcResp)
	case <-time.After(timeout):
		// TODO - I need to cleanup the channel here
		return nil, ErrTimeout
	}
}

func (c *Client[S, C]) doMsg(msg any) error {
	rpcMsg, err := c.MakeMessage(msg)
	if err != nil { return err }

	// Send over socket
	msgDat, err := rpcSerdes.Marshal(rpcMsg)
	if err != nil { return err }

	err = c.sock.Write(msgDat)
	if err != nil {
		// TODO - snuffing underlying error because if we coudln't send it means we are trying to reconnect.
		return ErrDisconnected
	}

	return nil
}

func (c *Client[S, C]) MakeRequest(req any) (Request, error) {
// func (c *Call[S, C, Req, Resp]) Make(req any) (Request, error) {
	dat, err := c.clientDef.Requests.Serialize(req)

	return Request{
		Id: c.rng.Uint32(),
		Data: dat,
	}, err
}

func (c *Client[S, C]) UnmakeResponse(rpcResp Response) (any, error) {
	anyResp, err := c.clientDef.Responses.Deserialize(rpcResp.Data)
	// var resp Resp
	// if err != nil { return resp, err }
	return anyResp, err
	// resp, ok := anyResp.(Resp)
	// if !ok { panic("Mismatched type!") }
	// return resp, err
}

func NewMessage[S, C, A any](client *Client[S, C], rpc MsgDef[A]) *Msg[S, C, A] {
	return &Msg[S, C, A]{
		client: client,
	}
}

// func NewMessage[S, C, A any](client *Client[S, C]) *Msg[S, C, A] {
// 	return &Msg[S, C, A]{
// 		client: client,
// 	}
// }
type Msg[S, C, A any] struct {
	client *Client[S, C]
}
func (m *Msg[S, C, A]) Send(req A) error {
	rpcMsg, err := m.Make(req)
	if err != nil { return err }

	// Send over socket
	reqDat, err := rpcSerdes.Marshal(rpcMsg)
	if err != nil { return err }

	err = m.client.sock.Write(reqDat)
	if err != nil { return err }

	return nil
}

func (m *Msg[S, C, A]) Make(req A) (Message, error) {
	dat, err := m.client.clientDef.Requests.Serialize(req)

	return Message{
		Data: dat,
	}, err
}

func (c *Client[S, C]) MakeMessage(msg any) (Message, error) {
	dat, err := c.clientDef.Requests.Serialize(msg)

	return Message{
		Data: dat,
	}, err
}
