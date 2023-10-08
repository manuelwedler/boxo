#!/bin/bash

TEST_NAME="unforwarded search timer"
RESULTS_DIR="results/unforwarded-search-timer/"

run_testground (){
    UNFORWARDED_TIME=$1
    PROXY_PROB=$2
    testground run single \
        --builder=docker:go \
        --runner=local:docker \
        --plan=rawa-bitswap \
        --testcase=rawa-test \
        --instances=50 \
        --wait \
        -tp run_count=100 \
        -tp conn_per_node=4 \ 
        -tp unforwarded_search_time=$UNFORWARDED_TIME \
        -tp proxy_transition_prob=$PROXY_PROB \
        -tp forward_graph_degree=4 \
        -tp closest_peerid_forward=false
}

run() {
    echo "running $TEST_NAME evaluation..."
    mkdir -p $RESULTS_DIR
    # proxy_transition_prob
    for p in 0.1 0.2
    do
        # unforwarded_search_time
        for u in `LANG=en_US seq 2 2 10`
        do 
            echo "running proxy_transition_prob=$p unforwarded_search_time=$u"
            id=`run_testground $u $p | tail -n 2 | awk -F 'run with ID: ' '{ print $2 }'`
            testground collect --runner=docker:go $id
            echo "collected $id"
            tar -xf $id.tgz
            rm $id.tgz
            d=p{$p}_u{$u}
            mv $id $d
            mv $d $RESULTS_DIR
        done
    done
}

run
