package monitor

import (
	"context"
	"sync"
	"time"

	"github.com/AdamSLevy/jsonrpc2/v14"
)

var Interval time.Duration = time.Second
var Timeout time.Duration = time.Second * 5

type Monitor struct {
	url    string
	client *jsonrpc2.Client

	heightMtx sync.Mutex
	height    int64
	minute    int64

	listenerMtx     sync.Mutex
	minuteListeners []chan Event
	heightListeners []chan int64
	errorListeners  []chan error

	close  chan interface{}
	closer sync.Once
}

type Event struct {
	DBHeight int64
	Height   int64
	Minute   int64
}

func NewMonitor(url string) (*Monitor, error) {
	m := new(Monitor)
	m.url = url

	m.client = new(jsonrpc2.Client)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	response, err := m.FactomdRequest(ctx)
	if err != nil {
		return nil, err
	}

	m.height = response.LeaderHeight
	m.minute = response.Minute

	m.close = make(chan interface{})

	go m.run(response)
	return m, nil
}

func (m *Monitor) GetCurrentMinute() (int64, int64) {
	m.heightMtx.Lock()
	defer m.heightMtx.Unlock()
	return m.height, m.minute
}

func (m *Monitor) NewMinuteListener() <-chan Event {
	m.listenerMtx.Lock()
	defer m.listenerMtx.Unlock()
	l := make(chan Event, 25)
	m.minuteListeners = append(m.minuteListeners, l)
	return l
}

func (m *Monitor) NewHeightListener() <-chan int64 {
	m.listenerMtx.Lock()
	defer m.listenerMtx.Unlock()
	l := make(chan int64, 6)
	m.heightListeners = append(m.heightListeners, l)
	return l
}

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
		newHeight := resp.LeaderHeight > m.height
		m.heightMtx.Lock()
		m.height = resp.LeaderHeight
		m.minute = resp.Minute
		m.heightMtx.Unlock()

		var e Event
		e.DBHeight = resp.DBHeight
		e.Height = resp.LeaderHeight
		e.Minute = resp.Minute

		m.notify(e, newHeight)
		return true
	}
	return false
}

func (m *Monitor) notify(e Event, height bool) {
	m.listenerMtx.Lock()
	defer m.listenerMtx.Unlock()

	if height {
		for _, l := range m.heightListeners {
			select {
			case l <- e.Height:
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

func (m *Monitor) FactomdRequest(ctx context.Context) (*MinuteResponse, error) {
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
