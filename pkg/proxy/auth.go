package proxy

import (
	"fmt"
	"net"
	"time"
)

// authenticatePassword performs password-based authentication for Redis instances
func (p *Proxy) authenticatePassword(conn net.Conn, password string) error {
	// Send AUTH command using RESP protocol
	// Format: *2\r\n$4\r\nAUTH\r\n$<length>\r\n<password>\r\n
	authCmd := fmt.Sprintf("*2\r\n$4\r\nAUTH\r\n$%d\r\n%s\r\n", len(password), password)

	// Set write deadline
	conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if _, err := conn.Write([]byte(authCmd)); err != nil {
		return fmt.Errorf("failed to send AUTH command: %w", err)
	}

	// Read response
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	response := make([]byte, 1024)
	n, err := conn.Read(response)
	if err != nil {
		return fmt.Errorf("failed to read AUTH response: %w", err)
	}

	// Check for success response (+OK\r\n)
	respStr := string(response[:n])
	if len(respStr) >= 5 && respStr[:5] == "+OK\r\n" {
		// Clear deadlines after successful auth
		conn.SetReadDeadline(time.Time{})
		conn.SetWriteDeadline(time.Time{})
		return nil
	}

	return fmt.Errorf("authentication failed: %s", respStr)
}
