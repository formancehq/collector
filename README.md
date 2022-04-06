# organization-collector

This repository implements two benthos components.
One input, and one output, each named "numary_collector".

The input is a http client whereas the output is a http server.

The output is able to fetch a token from the Numary auth server given a m2m token (got from the dashboard).
It keeps the token and refresh it when needed.
It also adds a header 'Organization' (from env var ORGANIZATION).

The input is in charge of validating the token, and checking access (using Organization header).
It also keep a cache of validated (or rejected) tokens to avoid repetitive checks.

## Input

```
input:
  numary_collector:
    introspect_url: "" # Auth server introspect url, following https://datatracker.ietf.org/doc/html/rfc7662
    address: 0.0.0.0:4196 # Listening address
    cache:
      ttl: 1m # Delay when the bearer will be evicted from cache
      num_counter: 10000 # From the doc of the caching library (https://github.com/dgraph-io/ristretto), this value five good performance if 10*max_cost 
      max_cost: 1000 # Allow to keep 1000 bearer tokens in cache, each bearer having a cost of 1
```

# Output

```
output:
  numary_collector:
    url: "" # Url of the collector
    organization: ${ORGANIZATION} # Organization to inject into headers
    auth: 
      url: ${AUTH_URL} # Auth server url where to exchange m2m tokens againt access token
      token: ${AUTH_TOKEN} # M2M token
```