export GLOG = warn
export BINLOG = warn
export HTTPLOG = warn
export GORACE = halt_on_error=1
export GNODES = 10
# how many benchmark iterations to average on
export BENCHTIME = "10x"
export PERF_RUNS ?= 3
export PERF_RESULTS_FILE ?= perf_results/namecoin_perf_results.txt

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

test_perf_namecoin:
	@GLOG=no go test -v ${JSONIFY} -timeout 20m -run 'Test_.*_Perf' -count 1 --tags=performance ./peer/tests/perf/ || true

perf_namecoin_runs:
	@mkdir -p $(dir $(PERF_RESULTS_FILE))
	@echo "### Namecoin perf runs ($$(date -Iseconds)) runs=$(PERF_RUNS)" | tee -a $(PERF_RESULTS_FILE)
	@i=1; \
	while [ $$i -le $(PERF_RUNS) ]; do \
		echo "--- run $$i ---" | tee -a $(PERF_RESULTS_FILE); \
		GLOG=no go test -v ${JSONIFY} -timeout 0 -run 'Test_.*_Perf' -count 1 --tags=performance ./peer/tests/perf/ | tee -a $(PERF_RESULTS_FILE); \
		echo "" >> $(PERF_RESULTS_FILE); \
		i=$$(( $$i + 1 )); \
	done


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

# Run both backend and frontend together
run:
	@echo "=========================================="
	@echo "Starting Peerster System"
	@echo "=========================================="
	@echo "Backend: http://$(PROXYADDR)"
	@echo "Frontend: http://localhost:5173"
	@echo "=========================================="
	@echo ""
	@trap 'kill 0' SIGINT; \
		(cd gui && echo "\033[34m[BACKEND]\033[0m Starting..." && go run gui.go start --proxyaddr $(PROXYADDR) --nodeaddr $(NODEADDR) 2>&1 | sed 's/^/\x1b[34m[BACKEND]\x1b[0m /') & \
		(cd frontend && npm install --silent && echo "\033[32m[FRONTEND]\033[0m Starting on http://localhost:5173..." && VITE_BACKEND_URL=http://$(PROXYADDR) npm run dev 2>&1 | sed 's/^/\x1b[32m[FRONTEND]\x1b[0m /') & \
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
