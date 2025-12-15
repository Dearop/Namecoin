export GLOG = warn
export BINLOG = warn
export HTTPLOG = warn
export GORACE = halt_on_error=1
export GNODES = 10
# how many benchmark iterations to average on
export BENCHTIME = "10x"

all: lint vet test

test: test_hw0 test_hw1 test_hw2 test_hw3

test_hw0: test_unit_hw0 test_int_hw0
test_hw1: test_unit_hw1 test_int_hw1
test_hw2: test_unit_hw2 test_int_hw2
test_hw3: test_unit_hw3 test_int_hw3

test_unit_hw0:
	go test -timeout 2m -v -race -run Test_HW0 ./peer/tests/unit

test_unit_hw1:
	go test -timeout 2m -v -race -run Test_HW1 ./peer/tests/unit

test_unit_hw2:
	go test -timeout 2m -v -race -run Test_HW2 ./peer/tests/unit

test_unit_hw3:
	go test -timeout 2m -v -race -run Test_HW3 ./peer/tests/unit

test_unit_POW:
	go test -timeout 2m -v -race -run 'Test(MineNonce|CheckWork)' ./peer/tests/unit

test_unit_transaction_service:
	go test -timeout 2m -v -race -run TestTransactionService ./peer/tests/unit

test_unit_wallet_manager:
	go test -timeout 2m -v -race -run TestTokenWalletManager ./peer/tests/unit

test_unit_namecoin_chain_service:
	go test -timeout 2m -v -race -run 'Test_NamecoinChainService' ./peer/tests/unit

test_unit_namecoin_resolver:
	go test -timeout 2m -v -race -run 'TestNamecoinDNS' ./peer/tests/unit

test_unit_namecoin_expiry:
	go test -timeout 2m -v -race -run 'TestNamecoinExpiry' ./peer/tests/unit

test_int_hw0:
	go test -timeout 5m -v -race -run Test_HW0 ./peer/tests/integration

test_int_hw1:
	go test -timeout 5m -v -race -run Test_HW1 ./peer/tests/integration

test_int_hw2:
	go test -timeout 5m -v -race -run Test_HW2 ./peer/tests/integration

test_int_hw3:
	go test -timeout 5m -v -race -run Test_HW3 ./peer/tests/integration

test_int_namecoin_dns:
	go test -timeout 5m -v -race -run TestNamecoinDNSServer ./peer/tests/integration

test_int_node_dns:
	go test -timeout 5m -v -race -run NodeDNS ./peer/tests/integration

test_int_namecoin_expiry:
	go test -timeout 5m -v -race -run TestNamecoinExpiryResolver ./peer/tests/integration

test_int_namecoin_longest_chain:
	go test -timeout 5m -v -race -run Test_Namecoin_Integration_LongestChain ./peer/tests/integration

# JSONIFY is set to "-json" in CI to format for GitHub, empty for displaying locally
# || true allows to ignore error code and allow for smoother output logging
test_bench_hw1:
	@GLOG=no go test -v ${JSONIFY} -timeout 15m -run Test_HW1 -v -count 1 --tags=performance -benchtime=${BENCHTIME} ./peer/tests/perf/ || true

test_bench_hw2:
	@GLOG=no go test -v ${JSONIFY} -timeout 21m -run Test_HW2 -v -count 1 --tags=performance -benchtime=3x ./peer/tests/perf/ || true

test_bench_hw3: test_bench_hw3_tlc test_bench_hw3_consensus

test_bench_hw3_tlc:
	@GLOG=no go test -v ${JSONIFY} -timeout 15s -run Test_HW3_BenchmarkTLC -v -count 1 --tags=performance -benchtime=${BENCHTIME} ./peer/tests/perf/ || true

test_bench_hw3_consensus:
	@GLOG=no go test -v ${JSONIFY} -timeout 12m -run Test_HW3_BenchmarkConsensus -v -count 1 --tags=performance -benchtime=${BENCHTIME} ./peer/tests/perf/ || true


lint:
	# Coding style static check.
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.60.1
	@go mod tidy
	golangci-lint run

vet:
	go vet ./...

.PHONY: frontend test_frontend test_frontend_coverage clean clean_frontend run

# Default proxy and node addresses for development
PROXYADDR ?= 127.0.0.1:8080
NODEADDR ?= 127.0.0.1:9000
FRONTENDPORT ?= 5173

# Run both backend and frontend together
run:
	@echo "=========================================="
	@echo "Starting Peerster System"
	@echo "=========================================="
	@echo "Backend: http://$(PROXYADDR)"
	@echo "Frontend: http://localhost:$(FRONTENDPORT)"
	@echo "=========================================="
	@echo ""
	@trap 'kill 0' SIGINT; \
		(cd gui && echo "\033[34m[BACKEND]\033[0m Starting..." && go run gui.go start --proxyaddr $(PROXYADDR) --nodeaddr $(NODEADDR) 2>&1 | sed 's/^/\x1b[34m[BACKEND]\x1b[0m /') & \
		(cd frontend && npm install --silent && echo "\033[32m[FRONTEND]\033[0m Starting on http://localhost:$(FRONTENDPORT)..." && VITE_BACKEND_URL=http://$(PROXYADDR) npm run dev -- --port $(FRONTENDPORT) 2>&1 | sed 's/^/\x1b[32m[FRONTEND]\x1b[0m /') & \
		wait

# Run two nodes with frontends for NameCoin testing
run_two_nodes:
	@echo "=========================================="
	@echo "Starting Two NameCoin Nodes"
	@echo "=========================================="
	@echo "Node 1 - Backend: http://127.0.0.1:8080"
	@echo "Node 1 - Frontend: http://localhost:5173"
	@echo ""
	@echo "Node 2 - Backend: http://127.0.0.1:8081"
	@echo "Node 2 - Frontend: http://localhost:5174"
	@echo "=========================================="
	@echo "Press Ctrl+C to stop all nodes"
	@echo "=========================================="
	@echo ""
	@echo "\033[36m[SETUP]\033[0m Cleaning up old blockchain data..."
	@rm -rf /tmp/node1 /tmp/node2
	@echo "\033[36m[SETUP]\033[0m Fresh start - both nodes will share the same blockchain"
	@echo ""
	@cleanup() { \
		echo ""; \
		echo "\033[31m[SHUTDOWN]\033[0m Stopping all processes..."; \
		pkill -P $$$$ 2>/dev/null || true; \
		kill 0 2>/dev/null || true; \
		sleep 1; \
		echo "\033[31m[SHUTDOWN]\033[0m All nodes stopped"; \
		exit 0; \
	}; \
	trap cleanup SIGINT SIGTERM; \
	(cd gui && echo "\033[34m[NODE1-BACKEND]\033[0m Starting..." && go run gui.go start --proxyaddr 127.0.0.1:8080 --nodeaddr 127.0.0.1:9000 --storagefolder /tmp/node1 --antientropy 10s --heartbeat 5s 2>&1 | sed 's/^/\x1b[34m[NODE1-BACKEND]\x1b[0m /') & \
	(cd frontend && npm install --silent && echo "\033[32m[NODE1-FRONTEND]\033[0m Starting on http://localhost:5173..." && VITE_BACKEND_URL=http://127.0.0.1:8080 npm run dev -- --port 5173 2>&1 | sed 's/^/\x1b[32m[NODE1-FRONTEND]\x1b[0m /') & \
	(cd gui && sleep 2 && echo "\033[35m[NODE2-BACKEND]\033[0m Starting..." && go run gui.go start --proxyaddr 127.0.0.1:8081 --nodeaddr 127.0.0.1:9001 --storagefolder /tmp/node2 --antientropy 10s --heartbeat 5s 2>&1 | sed 's/^/\x1b[35m[NODE2-BACKEND]\x1b[0m /') & \
	(cd frontend && sleep 3 && echo "\033[33m[NODE2-FRONTEND]\033[0m Starting on http://localhost:5174..." && VITE_BACKEND_URL=http://127.0.0.1:8081 npm run dev -- --port 5174 2>&1 | sed 's/^/\x1b[33m[NODE2-FRONTEND]\x1b[0m /') & \
	(sleep 8 && echo "\033[36m[CONNECTOR]\033[0m Connecting nodes..." && \
		curl -s -X POST http://127.0.0.1:8080/messaging/peers -H "Content-Type: application/json" -d '["127.0.0.1:9001"]' > /dev/null && \
		echo "\033[36m[CONNECTOR]\033[0m Node 1 added Node 2 as peer" && \
		curl -s -X POST http://127.0.0.1:8081/messaging/peers -H "Content-Type: application/json" -d '["127.0.0.1:9000"]' > /dev/null && \
		echo "\033[36m[CONNECTOR]\033[0m Node 2 added Node 1 as peer" && \
		echo "\033[36m[CONNECTOR]\033[0m ✓ Nodes are now connected on the same blockchain!") & \
	wait

frontend:
	cd frontend && npm install && npm run dev

test_frontend:
	cd frontend && \
		npm install && \
		npm run test -- --coverage=false

test_frontend_coverage:
	cd frontend && \
		npm install && \
		npm run test -- --coverage=true

clean_frontend:
	rm -rf frontend/coverage
	rm -rf frontend/node_modules
	rm -rf frontend/dist

clean: clean_frontend
