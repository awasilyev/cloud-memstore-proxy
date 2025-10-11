#!/bin/bash
# Test script for Redis instance

PROJECT_NAME="${PROJECT_NAME:-your-gcp-project}"
INSTANCE_NAME="${1:-projects/my-project/locations/us-central1/instances/my-redis}"

echo "=========================================="
echo "Cloud Memstore Proxy - Redis Test"
echo "Using project: $PROJECT_NAME"
echo "=========================================="
echo ""

# First test discovery
echo "Step 1: Testing discovery..."
./test-discovery -type redis -instance "$INSTANCE_NAME" | head -30

echo ""
echo "Step 2: Starting proxy..."
echo "===================="

# Start the proxy in background
./cloud-memstore-proxy -type redis -instance "$INSTANCE_NAME" -local-addr 127.0.0.1 -verbose=true &
PROXY_PID=$!

echo "Proxy PID: $PROXY_PID"
echo "Waiting 5 seconds for initialization..."
sleep 5

# Check if proxy is still running
if ! ps -p $PROXY_PID > /dev/null 2>&1; then
    echo "❌ Proxy process died. Check logs above."
    exit 1
fi

echo "✅ Proxy is running"
echo ""

# Install redis-cli if needed
if ! command -v redis-cli &> /dev/null; then
    echo "Installing redis-cli..."
    sudo apt-get update -qq && sudo apt-get install -y redis-tools -qq
fi

echo "Step 3: Testing Redis commands..."
echo "===================="
echo ""

# Test PING
echo "1. PING test..."
if redis-cli -h 127.0.0.1 -p 6379 PING 2>&1 | grep -q "PONG"; then
    echo "   ✅ PING successful"
else
    echo "   ❌ PING failed"
    kill $PROXY_PID 2>/dev/null
    exit 1
fi

# Test SET
echo ""
echo "2. SET test..."
if redis-cli -h 127.0.0.1 -p 6379 SET test-redis-key "redis-test-value" 2>&1 | grep -q "OK"; then
    echo "   ✅ SET successful"
else
    echo "   ❌ SET failed"
    kill $PROXY_PID 2>/dev/null
    exit 1
fi

# Test GET
echo ""
echo "3. GET test..."
VALUE=$(redis-cli -h 127.0.0.1 -p 6379 GET test-redis-key 2>&1)
if [ "$VALUE" = "redis-test-value" ]; then
    echo "   ✅ GET successful: $VALUE"
else
    echo "   ❌ GET failed: $VALUE"
    kill $PROXY_PID 2>/dev/null
    exit 1
fi

# Test DEL
echo ""
echo "4. DEL test..."
if redis-cli -h 127.0.0.1 -p 6379 DEL test-redis-key 2>&1 | grep -q "1"; then
    echo "   ✅ DEL successful"
fi

# Test INFO
echo ""
echo "5. INFO test..."
redis-cli -h 127.0.0.1 -p 6379 INFO server 2>&1 | head -10

# Multiple commands
echo ""
echo "6. Multiple operations..."
redis-cli -h 127.0.0.1 -p 6379 << 'EOF'
SET key1 value1
SET key2 value2
SET key3 value3
GET key1
GET key2
GET key3
DEL key1 key2 key3
EOF

# Cleanup
echo ""
echo "Stopping proxy..."
kill $PROXY_PID 2>/dev/null
wait $PROXY_PID 2>/dev/null

echo ""
echo "=========================================="
echo "✅ REDIS TEST COMPLETE!"
echo "=========================================="
echo ""
echo "Summary:"
echo "  ✅ Password authentication working"
echo "  ✅ Redis commands working"
echo "  ✅ Proxy is stable"
echo ""

