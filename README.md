# Factom Monitor

A library to listen to height events coming from a factom node, either minute events or block events.

## Motivation

I found myself writing similar code over and over again: a polling loop that reacts when the height or minute event in factomd moves. This library aims to be a lightweight implementation of that polling cycle so it can be imported in other projects.

## Polling

The polling algorithm looks at the time between events and will try to minimize requests by looking at the time between the last two successful minute events. If the interval was close to the expected time (59-61 seconds), the library will not poll for a full minute. It will poll at the specified interval otherwise. Unfortunately, since the API uses the internal time of only one node, it's not easy to tell if a 75 second minute means the node lagged behind 15 seconds or the network's minute ran late.

If you need a high precision or non-polling solution, use the [Factomd Live Feed API](https://github.com/FactomProject/factomd/tree/master/events).

## Usage

Import the monitor via

```go
import (
	monitor "github.com/WhoSoup/factom-monitor"
)
```

To create and stop a monitor:
```go
	monitor, err := monitor.NewMonitor("... url of factomd api endpoint ...")
	if err != nil {
		// ... handle error
    }
    
    monitor.Stop()
```




## Heights

There are two heights passed along in
LeaderHeight 