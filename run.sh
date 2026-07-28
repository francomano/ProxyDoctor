#!/bin/bash
# ProxyDoctor Run Script
# Convenience wrapper for running CLI or server

if [ $# -lt 1 ]; then
    echo "Usage: $0 <command> [args...]"
    echo ""
    echo "Commands:"
    echo "  server              Start HTTP server on :8080"
    echo "  cli <args>          Run CLI with arguments"
    echo "  test                Run unit tests"
    echo ""
    echo "Examples:"
    echo "  $0 server"
    echo "  $0 cli --plugins mcp_server"
    echo "  $0 cli diagnose --proxy http://localhost:8080"
    echo "  $0 cli diagnose --plugins route_trace"
    echo "  $0 cli list-checks"
    echo "  $0 test"
    exit 1
fi

CMD=$1
shift

case "$CMD" in
    server)
        echo "🚀 Starting ProxyDoctor server on :8080"
        go build -o ./bin/proxydoctor-server ./cmd/server
        ./bin/proxydoctor-server
        ;;
    cli)
        echo "🔍 Running ProxyDoctor CLI"
        go build -o ./bin/proxydoctor ./cmd/cli
        ./bin/proxydoctor "$@"
        ;;
    test)
        echo "🧪 Running tests..."
        go test -v ./...
        ;;
    *)
        echo "❌ Unknown command: $CMD"
        exit 1
        ;;
esac
