package tmux

import "strings"

type ControlOutputEvent struct {
	PaneID string
	Data   string
}

func ParseControlOutputLine(line string) (ControlOutputEvent, bool) {
	if strings.HasPrefix(line, "%output ") {
		rest := strings.TrimPrefix(line, "%output ")
		parts := strings.SplitN(rest, " ", 2)
		if len(parts) != 2 {
			return ControlOutputEvent{}, false
		}
		paneID := strings.TrimSpace(parts[0])
		if paneID == "" {
			return ControlOutputEvent{}, false
		}
		return ControlOutputEvent{PaneID: paneID, Data: decodeControlEscaped(parts[1])}, true
	}

	if strings.HasPrefix(line, "%extended-output ") {
		rest := strings.TrimPrefix(line, "%extended-output ")
		sep := strings.Index(rest, " : ")
		if sep < 0 {
			return ControlOutputEvent{}, false
		}
		head := strings.TrimSpace(rest[:sep])
		if head == "" {
			return ControlOutputEvent{}, false
		}
		fields := strings.Fields(head)
		if len(fields) == 0 {
			return ControlOutputEvent{}, false
		}
		paneID := strings.TrimSpace(fields[0])
		if paneID == "" {
			return ControlOutputEvent{}, false
		}
		return ControlOutputEvent{PaneID: paneID, Data: decodeControlEscaped(rest[sep+3:])}, true
	}

	return ControlOutputEvent{}, false
}

func decodeControlEscaped(raw string) string {
	if raw == "" {
		return ""
	}
	if strings.IndexByte(raw, '\\') < 0 {
		return raw
	}
	buf := make([]byte, 0, len(raw))
	for i := 0; i < len(raw); {
		if raw[i] == '\\' && i+3 < len(raw) && isOctal(raw[i+1]) && isOctal(raw[i+2]) && isOctal(raw[i+3]) {
			v := (raw[i+1]-'0')*64 + (raw[i+2]-'0')*8 + (raw[i+3] - '0')
			buf = append(buf, v)
			i += 4
			continue
		}
		buf = append(buf, raw[i])
		i++
	}
	return string(buf)
}

func isOctal(b byte) bool {
	return b >= '0' && b <= '7'
}
