package proxy

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// RESPType represents the type of RESP response
type RESPType byte

const (
	SimpleString RESPType = '+'
	Error        RESPType = '-'
	Integer      RESPType = ':'
	BulkString   RESPType = '$'
	Array        RESPType = '*'
)

// RESPValue represents a parsed RESP value
type RESPValue struct {
	Type  RESPType
	Str   string
	Int   int64
	Array []RESPValue
	Null  bool
}

// RESPReader wraps a bufio.Reader for parsing RESP protocol
type RESPReader struct {
	reader *bufio.Reader
}

// NewRESPReader creates a new RESP reader
func NewRESPReader(r io.Reader) *RESPReader {
	return &RESPReader{
		reader: bufio.NewReader(r),
	}
}

// ReadValue reads and parses a single RESP value
func (r *RESPReader) ReadValue() (*RESPValue, error) {
	typeByte, err := r.reader.ReadByte()
	if err != nil {
		return nil, err
	}

	switch RESPType(typeByte) {
	case SimpleString:
		return r.readSimpleString()
	case Error:
		return r.readError()
	case Integer:
		return r.readInteger()
	case BulkString:
		return r.readBulkString()
	case Array:
		return r.readArray()
	default:
		return nil, fmt.Errorf("unknown RESP type: %c", typeByte)
	}
}

// readSimpleString reads a simple string (+OK\r\n)
func (r *RESPReader) readSimpleString() (*RESPValue, error) {
	line, err := r.readLine()
	if err != nil {
		return nil, err
	}
	return &RESPValue{Type: SimpleString, Str: line}, nil
}

// readError reads an error (-ERR message\r\n or -MOVED slot ip:port\r\n)
func (r *RESPReader) readError() (*RESPValue, error) {
	line, err := r.readLine()
	if err != nil {
		return nil, err
	}
	return &RESPValue{Type: Error, Str: line}, nil
}

// readInteger reads an integer (:1000\r\n)
func (r *RESPReader) readInteger() (*RESPValue, error) {
	line, err := r.readLine()
	if err != nil {
		return nil, err
	}
	num, err := strconv.ParseInt(line, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid integer: %s", line)
	}
	return &RESPValue{Type: Integer, Int: num}, nil
}

// readBulkString reads a bulk string ($6\r\nfoobar\r\n)
func (r *RESPReader) readBulkString() (*RESPValue, error) {
	line, err := r.readLine()
	if err != nil {
		return nil, err
	}

	size, err := strconv.Atoi(line)
	if err != nil {
		return nil, fmt.Errorf("invalid bulk string size: %s", line)
	}

	// Handle null bulk string ($-1\r\n)
	if size < 0 {
		return &RESPValue{Type: BulkString, Null: true}, nil
	}

	// Read the string data plus \r\n
	buf := make([]byte, size+2)
	if _, err := io.ReadFull(r.reader, buf); err != nil {
		return nil, err
	}

	// Verify \r\n terminator
	if buf[size] != '\r' || buf[size+1] != '\n' {
		return nil, fmt.Errorf("invalid bulk string terminator")
	}

	return &RESPValue{Type: BulkString, Str: string(buf[:size])}, nil
}

// readArray reads an array (*2\r\n$3\r\nfoo\r\n$3\r\nbar\r\n)
func (r *RESPReader) readArray() (*RESPValue, error) {
	line, err := r.readLine()
	if err != nil {
		return nil, err
	}

	count, err := strconv.Atoi(line)
	if err != nil {
		return nil, fmt.Errorf("invalid array count: %s", line)
	}

	// Handle null array (*-1\r\n)
	if count < 0 {
		return &RESPValue{Type: Array, Null: true}, nil
	}

	arr := make([]RESPValue, count)
	for i := 0; i < count; i++ {
		val, err := r.ReadValue()
		if err != nil {
			return nil, err
		}
		arr[i] = *val
	}

	return &RESPValue{Type: Array, Array: arr}, nil
}

// readLine reads a line until \r\n
func (r *RESPReader) readLine() (string, error) {
	line, err := r.reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	// Remove \r\n
	if len(line) < 2 || line[len(line)-2] != '\r' {
		return "", fmt.Errorf("invalid line terminator")
	}
	return line[:len(line)-2], nil
}

// Serialize converts a RESPValue back to wire format
func (v *RESPValue) Serialize() []byte {
	var buf bytes.Buffer

	switch v.Type {
	case SimpleString:
		buf.WriteByte('+')
		buf.WriteString(v.Str)
		buf.WriteString("\r\n")

	case Error:
		buf.WriteByte('-')
		buf.WriteString(v.Str)
		buf.WriteString("\r\n")

	case Integer:
		buf.WriteByte(':')
		buf.WriteString(strconv.FormatInt(v.Int, 10))
		buf.WriteString("\r\n")

	case BulkString:
		buf.WriteByte('$')
		if v.Null {
			buf.WriteString("-1\r\n")
		} else {
			buf.WriteString(strconv.Itoa(len(v.Str)))
			buf.WriteString("\r\n")
			buf.WriteString(v.Str)
			buf.WriteString("\r\n")
		}

	case Array:
		buf.WriteByte('*')
		if v.Null {
			buf.WriteString("-1\r\n")
		} else {
			buf.WriteString(strconv.Itoa(len(v.Array)))
			buf.WriteString("\r\n")
			for _, elem := range v.Array {
				buf.Write(elem.Serialize())
			}
		}
	}

	return buf.Bytes()
}

// IsRedirectError checks if this is a MOVED or ASK error
func (v *RESPValue) IsRedirectError() bool {
	if v.Type != Error {
		return false
	}
	return strings.HasPrefix(v.Str, "MOVED ") || strings.HasPrefix(v.Str, "ASK ")
}

// RewriteRedirectError rewrites a MOVED or ASK error to use a different address
// Input format: "MOVED 3999 10.128.0.5:6379" or "ASK 3999 10.128.0.5:6379"
// Output format: "MOVED 3999 127.0.0.1:6381" or "ASK 3999 127.0.0.1:6381"
func (v *RESPValue) RewriteRedirectError(nodeMap map[string]string) bool {
	if !v.IsRedirectError() {
		return false
	}

	// Parse the error message
	parts := strings.Fields(v.Str)
	if len(parts) != 3 {
		return false
	}

	redirectType := parts[0] // "MOVED" or "ASK"
	slot := parts[1]         // slot number
	targetAddr := parts[2]   // "ip:port"

	// Look up the local address for this remote address
	localAddr, found := nodeMap[targetAddr]
	if !found {
		return false
	}

	// Rewrite the error message
	v.Str = fmt.Sprintf("%s %s %s", redirectType, slot, localAddr)
	return true
}
