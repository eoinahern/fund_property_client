# fund_property_client
client that collates data from funda.nl in a concurrent manner utilizing go routines, mutexes and blocking semaphores (to limit the number of routines making get calls to the
api at once). implements infinite retry strategy based on the rate limiting implemented in the api (essentially, keep trying to get the property data until the wait period finished.
the go routine will sleep. then retry after 20 seconds).finally, displays data in basic html tablature form.
