package protocol

import (
	"encoding/json"
	"errors"
	"strings"
)

type MuxEnvelope struct {
	ConnID string          `json:"conn_id"`
	Data   json.RawMessage `json:"data"`
}

func WrapMuxEnvelope(connID string, raw []byte) ([]byte, error) {
	connID = strings.TrimSpace(connID)
	if connID == "" {
		return nil, errors.New("missing conn_id")
	}

	env := MuxEnvelope{
		ConnID: connID,
		Data:   raw,
	}
	return json.Marshal(env)
}

func UnwrapMuxEnvelope(raw []byte) (string, []byte, error) {
	var env MuxEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return "", nil, err
	}
	if strings.TrimSpace(env.ConnID) == "" {
		return "", nil, errors.New("missing conn_id")
	}
	return env.ConnID, env.Data, nil
}
