# Privacy-Preserving Content Discovery for Bitswap Using Random Walks

## About

This repository is a fork of boxo, applying the changes of the master's thesis "Privacy-Preserving Content Discovery for Bitswap Using Random Walks".
The relevant code is located under the directories `bitswap` and `testplans`.

## Running the test plan

An installation of  [Testground](https://docs.testground.ai/) v0.6.0 is required.

The test scripts run each configuration 100 times. For this, Testground's task timeout needs to be increased. Please add a timeout of at least 60 minutes to your `$TESTGROUND_HOME/.env.toml`:

```toml
[daemon.scheduler]
task_timeout_min          = 60
```

To run the plan, first start your Testground daemon:

```
testground daemon
```

Import the test plan once. Run in a sepearte terminal:

```bash
cd testplans
testground plan import --from ./rawa-bitswap
```

The plan can be run with default parameters by:

```bash
testground run single \
        --builder=docker:go \
        --runner=local:docker \
        --plan=rawa-bitswap \
        --testcase=rawa-test \
        --instances=50 \
        --wait \
        -tp run_count=1 \
        -tp conn_per_node=4 
```

`testplans/rawa-bitswap/manifest.toml` shows the available parameters that can be configured.
The simulations conducted in the thesis can be run using the shell scripts inside `testplans/rawa-bitswap/scripts`, e.g. `sh performance.sh`.
See them for reference of possible parameter configurations.

The shell scripts produce a `results` directory with the measured data during the simulation.
This data can be plotted using the python scripts, also inside the `scripts` directory.
For running those, please make sure to have at least python v3.10 and the `requirements.txt` installed in a virtual environment.




## License

[SPDX-License-Identifier: Apache-2.0 OR MIT](LICENSE.md)
