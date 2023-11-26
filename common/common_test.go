package common

import (
	"fmt"
	"testing"
	"time"
)

func TestAddress(t *testing.T) {
	testCases := []struct {
		host      string
		port      uint64
		want      string
		withError bool
	}{
		{host: "localhost", port: 8080, want: "localhost:8080"},
		{host: "localhost", port: 0, withError: true},
		{host: "localhost", port: 65536, withError: true},
		{host: "localhost", port: 100_000_000, withError: true},
		{host: "127.0.0.1", port: 8080, want: "127.0.0.1:8080"},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(fmt.Sprintf("case_%d", i), func(t *testing.T) {
			got, err := Address(tc.host, tc.port)
			if tc.withError {
				if err == nil {
					t.Errorf("want error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("want nil, got %v", err)
				return
			}

			if got != tc.want {
				t.Fatalf("want %s, got %s", tc.want, got)
			}
		})
	}
}

func TestByteSize(t *testing.T) {
	testCases := []struct {
		size int
		want string
	}{
		{size: -1, want: "-1 B"},
		{size: 0, want: "0 B"},
		{size: 1, want: "1 B"},
		{size: 1023, want: "1023 B"},
		{size: KB, want: "1.00 KB"},
		{size: KB + 120, want: "1.12 KB"},
		{size: MB, want: "1.00 MB"},
		{size: MB + 130*KB, want: "1.13 MB"},
		{size: GB, want: "1.00 GB"},
		{size: 10*GB + 140*MB, want: "10.14 GB"},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(fmt.Sprintf("case_%d", i), func(t *testing.T) {
			got := ByteSize(tc.size)

			if got != tc.want {
				t.Errorf("want %s, got %s", tc.want, got)
			}
		})
	}
}

func TestSpeed(t *testing.T) {
	var delay = 10 * time.Millisecond

	testCases := []struct {
		name  string
		delay time.Duration
		count int64
		unit  SpeedUnit
		want  string
	}{
		{name: "zero_with_seconds", delay: 0, count: 0, unit: SpeedSeconds, want: "0.00 B/s"},
		{name: "zero_with_milliseconds", delay: 0, count: 0, unit: SpeedMilliseconds, want: "0.00 B/ms"},
		{name: "zero_with_microseconds", delay: 0, count: 0, unit: SpeedMicroseconds, want: "0.00 B/μs"},
		{name: "microseconds", delay: 5000 * time.Microsecond, count: 50, unit: SpeedMicroseconds, want: "0.01 B/μs"},
		{name: "milliseconds", delay: delay, count: 57, unit: SpeedMilliseconds, want: "5.70 B/ms"},
		{name: "seconds", delay: time.Second, count: 5, unit: SpeedSeconds, want: "5.00 B/s"},
		{name: "kilobytes", delay: delay, count: 105 * KB, unit: SpeedMilliseconds, want: "10.50 KB/ms"},
		{name: "megabytes", delay: delay, count: 105 * MB, unit: SpeedMilliseconds, want: "10.50 MB/ms"},
		{name: "gigabytes", delay: delay, count: 105 * GB, unit: SpeedMilliseconds, want: "10.50 GB/ms"},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			start := time.Now()
			time.Sleep(tc.delay)

			if got := Speed(start, tc.count, tc.unit); got != tc.want {
				t.Errorf("want %s, got %s", tc.want, got)
			}
		})
	}
}
