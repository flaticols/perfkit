package server

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/flaticols/perfkit/internal/k6"
	"github.com/flaticols/perfkit/internal/models"
	"github.com/flaticols/perfkit/internal/pprof"
	"github.com/google/uuid"
)

func (s *Server) handlePprofIngest(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Parse pprof profile
	parsed, err := pprof.Parse(body)
	if err != nil {
		http.Error(w, "Failed to parse pprof: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Extract metadata from query params
	profileType := r.URL.Query().Get("type")
	if profileType == "" {
		profileType = string(parsed.Type)
	}
	if !models.ProfileType(profileType).IsValid() {
		http.Error(w, "Invalid profile type: "+profileType, http.StatusBadRequest)
		return
	}

	project := r.URL.Query().Get("project")
	if project == "" {
		project = s.cfg.Project
	}

	session := r.URL.Query().Get("session")
	source := r.URL.Query().Get("source")
	name := r.URL.Query().Get("name")
	if name == "" {
		name = profileType + "-" + time.Now().Format("20060102-150405")
	}

	// Build profile record
	now := time.Now()
	profile := &models.Profile{
		ID:          uuid.New().String(),
		CreatedAt:   now,
		UpdatedAt:   now,
		Name:        name,
		ProfileType: models.ProfileType(profileType),
		Project:     project,
		Session:     session,
		Source:      source,
		RawData:     body,
		RawSize:     len(body),
		ProfileTime: &now,
		DurationNS:  parsed.DurationNS,
	}

	// Set quick-access fields
	if parsed.TotalSamples > 0 {
		profile.TotalSamples = &parsed.TotalSamples
	}
	if parsed.TotalValue > 0 {
		profile.TotalValue = &parsed.TotalValue
	}

	// Marshal metrics
	if parsed.Metrics != nil {
		metricsJSON, err := json.Marshal(parsed.Metrics)
		if err == nil {
			profile.Metrics = metricsJSON
		}
	}

	// Handle tags
	tags := r.URL.Query()["tag"]
	profile.Tags = append(s.cfg.DefaultTags, tags...)

	// Handle cumulative flag
	if r.URL.Query().Get("cumulative") == "true" {
		profile.IsCumulative = true
	}

	if err := s.store.SaveProfile(r.Context(), profile); err != nil {
		log.Printf("Failed to save profile: %v", err)
		http.Error(w, "Failed to save profile", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"id":      profile.ID,
		"message": "Profile ingested successfully",
	})
}

func (s *Server) handleListProfiles(w http.ResponseWriter, r *http.Request) {
	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}

	offset := 0
	if o := r.URL.Query().Get("offset"); o != "" {
		if n, err := strconv.Atoi(o); err == nil && n >= 0 {
			offset = n
		}
	}

	profileType := r.URL.Query().Get("type")
	if profileType != "" && !models.ProfileType(profileType).IsValid() {
		http.Error(w, "Invalid profile type: "+profileType, http.StatusBadRequest)
		return
	}
	project := r.URL.Query().Get("project")

	profiles, err := s.store.ListProfiles(r.Context(), limit, offset, profileType, project)
	if err != nil {
		log.Printf("Failed to list profiles: %v", err)
		http.Error(w, "Failed to list profiles", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(profiles)
}

func (s *Server) handleGetProfile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "Missing profile ID", http.StatusBadRequest)
		return
	}

	profile, err := s.store.GetProfile(r.Context(), id)
	if err != nil {
		log.Printf("Failed to get profile: %v", err)
		http.Error(w, "Profile not found", http.StatusNotFound)
		return
	}

	// Check if raw data requested
	if r.URL.Query().Get("raw") == "true" {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", "attachment; filename="+profile.Name+".pb.gz")
		w.Write(profile.RawData)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(profile)
}

func (s *Server) handleCompareProfiles(w http.ResponseWriter, r *http.Request) {
	idsParam := r.URL.Query().Get("ids")
	if idsParam == "" {
		http.Error(w, "Missing ids parameter", http.StatusBadRequest)
		return
	}

	ids := strings.Split(idsParam, ",")
	if len(ids) < 2 {
		http.Error(w, "At least 2 profile IDs required for comparison", http.StatusBadRequest)
		return
	}

	profiles := make([]*models.Profile, 0, len(ids))
	var expectedType models.ProfileType

	for i, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}

		profile, err := s.store.GetProfile(r.Context(), id)
		if err != nil {
			log.Printf("Failed to get profile %s: %v", id, err)
			http.Error(w, "Profile not found: "+id, http.StatusNotFound)
			return
		}

		// Validate same type
		if i == 0 {
			expectedType = profile.ProfileType
		} else if profile.ProfileType != expectedType {
			http.Error(w, "All profiles must be of the same type", http.StatusBadRequest)
			return
		}

		// Don't include raw data in comparison response
		profile.RawData = nil
		profiles = append(profiles, profile)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(profiles)
}

func (s *Server) handleK6Ingest(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Parse k6 summary JSON
	parsed, err := k6.Parse(body)
	if err != nil {
		http.Error(w, "Failed to parse k6 summary: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Extract metadata from query params
	project := r.URL.Query().Get("project")
	if project == "" {
		project = s.cfg.Project
	}

	session := r.URL.Query().Get("session")
	source := r.URL.Query().Get("source")
	name := r.URL.Query().Get("name")
	if name == "" {
		name = "k6-" + time.Now().Format("20060102-150405")
	}

	// Build profile record
	now := time.Now()
	profile := &models.Profile{
		ID:          uuid.New().String(),
		CreatedAt:   now,
		UpdatedAt:   now,
		Name:        name,
		ProfileType: models.ProfileTypeK6,
		Project:     project,
		Session:     session,
		Source:      source,
		RawData:     body,
		RawSize:     len(body),
		ProfileTime: &now,
		DurationNS:  parsed.DurationMS * 1_000_000, // Convert ms to ns
	}

	// Set k6 quick-access fields
	if parsed.Metrics != nil {
		if parsed.Metrics.P95 > 0 {
			profile.K6P95 = &parsed.Metrics.P95
		}
		if parsed.Metrics.P99 > 0 {
			profile.K6P99 = &parsed.Metrics.P99
		}
		if parsed.Metrics.RPS > 0 {
			profile.K6RPS = &parsed.Metrics.RPS
		}
		profile.K6ErrorRate = &parsed.Metrics.ErrorRate
		if parsed.DurationMS > 0 {
			profile.K6DurationMS = &parsed.DurationMS
		}

		// Marshal metrics
		metricsJSON, err := json.Marshal(parsed.Metrics)
		if err == nil {
			profile.Metrics = metricsJSON
		}
	}

	// Handle tags
	tags := r.URL.Query()["tag"]
	profile.Tags = append(s.cfg.DefaultTags, tags...)

	if err := s.store.SaveProfile(r.Context(), profile); err != nil {
		log.Printf("Failed to save k6 profile: %v", err)
		http.Error(w, "Failed to save profile", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"id":      profile.ID,
		"message": "K6 profile ingested successfully",
	})
}
