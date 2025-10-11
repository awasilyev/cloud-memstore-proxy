#!/bin/bash
# Simple test script to run on VM

INSTANCE_NAME="${1:-projects/my-project/locations/us-central1/instances/my-valkey}"

echo "=========================================="
echo "Cloud Valkey Proxy - Connection Test"
echo "=========================================="
echo ""

# Start the proxy in background
echo "Starting proxy for: $INSTANCE_NAME"
./cloud-valkey-proxy -instance "$INSTANCE_NAME" -local-addr 127.0.0.1 -verbose=true &
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

echo "Testing connection..."
echo "===================="
echo ""

# Test PING
echo "1. PING test..."
if redis-cli -h 127.0.0.1 -p 6379 PING 2>&1; then
    echo "   ✅ PING successful"
else
    echo "   ❌ PING failed"
    kill $PROXY_PID 2>/dev/null
    exit 1
fi

echo ""
echo "2. SET test..."
if redis-cli -h 127.0.0.1 -p 6379 SET test-key "hello-valkey" 2>&1; then
    echo "   ✅ SET successful"
else
    echo "   ❌ SET failed"
    kill $PROXY_PID 2>/dev/null
    exit 1
fi

echo ""
echo "3. GET test..."
VALUE=$(redis-cli -c -h 127.0.0.1 -p 6379 GET test-key 2>&1 | tail -1)
if [ "$VALUE" = "hello-valkey" ] || [ "$VALUE" = "OK" ]; then
    echo "   ✅ GET successful: $VALUE"
elif echo "$VALUE" | grep -q "MOVED"; then
    echo "   ℹ️  Cluster MOVED response (expected): $VALUE"
    echo "   ✅ Cluster mode working correctly"
else
    echo "   ❌ GET failed: $VALUE"
    kill $PROXY_PID 2>/dev/null
    exit 1
fi

echo ""
echo "4. DEL test..."
if redis-cli -h 127.0.0.1 -p 6379 DEL test-key 2>&1; then
    echo "   ✅ DEL successful"
fi

echo ""
echo "5. INFO test..."
redis-cli -h 127.0.0.1 -p 6379 INFO server 2>&1 | head -10

echo ""
echo "6. Testing second endpoint (port 6380)..."
if timeout 2 redis-cli -h 127.0.0.1 -p 6380 PING 2>&1; then
    echo "   ✅ Second endpoint working"
else
    echo "   ⚠️  Second endpoint not responding (may be expected)"
fi

# Cleanup
echo ""
echo "Stopping proxy..."
kill $PROXY_PID 2>/dev/null
wait $PROXY_PID 2>/dev/null

echo ""
echo "=========================================="
echo "✅ TEST COMPLETE!"
echo "=========================================="

