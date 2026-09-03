package truenas

import "strings"

// MethodRisk describes whether a method requires destructive-action
// confirmation in a higher-level client.
type MethodRisk string

const (
	MethodRiskNormal      MethodRisk = "normal"
	MethodRiskDestructive MethodRisk = "destructive"
)

// MethodMetadata carries safety metadata that is not part of the RPC payload.
type MethodMetadata struct {
	Name string
	Risk MethodRisk
}

// MetadataForMethod marks the destructive families from the CharmTrue guide.
func MetadataForMethod(method string) MethodMetadata {
	method = strings.TrimSpace(method)
	parts := strings.Split(method, ".")
	operation := parts[len(parts)-1]
	risk := MethodRiskNormal
	switch operation {
	case "delete", "wipe", "reset", "rollback", "reboot", "shutdown":
		risk = MethodRiskDestructive
	}
	return MethodMetadata{Name: method, Risk: risk}
}
