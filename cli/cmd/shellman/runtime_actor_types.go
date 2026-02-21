package main

import "shellman/cli/internal/protocol"

type connOutboundMessage struct {
	ConnID  string
	Message protocol.Message
}
