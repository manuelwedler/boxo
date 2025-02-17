#!/bin/bash

TEST_NAME="privacy (first-spy estimator) with latency variation"
RESULTS_DIR="../../../results_latency/privacy-first-spy/"

run_testground (){
    FORWARD_DEGREE=$1
    PROXY_PROB=$2
    LATENCY=$3
    testground run single \
        --builder=docker:go \
        --runner=local:docker \
        --plan=rawa-bitswap \
        --testcase=rawa-test \
        --instances=50 \
        --wait \
        -tp run_count=100 \
        -tp conn_per_node=4 \
        -tp unforwarded_search_time=4 \
        -tp proxy_transition_prob=$PROXY_PROB \
        -tp forward_graph_degree=$FORWARD_DEGREE \
        -tp closest_peerid_forward=false \
        -tp first_spy=true \
        -tp net_latency=$LATENCY
}

run() {
    echo "running $TEST_NAME evaluation..."
    mkdir -p $RESULTS_DIR
    # forward_graph_degree
    for n in 0 2 4
    do
        # proxy_transition_prob
        for p in 0.05 0.1 0.2 0.3
        do 
            # latency
            for l in 25 50
            do
                echo "running forward_graph_degree=$n proxy_transition_prob=$p latency=$l"
                id=`run_testground $n $p $l | tail -n 2 | awk -F 'run with ID: ' '{ print $2 }'`
                testground collect --runner=docker:go $id
                echo "collected $id"
                tar -xf $id.tgz
                rm $id.tgz
                d=n{$n}_p{$p}_l{$l}
                mv $id $d
                mv $d $RESULTS_DIR
            done
        done
    done
}

run
