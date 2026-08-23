package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func fastOpts() PollOptions {
	return PollOptions{InitialInterval: 5 * time.Millisecond, MaxInterval: 10 * time.Millisecond, Timeout: 2 * time.Second}
}

func TestWaitUntilDoneTerminal(t *testing.T) {
	var polls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&polls, 1)
		status := "in_progress"
		if n >= 3 {
			status = "ended"
		}
		fmt.Fprintf(w, `{"id":"b1","status":%q}`, status)
	}))
	defer srv.Close()
	bc := NewBatchClient("k", srv.URL)
	res, err := bc.WaitUntilDone(context.Background(), "b1", fastOpts())
	if err != nil {
		t.Fatalf("WaitUntilDone: %v", err)
	}
	if res.Status != "ended" {
		t.Fatalf("status = %q", res.Status)
	}
	if atomic.LoadInt32(&polls) < 3 {
		t.Fatalf("expected >=3 polls, got %d", polls)
	}
}

func TestWaitUntilDoneTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"id":"b1","status":"in_progress"}`)
	}))
	defer srv.Close()
	bc := NewBatchClient("k", srv.URL)
	opts := PollOptions{InitialInterval: 5 * time.Millisecond, MaxInterval: 10 * time.Millisecond, Timeout: 80 * time.Millisecond}
	if _, err := bc.WaitUntilDone(context.Background(), "b1", opts); err == nil || !strings.Contains(err.Error(), "not terminal") {
		t.Fatalf("err = %v", err)
	}
}

func TestWaitUntilDoneHonorsRetryAfterOn429(t *testing.T) {
	var saw429 int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.CompareAndSwapInt32(&saw429, 0, 1) {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		fmt.Fprint(w, `{"id":"b1","status":"ended"}`)
	}))
	defer srv.Close()
	bc := NewBatchClient("k", srv.URL)
	start := time.Now()
	res, err := bc.WaitUntilDone(context.Background(), "b1", fastOpts())
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "ended" {
		t.Fatal("wrong status")
	}
	// Retry-After: 1 second must have been honored (>= ~900ms elapsed).
	if elapsed := time.Since(start); elapsed < 900*time.Millisecond {
		t.Fatalf("Retry-After not honored; elapsed=%v", elapsed)
	}
}

func TestWaitUntilDoneFailsFastOn401(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	bc := NewBatchClient("k", srv.URL)
	_, err := bc.WaitUntilDone(context.Background(), "b1", fastOpts())
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("err = %v", err)
	}
}

const resultsJSONL = "{\"custom_id\":\"r1\",\"result\":{\"type\":\"succeeded\"}}\n" +
	"{\"custom_id\":\"r2\",\"error\":{\"type\":\"invalid_request\"}}\n"

func TestRequestResultsParsesJSONL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, resultsJSONL)
	}))
	defer srv.Close()
	bc := NewBatchClient("k", srv.URL)
	rows, err := bc.RequestResults(context.Background(), "b1")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d", len(rows))
	}
	if rows[0].CustomID != "r1" || len(rows[0].Result) == 0 {
		t.Fatalf("row0 = %+v", rows[0])
	}
	if rows[1].CustomID != "r2" || len(rows[1].Error) == 0 {
		t.Fatalf("row1 = %+v", rows[1])
	}
}

func TestPollRequestResultToleratesLag(t *testing.T) {
	// First results fetch returns empty (eventual consistency), second has the row.
	var fetches int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/results"):
			if atomic.AddInt32(&fetches, 1) == 1 {
				fmt.Fprint(w, "")
				return
			}
			fmt.Fprint(w, resultsJSONL)
		default:
			fmt.Fprint(w, `{"id":"b1","status":"ended"}`)
		}
	}))
	defer srv.Close()
	bc := NewBatchClient("k", srv.URL)
	row, err := bc.PollRequestResult(context.Background(), "b1", "r2", fastOpts())
	if err != nil {
		t.Fatal(err)
	}
	if row.CustomID != "r2" || len(row.Error) == 0 {
		t.Fatalf("row = %+v", row)
	}
	if atomic.LoadInt32(&fetches) < 2 {
		t.Fatal("expected a retry after empty results")
	}
}

func TestPollRequestResultNotVisibleAfterExtraAttempts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/results") {
			fmt.Fprint(w, `{"custom_id":"other","result":{}}`)
			return
		}
		fmt.Fprint(w, `{"id":"b1","status":"ended"}`)
	}))
	defer srv.Close()
	bc := NewBatchClient("k", srv.URL)
	_, err := bc.PollRequestResult(context.Background(), "b1", "missing", fastOpts())
	if !errors.Is(err, ErrResultNotVisible) {
		t.Fatalf("err = %v, want ErrResultNotVisible", err)
	}
}

func TestBackoffDelayRetryAfterOverride(t *testing.T) {
	o := fastOpts()
	d := backoffDelay(0, strconv.Itoa(60), o)
	if d < 59*time.Second {
		t.Fatalf("Retry-After override ignored: %v", d)
	}
	// Without header: exponential with cap.
	if got := backoffDelay(20, "", o); got > o.MaxInterval*2 {
		t.Fatalf("cap exceeded: %v", got)
	}
}
