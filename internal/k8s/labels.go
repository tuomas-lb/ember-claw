package k8s

import "fmt"

const (
	LabelManagedBy = "app.kubernetes.io/managed-by"
	LabelInstance  = "app.kubernetes.io/instance"
	LabelName      = "app.kubernetes.io/name"
	LabelComponent = "app.kubernetes.io/component"

	ManagedByValue = "eclaw"
	NameValue      = "picoclaw"
	ComponentValue = "ai-assistant"
)

// InstanceLabels returns the full label map for a PicoClaw instance.
// All resources belonging to an instance share these labels.
func InstanceLabels(name string) map[string]string {
	return map[string]string{
		LabelManagedBy: ManagedByValue,
		LabelInstance:  name,
		LabelName:      NameValue,
		LabelComponent: ComponentValue,
	}
}

// InstanceSelector returns a comma-separated label selector string that
// matches all resources belonging to a specific managed instance.
func InstanceSelector(name string) string {
	return fmt.Sprintf("%s=%s,%s=%s", LabelManagedBy, ManagedByValue, LabelInstance, name)
}

// ManagedSelector returns a label selector string that matches all resources
// managed by eclaw (across all instances).
func ManagedSelector() string {
	return fmt.Sprintf("%s=%s", LabelManagedBy, ManagedByValue)
}
