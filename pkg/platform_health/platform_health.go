package platform_health

import (
	"fmt"
	"log/slog"
	"strings"
)

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

func (s *HealthCheckResponse) Unhealthy(msg string) *HealthCheckResponse {
	s.Status = Status_UNHEALTHY
	s.Message = msg
	return s
}

func (s *HealthCheckResponse) Flatten(parent string) (components []*HealthCheckResponse) {
	components = make([]*HealthCheckResponse, 0, 1+len(s.Components))

	pathName := fmt.Sprintf("%s/%s", strings.TrimSuffix(parent, "/"), s.Name)

	if s.Name != "" {
		components = append(components, &HealthCheckResponse{
			Type:     s.Type,
			Name:     pathName,
			Status:   s.Status,
			Message:  s.Message,
			Details:  s.Details,
			Duration: s.Duration,
		})
	}

	for _, component := range s.Components {
		components = append(components, component.Flatten(pathName)...)
	}
	return components
}
