package ratelimiter

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/Clever/leakybucket"
	"github.com/nacholopez/sphinx/common"
	"github.com/nacholopez/sphinx/config"
	"github.com/nacholopez/sphinx/limit"
)

func returnLastAddStatus(rateLimiter RateLimiter, request common.Request, numAdds int) ([]Status, error) {
	statuses := []Status{}
	var err error
	for i := 0; i < numAdds; i++ {
		if statuses, err = rateLimiter.Add(request); err != nil {
			return statuses, err
		}
	}
	return statuses, nil
}

func checkLastStatusForRequests(ratelimiter RateLimiter,
	request common.Request, numAdds int, expectedStatuses []Status) error {

	if statuses, err := returnLastAddStatus(ratelimiter, request, numAdds); err != nil {
		return err
	} else if len(statuses) != len(expectedStatuses) {
		return fmt.Errorf("expected to match %d buckets. Got: %d", len(expectedStatuses),
			len(statuses))
	} else {
		for i, status := range expectedStatuses {
			if status.Remaining != statuses[i].Remaining && status.Name != statuses[i].Name {
				return fmt.Errorf("expected %d remaining for the %s limit. Found: %d Remaining, %s Limit",
					statuses[i].Remaining, statuses[i].Name, status.Remaining, status.Name)
			}
		}
	}

	return nil
}

// ratelimiter is initialized properly based on config
func TestNew(t *testing.T) {

	config, err := config.New("../example.yaml")
	if err != nil {
		t.Error("could not load example configuration")
	}

	rater, err := New(config)
	ratelimiter := rater.(*rateLimiter)
	if err != nil {
		t.Errorf("Error while instantiating ratelimiter: %s", err.Error())
	}
	if len(ratelimiter.limits) != len(config.Limits) {
		t.Error("expected number of limits in configuration to match instantiated limits")
	}
}

// adds different kinds of requests and checks limit Status
// focusses on single bucket adds
func TestSimpleAdd(t *testing.T) {
	config, err := config.New("../example.yaml")
	if err != nil {
		t.Error("could not load example configuration")
	}
	ratelimiter, err := New(config)

	request := common.Request{
		"path": "/special/resources/123",
		"headers": http.Header{
			"Authorization":   []string{"Bearer 12345"},
			"X-Forwarded-For": []string{"IP1", "IP2"},
		},
		"remoteaddr": "127.0.0.1",
	}
	if err = checkLastStatusForRequests(
		ratelimiter, request, 5, []Status{
			Status{Remaining: 195, Name: "bearer-special"}}); err != nil {
		t.Error(err)
	}

	request = common.Request{
		"path": "/resources/123",
		"headers": http.Header{
			"Authorization": []string{"Basic 12345"},
		},
	}

	if err = checkLastStatusForRequests(
		ratelimiter, request, 1, []Status{
			Status{Remaining: 195, Name: "basic-simple"}}); err != nil {
		t.Error(err)
	}

	if status, err := returnLastAddStatus(ratelimiter, request, 200); err == nil {
		t.Fatal("expected error")
	} else if err != leakybucket.ErrorFull {
		t.Fatalf("expected ErrorFull, received %#v", err)
	} else if len(status) != 1 {
		t.Fatalf("expected one status, found %d", len(status))
	} else if status[0].Remaining != 0 {
		t.Fatalf("expected 0 remaining, found %d", status[0].Remaining)
	} else if status[0].Name != "basic-simple" {
		t.Fatalf("expected 'basic-simple' limit, found '%s'", status[0].Name)
	}
}

// Test to ensure we build the correct nil Ratelimiter status
func TestNilStatus(t *testing.T) {
	if NilStatus.Capacity != 1 {
		t.Fatalf("Expected NilStatus capacity to be 1")
	}

	if NilStatus.Reset.Unix() > time.Now().Unix() {
		t.Fatalf("Expected NilStatus reset time to be in the past")
	}

	if NilStatus.Remaining != 1 {
		t.Fatalf("Expected NilStatus remaining to be 1")
	}

	if NilStatus.Name != "Unknown" {
		t.Fatalf("Expected NilStatus name to be 'Unknown'")
	}
}

type NeverMatch struct{}

func (m NeverMatch) Name() string {
	return "name"
}
func (m NeverMatch) Match(common.Request) bool {
	return false
}
func (m NeverMatch) Add(common.Request) (leakybucket.BucketState, error) {
	return leakybucket.BucketState{}, nil
}

func createRateLimiter(numLimits int) RateLimiter {
	rateLimiter := &rateLimiter{}
	limits := []limit.Limit{}
	limit := &NeverMatch{}
	for i := 0; i < numLimits; i++ {
		limits = append(limits, limit)
	}
	rateLimiter.limits = limits
	return rateLimiter
}

var benchAdd = func(b *testing.B, numLimits int) {
	rateLimiter := createRateLimiter(numLimits)
	request := common.Request{}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rateLimiter.Add(request)
	}
}

func BenchmarkAdd1(b *testing.B) {
	benchAdd(b, 1)
}

func BenchmarkAdd100(b *testing.B) {
	benchAdd(b, 100)
}
