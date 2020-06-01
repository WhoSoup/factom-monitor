package monitor

import (
	"context"
	"sync"
	"time"

	"github.com/AdamSLevy/jsonrpc2"
)

var Interval time.Duration = time.Second

type Monitor struct {
	url    string
	client *jsonrpc2.Client

	height int64
	minute int64

	listenerMtx sync.Mutex
	listeners   []chan Event

	close  chan interface{}
	closer sync.Once
}

type Event struct {
	Err          error
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

	m.height = response.DBHeight
	m.minute = response.Minute

	m.close = make(chan interface{})

	go m.run(response)
	return m, nil
}

func (m *Monitor) NewListener() <-chan Event {
	m.listenerMtx.Lock()
	defer m.listenerMtx.Unlock()
	l := make(chan Event, 20)
	m.listeners = append(m.listeners, l)
	return l
}

func (m *Monitor) run(resp *MinuteResponse) {
	for resp != nil {
		// wait for next minute
		defMinute := time.Duration(resp.DBlockSeconds) * time.Second / 10
		minuteStart := time.Unix(resp.MinuteStartTime, 0)
		serverTime := time.Unix(resp.Time, 0)

		wait := serverTime.Add(defMinute).Sub(minuteStart)
		select {
		case <-m.close:
			return
		case <-time.After(wait):
		}

		if resp = m.pollLoop(); resp != nil {
			m.notify(resp)
		}
	}
}

func (m *Monitor) send(event Event) {
	m.listenerMtx.Lock()
	defer m.listenerMtx.Lock()
	for _, l := range m.listeners {
		select {
		case l <- event:
		default: // nobody is reading
		}
	}
}
func (m *Monitor) notify(resp *MinuteResponse) {
	var e Event
	e.DBHeight = resp.DBHeight
	e.Minute = resp.Minute
	m.send(e)
}

func (m *Monitor) notifyErr(err error) {

}

func (m *Monitor) pollLoop() *MinuteResponse {
	for {
		resp := m.poll()
		if resp != nil {
			return resp
		}

		select {
		case <-m.close:
			return nil
		case <-time.After(Interval):
		}
	}
}

func (m *Monitor) poll() *MinuteResponse {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	res, err := m.CurrentMinute(ctx)
	if err != nil {
		m.notifyErr(err)
		return nil
	}
	return res
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
