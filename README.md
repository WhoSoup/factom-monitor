# Factom Monitor

A library to listen to height events coming from a factom node, either minute events or block events.

## Motivation

I found myself writing similar code over and over again: a polling loop that reacts when the height or minute event in factomd moves. This library aims to be a lightweight implementation of that polling cycle so it can be imported in other projects.

## Polling

The polling algorithm looks at the time between events and will try to minimize requests by looking at the time between the last two successful minute events. If the interval was close to the expected time (59-61 seconds), the library will not poll for a full minute. It will poll at the specified interval otherwise. Unfortunately, since the API uses the internal time of only one node, it's not easy to tell if a 75 second minute means the node lagged behind 15 seconds or the network's minute ran late.

If you need a high precision or non-polling solution, use the [Factomd Live Feed API](https://github.com/FactomProject/factomd/tree/master/events).

## Usage

## Event Object

```go

```

The Event object contains two heights: `Height` and `DBHeight`. `Height` is the most recently completed block in the network, whereas `DBHeight` is the height of the factom node's database. Most of the time, they are the same. During minute 0, `DBHeight` will be `Height - 1`. At the end of minute 0, the factom node verifies all the signatures on the network and save the completely block in the database.

The minutes are not for `Height` but rather the block that is currently being worked on. An Event response with a Height of N and minute M is currently working on minute M of block **N+1**. 

### Importing the Monitor

```go
import (
	monitor "github.com/WhoSoup/factom-monitor"
)
```

### Start and Stop the Monitor
```go
	monitor, err := monitor.NewMonitor("... url of factomd api endpoint ...")
	if err != nil {
		// ... handle error
    }
    
    monitor.Stop()
```

### Listen to Minutes

The listener receives an Event object.

```go
    listener := monitor.NewMinuteListener()
    for event := range listener {
        // process minute event
    }
```

More than one minute listener can be created. Each goroutine should have its own listener, two goroutines cannot read from the same listener.

### Listen to Heights

This listener returns an int64 representing the height of the most recently completed block in the network.

```go
    listener := monitor.NewHeightListener()
    for height := range listener {
        // process minute event
    }
```

More than one height listener can be created. Each goroutine should have its own listener, two goroutines cannot read from the same listener.

## Example

```go
package main

import (
	"fmt"
	"log"

	monitor "github.com/WhoSoup/factom-monitor"
)

func main() {
	// create a new monitor using the open node
	monitor, err := monitor.NewMonitor("https://api.factomd.net/v2")
	if err != nil {
		log.Fatalf("unable to start monitor: %v", err)
	}

	// prints out all errors
	go func() {
		for err := range monitor.NewErrorListener() {
			log.Printf("An error occurred: %v", err)
		}
	}()

	// only listen to new db heights
	go func() {
		height := int64(0)
		for event := range monitor.NewMinuteListener() {
			if event.DBHeight > height {
				fmt.Printf("Height %d is saved in the database\n", event.DBHeight)
				height = event.DBHeight
			}
		}
	}()

	// listen to new heights
	go func() {
		height := int64(0)
		for h := range monitor.NewHeightListener() {
			if h > height {
				fmt.Printf("The network finished block %d and is now working on block %d\n", h, h+1)
				height = h
			}
		}
	}()

	// listen to all minute events
	for event := range monitor.NewMinuteListener() {
		fmt.Printf("The network is working on Block %d Minute %d\n", event.Height+1, event.Minute)
	}
}
```