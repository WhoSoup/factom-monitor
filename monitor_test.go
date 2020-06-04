package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"
)

type testServer struct {
	height int64
	minute int64
	server *http.Server
	t      *testing.T
	mtx    sync.Mutex

	blockstart  time.Time
	minutestart time.Time
	blocktime   time.Duration

	runner chan interface{}
	once   sync.Once
}

func newTestServer(addr string, height, minute int64, blocktime time.Duration, t *testing.T) *testServer {
	ts := new(testServer)
	ts.t = t
	ts.height = height
	ts.minute = minute
	ts.blockstart = time.Now()
	ts.minutestart = ts.blockstart
	ts.blocktime = blocktime

	mux := http.NewServeMux()
	mux.HandleFunc("/v2", ts.api)

	ts.server = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	ts.runner = make(chan interface{})

	go ts.listen()
	//time.Sleep(time.Millisecond * 50)
	return ts
}

func (ts *testServer) api(rw http.ResponseWriter, r *http.Request) {
	ts.mtx.Lock()
	defer ts.mtx.Unlock()
	resp := new(MinuteResponse)
	resp.LeaderHeight = ts.height
	resp.Minute = ts.minute
	resp.DBHeight = ts.height
	if resp.Minute == 0 {
		resp.DBHeight--
	}
	/*resp.BlockStartTime = ts.blockstart.UnixNano()
	resp.MinuteStartTime = ts.blockstart.UnixNano()
	resp.Time = time.Now().UnixNano()*/
	resp.DBlockSeconds = int64(ts.blocktime.Seconds())

	rpc := make(map[string]interface{})
	rpc["jsonrpc"] = "2.0"
	rpc["id"] = 0
	rpc["result"] = resp

	js, err := json.Marshal(rpc)
	if err != nil {
		ts.t.Error(err)
		return
	}
	fmt.Printf("server json response: %s\n", string(js))

	_, err = rw.Write(js)
	if err != nil {
		ts.t.Error(err)
	}
}

func (ts *testServer) listen() {
	if err := ts.server.ListenAndServe(); err != nil {
		if err != http.ErrServerClosed {
			ts.t.Error(err)
		}
	}
}

func (ts *testServer) run() {
	ticker := time.NewTicker(ts.blocktime / 10)
	for range ticker.C {
		select {
		case <-ts.runner:
			return
		default:
		}

		ts.tick()
	}
}

func (ts *testServer) stop() {
	ts.once.Do(func() {
		ts.server.Shutdown(context.Background())
		close(ts.runner)
	})
}

func (ts *testServer) tick() {
	ts.mtx.Lock()
	ts.minute++
	ts.minutestart = time.Now()
	if ts.minute > 9 {
		ts.height++
		ts.minute = 0
		ts.blockstart = ts.minutestart
	}
	ts.mtx.Unlock()
}

func TestMonitor_GetCurrentMinute(t *testing.T) {
	s := newTestServer("localhost:9888", 10, 5, time.Second*6, t)
	defer s.stop()

	m, err := NewMonitor("http://localhost:9888/v2")
	if err != nil {
		t.Fatal(err)
	}

	hh, mm := m.GetCurrentMinute()
	if hh != 10 || mm != 5 {
		t.Errorf("unexpected results. got = [%d/%d], want = [10/5]", hh, mm)
	}

}

func TestMonitor_Listeners(t *testing.T) {
	minute := time.Second
	s := newTestServer("localhost:9888", 0, 0, minute*10, t)
	//go s.run()
	defer s.stop()

	m, err := NewMonitor("http://localhost:9888/v2")
	if err != nil {
		t.Fatal(err)
	}

	ogi := Interval
	Interval = time.Millisecond * 100

	eventCount := make([]int, 16)

	for i := 0; i < 8; i++ {
		go func(j int) {
			ml := m.NewMinuteListener()
			var mm, hh int64
			for e := range ml {
				eventCount[j]++

				if !((hh == e.Height && mm+1 == e.Minute) || (hh+1 == e.Height && (mm+1)%10 == e.Minute)) {
					t.Errorf("event ouf of sequence for minute listener %d. prev = (%d, %d), got = (%d, %d)", j, hh, mm, e.Height, e.Minute)
				}
				hh = e.Height
				mm = e.Minute
			}
		}(i)
	}
	for i := 8; i < 16; i++ {
		go func(j int) {
			hl := m.NewHeightListener()
			var h int64
			for e := range hl {
				eventCount[j]++

				if h+1 != e {
					t.Errorf("event ouf of sequence for height listener %d. prev = %d, got = %d", j, h, e)
				}
				h = e
			}
		}(i)
	}

	ticks := 11 // only a single height transition
	ticked := 0

	ticker := time.NewTicker(minute)
	for range ticker.C {
		s.tick()

		ticked++
		if ticked == ticks {
			break
		}
	}

	time.Sleep(minute)
	for i, c := range eventCount {
		if i < 8 && c != ticks {
			t.Errorf("minute listener %d only has %d of %d ticks", i, c, ticks)
		}
		if i >= 8 && c != 1 {
			t.Errorf("block listener %d only has %d of %d ticks", i, c, 2)
		}
	}

	Interval = ogi
}

func TestMonitor_Errors(t *testing.T) {
	o1, o2 := Timeout, Interval
	Timeout = time.Second
	Interval = time.Millisecond * 250
	minute := time.Second
	s := newTestServer("localhost:9887", 0, 0, minute*10, t)
	defer s.stop()
	go s.run()

	f, err := NewMonitor("http://localhost:9887/v3")
	if err == nil {
		fmt.Printf("%+v\n", f)
		t.Fatalf("monitor did not error on bad url")
	}

	m, err := NewMonitor("http://localhost:9887/v2")
	if err != nil {
		t.Fatal(err)
	}

	errors := 0
	go func() {
		el := m.NewErrorListener()
		for e := range el {
			fmt.Println("received error", e)
			errors++
		}
	}()

	// allow one valid tick
	<-m.NewMinuteListener()

	if errors > 0 {
		t.Errorf("unexpected errors. want = 0, got = %d", errors)
	}

	s.stop()

	time.Sleep(minute * 2)

	if errors == 0 {
		t.Errorf("unexpected lack of errors during 2 intervals. want = 2ish, got = %d", errors)
	}

	Timeout, Interval = o1, o2
}

func TestMonitor_Stop(t *testing.T) {
	s := newTestServer("localhost:9886", 0, 0, time.Second*10, t)
	defer s.stop()

	m, err := NewMonitor("http://localhost:9886/v2")
	if err != nil {
		t.Fatal(err)
	}
	listener := m.NewMinuteListener()

	go func() {
		s.tick()
		time.Sleep(time.Millisecond * 250)
		s.tick()
		time.Sleep(time.Millisecond * 250)
	}()

	<-listener
	m.Stop()

	time.Sleep(time.Second)

	select {
	case e := <-listener:
		t.Errorf("received event after stop: %+v", e)
	default:
	}
}
