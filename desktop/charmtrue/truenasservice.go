package main

// TrueNASService is the backend boundary for TrueNAS API operations.
// Connection and credential management will be added behind this service.
type TrueNASService struct{}

type AppInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Status  string `json:"status"`
}

func (s *TrueNASService) AppInfo() AppInfo {
	return AppInfo{
		Name:    "CharmTrue",
		Version: "0.1.0",
		Status:  "No TrueNAS system configured",
	}
}
