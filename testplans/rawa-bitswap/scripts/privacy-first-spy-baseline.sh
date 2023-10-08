#!/bin/bash

TEST_NAME="baseline privacy (first-spy estimator)"
RESULTS_DIR="results/baseline-privacy-first-spy/"

run_testground (){
    testground run single \
        --builder=docker:go \
        --runner=local:docker \
        --plan=rawa-bitswap \
        --testcase=rawa-test \
        --build-cfg modfile="baseline.go.mod" \
        --instances=50 \
        --wait \
        -tp run_count=100 \
        -tp conn_per_node=4 \
        -tp first_spy=true
}

run() {
    echo "running $TEST_NAME evaluation..."
    mkdir -p $RESULTS_DIR
    id=`run_testground | tail -n 2 | awk -F 'run with ID: ' '{ print $2 }'`
    testground collect --runner=docker:go $id
    echo "collected $id"
    tar -xf $id.tgz
    rm $id.tgz
    d=run
    mv $id $d
    mv $d $RESULTS_DIR
}

run
