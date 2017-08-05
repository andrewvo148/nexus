package server

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gammazero/nexus/logger"
	"github.com/gammazero/nexus/router"
	"github.com/gammazero/nexus/transport"
	"github.com/gammazero/nexus/transport/serialize"
	"github.com/gorilla/websocket"
)

const (
	jsonWebsocketProtocol    = "wamp.2.json"
	msgpackWebsocketProtocol = "wamp.2.msgpack"
)

type protocol struct {
	payloadType int
	serializer  serialize.Serializer
}

var log logger.Logger

// WebsocketServer handles websocket connections.
type WebsocketServer struct {
	router.Router
	Upgrader *websocket.Upgrader

	protocols map[string]protocol

	// The serializer to use for text frames. Defaults to JSONSerializer.
	TextSerializer serialize.Serializer
	// The serializer to use for binary frames. Defaults to JSONSerializer.
	BinarySerializer serialize.Serializer
}

// NewWebsocketServer takes a router instance and creates a new websocket
// server.
func NewWebsocketServer(r router.Router) *WebsocketServer {
	log = router.Logger()
	s := &WebsocketServer{
		Router:    r,
		protocols: map[string]protocol{},
	}

	s.Upgrader = &websocket.Upgrader{}
	s.AddProtocol(jsonWebsocketProtocol, websocket.TextMessage,
		&serialize.JSONSerializer{})
	s.AddProtocol(msgpackWebsocketProtocol, websocket.BinaryMessage,
		&serialize.MessagePackSerializer{})
	return s
}

// AddProtocol registers a serializer for protocol and payload type.
func (s *WebsocketServer) AddProtocol(proto string, payloadType int, serializer serialize.Serializer) error {
	log.Println("AddProtocol:", proto)
	if payloadType != websocket.TextMessage && payloadType != websocket.BinaryMessage {
		return fmt.Errorf("invalid payload type: %d", payloadType)
	}
	if _, ok := s.protocols[proto]; ok {
		return errors.New("protocol already registered: " + proto)
	}
	s.protocols[proto] = protocol{payloadType, serializer}
	s.Upgrader.Subprotocols = append(s.Upgrader.Subprotocols, proto)
	return nil
}

// ServeHTTP handles HTTP connections.
func (s *WebsocketServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Println("WebsocketServer.ServeHTTP", r.Method, r.RequestURI)
	conn, err := s.Upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("error upgrading to websocket connection:", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.handleWebsocket(conn)
}

func (s *WebsocketServer) handleWebsocket(conn *websocket.Conn) {
	var serializer serialize.Serializer
	var payloadType int
	// Get serializer and payload type for protocol.
	if proto, ok := s.protocols[conn.Subprotocol()]; ok {
		serializer = proto.serializer
		payloadType = proto.payloadType
	} else {
		// Although gorilla rejects connections with unregistered protocols,
		// other websocket implementations may not.
		switch conn.Subprotocol() {
		case jsonWebsocketProtocol:
			serializer = &serialize.JSONSerializer{}
			payloadType = websocket.TextMessage
		case msgpackWebsocketProtocol:
			serializer = &serialize.MessagePackSerializer{}
			payloadType = websocket.BinaryMessage
		default:
			conn.Close()
			return
		}
	}

	// Create a websocket peer from the websocket connection and attach the
	// peer to the router.
	peer := transport.NewWebsocketPeer(conn, serializer, payloadType, log)
	if err := s.Router.Attach(peer); err != nil {
		log.Println(err)
	}
}
