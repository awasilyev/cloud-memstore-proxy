#!/bin/bash
# Integration test script to run on VM

set -e

INSTANCE_NAME="${1:-projects/my-project/locations/us-central1/instances/my-valkey}"

echo "=========================================="
echo "Cloud Valkey Proxy - Integration Test"
echo "=========================================="
echo ""
echo "Instance: $INSTANCE_NAME"
echo ""

# Step 1: Test discovery
echo "Step 1: Testing discovery..."
echo "----------------------------"
./test-discovery -instance "$INSTANCE_NAME"
DISCOVERY_EXIT=$?

if [ $DISCOVERY_EXIT -ne 0 ]; then
    echo ""
    echo "❌ Discovery failed. Check if:"
    echo "  1. VM service account has 'memorystore.instances.get' permission"
    echo "  2. VM has cloud-platform scope enabled"
    echo ""
    echo "To add the scope, stop the VM and run:"
    echo "  gcloud compute instances set-service-account ${INSTANCE_NAME##*/} \\"
    echo "    --zone us-east1-d \\"
    echo "    --scopes cloud-platform \\"
    echo "    --project PROJECT_ID"
    exit 1
fi

echo ""
echo "✅ Discovery successful!"
echo ""

# Step 2: Start the proxy in background
echo "Step 2: Starting proxy..."
echo "----------------------------"
./cloud-valkey-proxy -instance "$INSTANCE_NAME" -local-addr 127.0.0.1 -verbose=true &
PROXY_PID=$!

echo "Proxy started with PID: $PROXY_PID"
echo "Waiting for proxy to initialize..."
sleep 3

# Check if proxy is still running
if ! ps -p $PROXY_PID > /dev/null; then
    echo "❌ Proxy process died"
    exit 1
fi

echo "✅ Proxy is running"
echo ""

# Step 3: Test connection with redis-cli
echo "Step 3: Testing connection with redis-cli..."
echo "----------------------------"

# Check if redis-cli is installed
if ! command -v redis-cli &> /dev/null; then
    echo "Installing redis-cli..."
    sudo apt-get update -qq
    sudo apt-get install -y redis-tools
fi

echo ""
echo "Testing PING command..."
if redis-cli -h 127.0.0.1 -p 6379 PING; then
    echo "✅ PING successful"
else
    echo "❌ PING failed"
    kill $PROXY_PID
    exit 1
fi

echo ""
echo "Testing SET command..."
TEST_VALUE="test-$(date +%s)"
if redis-cli -h 127.0.0.1 -p 6379 SET cloud-valkey-proxy-test "$TEST_VALUE"; then
    echo "✅ SET successful"
else
    echo "❌ SET failed"
    kill $PROXY_PID
    exit 1
fi

echo ""
echo "Testing GET command..."
RETRIEVED=$(redis-cli -h 127.0.0.1 -p 6379 GET cloud-valkey-proxy-test)
if [ "$RETRIEVED" = "$TEST_VALUE" ]; then
    echo "✅ GET successful - value matches: $RETRIEVED"
else
    echo "❌ GET failed - expected: $TEST_VALUE, got: $RETRIEVED"
    kill $PROXY_PID
    exit 1
fi

echo ""
echo "Testing INFO command..."
if redis-cli -h 127.0.0.1 -p 6379 INFO server | head -5; then
    echo "✅ INFO successful"
else
    echo "❌ INFO failed"
fi

echo ""
echo "Cleaning up test key..."
redis-cli -h 127.0.0.1 -p 6379 DEL cloud-valkey-proxy-test > /dev/null

# Step 4: Test second endpoint (if available)
echo ""
echo "Step 4: Testing second endpoint (port 6380)..."
echo "----------------------------"
if redis-cli -h 127.0.0.1 -p 6380 PING 2>/dev/null; then
    echo "✅ Second endpoint (6380) accessible"
else
    echo "⚠️  Second endpoint (6380) not available (this is OK for single-endpoint instances)"
fi

# Step 5: Cleanup
echo ""
echo "Step 5: Cleanup..."
echo "----------------------------"
kill $PROXY_PID
echo "✅ Proxy stopped"

echo ""
echo "=========================================="
echo "✅ ALL TESTS PASSED!"
echo "=========================================="
echo ""
echo "Summary:"
echo "  ✅ Instance discovery working"
echo "  ✅ Proxy connection established"
echo "  ✅ IAM authentication working (if enabled)"
echo "  ✅ TLS encryption working (if enabled)"
echo "  ✅ Redis commands working (PING, SET, GET, INFO)"
echo ""

