package monitor

import (
	"context"
	"sync"
	"time"

	"github.com/AdamSLevy/jsonrpc2"
)

var Interval time.Duration = time.Second
var Timeout time.Duration = time.Second * 5

type Monitor struct {
	url    string
	client *jsonrpc2.Client

	height int64
	minute int64

	listener chan Event

	close  chan interface{}
	closer sync.Once
}

type Event struct {
	DBHeight     int64
	LeaderHeight int64
	Minute       int64
}

func NewMonitor(url string) (*Monitor, error) {
	m := new(Monitor)
	m.url = url

	m.client = new(jsonrpc2.Client)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	response, err := m.CurrentMinute(ctx)
	if err != nil {
		return nil, err
	}

	m.height = response.LeaderHeight
	m.minute = response.Minute

	m.close = make(chan interface{})
	m.listener = make(chan Event, 10)

	go m.run(response)
	return m, nil
}

func (m *Monitor) Listener() <-chan Event {
	return m.listener
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
		resp, err := m.CurrentMinute(ctx)
		if err != nil {
			cancel()
			continue
		}
		cancel()

		if m.newHeight(resp) {
			diff := minute - time.Since(last)
			if diff < 0 {
				diff = -diff
			}
			last = time.Now()
			if diff < Interval {
				select {
				case <-m.close:
					return
				case <-time.After(minute - Interval):
				}
			}
		}
	}
}

func (m *Monitor) newHeight(resp *MinuteResponse) bool {
	resp.Minute %= 10
	if resp.LeaderHeight > m.height || (resp.LeaderHeight == m.height && resp.Minute > m.minute) {
		m.height = resp.LeaderHeight
		m.minute = resp.Minute
		m.notify(resp)
		return true
	}
	return false
}

func (m *Monitor) notify(resp *MinuteResponse) {
	var e Event
	e.DBHeight = resp.DBHeight
	e.LeaderHeight = resp.LeaderHeight
	e.Minute = resp.Minute
	select {
	case m.listener <- e:
	default: // nobody is reading
	}
}

func (m *Monitor) CurrentMinute(ctx context.Context) (*MinuteResponse, error) {
	res := new(MinuteResponse)
	if err := m.client.Request(ctx, m.url, "current-minute", nil, res); err != nil {
		return nil, err
	}
	return res, nil
}

func (m *Monitor) Stop() {
	m.closer.Do(func() {
		close(m.close)
	})
}
