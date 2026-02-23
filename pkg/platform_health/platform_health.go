package platform_health

import (
	"fmt"
	"log/slog"
	"strings"
)

type UnhealthyError struct{}

func (e *UnhealthyError) Error() string {
	return "UNHEALTHY"
}

func (s *HealthCheckResponse) LogStatus(log *slog.Logger) {
	if s.Status == Status_HEALTHY {
		log.Info("success")
	} else {
		log.Warn("failure")
	}
}

func (s *HealthCheckResponse) Healthy() *HealthCheckResponse {
	s.Status = Status_HEALTHY
	return s
}

func (s *HealthCheckResponse) Unhealthy(msgs ...string) *HealthCheckResponse {
	s.Status = Status_UNHEALTHY
	s.Messages = append(s.Messages, msgs...)
	return s
}

func (s *HealthCheckResponse) IsHealthy() error {
	if s.Status != Status_HEALTHY {
		return &UnhealthyError{}
	}
	return nil
}

func (s *HealthCheckResponse) Flatten(parentPath, parentType string) (components []*HealthCheckResponse) {
	components = make([]*HealthCheckResponse, 0, 1+len(s.Components))

	// Determine effective type (own type, or inherit from parent)
	effectiveType := s.Type
	if effectiveType == "" {
		effectiveType = parentType
	}

	pathName := s.Name
	if effectiveType != "" {
		if parentPath != "" {
			pathName = fmt.Sprintf("%s/%s", strings.TrimSuffix(parentPath, "/"), pathName)
		}

		if effectiveType != "satellite" {
			components = append(components, &HealthCheckResponse{
				Type:     effectiveType,
				Name:     pathName,
				Status:   s.Status,
				Messages: s.Messages,
				Details:  s.Details,
				Duration: s.Duration,
			})
		}
	}

	for _, component := range s.Components {
		components = append(components, component.Flatten(pathName, effectiveType)...)
	}
	return components
}
