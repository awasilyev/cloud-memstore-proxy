package proxy

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/awasilyev/cloud-memstore-proxy/pkg/logger"
)

// ClusterNode represents a node in the Redis/Valkey cluster
type ClusterNode struct {
	ID      string
	Address string // IP:port format
	Port    int
	Flags   string // master, slave, myself, etc.
	Role    string // master or slave
}

// DiscoverClusterTopology connects to a cluster node and discovers all cluster members
// Returns a list of all nodes in the cluster
func DiscoverClusterTopology(conn net.Conn) ([]ClusterNode, error) {
	// Send CLUSTER NODES command
	cmd := "*2\r\n$7\r\nCLUSTER\r\n$5\r\nNODES\r\n"

	conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if _, err := conn.Write([]byte(cmd)); err != nil {
		return nil, fmt.Errorf("failed to send CLUSTER NODES command: %w", err)
	}

	// Read response
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	reader := bufio.NewReader(conn)

	// Read first byte to check response type
	typeByte, err := reader.ReadByte()
	if err != nil {
		return nil, fmt.Errorf("failed to read response type: %w", err)
	}

	// If it's an error, this is not a cluster instance
	if typeByte == '-' {
		line, _ := reader.ReadString('\n')
		return nil, fmt.Errorf("not a cluster instance: %s", line)
	}

	// Should be a bulk string ($<length>\r\n<data>\r\n)
	if typeByte != '$' {
		return nil, fmt.Errorf("unexpected response type: %c", typeByte)
	}

	// Read the length line
	lengthLine, err := reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("failed to read length: %w", err)
	}

	var length int
	if _, err := fmt.Sscanf(lengthLine, "%d\r\n", &length); err != nil {
		return nil, fmt.Errorf("invalid length format: %s", lengthLine)
	}

	// Read the actual data
	data := make([]byte, length)
	if _, err := reader.Read(data); err != nil {
		return nil, fmt.Errorf("failed to read data: %w", err)
	}

	// Read trailing \r\n
	reader.ReadByte()
	reader.ReadByte()

	// Clear deadlines
	conn.SetReadDeadline(time.Time{})
	conn.SetWriteDeadline(time.Time{})

	// Parse the cluster nodes output
	return parseClusterNodes(string(data))
}

// parseClusterNodes parses the output of CLUSTER NODES command
// Format: <id> <ip:port@cport[,hostname]> <flags> <master> <ping-sent> <pong-recv> <config-epoch> <link-state> <slot> <slot> ... <slot>
// Example: 07c37dfeb235213a872192d90877d0cd55635b91 127.0.0.1:30004@31004 slave e7d1eecce10fd6bb5eb35b9f99a514335d9ba9ca 0 1426238317239 4 connected
func parseClusterNodes(output string) ([]ClusterNode, error) {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	nodes := make([]ClusterNode, 0, len(lines))

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 8 {
			logger.Debug(fmt.Sprintf("Skipping invalid cluster node line: %s", line))
			continue
		}

		nodeID := fields[0]
		addressField := fields[1]
		flags := fields[2]

		// Parse address field: "ip:port@cport" or "ip:port@cport,hostname"
		// We only care about the "ip:port" part
		address := addressField
		if idx := strings.Index(address, "@"); idx != -1 {
			address = address[:idx]
		}
		if idx := strings.Index(address, ","); idx != -1 {
			address = address[:idx]
		}

		// Extract port from address
		var port int
		parts := strings.Split(address, ":")
		if len(parts) == 2 {
			fmt.Sscanf(parts[1], "%d", &port)
		}

		// Determine role
		role := "slave"
		if strings.Contains(flags, "master") {
			role = "master"
		}

		node := ClusterNode{
			ID:      nodeID,
			Address: address,
			Port:    port,
			Flags:   flags,
			Role:    role,
		}

		nodes = append(nodes, node)
	}

	return nodes, nil
}

// FilterUniqueNodes removes duplicate nodes (by address) and the current node (with "myself" flag)
func FilterUniqueNodes(nodes []ClusterNode, currentAddress string) []ClusterNode {
	seen := make(map[string]bool)
	unique := make([]ClusterNode, 0, len(nodes))

	for _, node := range nodes {
		// Skip the current node (already has a proxy)
		if node.Address == currentAddress {
			continue
		}

		// Skip nodes with "myself" flag
		if strings.Contains(node.Flags, "myself") {
			continue
		}

		// Skip duplicates
		if seen[node.Address] {
			continue
		}

		seen[node.Address] = true
		unique = append(unique, node)
	}

	return unique
}
