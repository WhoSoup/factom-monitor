package monitor

import (
	"context"
	"sync"

	"github.com/AdamSLevy/jsonrpc2"
)

type Monitor struct {
	conf   Config
	client *jsonrpc2.Client

	height int64
	minute int64

	close  chan interface{}
	closer sync.Once

	cancel context.CancelFunc
}

func NewMonitor(conf *Config) (*Monitor, error) {
	m := new(Monitor)
	m.conf = *conf // can't be changed from outside
	m.client = new(jsonrpc2.Client)

	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	response, err := m.Poll(ctx)
	if err != nil {
		return nil, err
	}

	m.height = response.DBHeight
	m.minute = response.Minute

	m.close = make(chan interface{})

	go m.run()
	return m, nil
}

func (m *Monitor) run() {
	for {
		select {
		case <-m.close:
			return
		default:
		}

		ctx, cancel := context.WithCancel(context.Background())
		m.cancel = cancel
		m.Poll(ctx)

	}
}

func (m *Monitor) Poll(ctx context.Context) (*MinuteResponse, error) {
	res := new(MinuteResponse)
	if err := m.client.Request(ctx, m.conf.FactomdURL, "current-minute", nil, res); err != nil {
		return nil, err
	}
	return res, nil
}

func (m *Monitor) Stop() {
	m.closer.Do(func() {
		close(m.close)
		m.cancel()
	})
}
