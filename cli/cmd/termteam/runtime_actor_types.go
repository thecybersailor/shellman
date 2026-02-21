package main

import "termteam/cli/internal/protocol"

type connOutboundMessage struct {
	ConnID  string
	Message protocol.Message
}
