package pprof

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"sort"

	"github.com/flaticols/perfkit/internal/models"
	"github.com/google/pprof/profile"
)

type ParsedProfile struct {
	Type         models.ProfileType
	DurationNS   int64
	TotalSamples int64
	TotalValue   int64
	Metrics      any
}

func Parse(data []byte) (*ParsedProfile, error) {
	// Try to decompress if gzipped
	reader := bytes.NewReader(data)
	var r io.Reader = reader

	if gr, err := gzip.NewReader(reader); err == nil {
		r = gr
		defer gr.Close()
	} else {
		reader.Seek(0, io.SeekStart)
	}

	p, err := profile.Parse(r)
	if err != nil {
		return nil, fmt.Errorf("parse profile: %w", err)
	}

	result := &ParsedProfile{
		DurationNS: p.DurationNanos,
	}

	// Determine profile type from sample types
	result.Type = detectProfileType(p)

	// Calculate totals and extract metrics based on type
	switch result.Type {
	case models.ProfileTypeCPU:
		result.Metrics = extractCPUMetrics(p)
	case models.ProfileTypeHeap:
		result.Metrics = extractHeapMetrics(p)
	case models.ProfileTypeMutex:
		result.Metrics = extractMutexMetrics(p)
	case models.ProfileTypeBlock:
		result.Metrics = extractBlockMetrics(p)
	case models.ProfileTypeGoroutine:
		result.Metrics = extractGoroutineMetrics(p)
	}

	// Calculate totals
	for _, sample := range p.Sample {
		result.TotalSamples++
		if len(sample.Value) > 0 {
			result.TotalValue += sample.Value[0]
		}
	}

	return result, nil
}

func detectProfileType(p *profile.Profile) models.ProfileType {
	for _, st := range p.SampleType {
		switch st.Type {
		case "cpu", "samples":
			if st.Unit == "nanoseconds" || st.Unit == "count" {
				return models.ProfileTypeCPU
			}
		case "alloc_objects", "alloc_space", "inuse_objects", "inuse_space":
			return models.ProfileTypeHeap
		case "contentions", "delay":
			return models.ProfileTypeMutex
		case "block":
			return models.ProfileTypeBlock
		case "goroutine":
			return models.ProfileTypeGoroutine
		}
	}
	return models.ProfileTypeCPU
}

func extractCPUMetrics(p *profile.Profile) *models.CPUMetrics {
	metrics := &models.CPUMetrics{
		SampleCount: int64(len(p.Sample)),
	}

	funcValues := make(map[string]int64)
	var totalValue int64

	for _, sample := range p.Sample {
		if len(sample.Value) == 0 || len(sample.Location) == 0 {
			continue
		}
		value := sample.Value[0]
		totalValue += value

		for _, loc := range sample.Location {
			for _, line := range loc.Line {
				if line.Function != nil {
					funcValues[line.Function.Name] += value
				}
			}
		}
	}

	metrics.TotalCPUTimeNS = totalValue
	metrics.TopFunctions = topFunctions(funcValues, totalValue, 10)

	return metrics
}

func extractHeapMetrics(p *profile.Profile) *models.HeapMetrics {
	metrics := &models.HeapMetrics{}

	// Find indices for different value types
	var allocSpaceIdx, allocObjIdx, inuseSpaceIdx, inuseObjIdx int = -1, -1, -1, -1
	for i, st := range p.SampleType {
		switch st.Type {
		case "alloc_space":
			allocSpaceIdx = i
		case "alloc_objects":
			allocObjIdx = i
		case "inuse_space":
			inuseSpaceIdx = i
		case "inuse_objects":
			inuseObjIdx = i
		}
	}

	funcValues := make(map[string]int64)

	for _, sample := range p.Sample {
		if allocSpaceIdx >= 0 && allocSpaceIdx < len(sample.Value) {
			metrics.AllocSize += sample.Value[allocSpaceIdx]
		}
		if allocObjIdx >= 0 && allocObjIdx < len(sample.Value) {
			metrics.AllocObjects += sample.Value[allocObjIdx]
		}
		if inuseSpaceIdx >= 0 && inuseSpaceIdx < len(sample.Value) {
			metrics.InuseSize += sample.Value[inuseSpaceIdx]
		}
		if inuseObjIdx >= 0 && inuseObjIdx < len(sample.Value) {
			metrics.InuseObjects += sample.Value[inuseObjIdx]
		}

		if len(sample.Location) > 0 {
			for _, loc := range sample.Location {
				for _, line := range loc.Line {
					if line.Function != nil && allocSpaceIdx >= 0 {
						funcValues[line.Function.Name] += sample.Value[allocSpaceIdx]
					}
				}
			}
		}
	}

	metrics.TopAllocators = topFunctions(funcValues, metrics.AllocSize, 10)

	return metrics
}

func extractMutexMetrics(p *profile.Profile) *models.MutexMetrics {
	metrics := &models.MutexMetrics{}
	funcValues := make(map[string]int64)

	for _, sample := range p.Sample {
		if len(sample.Value) >= 2 {
			metrics.ContentionCount += sample.Value[0]
			metrics.ContentionTimeNS += sample.Value[1]
		}

		for _, loc := range sample.Location {
			for _, line := range loc.Line {
				if line.Function != nil && len(sample.Value) >= 2 {
					funcValues[line.Function.Name] += sample.Value[1]
				}
			}
		}
	}

	metrics.TopContenders = topFunctions(funcValues, metrics.ContentionTimeNS, 10)

	return metrics
}

func extractBlockMetrics(p *profile.Profile) *models.BlockMetrics {
	metrics := &models.BlockMetrics{}
	funcValues := make(map[string]int64)

	for _, sample := range p.Sample {
		if len(sample.Value) >= 2 {
			metrics.BlockingCount += sample.Value[0]
			metrics.BlockingTimeNS += sample.Value[1]
		}

		for _, loc := range sample.Location {
			for _, line := range loc.Line {
				if line.Function != nil && len(sample.Value) >= 2 {
					funcValues[line.Function.Name] += sample.Value[1]
				}
			}
		}
	}

	metrics.TopBlockers = topFunctions(funcValues, metrics.BlockingTimeNS, 10)

	return metrics
}

func extractGoroutineMetrics(p *profile.Profile) *models.GoroutineMetrics {
	metrics := &models.GoroutineMetrics{
		GoroutineCount: int64(len(p.Sample)),
	}

	stackCounts := make(map[string]int64)

	for _, sample := range p.Sample {
		var stack []string
		for _, loc := range sample.Location {
			for _, line := range loc.Line {
				if line.Function != nil {
					stack = append(stack, line.Function.Name)
				}
			}
		}

		key := ""
		for _, s := range stack {
			key += s + "\n"
		}
		stackCounts[key]++
	}

	// Get top stacks
	type kv struct {
		stack string
		count int64
	}
	var sorted []kv
	for k, v := range stackCounts {
		sorted = append(sorted, kv{k, v})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].count > sorted[j].count
	})

	for i := 0; i < 10 && i < len(sorted); i++ {
		metrics.TopStacks = append(metrics.TopStacks, models.StackSample{
			Count: sorted[i].count,
			Stack: splitStack(sorted[i].stack),
		})
	}

	return metrics
}

func topFunctions(funcValues map[string]int64, total int64, n int) []models.FunctionSample {
	type kv struct {
		name  string
		value int64
	}
	var sorted []kv
	for k, v := range funcValues {
		sorted = append(sorted, kv{k, v})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].value > sorted[j].value
	})

	var result []models.FunctionSample
	for i := 0; i < n && i < len(sorted); i++ {
		pct := float64(0)
		if total > 0 {
			pct = float64(sorted[i].value) / float64(total) * 100
		}
		result = append(result, models.FunctionSample{
			Name:    sorted[i].name,
			Value:   sorted[i].value,
			Percent: pct,
		})
	}

	return result
}

func splitStack(s string) []string {
	var result []string
	var current string
	for _, c := range s {
		if c == '\n' {
			if current != "" {
				result = append(result, current)
				current = ""
			}
		} else {
			current += string(c)
		}
	}
	if current != "" {
		result = append(result, current)
	}
	return result
}
