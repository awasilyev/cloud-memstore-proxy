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
	ID             string
	Address        string // IP:client_port format (e.g., "10.96.0.3:6379")
	Port           int    // Client port
	ClusterBusPort int    // Cluster bus port (used by CLUSTER SLOTS)
	Flags          string // master, replica, myself, etc.
	Role           string // master or replica
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
	if _, err := reader.ReadByte(); err != nil {
		return nil, fmt.Errorf("failed to read trailing \\r: %w", err)
	}
	if _, err := reader.ReadByte(); err != nil {
		return nil, fmt.Errorf("failed to read trailing \\n: %w", err)
	}

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

		logger.Debug(fmt.Sprintf("Raw CLUSTER NODES address field: '%s'", addressField))

		// Parse address field: "ip:cluster_bus_port@client_port" or "ip:cluster_bus_port@client_port,hostname"
		// Format: cluster_bus_port is for cluster communication (CLUSTER SLOTS), client_port is for client connections
		address := addressField
		var clientPort, clusterBusPort int

		// Extract client port (after @) - this is what we use for proxy connections
		if idx := strings.Index(address, "@"); idx != -1 {
			afterAt := address[idx+1:]
			// Remove hostname if present (format: "@cport,hostname")
			if commaIdx := strings.Index(afterAt, ","); commaIdx != -1 {
				afterAt = afterAt[:commaIdx]
			}
			if _, err := fmt.Sscanf(afterAt, "%d", &clientPort); err != nil {
				logger.Debug(fmt.Sprintf("Failed to parse client port from %s: %v", addressField, err))
			}
			address = address[:idx] // Now address is "ip:cluster_bus_port"
		} else {
			// If no @, assume the port is the client port (fallback)
			clientPort = 6379
		}

		// Remove hostname if present in cluster bus address part
		if idx := strings.Index(address, ","); idx != -1 {
			address = address[:idx]
		}

		// Extract cluster bus port from address (this is what CLUSTER SLOTS returns)
		parts := strings.Split(address, ":")
		if len(parts) == 2 {
			if _, err := fmt.Sscanf(parts[1], "%d", &clusterBusPort); err != nil {
				logger.Debug(fmt.Sprintf("Failed to parse cluster bus port from %s: %v", address, err))
				continue
			}
		}

		// Determine role
		role := "replica"
		if strings.Contains(flags, "master") {
			role = "master"
		}

		// Create client address for proxy connections
		clientAddr := fmt.Sprintf("%s:%d", parts[0], clientPort)

		node := ClusterNode{
			ID:             nodeID,
			Address:        clientAddr,
			Port:           clientPort,
			ClusterBusPort: clusterBusPort,
			Flags:          flags,
			Role:           role,
		}

		logger.Debug(fmt.Sprintf("Parsed cluster node: ID=%s, Address=%s, Port=%d, ClusterBusPort=%d, Role=%s",
			node.ID, node.Address, node.Port, node.ClusterBusPort, node.Role))

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
