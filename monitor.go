package monitor

import (
	"context"
	"sync"
	"time"

	"github.com/AdamSLevy/jsonrpc2/v14"
)

// Interval specifies the minimum time spent between API requests
var Interval time.Duration = time.Second

// Timeout specifies the maximum time an API request can take
var Timeout time.Duration = time.Second * 5

// Monitor is responsible for polling the factom node and managing listeners
type Monitor struct {
	url    string
	client *jsonrpc2.Client

	heightMtx sync.Mutex
	height    int64
	dbheight  int64
	minute    int64

	listenerMtx       sync.Mutex
	minuteListeners   []chan Event
	heightListeners   []chan int64
	dbheightListeners []chan int64
	errorListeners    []chan error

	close  chan interface{}
	closer sync.Once
}

// Event contains the data sent to minute listeners.
type Event struct {
	// The most recent block saved in the node's database
	DBHeight int64
	// The most recently completed block in the network
	Height int64
	// The minute the network is currently working on
	Minute int64
}

// NewMonitor creates a new monitor that begins polling the provided url immediately.
// If the initial request does not work, an error is returned.
// Starts a goroutine that can be stopped via monitor.Stop().
func NewMonitor(url string) (*Monitor, error) {
	m := new(Monitor)
	m.url = url

	m.client = new(jsonrpc2.Client)

	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()
	response, err := m.FactomdRequest(ctx)
	if err != nil {
		return nil, err
	}

	m.height = response.LeaderHeight
	m.minute = response.Minute
	m.dbheight = response.DBHeight

	m.close = make(chan interface{})

	go m.run(response)
	return m, nil
}

// GetCurrentMinute returns the most recent Height and Minute the monitor has received
func (m *Monitor) GetCurrentMinute() (int64, int64, int64) {
	m.heightMtx.Lock()
	defer m.heightMtx.Unlock()
	return m.height, m.dbheight, m.minute
}

// NewMinuteListener spawns a new listener that receives events for every minute.
// Each reader must have its own listener.
func (m *Monitor) NewMinuteListener() <-chan Event {
	m.listenerMtx.Lock()
	defer m.listenerMtx.Unlock()
	l := make(chan Event, 25)
	m.minuteListeners = append(m.minuteListeners, l)
	return l
}

// NewHeightListener spawns a new listener that receives events every time a new height is attained.
// Each reader must have its own listener.
func (m *Monitor) NewHeightListener() <-chan int64 {
	m.listenerMtx.Lock()
	defer m.listenerMtx.Unlock()
	l := make(chan int64, 6)
	m.heightListeners = append(m.heightListeners, l)
	return l
}

// NewDBHeightListener spawns a new listener that receives events every time a new DBHeight is attained.
// Each reader must have its own listener.
func (m *Monitor) NewDBHeightListener() <-chan int64 {
	m.listenerMtx.Lock()
	defer m.listenerMtx.Unlock()
	l := make(chan int64, 6)
	m.dbheightListeners = append(m.dbheightListeners, l)
	return l
}

// NewErrorListener spawns a new listener that receives error events from malfunctioning API requests.
// Single errors are usually recoverable and the monitor will continue to poll.
// A high frequency of errors means the monitor is unable to reach the node.
// Each reader must have its own listener.
func (m *Monitor) NewErrorListener() <-chan error {
	m.listenerMtx.Lock()
	defer m.listenerMtx.Unlock()
	l := make(chan error, 6)
	m.errorListeners = append(m.errorListeners, l)
	return l
}

func (m *Monitor) run(resp *MinuteResponse) {
	minute := time.Duration(resp.DBlockSeconds) * time.Second / 10
	ticker := time.NewTicker(Interval)
	last := time.Now()

	for {
		select {
		case <-m.close:
			return
		case <-ticker.C:
		}

		ctx, cancel := context.WithTimeout(context.Background(), Timeout)
		resp, err := m.FactomdRequest(ctx)
		if err != nil {
			m.notifyError(err)
			cancel()
			continue
		}
		cancel()

		if m.newHeight(resp) { // sends out event
			diff := minute - time.Since(last)
			if diff < 0 { // absolute value
				diff = -diff
			}
			last = time.Now()
			if diff < Interval {
				select {
				case <-m.close:
					return
				case <-time.After(minute - Interval): // return a little early
				}
			}
		}
	}
}

// returns true if a new height was reached and sends out event
func (m *Monitor) newHeight(resp *MinuteResponse) bool {
	// occasionally the node will return a minute 10 event but that's just an internal state, not a real minute
	// height n minute 10 will be treated as height n minute 0, ie outdated
	resp.Minute %= 10
	if resp.LeaderHeight > m.height || (resp.LeaderHeight == m.height && resp.Minute > m.minute) {
		newHeight := resp.LeaderHeight > m.height
		newDBHeight := resp.DBHeight > m.dbheight
		m.heightMtx.Lock()
		m.height = resp.LeaderHeight
		m.minute = resp.Minute
		m.dbheight = resp.DBHeight
		m.heightMtx.Unlock()

		var e Event
		e.DBHeight = resp.DBHeight
		e.Height = resp.LeaderHeight
		e.Minute = resp.Minute

		m.notify(e, newHeight, newDBHeight)
		return true
	}

	return false
}

// notify all listeners of a new event
func (m *Monitor) notify(e Event, height, dbheight bool) {
	m.listenerMtx.Lock()
	defer m.listenerMtx.Unlock()

	if height {
		for _, l := range m.heightListeners {
			select {
			case l <- e.Height: // only int64
			default:
			}
		}
	}

	if dbheight {
		for _, l := range m.dbheightListeners {
			select {
			case l <- e.DBHeight: // only int64
			default:
			}
		}
	}

	for _, l := range m.minuteListeners {
		select {
		case l <- e:
		default:
		}
	}
}

func (m *Monitor) notifyError(err error) {
	m.listenerMtx.Lock()
	defer m.listenerMtx.Unlock()
	for _, l := range m.errorListeners {
		select {
		case l <- err:
		default:
		}
	}
}

// FactomdRequest sends a "current-minute" API request to the configured node.
func (m *Monitor) FactomdRequest(ctx context.Context) (*MinuteResponse, error) {
	res := new(MinuteResponse)
	if err := m.client.Request(ctx, m.url, "current-minute", nil, res); err != nil {
		return nil, err
	}
	return res, nil
}

// Stop will shut down the monitor and halt all polling.
// A monitor that has been stopped cannot be started again.
func (m *Monitor) Stop() {
	m.closer.Do(func() {
		close(m.close)
	})
}
