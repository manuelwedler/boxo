#!/bin/bash

TEST_NAME="baseline privacy (first-spy estimator) with latency variation"
RESULTS_DIR="../../../results_latency/baseline-privacy-first-spy/"

run_testground (){
    LATENCY=$1
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
        -tp first_spy=true \
        -tp net_latency=$LATENCY
}

run() {
    echo "running $TEST_NAME evaluation..."
    mkdir -p $RESULTS_DIR
    # latency
    for l in 25 50
    do
        echo "latency=$l"
        id=`run_testground $l | tail -n 2 | awk -F 'run with ID: ' '{ print $2 }'`
        testground collect --runner=docker:go $id
        echo "collected $id"
        tar -xf $id.tgz
        rm $id.tgz
        d=l{$l}
        mv $id $d
        mv $d $RESULTS_DIR
    done
}

run
