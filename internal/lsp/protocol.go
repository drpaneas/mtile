// Package lsp serves mTile's Lua dialect over the Language Server Protocol.
package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

const maxMessageSize = 16 << 20

type message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func readFrame(reader *bufio.Reader) ([]byte, error) {
	length := -1
	headerBytes := 0
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF && headerBytes != 0 {
				err = io.ErrUnexpectedEOF
			}
			return nil, err
		}
		headerBytes += len(line)
		if headerBytes > 8192 {
			return nil, fmt.Errorf("LSP header exceeds 8192 bytes")
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			return nil, fmt.Errorf("invalid LSP header")
		}
		if strings.EqualFold(key, "Content-Length") {
			if length != -1 {
				return nil, fmt.Errorf("duplicate Content-Length")
			}
			length, err = strconv.Atoi(strings.TrimSpace(value))
			if err != nil || length < 0 || length > maxMessageSize {
				return nil, fmt.Errorf("invalid LSP Content-Length")
			}
		}
	}
	if length < 0 {
		return nil, fmt.Errorf("missing LSP Content-Length")
	}
	data := make([]byte, length)
	_, err := io.ReadFull(reader, data)
	return data, err
}

func writeFrame(writer io.Writer, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(writer, "Content-Length: %d\r\n\r\n", len(data)); err != nil {
		return err
	}
	_, err = writer.Write(data)
	return err
}

type position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type textRange struct {
	Start position `json:"start"`
	End   position `json:"end"`
}

type documentID struct {
	URI string `json:"uri"`
}
