package k6

import (
	"encoding/json"
	"fmt"

	"github.com/flaticols/perfkit/internal/models"
)

// K6Summary represents the structure of k6's --summary-export JSON output
type K6Summary struct {
	Metrics map[string]K6Metric `json:"metrics"`
	Root    K6RootGroup         `json:"root_group"`
}

type K6Metric struct {
	Type     string                 `json:"type"`
	Contains string                 `json:"contains"`
	Values   map[string]interface{} `json:"values"`
}

type K6RootGroup struct {
	Duration float64 `json:"duration"`
	Checks   int     `json:"checks"`
}

// ParsedK6 represents a parsed k6 test result
type ParsedK6 struct {
	Metrics    *models.K6Metrics
	DurationMS int64
}

// Parse parses k6 JSON summary data
func Parse(data []byte) (*ParsedK6, error) {
	var summary K6Summary
	if err := json.Unmarshal(data, &summary); err != nil {
		return nil, fmt.Errorf("parse k6 json: %w", err)
	}

	result := &ParsedK6{
		Metrics:    &models.K6Metrics{},
		DurationMS: int64(summary.Root.Duration),
	}

	// Extract http_req_duration percentiles
	if metric, ok := summary.Metrics["http_req_duration"]; ok {
		if vals := metric.Values; vals != nil {
			if v, ok := vals["p(50)"].(float64); ok {
				result.Metrics.P50 = v
			}
			if v, ok := vals["p(95)"].(float64); ok {
				result.Metrics.P95 = v
			}
			if v, ok := vals["p(99)"].(float64); ok {
				result.Metrics.P99 = v
			}
			if v, ok := vals["min"].(float64); ok {
				result.Metrics.Min = v
			}
			if v, ok := vals["max"].(float64); ok {
				result.Metrics.Max = v
			}
			if v, ok := vals["avg"].(float64); ok {
				result.Metrics.Mean = v
			}
		}
	}

	// Extract RPS from http_reqs
	if metric, ok := summary.Metrics["http_reqs"]; ok {
		if vals := metric.Values; vals != nil {
			if v, ok := vals["rate"].(float64); ok {
				result.Metrics.RPS = v
			}
			if v, ok := vals["count"].(float64); ok {
				result.Metrics.TotalRequests = int64(v)
			}
		}
	}

	// Extract VUs
	if metric, ok := summary.Metrics["vus"]; ok {
		if vals := metric.Values; vals != nil {
			if v, ok := vals["value"].(float64); ok {
				result.Metrics.VUs = int(v)
			}
		}
	}

	if metric, ok := summary.Metrics["vus_max"]; ok {
		if vals := metric.Values; vals != nil {
			if v, ok := vals["value"].(float64); ok {
				result.Metrics.VUsMax = int(v)
			}
		}
	}

	// Calculate error rate from checks if available
	// k6's check success rate can be used to derive error rate
	if metric, ok := summary.Metrics["checks"]; ok {
		if vals := metric.Values; vals != nil {
			if rate, ok := vals["rate"].(float64); ok {
				// rate is success rate (0-1), error rate is 1 - rate
				result.Metrics.ErrorRate = 1.0 - rate
			}
			// If we have passes and fails, calculate directly
			if passes, pok := vals["passes"].(float64); pok {
				if fails, fok := vals["fails"].(float64); fok {
					total := passes + fails
					if total > 0 {
						result.Metrics.ErrorRate = fails / total
					}
				}
			}
		}
	}

	// If we have http_req_failed metric, use that for error rate
	if metric, ok := summary.Metrics["http_req_failed"]; ok {
		if vals := metric.Values; vals != nil {
			if rate, ok := vals["rate"].(float64); ok {
				result.Metrics.ErrorRate = rate
			}
			if fails, ok := vals["passes"].(float64); ok {
				result.Metrics.FailedRequests = int64(fails)
			}
		}
	}

	// Set duration in metrics
	result.Metrics.DurationMS = result.DurationMS

	return result, nil
}
