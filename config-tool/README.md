# dsc-config

Guided configuration of the FIWARE Data Space Connector chart. Design:
[`doc/config-tool/`](../doc/config-tool/).

## Building

```shell
make build   # bin/dsc-config
make test
make lint
```

## Running

`dsc-config` with no arguments starts a local server, prints its URL and opens it in the
default browser. Ctrl+C stops it. If no browser opens, or the wrong one does, open the
printed URL yourself. `dsc-config --no-browser` only prints the URL.

## Security model

- The server listens on `127.0.0.1` with a random port and answers only for the hosts
  `127.0.0.1:<port>` and `localhost:<port>`.
- The printed URL holds a per-run token that stays valid until the process exits. Do not share
  it. Opening it sets a session cookie; state-changing requests also need the server's own
  `Origin`.
- The token never goes on a command line. The browser is started with a single-use launch code
  that expires after 60 seconds. On a shared host another account could race for that code, so
  use `--no-browser` there and paste the printed URL into the browser.
