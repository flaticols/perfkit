package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

// NullableJSON represents a json.RawMessage that can be NULL in the database
type NullableJSON json.RawMessage

func (n *NullableJSON) Scan(value interface{}) error {
	if value == nil {
		*n = nil
		return nil
	}
	switch v := value.(type) {
	case []byte:
		*n = NullableJSON(v)
	case string:
		*n = NullableJSON(v)
	default:
		return fmt.Errorf("unsupported type for NullableJSON: %T", value)
	}
	return nil
}

func (n NullableJSON) Value() (driver.Value, error) {
	if n == nil {
		return nil, nil
	}
	return []byte(n), nil
}

func (n NullableJSON) MarshalJSON() ([]byte, error) {
	if n == nil {
		return []byte("null"), nil
	}
	return json.RawMessage(n).MarshalJSON()
}

func (n *NullableJSON) UnmarshalJSON(data []byte) error {
	if data == nil || string(data) == "null" {
		*n = nil
		return nil
	}
	*n = NullableJSON(data)
	return nil
}

type ProfileType string

const (
	ProfileTypeCPU          ProfileType = "cpu"
	ProfileTypeHeap         ProfileType = "heap"
	ProfileTypeMutex        ProfileType = "mutex"
	ProfileTypeBlock        ProfileType = "block"
	ProfileTypeGoroutine    ProfileType = "goroutine"
	ProfileTypeGC           ProfileType = "gc"
	ProfileTypeK6           ProfileType = "k6"
	ProfileTypeAllocs       ProfileType = "allocs"
	ProfileTypeThreadCreate ProfileType = "threadcreate"
)

var validProfileTypes = map[ProfileType]bool{
	ProfileTypeCPU:          true,
	ProfileTypeHeap:         true,
	ProfileTypeMutex:        true,
	ProfileTypeBlock:        true,
	ProfileTypeGoroutine:    true,
	ProfileTypeGC:           true,
	ProfileTypeK6:           true,
	ProfileTypeAllocs:       true,
	ProfileTypeThreadCreate: true,
}

// Cumulative profiles accumulate data since program start
var cumulativeProfileTypes = map[ProfileType]bool{
	ProfileTypeBlock:  true,
	ProfileTypeMutex:  true,
	ProfileTypeAllocs: true,
}

func (pt ProfileType) IsValid() bool {
	return validProfileTypes[pt]
}

func (pt ProfileType) IsCumulative() bool {
	return cumulativeProfileTypes[pt]
}

type Profile struct {
	ID        string    `db:"id" json:"id"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`

	Name        string      `db:"name" json:"name"`
	ProfileType ProfileType `db:"profile_type" json:"profile_type"`
	Project     string      `db:"project" json:"project"`
	Session     string      `db:"session" json:"session,omitempty"`
	Tags        []string    `db:"-" json:"tags"`
	TagsJSON    string      `db:"tags" json:"-"`
	Source      string      `db:"source" json:"source"`

	RawData      []byte `db:"raw_data" json:"-"`
	RawSize      int    `db:"raw_size" json:"raw_size"`
	IsCumulative bool   `db:"is_cumulative" json:"is_cumulative,omitempty"`

	ProfileTime *time.Time `db:"profile_time" json:"profile_time,omitempty"`
	DurationNS  int64      `db:"duration_ns" json:"duration_ns,omitempty"`

	Metrics NullableJSON `db:"metrics" json:"metrics"`

	// pprof quick-access fields
	TotalSamples *int64 `db:"total_samples" json:"total_samples,omitempty"`
	TotalValue   *int64 `db:"total_value" json:"total_value,omitempty"`

	// k6 quick-access fields
	K6P95        *float64 `db:"k6_p95" json:"k6_p95,omitempty"`
	K6P99        *float64 `db:"k6_p99" json:"k6_p99,omitempty"`
	K6RPS        *float64 `db:"k6_rps" json:"k6_rps,omitempty"`
	K6ErrorRate  *float64 `db:"k6_error_rate" json:"k6_error_rate,omitempty"`
	K6DurationMS *int64   `db:"k6_duration_ms" json:"k6_duration_ms,omitempty"`
}

func (p *Profile) UnmarshalTags() error {
	if p.TagsJSON == "" || p.TagsJSON == "null" {
		p.Tags = []string{}
		return nil
	}
	return json.Unmarshal([]byte(p.TagsJSON), &p.Tags)
}

func (p *Profile) MarshalTags() error {
	if p.Tags == nil {
		p.Tags = []string{}
	}
	data, err := json.Marshal(p.Tags)
	if err != nil {
		return err
	}
	p.TagsJSON = string(data)
	return nil
}

// Metric types for each profile type

type FunctionSample struct {
	Name    string  `json:"name"`
	File    string  `json:"file,omitempty"`
	Line    int     `json:"line,omitempty"`
	Value   int64   `json:"value"`
	Percent float64 `json:"percent"`
}

type StackSample struct {
	Count int64    `json:"count"`
	Stack []string `json:"stack"`
}

type CPUMetrics struct {
	TotalCPUTimeNS int64            `json:"total_cpu_time_ns"`
	SampleCount    int64            `json:"sample_count"`
	TopFunctions   []FunctionSample `json:"top_functions"`
}

type HeapMetrics struct {
	AllocSize     int64            `json:"alloc_size"`
	AllocObjects  int64            `json:"alloc_objects"`
	InuseSize     int64            `json:"inuse_size"`
	InuseObjects  int64            `json:"inuse_objects"`
	TopAllocators []FunctionSample `json:"top_allocators"`
}

type MutexMetrics struct {
	ContentionTimeNS int64            `json:"contention_time_ns"`
	ContentionCount  int64            `json:"contention_count"`
	TopContenders    []FunctionSample `json:"top_contenders"`
}

type BlockMetrics struct {
	BlockingTimeNS int64            `json:"blocking_time_ns"`
	BlockingCount  int64            `json:"blocking_count"`
	TopBlockers    []FunctionSample `json:"top_blockers"`
}

type GoroutineMetrics struct {
	GoroutineCount int64         `json:"goroutine_count"`
	TopStacks      []StackSample `json:"top_stacks"`
}

type GCMetrics struct {
	PauseTimeTotalNS int64 `json:"pause_time_total_ns"`
	PauseCount       int64 `json:"pause_count"`
	HeapGoal         int64 `json:"heap_goal"`
	LastPauseNS      int64 `json:"last_pause_ns"`
}

type K6Metrics struct {
	P50            float64 `json:"p50_ms"`
	P95            float64 `json:"p95_ms"`
	P99            float64 `json:"p99_ms"`
	Mean           float64 `json:"mean_ms"`
	Min            float64 `json:"min_ms"`
	Max            float64 `json:"max_ms"`
	RPS            float64 `json:"rps"`
	ErrorRate      float64 `json:"error_rate"`
	TotalRequests  int64   `json:"total_requests"`
	FailedRequests int64   `json:"failed_requests"`
	DurationMS     int64   `json:"duration_ms"`
	VUs            int     `json:"vus"`
	VUsMax         int     `json:"vus_max"`
}
