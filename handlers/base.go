package handlers

// MessageHandler adalah interface dasar untuk semua protobuf message handlers
type MessageHandler interface {
	// Handle memproses raw binary protobuf message
	Handle(data []byte) error

	// GetMessageType mengembalikan tipe message yang di-handle
	GetMessageType() string
}

// ProtoMessageHandler extends MessageHandler for protobuf wrapper messages
type ProtoMessageHandler interface {
	MessageHandler

	// HandleProto processes the protobuf wrapper message
	HandleProto(wrapper interface{}) error
}
